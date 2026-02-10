// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	// Import all Kubernetes client auth plugins (for example Azure, GCP, OIDC, and other auth plugins)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/klog/v2"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/go-logr/logr"
	kubearchivev1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	"github.com/kubearchive/kubearchive/cmd/operator/internal/controller"
	"github.com/kubearchive/kubearchive/pkg/logging"
	"github.com/kubearchive/kubearchive/pkg/observability"
	//+kubebuilder:scaffold:imports
)

const (
	otelServiceName          = "kubearchive.operator"
	webhookServerPort        = "9443"
	webhookConnectionTimeout = 5 * time.Second
)

var (
	webhookLastCheckFailed = true // Start as true to log first success
	webhookStateMutex      sync.Mutex
	version                = "main"
	commit                 = ""
	date                   = ""
	scheme                 = runtime.NewScheme()
	k9eNs                  = os.Getenv("KUBEARCHIVE_NAMESPACE")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(kubearchivev1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func webhookReadinessCheck(req *http.Request) error {
	serverAddr := fmt.Sprintf("localhost:%s", webhookServerPort)

	dialer := &net.Dialer{Timeout: webhookConnectionTimeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", serverAddr, &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
	})

	if err != nil {
		webhookStateMutex.Lock()
		webhookLastCheckFailed = true
		webhookStateMutex.Unlock()
		slog.Info("Webhook server not ready", "address", serverAddr, "error", err)
		return err
	}

	conn.Close()

	// Only log success message the first time after a failure (or first time ever)
	webhookStateMutex.Lock()
	if webhookLastCheckFailed {
		slog.Info("Webhook server ready", "address", serverAddr)
		webhookLastCheckFailed = false
	}
	webhookStateMutex.Unlock()

	return nil
}

func main() {
	if err := logging.ConfigureLogging(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	err := observability.Start(otelServiceName)
	if err != nil {
		log.Printf("Could not start OpenTelemetry: %s\n", err)
	}

	klog.SetSlogLogger(slog.Default()) // klog is used by the election leader process

	var enableLeaderElection bool
	var probeAddr string
	var enableHTTP2 bool
	var leaseDuration time.Duration
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for operator. "+
			"Enabling this will ensure there is only one active operator.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.DurationVar(&leaseDuration, "leader-lease-duration", 15*time.Second, "Duration of the leader lease, defaults to 15s.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(logr.FromSlogHandler(slog.Default().Handler()))

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancelation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		slog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	config := ctrl.GetConfigOrDie()
	config.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return otelhttp.NewTransport(rt)
	})

	cacheOptions := crcache.Options{
		ByObject: map[client.Object]crcache.ByObject{
			&corev1.ServiceAccount{}: {
				Namespaces: make(map[string]crcache.Config),
				Label:      k8slabels.Everything(),
			},
			&kubearchivev1.ClusterKubeArchiveConfig{}: {
				Label: k8slabels.Everything(),
			},
			&kubearchivev1.KubeArchiveConfig{}: {
				Namespaces: make(map[string]crcache.Config),
				Label:      k8slabels.Everything(),
			},
			&rbacv1.ClusterRole{}: {
				Label: k8slabels.Everything(),
			},
			&rbacv1.ClusterRoleBinding{}: {
				Label: k8slabels.Everything(),
			},
			&rbacv1.Role{}: {
				Namespaces: make(map[string]crcache.Config),
				Label:      k8slabels.Everything(),
			},
			&rbacv1.RoleBinding{}: {
				Namespaces: make(map[string]crcache.Config),
				Label:      k8slabels.Everything(),
			},
		},
		DefaultNamespaces: map[string]crcache.Config{
			k9eNs: {LabelSelector: k8slabels.Everything()},
		},
		DefaultLabelSelector: k8slabels.Nothing(),
	}

	mgrOptions := ctrl.Options{
		Scheme:                 scheme,
		Cache:                  cacheOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaseDuration:          &leaseDuration,
		LeaderElectionID:       "e7a70c64.kubearchive.org",
		Logger:                 logr.FromSlogHandler(slog.Default().Handler()),
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the operator stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the operator stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	}

	if os.Getenv(observability.EnablePprofEnvVar) == "true" {
		mgrOptions.PprofBindAddress = ":8888"
	}

	mgr, err := ctrl.NewManager(config, mgrOptions)
	if err != nil {
		slog.Error("unable to start operator", "err", err)
		os.Exit(1)
	}

	if err = controller.LoadConfiguration(context.Background()); err != nil {
		slog.Error("unable to load operator configuration", "err", err)
		os.Exit(1)
	}

	if err = (&controller.KubeArchiveConfigReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Mapper: mgr.GetRESTMapper(),
	}).SetupKubeArchiveConfigWithManager(mgr); err != nil {
		slog.Error("unable to create controller", "controller", "KubeArchiveConfig", "err", err)
		os.Exit(1)
	}

	if err = (&controller.ClusterKubeArchiveConfigReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Mapper: mgr.GetRESTMapper(),
	}).SetupClusterKubeArchiveConfigWithManager(mgr); err != nil {
		slog.Error("unable to create controller", "controller", "ClusterKubeArchiveConfig", "err", err)
		os.Exit(1)
	}

	slog.Info("registering SinkFilter controller")
	if err = (&controller.SinkFilterReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Mapper: mgr.GetRESTMapper(),
	}).SetupWithManager(mgr); err != nil {
		slog.Error("unable to create controller", "controller", "SinkFilter", "err", err)
		os.Exit(1)
	}

	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = kubearchivev1.SetupCKACWebhookWithManager(mgr); err != nil {
			slog.Error("unable to create webhook", "webhook", "ClusterKubeArchiveConfig", "err", err)
			os.Exit(1)
		}
		if err = kubearchivev1.SetupKACWebhookWithManager(mgr); err != nil {
			slog.Error("unable to create webhook", "webhook", "KubeArchiveConfig", "err", err)
			os.Exit(1)
		}
		if err = kubearchivev1.SetupSinkFilterWebhookWithManager(mgr); err != nil {
			slog.Error("unable to create webhook", "webhook", "SinkFilter", "err", err)
			os.Exit(1)
		}
		if err = kubearchivev1.SetupNamespaceVacuumConfigWebhookWithManager(mgr); err != nil {
			slog.Error("unable to create webhook", "webhook", "NamespaceVacuumConfig", "err", err)
			os.Exit(1)
		}
		if err = kubearchivev1.SetupClusterVacuumConfigWebhookWithManager(mgr); err != nil {
			slog.Error("unable to create webhook", "webhook", "ClusterVacuumConfig", "err", err)
			os.Exit(1)
		}
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		slog.Error("unable to set up health check", "err", err)
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", webhookReadinessCheck); err != nil {
		slog.Error("unable to set up ready check", "err", err)
		os.Exit(1)
	}

	slog.Info("starting operator", "version", version, "commit", commit, "built", date)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		slog.Error("problem running operator", "err", err)
		os.Exit(1)
	}
}
