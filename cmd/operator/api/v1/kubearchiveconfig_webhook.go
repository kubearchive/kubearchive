// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/kubearchive/kubearchive/pkg/cel"
	"github.com/kubearchive/kubearchive/pkg/constants"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func SetupKACWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&KubeArchiveConfig{}).
		WithValidator(&KubeArchiveConfigCustomValidator{
			kubearchiveResourceName: "kubearchive",
			client:                  mgr.GetClient(),
		}).
		WithDefaulter(&KubeArchiveConfigCustomDefaulter{}).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-kubearchive-org-v1-kubearchiveconfig,mutating=true,failurePolicy=fail,sideEffects=None,groups=kubearchive.org,resources=kubearchiveconfigs,verbs=create;update,versions=v1,name=mkubearchiveconfig.kb.io,admissionReviewVersions=v1

// +kubebuilder:object:generate=false
type KubeArchiveConfigCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &KubeArchiveConfigCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type
func (kaccd *KubeArchiveConfigCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	kac, ok := obj.(*KubeArchiveConfig)
	if !ok {
		return fmt.Errorf("expected an KubeArchiveConfig object but got %T", obj)
	}
	slog.Info("kubearchiveconfig default", "namespace", kac.Namespace, "name", kac.Name)

	// Set default values for KeepLastWhen rules
	for i := range kac.Spec.Resources {
		if kac.Spec.Resources[i].KeepLastWhen != nil {
			for j := range kac.Spec.Resources[i].KeepLastWhen.Keep {
				if kac.Spec.Resources[i].KeepLastWhen.Keep[j].SortBy == "" {
					kac.Spec.Resources[i].KeepLastWhen.Keep[j].SortBy = "metadata.creationTimestamp"
				}
			}
		}
	}

	return nil
}

//+kubebuilder:webhook:path=/validate-kubearchive-org-v1-kubearchiveconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=kubearchive.org,resources=kubearchiveconfigs,verbs=create;update,versions=v1,name=vkubearchiveconfig.kb.io,admissionReviewVersions=v1

// +kubebuilder:object:generate=false
type KubeArchiveConfigCustomValidator struct {
	kubearchiveResourceName string
	client                  client.Client
}

var _ webhook.CustomValidator = &KubeArchiveConfigCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (kaccv *KubeArchiveConfigCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	kac, ok := obj.(*KubeArchiveConfig)
	if !ok {
		return nil, fmt.Errorf("expected an KubeArchiveConfig object but got %T", obj)
	}
	slog.Info("kubearchiveconfig validate create", "namespace", kac.Namespace, "name", kac.Name)

	return kaccv.validateKAC(ctx, kac)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (kaccv *KubeArchiveConfigCustomValidator) ValidateUpdate(ctx context.Context, _ runtime.Object, new runtime.Object) (admission.Warnings, error) {
	kac, ok := new.(*KubeArchiveConfig)
	if !ok {
		return nil, fmt.Errorf("expected an KubeArchiveConfig object but got %T", new)
	}
	slog.Info("kubearchiveconfig validate update", "namespace", kac.Namespace, "name", kac.Name)

	return kaccv.validateKAC(ctx, kac)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (kaccv *KubeArchiveConfigCustomValidator) ValidateDelete(_ context.Context, new runtime.Object) (admission.Warnings, error) {
	kac, ok := new.(*KubeArchiveConfig)
	if !ok {
		return nil, fmt.Errorf("expected an KubeArchiveConfig object but got %T", new)
	}
	slog.Info("kubearchiveconfig validate delete", "namespace", kac.Namespace, "name", kac.Name)

	return nil, nil
}

func (kaccv *KubeArchiveConfigCustomValidator) validateKAC(ctx context.Context, kac *KubeArchiveConfig) (admission.Warnings, error) {
	errList := make([]error, 0)
	if kac.Namespace == constants.KubeArchiveNamespace {
		return nil, fmt.Errorf("cannot create KubeArchiveConfig in the '%s' namespace", kac.Namespace)
	}
	if kac.Name != kaccv.kubearchiveResourceName {
		errList = append(errList, fmt.Errorf("invalid resource name '%s', resource must be named '%s'",
			kac.Name, kaccv.kubearchiveResourceName))
	}

	// Fetch ClusterKubeArchiveConfig if any resource has keepLastWhen (to check for duplicates or overrides)
	var ckac *ClusterKubeArchiveConfig
	var ckacErr error
	needsClusterConfig := false
	for _, resource := range kac.Spec.Resources {
		if resource.KeepLastWhen != nil {
			needsClusterConfig = true
			break
		}
	}
	if needsClusterConfig {
		ckac = &ClusterKubeArchiveConfig{}
		ckacErr = kaccv.client.Get(ctx, types.NamespacedName{Name: kaccv.kubearchiveResourceName}, ckac)
		if ckacErr != nil && !apierrors.IsNotFound(ckacErr) {
			return nil, fmt.Errorf("failed to fetch ClusterKubeArchiveConfig: %w", ckacErr)
		}
	}

	for resourceIdx, resource := range kac.Spec.Resources {
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

		// Validate KeepLastWhen rules
		if resource.KeepLastWhen != nil {
			seenKeepCELExpressions := make(map[string]int)
			for i, rule := range resource.KeepLastWhen.Keep {
				when := normalizeString(rule.When)
				if when == "" {
					errList = append(errList, fmt.Errorf("keepLastWhen.keep[%d].when is required", i))
				} else {
					_, err := cel.CompileCELExpr(when)
					if err != nil {
						errList = append(errList, fmt.Errorf("keepLastWhen.keep[%d].when: %w", i, err))
					} else {
						durErrors := validateDurationString(when)
						for _, durErr := range durErrors {
							errList = append(errList, fmt.Errorf("keepLastWhen.keep[%d].when: %w", i, durErr))
						}
					}

					if existingIdx, exists := seenKeepCELExpressions[when]; exists {
						errList = append(errList, fmt.Errorf("keepLastWhen.keep[%d].when CEL expression duplicates keep[%d]; duplicate expressions are not allowed", i, existingIdx))
					} else {
						seenKeepCELExpressions[when] = i
					}
				}
				if rule.Count < 0 {
					errList = append(errList, fmt.Errorf("keepLastWhen.keep[%d].count must be greater than or equal to 0", i))
				}
			}

			// Check for duplicate CEL expressions with ClusterKubeArchiveConfig
			if ckacErr == nil && ckac != nil {
				// Find matching ClusterKubeArchiveConfigResource by selector
				var matchingClusterResource *ClusterKubeArchiveConfigResource
				for _, ckacResource := range ckac.Spec.Resources {
					if ckacResource.Selector.Kind == resource.Selector.Kind &&
						ckacResource.Selector.APIVersion == resource.Selector.APIVersion {
						matchingClusterResource = &ckacResource
						break
					}
				}

				if matchingClusterResource != nil {
					// Build a set of cluster rule CEL expressions
					clusterRuleCELs := make(map[string]string)
					for _, clusterRule := range matchingClusterResource.KeepLastWhen {
						clusterWhen := normalizeString(clusterRule.When)
						clusterRuleCELs[clusterWhen] = clusterRule.Name
					}

					// Check for duplicate CEL expressions in Keep rules
					for i, keepRule := range resource.KeepLastWhen.Keep {
						keepWhen := normalizeString(keepRule.When)
						if clusterRuleName, exists := clusterRuleCELs[keepWhen]; exists {
							errList = append(errList, fmt.Errorf("keepLastWhen.keep[%d].when CEL expression matches ClusterKubeArchiveConfig rule '%s'; duplicate expressions are not allowed", i, clusterRuleName))
						}
					}
				}
			}

			// Validate overrides against ClusterKubeArchiveConfig
			if len(resource.KeepLastWhen.Override) > 0 {
				if ckacErr != nil {
					if apierrors.IsNotFound(ckacErr) {
						errList = append(errList, fmt.Errorf("keepLastWhen.override specified but ClusterKubeArchiveConfig '%s' not found", kaccv.kubearchiveResourceName))
					} else {
						errList = append(errList, fmt.Errorf("failed to fetch ClusterKubeArchiveConfig: %w", ckacErr))
					}
				} else {
					// Find matching ClusterKubeArchiveConfigResource by selector
					var matchingClusterResource *ClusterKubeArchiveConfigResource
					for _, ckacResource := range ckac.Spec.Resources {
						if ckacResource.Selector.Kind == resource.Selector.Kind &&
							ckacResource.Selector.APIVersion == resource.Selector.APIVersion {
							matchingClusterResource = &ckacResource
							break
						}
					}

					if matchingClusterResource == nil {
						errList = append(errList, fmt.Errorf("resource[%d]: no matching resource found in ClusterKubeArchiveConfig for selector %s/%s", resourceIdx, resource.Selector.APIVersion, resource.Selector.Kind))
					} else {
						// Build a map of cluster rule names to counts
						clusterRules := make(map[string]int)
						for _, clusterRule := range matchingClusterResource.KeepLastWhen {
							clusterRules[clusterRule.Name] = clusterRule.Count
						}

						// Validate each override
						for i, override := range resource.KeepLastWhen.Override {
							if override.Name == "" {
								errList = append(errList, fmt.Errorf("keepLastWhen.override[%d].name is required", i))
								continue
							}
							if override.Count < 0 {
								errList = append(errList, fmt.Errorf("keepLastWhen.override[%d].count must be greater than or equal to 0", i))
								continue
							}

							clusterCount, exists := clusterRules[override.Name]
							if !exists {
								errList = append(errList, fmt.Errorf("keepLastWhen.override[%d].name '%s' does not match any rule in ClusterKubeArchiveConfig", i, override.Name))
							} else if override.Count > clusterCount {
								errList = append(errList, fmt.Errorf("keepLastWhen.override[%d].count (%d) must be <= ClusterKubeArchiveConfig rule '%s' count (%d)", i, override.Count, override.Name, clusterCount))
							}
						}
					}
				}
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

	durations := getDurationCalls(expr)

	errList := make([]error, 0)
	for _, match := range durations {
		prg, err := cel.CompileCELExpr(match)
		if err != nil {
			errList = append(errList, err)
		} else {
			_, err := cel.ExecuteCEL(context.Background(), prg, &emptyObj)
			if errors.Is(err, BadConversion) {
				errList = append(errList, fmt.Errorf("invalid duration string '%s'", match))
			}
		}
	}
	return errList
}

// Returns a slice of duration(...) strings
func getDurationCalls(expr string) []string {
	durations := []string{}
	// match duration(, duration (, etc.
	re := regexp.MustCompile(`duration *\(`)
	locs := re.FindAllStringIndex(expr, -1)
	if locs == nil {
		return durations
	}
	for _, loc := range locs {
		if loc == nil || len(loc) != 2 {
			continue
		}
		innerParenCount := 1
		for i := loc[1]; i < len(expr); i++ {
			switch expr[i] {
			case '(':
				innerParenCount++
			case ')':
				innerParenCount--
			}
			if innerParenCount == 0 {
				durations = append(durations, expr[loc[0]:i+1])
				break
			}
		}
	}
	return durations
}

func normalizeString(s string) string {
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
