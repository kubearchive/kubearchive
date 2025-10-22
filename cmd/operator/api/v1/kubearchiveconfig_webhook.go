// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/kubearchive/kubearchive/pkg/cel"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var kaclog = logf.Log.WithName("kubearchiveconfig-resource")

func SetupKACWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&KubeArchiveConfig{}).
		WithValidator(&KubeArchiveConfigCustomValidator{kubearchiveResourceName: "kubearchive"}).
		WithDefaulter(&KubeArchiveConfigCustomDefaulter{}).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-kubearchive-org-v1-kubearchiveconfig,mutating=true,failurePolicy=fail,sideEffects=None,groups=kubearchive.org,resources=kubearchiveconfigs,verbs=create;update,versions=v1,name=mkubearchiveconfig.kb.io,admissionReviewVersions=v1

type KubeArchiveConfigCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &KubeArchiveConfigCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type
func (kaccd *KubeArchiveConfigCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	kac, ok := obj.(*KubeArchiveConfig)
	if !ok {
		return fmt.Errorf("expected an KubeArchiveConfig object but got %T", obj)
	}
	kaclog.Info("default", "namespace", kac.Namespace, "name", kac.Name)
	return nil
}

//+kubebuilder:webhook:path=/validate-kubearchive-org-v1-kubearchiveconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=kubearchive.org,resources=kubearchiveconfigs,verbs=create;update,versions=v1,name=vkubearchiveconfig.kb.io,admissionReviewVersions=v1

type KubeArchiveConfigCustomValidator struct {
	kubearchiveResourceName string
}

var _ webhook.CustomValidator = &KubeArchiveConfigCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (kaccv *KubeArchiveConfigCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	kac, ok := obj.(*KubeArchiveConfig)
	if !ok {
		return nil, fmt.Errorf("expected an KubeArchiveConfig object but got %T", obj)
	}
	kaclog.Info("validate create", "namespace", kac.Namespace, "name", kac.Name)

	return kaccv.validateKAC(kac)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (kaccv *KubeArchiveConfigCustomValidator) ValidateUpdate(_ context.Context, _ runtime.Object, new runtime.Object) (admission.Warnings, error) {
	kac, ok := new.(*KubeArchiveConfig)
	if !ok {
		return nil, fmt.Errorf("expected an KubeArchiveConfig object but got %T", new)
	}
	kaclog.Info("validate update", "namespace", kac.Namespace, "name", kac.Name)

	return kaccv.validateKAC(kac)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (kaccv *KubeArchiveConfigCustomValidator) ValidateDelete(_ context.Context, new runtime.Object) (admission.Warnings, error) {
	kac, ok := new.(*KubeArchiveConfig)
	if !ok {
		return nil, fmt.Errorf("expected an KubeArchiveConfig object but got %T", new)
	}
	kaclog.Info("validate delete", "namespace", kac.Namespace, "name", kac.Name)

	return nil, nil
}

func (kaccv *KubeArchiveConfigCustomValidator) validateKAC(kac *KubeArchiveConfig) (admission.Warnings, error) {
	errList := make([]error, 0)
	if kac.Name != kaccv.kubearchiveResourceName {
		errList = append(errList, fmt.Errorf("invalid resource name '%s', resource must be named '%s'",
			kac.Name, kaccv.kubearchiveResourceName))
	}

	for _, resource := range kac.Spec.Resources {
		if resource.ArchiveWhen != "" {
			_, err := cel.CompileCELExpr(resource.ArchiveWhen)
			if err != nil {
				errList = append(errList, err)
			} else {
				errList = append(errList, validateDurationString(resource.ArchiveWhen)...)
			}
		}
		if resource.DeleteWhen != "" {
			_, err := cel.CompileCELExpr(resource.DeleteWhen)
			if err != nil {
				errList = append(errList, err)
			} else {
				errList = append(errList, validateDurationString(resource.DeleteWhen)...)
			}
		}
		if resource.ArchiveOnDelete != "" {
			_, err := cel.CompileCELExpr(resource.ArchiveOnDelete)
			if err != nil {
				errList = append(errList, err)
			} else {
				errList = append(errList, validateDurationString(resource.ArchiveOnDelete)...)
			}
		}
	}
	return nil, errors.Join(errList...)
}

func validateDurationString(expr string) []error {
	emptyObj := unstructured.Unstructured{
		Object: map[string]interface{}{},
	}
	var BadConversion = errors.New("type conversion error from 'string' to 'google.protobuf.Duration'")

	re := regexp.MustCompile(`(duration *\([^)]+\))`)
	matches := re.FindAllString(expr, -1)

	errList := make([]error, 0)
	for _, match := range matches {
		prg, err := cel.CompileCELExpr(match)
		if err != nil {
			errList = append(errList, err)
		} else {
			_, err := cel.ExecuteCEL(context.Background(), *prg, &emptyObj)
			if errors.Is(err, BadConversion) {
				errList = append(errList, fmt.Errorf("invalid duration string '%s'", match))
			}
		}
	}
	return errList
}
