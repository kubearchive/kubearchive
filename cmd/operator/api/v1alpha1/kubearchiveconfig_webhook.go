// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	cel "github.com/kubearchive/kubearchive/pkg/cel"
)

const kubearchiveResourceName = "kubearchive"

// log is for logging in this package.
var kubearchiveconfiglog = logf.Log.WithName("kubearchiveconfig-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (kac *KubeArchiveConfig) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(kac).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-kubearchive-kubearchive-org-v1alpha1-kubearchiveconfig,mutating=true,failurePolicy=fail,sideEffects=None,groups=kubearchive.kubearchive.org,resources=kubearchiveconfigs,verbs=create;update,versions=v1alpha1,name=mkubearchiveconfig.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &KubeArchiveConfig{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (kac *KubeArchiveConfig) Default() {
	kubearchiveconfiglog.Info("default", "namespace", kac.Namespace, "name", kac.Name)
}

//+kubebuilder:webhook:path=/validate-kubearchive-kubearchive-org-v1alpha1-kubearchiveconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=kubearchive.kubearchive.org,resources=kubearchiveconfigs,verbs=create;update,versions=v1alpha1,name=vkubearchiveconfig.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &KubeArchiveConfig{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (kac *KubeArchiveConfig) ValidateCreate() (admission.Warnings, error) {
	kubearchiveconfiglog.Info("validate create", "namespace", kac.Namespace, "name", kac.Name)

	if kac.Name != kubearchiveResourceName {
		return nil, fmt.Errorf("The KubeArchiveConfig resource must be named '%s'.", kubearchiveResourceName)
	}

	return kac.validateCELExpressions()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (kac *KubeArchiveConfig) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	kubearchiveconfiglog.Info("validate update", "namespace", kac.Namespace, "name", kac.Name)

	if kac.Name != kubearchiveResourceName {
		return nil, fmt.Errorf("Cannot update KubeArchiveConfig resource named '%s'.", kubearchiveResourceName)
	}

	return kac.validateCELExpressions()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (kac *KubeArchiveConfig) ValidateDelete() (admission.Warnings, error) {
	kubearchiveconfiglog.Info("validate delete", "namespace", kac.Namespace, "name", kac.Name)

	return nil, nil
}

func (kac *KubeArchiveConfig) validateCELExpressions() (admission.Warnings, error) {
	errList := make([]error, 0)
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
