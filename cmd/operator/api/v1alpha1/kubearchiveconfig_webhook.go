// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"errors"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	cel "github.com/kubearchive/kubearchive/pkg/cel"
)

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
	kubearchiveconfiglog.Info("default", "name", kac.Name)
}

//+kubebuilder:webhook:path=/validate-kubearchive-kubearchive-org-v1alpha1-kubearchiveconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=kubearchive.kubearchive.org,resources=kubearchiveconfigs,verbs=create;update,versions=v1alpha1,name=vkubearchiveconfig.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &KubeArchiveConfig{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (kac *KubeArchiveConfig) ValidateCreate() (admission.Warnings, error) {
	kubearchiveconfiglog.Info("validate create", "name", kac.Name)

	errs := kac.validateSingleton()
	if errs != nil {
		return nil, errs
	}

	return kac.validateCELExpressions()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (kac *KubeArchiveConfig) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	kubearchiveconfiglog.Info("validate update", "name", kac.Name)

	return kac.validateCELExpressions()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (kac *KubeArchiveConfig) ValidateDelete() (admission.Warnings, error) {
	kubearchiveconfiglog.Info("validate delete", "name", kac.Name)

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

//nolint:unparam
func (kac *KubeArchiveConfig) validateSingleton() error {
	cfg, err := config.GetConfig()
	if err != nil {
		return err
	}

	scheme := runtime.NewScheme()
	err = AddToScheme(scheme)
	if err != nil {
		return err
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	kacs := &KubeArchiveConfigList{}
	err = c.List(context.Background(), kacs, client.InNamespace(kac.Namespace))
	if err != nil {
		return err
	}

	if len(kacs.Items) > 0 {
		err = errors.New("A KubeArchiveConfig resource already exists in this namespace, and only one is allowed.")
		return err
	}

	return nil
}
