// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/kubearchive/kubearchive/pkg/cel"
)

// log is for logging in this package.
var kubearchiveconfiglog = logf.Log.WithName("kubearchiveconfig-resource")

func SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&KubeArchiveConfig{}).
		WithValidator(&KubeArchiveConfigCustomValidator{kubearchiveResourceName: "kubearchive"}).
		WithDefaulter(&KubeArchiveConfigCustomDefaulter{}).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-kubearchive-kubearchive-org-v1alpha1-kubearchiveconfig,mutating=true,failurePolicy=fail,sideEffects=None,groups=kubearchive.kubearchive.org,resources=kubearchiveconfigs,verbs=create;update,versions=v1alpha1,name=mkubearchiveconfig.kb.io,admissionReviewVersions=v1

type KubeArchiveConfigCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &KubeArchiveConfigCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type
func (kaccd *KubeArchiveConfigCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	kac, ok := obj.(*KubeArchiveConfig)
	if !ok {
		return fmt.Errorf("expected an KubeArchiveConfig object but got %T", obj)
	}
	kubearchiveconfiglog.Info("default", "namespace", kac.Namespace, "name", kac.Name)
	return nil
}

//+kubebuilder:webhook:path=/validate-kubearchive-kubearchive-org-v1alpha1-kubearchiveconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=kubearchive.kubearchive.org,resources=kubearchiveconfigs,verbs=create;update,versions=v1alpha1,name=vkubearchiveconfig.kb.io,admissionReviewVersions=v1

type KubeArchiveConfigCustomValidator struct {
	kubearchiveResourceName string
}

var _ webhook.CustomValidator = &KubeArchiveConfigCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (kaccv *KubeArchiveConfigCustomValidator) ValidateCreate(
	_ context.Context, obj runtime.Object,
) (admission.Warnings, error) {
	kac, ok := obj.(*KubeArchiveConfig)
	if !ok {
		return nil, fmt.Errorf("expected an KubeArchiveConfig object but got %T", obj)
	}
	kubearchiveconfiglog.Info("validate create", "namespace", kac.Namespace, "name", kac.Name)

	return kaccv.validateKAC(kac)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (kaccv *KubeArchiveConfigCustomValidator) ValidateUpdate(
	_ context.Context, _ runtime.Object, new runtime.Object,
) (admission.Warnings, error) {
	kac, ok := new.(*KubeArchiveConfig)
	if !ok {
		return nil, fmt.Errorf("expected an KubeArchiveConfig object but got %T", new)
	}
	kubearchiveconfiglog.Info("validate update", "namespace", kac.Namespace, "name", kac.Name)

	return kaccv.validateKAC(kac)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (kaccv *KubeArchiveConfigCustomValidator) ValidateDelete(
	_ context.Context, new runtime.Object,
) (admission.Warnings, error) {
	kac, ok := new.(*KubeArchiveConfig)
	if !ok {
		return nil, fmt.Errorf("expected an KubeArchiveConfig object but got %T", new)
	}
	kubearchiveconfiglog.Info("validate delete", "namespace", kac.Namespace, "name", kac.Name)

	return nil, nil
}

func (kaccv *KubeArchiveConfigCustomValidator) validateKAC(kac *KubeArchiveConfig) (admission.Warnings, error) {
	errList := make([]error, 0)
	if kac.Name != kaccv.kubearchiveResourceName {
		return nil, fmt.Errorf("invalid resource name '%s'",
			kaccv.kubearchiveResourceName)
	}
	for _, resource := range kac.Spec.Resources {
		if resource.ArchiveWhen != "" {
			_, err := cel.CompileOrCELExpression(resource.ArchiveWhen)
			if err != nil {
				errList = append(errList, err)
			}
		}
		if resource.DeleteWhen != "" {
			_, err := cel.CompileOrCELExpression(resource.DeleteWhen)
			if err != nil {
				errList = append(errList, err)
			}
		}
		if resource.ArchiveOnDelete != "" {
			_, err := cel.CompileOrCELExpression(resource.ArchiveOnDelete)
			if err != nil {
				errList = append(errList, err)
			}
		}
	}
	return nil, errors.Join(errList...)
}
