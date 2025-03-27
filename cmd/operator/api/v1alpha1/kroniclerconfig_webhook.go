// Copyright Kronicler Authors
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

	"github.com/kronicler/kronicler/pkg/cel"
)

// log is for logging in this package.
var kroniclerconfiglog = logf.Log.WithName("kroniclerconfig-resource")

func SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&KroniclerConfig{}).
		WithValidator(&KroniclerConfigCustomValidator{kroniclerResourceName: "kronicler"}).
		WithDefaulter(&KroniclerConfigCustomDefaulter{}).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-kronicler-kronicler-org-v1alpha1-kroniclerconfig,mutating=true,failurePolicy=fail,sideEffects=None,groups=kronicler.kronicler.org,resources=kroniclerconfigs,verbs=create;update,versions=v1alpha1,name=mkroniclerconfig.kb.io,admissionReviewVersions=v1

type KroniclerConfigCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &KroniclerConfigCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type
func (kroncd *KroniclerConfigCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	kron, ok := obj.(*KroniclerConfig)
	if !ok {
		return fmt.Errorf("expected an KroniclerConfig object but got %T", obj)
	}
	kroniclerconfiglog.Info("default", "namespace", kron.Namespace, "name", kron.Name)
	return nil
}

//+kubebuilder:webhook:path=/validate-kronicler-kronicler-org-v1alpha1-kroniclerconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=kronicler.kronicler.org,resources=kroniclerconfigs,verbs=create;update,versions=v1alpha1,name=vkroniclerconfig.kb.io,admissionReviewVersions=v1

type KroniclerConfigCustomValidator struct {
	kroniclerResourceName string
}

var _ webhook.CustomValidator = &KroniclerConfigCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (kroncv *KroniclerConfigCustomValidator) ValidateCreate(
	_ context.Context, obj runtime.Object,
) (admission.Warnings, error) {
	kron, ok := obj.(*KroniclerConfig)
	if !ok {
		return nil, fmt.Errorf("expected an KroniclerConfig object but got %T", obj)
	}
	kroniclerconfiglog.Info("validate create", "namespace", kron.Namespace, "name", kron.Name)

	return kroncv.validateKroniclerConfig(kron)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (kroncv *KroniclerConfigCustomValidator) ValidateUpdate(
	_ context.Context, _ runtime.Object, new runtime.Object,
) (admission.Warnings, error) {
	kron, ok := new.(*KroniclerConfig)
	if !ok {
		return nil, fmt.Errorf("expected an KroniclerConfig object but got %T", new)
	}
	kroniclerconfiglog.Info("validate update", "namespace", kron.Namespace, "name", kron.Name)

	return kroncv.validateKroniclerConfig(kron)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (kroncv *KroniclerConfigCustomValidator) ValidateDelete(
	_ context.Context, new runtime.Object,
) (admission.Warnings, error) {
	kron, ok := new.(*KroniclerConfig)
	if !ok {
		return nil, fmt.Errorf("expected an KroniclerConfig object but got %T", new)
	}
	kroniclerconfiglog.Info("validate delete", "namespace", kron.Namespace, "name", kron.Name)

	return nil, nil
}

func (kroncv *KroniclerConfigCustomValidator) validateKroniclerConfig(kron *KroniclerConfig) (admission.Warnings, error) {
	errList := make([]error, 0)
	if kron.Name != kroncv.kroniclerResourceName {
		return nil, fmt.Errorf("invalid resource name '%s'",
			kroncv.kroniclerResourceName)
	}
	for _, resource := range kron.Spec.Resources {
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
