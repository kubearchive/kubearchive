// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"errors"
	"fmt"

	"github.com/kubearchive/kubearchive/pkg/cel"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var ckaclog = logf.Log.WithName("clusterkubearchiveconfig-resource")

func SetupCKACWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&ClusterKubeArchiveConfig{}).
		WithValidator(&ClusterKubeArchiveConfigCustomValidator{kubearchiveResourceName: "kubearchive"}).
		WithDefaulter(&ClusterKubeArchiveConfigCustomDefaulter{}).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-kubearchive-kubearchive-org-v1alpha1-clusterkubearchiveconfig,mutating=true,failurePolicy=fail,sideEffects=None,groups=kubearchive.kubearchive.org,resources=clusterkubearchiveconfig,verbs=create;update,versions=v1alpha1,name=mclusterkubearchiveconfig.kb.io,admissionReviewVersions=v1

type ClusterKubeArchiveConfigCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &ClusterKubeArchiveConfigCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type
func (ckaccd *ClusterKubeArchiveConfigCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	ckac, ok := obj.(*ClusterKubeArchiveConfig)
	if !ok {
		return fmt.Errorf("expected an ClusterKubeArchiveConfig object but got %T", obj)
	}
	ckaclog.Info("default", "name", ckac.Name)
	return nil
}

//+kubebuilder:webhook:path=/validate-kubearchive-kubearchive-org-v1alpha1-clusterkubearchiveconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=kubearchive.kubearchive.org,resources=clusterkubearchiveconfig,verbs=create;update,versions=v1alpha1,name=vclusterkubearchiveconfig.kb.io,admissionReviewVersions=v1

type ClusterKubeArchiveConfigCustomValidator struct {
	kubearchiveResourceName string
}

var _ webhook.CustomValidator = &ClusterKubeArchiveConfigCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (ckaccv *ClusterKubeArchiveConfigCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	ckac, ok := obj.(*ClusterKubeArchiveConfig)
	if !ok {
		return nil, fmt.Errorf("expected an ClusterKubeArchiveConfig object but got %T", obj)
	}
	ckaclog.Info("validate create", "name", ckac.Name)

	return ckaccv.validateKAC(ckac)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (ckaccv *ClusterKubeArchiveConfigCustomValidator) ValidateUpdate(_ context.Context, _ runtime.Object, new runtime.Object) (admission.Warnings, error) {
	ckac, ok := new.(*ClusterKubeArchiveConfig)
	if !ok {
		return nil, fmt.Errorf("expected an ClusterKubeArchiveConfig object but got %T", new)
	}
	ckaclog.Info("validate update", "name", ckac.Name)

	return ckaccv.validateKAC(ckac)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (ckaccv *ClusterKubeArchiveConfigCustomValidator) ValidateDelete(_ context.Context, new runtime.Object) (admission.Warnings, error) {
	ckac, ok := new.(*ClusterKubeArchiveConfig)
	if !ok {
		return nil, fmt.Errorf("expected an ClusterKubeArchiveConfig object but got %T", new)
	}
	ckaclog.Info("validate delete", "name", ckac.Name)

	return nil, nil
}

func (ckaccv *ClusterKubeArchiveConfigCustomValidator) validateKAC(ckac *ClusterKubeArchiveConfig) (admission.Warnings, error) {
	errList := make([]error, 0)
	if ckac.Name != ckaccv.kubearchiveResourceName {
		errList = append(errList, fmt.Errorf("invalid resource name '%s', resource must be named '%s'",
			ckac.Name, ckaccv.kubearchiveResourceName))
	}
	for _, resource := range ckac.Spec.Resources {
		if resource.ArchiveWhen != "" {
			_, err := cel.CompileCELExpr(resource.ArchiveWhen)
			if err != nil {
				errList = append(errList, err)
			}
		}
		if resource.DeleteWhen != "" {
			_, err := cel.CompileCELExpr(resource.DeleteWhen)
			if err != nil {
				errList = append(errList, err)
			}
		}
		if resource.ArchiveOnDelete != "" {
			_, err := cel.CompileCELExpr(resource.ArchiveOnDelete)
			if err != nil {
				errList = append(errList, err)
			}
		}
	}
	return nil, errors.Join(errList...)
}
