// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	kubearchiveorgv1 "github.com/kubearchive/kubearchive/cmd/installer/api/v1"
	"gopkg.in/yaml.v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	resourceFinalizerName = "kubearchive.org/installer-finalizer"
)

// KubeArchiveInstallationReconciler reconciles a KubeArchiveInstallation object
type KubeArchiveInstallationReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kubearchive.org,resources="*",verbs="*"
// +kubebuilder:rbac:groups=kubearchive.org,resources=kubearchiveinstallations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubearchive.org,resources=kubearchiveinstallations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kubearchive.org,resources=kubearchiveinstallations/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces;services,verbs=create;update;patch;get;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=create;patch;update;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=create;patch;update;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=create;patch;update;delete;get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;clusterroles;clusterrolebindings;rolebindings,verbs=create;patch;update;bind;delete;escalate;get;list;watch
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=tokenreviews,verbs=create;patch;update;delete
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=subjectaccessreviews,verbs=create;patch;update;delete
// +kubebuilder:rbac:groups="",resources=configmaps;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=issuers;certificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=cronjobs;jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *KubeArchiveInstallationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Reconciling...")

	kaInstallation := kubearchiveorgv1.KubeArchiveInstallation{}
	if err := r.Client.Get(ctx, req.NamespacedName, &kaInstallation); err != nil {
		log.Info("KubeArchive instance not found. Ignoring error...")
		// Resource deletions trigger reconciliation, so we need to ignore not found
		// as the resource may not exist anymore. Not ignoring them may cause
		// an infinite amount of reconcile attempts. We will get more notifications eventually.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Add the finalizer if not present, we leave as the update notificatio will trigger the reconcile loop again
	if kaInstallation.DeletionTimestamp.IsZero() && !controllerutil.ContainsFinalizer(&kaInstallation, resourceFinalizerName) {
		controllerutil.AddFinalizer(&kaInstallation, resourceFinalizerName)
		if err := r.Client.Update(ctx, &kaInstallation); err != nil {
			return ctrl.Result{}, err
		}
	}

	// If there is a DeletionTimestamp and it contains the finalizer, do the cleanup. We return early to avoid installing KubeArchive again
	if !kaInstallation.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(&kaInstallation, resourceFinalizerName) {
		log.Info("Deleting KubeArchive installation", "version", kaInstallation.Spec.Version)
		for _, manifest := range kaInstallation.Status.Manifests {
			obj := &unstructured.Unstructured{}
			obj.SetAPIVersion(manifest.APIVersion)
			obj.SetKind(manifest.Kind)
			obj.SetName(manifest.Name)
			obj.SetNamespace(manifest.Namespace)
			log.Info("Deleting resource", "kind", manifest.Kind, "name", manifest.Name, "namespace", manifest.Namespace)
			if err := r.Client.Delete(ctx, obj); err != nil {
				if !apierrors.IsNotFound(err) {
					return ctrl.Result{}, err
				}
			}
		}

		controllerutil.RemoveFinalizer(&kaInstallation, resourceFinalizerName)
		if err := r.Client.Update(ctx, &kaInstallation); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	version := kaInstallation.Spec.Version
	log.Info("Installing KubeArchive version", "version", version)

	httpClient := http.Client{Timeout: 30 * time.Second}
	downloadURL := fmt.Sprintf("https://github.com/kubearchive/kubearchive/releases/download/%s/kubearchive.yaml", version)
	resp, err := httpClient.Get(downloadURL)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ctrl.Result{}, fmt.Errorf("version %s not found", version)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // Max 20MB
	if err != nil {
		return ctrl.Result{}, err
	}

	var resources []unstructured.Unstructured
	for resourceStr := range strings.SplitSeq(string(body), "---") {
		var res map[string]any
		err = yaml.Unmarshal([]byte(resourceStr), &res)
		if err != nil {
			return ctrl.Result{}, err
		}

		if res == nil {
			continue
		}
		resource := unstructured.Unstructured{Object: res}
		resources = append(resources, resource)
	}

	var manifests []kubearchiveorgv1.Manifest
	for _, resource := range resources {
		err := r.Client.Patch(ctx, &resource, client.Apply, &client.PatchOptions{FieldManager: "kubearchive-installer"})
		if err != nil {
			log.Error(err, "failed to apply resource", "kind", resource.GetKind(), "name", resource.GetName())
			return ctrl.Result{}, err
		}

		manifests = append(manifests, kubearchiveorgv1.Manifest{
			APIVersion: resource.GetAPIVersion(),
			Kind:       resource.GetKind(),
			Name:       resource.GetName(),
			Namespace:  resource.GetNamespace(),
		})
	}

	if slices.Equal(kaInstallation.Status.Manifests, manifests) {
		log.Info("Manifests unchanged, skipping apply")
		return ctrl.Result{}, nil
	}

	kaInstallation.Status.Manifests = manifests
	if err := r.Client.Status().Update(ctx, &kaInstallation); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubeArchiveInstallationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubearchiveorgv1.KubeArchiveInstallation{}).
		Named("kubearchiveinstallation").
		Complete(r)
}
