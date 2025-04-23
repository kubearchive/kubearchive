// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"errors"
	"fmt"

	"github.com/kubearchive/kubearchive/pkg/constants"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var cvlog = logf.Log.WithName("clustervacuumconfig-resource")

func SetupClusterVacuumConfigWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&ClusterVacuumConfig{}).
		WithValidator(&ClusterVacuumConfigCustomValidator{}).
		WithDefaulter(&ClusterVacuumConfigCustomDefaulter{}).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-kubearchive-kubearchive-org-v1alpha1-clustervacuumconfig,mutating=true,failurePolicy=fail,sideEffects=None,groups=kubearchive.kubearchive.org,resources=clustervacuumconfig,verbs=create;update,versions=v1alpha1,name=mclustervacuumconfig.kb.io,admissionReviewVersions=v1

type ClusterVacuumConfigCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &ClusterVacuumConfigCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type
func (cvcd *ClusterVacuumConfigCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	cv, ok := obj.(*ClusterVacuumConfig)
	if !ok {
		return fmt.Errorf("expected a ClusterVacuumConfig object but got %T", obj)
	}
	cvlog.Info("default", "name", cv.Name)
	return nil
}

//+kubebuilder:webhook:path=/validate-kubearchive-kubearchive-org-v1alpha1-clustervacuumconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=kubearchive.kubearchive.org,resources=clustervacuumconfig,verbs=create;update,versions=v1alpha1,name=vclustervacuumconfig.kb.io,admissionReviewVersions=v1

type ClusterVacuumConfigCustomValidator struct {
}

var _ webhook.CustomValidator = &ClusterVacuumConfigCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (cvcv *ClusterVacuumConfigCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	cv, ok := obj.(*ClusterVacuumConfig)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterVacuumConfig object but got %T", obj)
	}
	cvlog.Info("validate create", "name", cv.Name)

	return cvcv.validateClusterVacuumConfig(cv)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (cvcv *ClusterVacuumConfigCustomValidator) ValidateUpdate(_ context.Context, _ runtime.Object, new runtime.Object) (admission.Warnings, error) {
	cv, ok := new.(*ClusterVacuumConfig)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterVacuumConfig object but got %T", new)
	}
	cvlog.Info("validate update", "name", cv.Name)

	return cvcv.validateClusterVacuumConfig(cv)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (cvcv *ClusterVacuumConfigCustomValidator) ValidateDelete(_ context.Context, new runtime.Object) (admission.Warnings, error) {
	cv, ok := new.(*ClusterVacuumConfig)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterVacuumConfig object but got %T", new)
	}
	cvlog.Info("validate delete", "name", cv.Name)

	return nil, nil
}

func (cvcv *ClusterVacuumConfigCustomValidator) validateClusterVacuumConfig(cv *ClusterVacuumConfig) (admission.Warnings, error) {
	errList := make([]error, 0)
	if cv.Namespace != constants.KubeArchiveNamespace {
		errList = append(errList, fmt.Errorf("invalid namespace name '%s', resource must be in namespace '%s'",
			cv.Namespace, constants.KubeArchiveNamespace))
	}

	for _, ns := range cv.Spec.Namespaces {
		err := validateResources(dynaClient, ns.Name, ns.Resources)
		if err != nil {
			errList = append(errList, err)
		}
	}

	return nil, errors.Join(errList...)
}
