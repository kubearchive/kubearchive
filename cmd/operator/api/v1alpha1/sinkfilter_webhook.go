// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"errors"
	"fmt"

	"github.com/kubearchive/kubearchive/pkg/cel"
	"github.com/kubearchive/kubearchive/pkg/constants"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var sflog = logf.Log.WithName("sinkfilter-resource")

func SetupSinkFilterWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&SinkFilter{}).
		WithValidator(&SinkFilterCustomValidator{sinkFilterResourceName: constants.SinkFilterResourceName}).
		WithDefaulter(&SinkFilterCustomDefaulter{}).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-kubearchive-kubearchive-org-v1alpha1-sinkfilter,mutating=true,failurePolicy=fail,sideEffects=None,groups=kubearchive.kubearchive.org,resources=sinkfilter,verbs=create;update,versions=v1alpha1,name=msinkfilter.kb.io,admissionReviewVersions=v1

type SinkFilterCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &SinkFilterCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type
func (sfcd *SinkFilterCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	sf, ok := obj.(*SinkFilter)
	if !ok {
		return fmt.Errorf("expected an SinkFilter object but got %T", obj)
	}
	sflog.Info("default", "name", sf.Name)
	return nil
}

//+kubebuilder:webhook:path=/validate-kubearchive-kubearchive-org-v1alpha1-sinkfilter,mutating=false,failurePolicy=fail,sideEffects=None,groups=kubearchive.kubearchive.org,resources=sinkfilter,verbs=create;update,versions=v1alpha1,name=vsinkfilter.kb.io,admissionReviewVersions=v1

type SinkFilterCustomValidator struct {
	sinkFilterResourceName string
}

var _ webhook.CustomValidator = &SinkFilterCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (sfcv *SinkFilterCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	sf, ok := obj.(*SinkFilter)
	if !ok {
		return nil, fmt.Errorf("expected an SinkFilter object but got %T", obj)
	}
	sflog.Info("validate create", "name", sf.Name)

	return sfcv.validateSinkFilter(sf)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (sfcv *SinkFilterCustomValidator) ValidateUpdate(_ context.Context, _ runtime.Object, new runtime.Object) (admission.Warnings, error) {
	sf, ok := new.(*SinkFilter)
	if !ok {
		return nil, fmt.Errorf("expected an SinkFilter object but got %T", new)
	}
	sflog.Info("validate update", "name", sf.Name)

	return sfcv.validateSinkFilter(sf)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (sfcv *SinkFilterCustomValidator) ValidateDelete(_ context.Context, new runtime.Object) (admission.Warnings, error) {
	sf, ok := new.(*SinkFilter)
	if !ok {
		return nil, fmt.Errorf("expected an SinkFilter object but got %T", new)
	}
	sflog.Info("validate delete", "name", sf.Name)

	return nil, nil
}

func (sfcv *SinkFilterCustomValidator) validateSinkFilter(sf *SinkFilter) (admission.Warnings, error) {
	errList := make([]error, 0)
	if sf.Name != sfcv.sinkFilterResourceName {
		errList = append(errList, fmt.Errorf("invalid resource name '%s', resource must be named '%s'",
			sf.Name, sfcv.sinkFilterResourceName))
	}
	if sf.Namespace != constants.KubeArchiveNamespace {
		errList = append(errList, fmt.Errorf("invalid namespace name '%s', resource must be in namespace '%s'",
			sf.Namespace, constants.KubeArchiveNamespace))
	}
	for _, resources := range sf.Spec.Namespaces {
		for _, resource := range resources {
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
	}
	return nil, errors.Join(errList...)
}
