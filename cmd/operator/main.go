// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/tls"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	// Import all Kubernetes client auth plugins (for example Azure, GCP, OIDC, and other auth plugins)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/klog/v2"

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
	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1alpha1"
	"github.com/kubearchive/kubearchive/cmd/operator/internal/controller"
	"github.com/kubearchive/kubearchive/pkg/logging"
	"github.com/kubearchive/kubearchive/pkg/observability"

	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	//+kubebuilder:scaffold:imports
)

const (
	otelServiceName = "kubearchive.operator"
)

var (
	version = "main"
	commit  = ""
	date    = ""
	scheme  = runtime.NewScheme()
	k9eNs   = os.Getenv("KUBEARCHIVE_NAMESPACE")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(kubearchiveapi.AddToScheme(scheme))
	utilruntime.Must(sourcesv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
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
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for operator. "+
			"Enabling this will ensure there is only one active operator.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
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
			&kubearchiveapi.KubeArchiveConfig{}: {
				Namespaces: make(map[string]crcache.Config),
				Label:      k8slabels.Everything(),
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
		mgrOptions.PprofBindAddress = ":8082"
	}

	mgr, err := ctrl.NewManager(config, mgrOptions)
	if err != nil {
		slog.Error("unable to start operator", "err", err)
		os.Exit(1)
	}

	if err = (&controller.KubeArchiveConfigReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Mapper: mgr.GetRESTMapper(),
	}).SetupWithManager(mgr); err != nil {
		slog.Error("unable to create controller", "controller", "KubeArchiveConfig", "err", err)
		os.Exit(1)
	}
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = kubearchiveapi.SetupWebhookWithManager(mgr); err != nil {
			slog.Error("unable to create webhook", "webhook", "KubeArchiveConfig", "err", err)
			os.Exit(1)
		}
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		slog.Error("unable to set up health check", "err", err)
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		slog.Error("unable to set up ready check", "err", err)
		os.Exit(1)
	}

	slog.Info("starting operator", "version", version, "commit", commit, "built", date)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		slog.Error("problem running operator", "err", err)
		os.Exit(1)
	}
}
