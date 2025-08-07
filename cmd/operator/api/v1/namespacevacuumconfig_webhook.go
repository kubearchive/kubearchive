// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"errors"
	"fmt"

	"github.com/kubearchive/kubearchive/pkg/constants"
	"github.com/kubearchive/kubearchive/pkg/k8sclient"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var nvlog = logf.Log.WithName("namespacevacuumconfig-resource")
var dynaClient dynamic.Interface

func SetupNamespaceVacuumConfigWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&NamespaceVacuumConfig{}).
		WithValidator(&NamespaceVacuumConfigCustomValidator{}).
		WithDefaulter(&NamespaceVacuumConfigCustomDefaulter{}).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-kubearchive-org-v1-namespacevacuumconfig,mutating=true,failurePolicy=fail,sideEffects=None,groups=kubearchive.org,resources=namespacevacuumconfig,verbs=create;update,versions=v1,name=mnamespacevacuumconfig.kb.io,admissionReviewVersions=v1

type NamespaceVacuumConfigCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &NamespaceVacuumConfigCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type
func (cvcd *NamespaceVacuumConfigCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	cv, ok := obj.(*NamespaceVacuumConfig)
	if !ok {
		return fmt.Errorf("expected a NamespaceVacuumConfig object but got %T", obj)
	}
	nvlog.Info("default", "name", cv.Name)
	return nil
}

//+kubebuilder:webhook:path=/validate-kubearchive-org-v1-namespacevacuumconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=kubearchive.org,resources=namespacevacuumconfig,verbs=create;update,versions=v1,name=vnamespacevacuumconfig.kb.io,admissionReviewVersions=v1

type NamespaceVacuumConfigCustomValidator struct {
}

var _ webhook.CustomValidator = &NamespaceVacuumConfigCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (nvcv *NamespaceVacuumConfigCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	cv, ok := obj.(*NamespaceVacuumConfig)
	if !ok {
		return nil, fmt.Errorf("expected a NamespaceVacuumConfig object but got %T", obj)
	}
	nvlog.Info("validate create", "name", cv.Name)

	return nvcv.validateNamespaceVacuumConfig(cv)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (nvcv *NamespaceVacuumConfigCustomValidator) ValidateUpdate(_ context.Context, _ runtime.Object, new runtime.Object) (admission.Warnings, error) {
	cv, ok := new.(*NamespaceVacuumConfig)
	if !ok {
		return nil, fmt.Errorf("expected a NamespaceVacuumConfig object but got %T", new)
	}
	nvlog.Info("validate update", "name", cv.Name)

	return nvcv.validateNamespaceVacuumConfig(cv)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (nvcv *NamespaceVacuumConfigCustomValidator) ValidateDelete(_ context.Context, new runtime.Object) (admission.Warnings, error) {
	cv, ok := new.(*NamespaceVacuumConfig)
	if !ok {
		return nil, fmt.Errorf("expected a NamespaceVacuumConfig object but got %T", new)
	}
	nvlog.Info("validate delete", "name", cv.Name)

	return nil, nil
}

func (nvcv *NamespaceVacuumConfigCustomValidator) validateNamespaceVacuumConfig(cv *NamespaceVacuumConfig) (admission.Warnings, error) {
	errList := make([]error, 0)

	err := validateResources(dynaClient, cv.Namespace, cv.Spec.Resources)
	if err != nil {
		errList = append(errList, err)
	}

	return nil, errors.Join(errList...)
}

func validateResources(client dynamic.Interface, namespace string, resources []sourcesv1.APIVersionKind) error {
	gres, err := getGlobalResourceSet(client)
	if err != nil {
		return err
	}
	nres, err := getNamespaceResourceSet(client, namespace)
	if err != nil {
		return err
	}

	errList := make([]error, 0)
	for _, resource := range resources {
		_, gok := gres[resource]
		_, nok := nres[resource]
		if !gok && !nok {
			errList = append(errList, errors.New("Resource with APIVersion '"+resource.APIVersion+"' and kind '"+
				resource.Kind+"' in namespace '"+namespace+"' is not configured for KubeArchive"))
		}
	}
	return errors.Join(errList...)
}

func getGlobalResourceSet(client dynamic.Interface) (map[sourcesv1.APIVersionKind]struct{}, error) {
	resources := map[sourcesv1.APIVersionKind]struct{}{}

	object, err := client.Resource(ClusterKubeArchiveConfigGVR).Get(context.Background(), constants.KubeArchiveConfigResourceName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return resources, nil
	} else if err != nil {
		return nil, err
	}
	ckac, err := ConvertUnstructuredToClusterKubeArchiveConfig(object)
	if err != nil {
		return nil, err
	}

	for _, resource := range ckac.Spec.Resources {
		resources[sourcesv1.APIVersionKind{APIVersion: resource.Selector.APIVersion, Kind: resource.Selector.Kind}] = struct{}{}
	}

	return resources, nil
}

func getNamespaceResourceSet(client dynamic.Interface, namespace string) (map[sourcesv1.APIVersionKind]struct{}, error) {
	object, err := client.Resource(KubeArchiveConfigGVR).Namespace(namespace).Get(context.Background(), constants.KubeArchiveConfigResourceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("in namespace '%s': %v", namespace, err)
	}
	kac, err := ConvertUnstructuredToKubeArchiveConfig(object)
	if err != nil {
		return nil, fmt.Errorf("in namespace '%s': %v", namespace, err)
	}

	resources := map[sourcesv1.APIVersionKind]struct{}{}
	for _, resource := range kac.Spec.Resources {
		resources[sourcesv1.APIVersionKind{APIVersion: resource.Selector.APIVersion, Kind: resource.Selector.Kind}] = struct{}{}
	}

	return resources, nil
}

func init() {
	var err error
	dynaClient, err = k8sclient.NewInstrumentedDynamicClient()
	if err != nil {
		nvlog.Error(err, "Unable to get dynamic client")
	}
}
