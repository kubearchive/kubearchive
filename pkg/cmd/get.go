// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

type GetOptions struct {
	genericiooptions.IOStreams
	KARetrieverCommand
	AllNamespaces      bool
	APIPath            string
	ResourceInfo       *ResourceInfo
	Name               string
	LabelSelector      string
	OutputFormat       *string
	JSONYamlPrintFlags *genericclioptions.JSONYamlPrintFlags
	IsValidOutput      bool
	InCluster          bool
	Archived           bool
}

// ResourceWithAvailability tracks a resource and its availability in different APIs
type ResourceWithAvailability struct {
	Resource  *unstructured.Unstructured
	InCluster bool
	Archived  bool
}

func NewGetOptions() *GetOptions {
	outputFormat := ""
	return &GetOptions{
		OutputFormat:       &outputFormat,
		JSONYamlPrintFlags: genericclioptions.NewJSONYamlPrintFlags(),
		IOStreams: genericiooptions.IOStreams{
			In:     os.Stdin,
			Out:    os.Stdout,
			ErrOut: os.Stderr,
		},
		KARetrieverCommand: NewKARetrieverOptions(),
	}
}

func NewGetCmd() *cobra.Command {
	o := NewGetOptions()

	cmd := &cobra.Command{
		Use:           "get [RESOURCE[.VERSION[.GROUP]]] [NAME]",
		Short:         "Command to get resources from KubeArchive",
		Args:          cobra.RangeArgs(1, 2),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.Complete(args); err != nil {
				return err
			}
			return o.Run()
		},
	}

	cmd.Flags().BoolVarP(&o.AllNamespaces, "all-namespaces", "A", o.AllNamespaces, "If present, list the requested object(s) across all namespaces. Namespace in current context is ignored even if specified with --namespace.")
	cmd.Flags().StringVarP(&o.LabelSelector, "selector", "l", o.LabelSelector, "Selector (label query) to filter on, supports '=', '==', '!=', 'in', 'notin'.(e.g. -l key1=value1,key2=value2,key3 in (value3)). Matching objects must satisfy all of the specified label constraints.")
	cmd.Flags().BoolVar(&o.InCluster, "in-cluster", true, "Include resources from the Kubernetes cluster.")
	cmd.Flags().BoolVar(&o.Archived, "archived", true, "Include resources from KubeArchive.")
	o.AddRetrieverFlags(cmd.Flags())
	o.JSONYamlPrintFlags.AddFlags(cmd)
	cmd.Flags().StringVarP(o.OutputFormat, "output", "o", *o.OutputFormat, fmt.Sprintf(`Output format. One of: (%s).`, strings.Join(o.JSONYamlPrintFlags.AllowedFormats(), ", ")))

	return cmd
}

func (o *GetOptions) Complete(args []string) error {
	err := o.CompleteRetriever()
	if err != nil {
		return err
	}

	// Parse arguments - first is resource type, second (optional) is name
	if len(args) >= 2 {
		o.Name = args[1]
	}

	// Validate that name and label selector are not used together
	if o.Name != "" && o.LabelSelector != "" {
		return fmt.Errorf("cannot specify both a resource name and a label selector")
	}

	// Validate that at least one flag is true
	if !o.InCluster && !o.Archived {
		return fmt.Errorf("at least one of --in-cluster or --archived must be true")
	}

	// Parse and resolve resource specification using discovery
	resourceInfo, err := o.ResolveResourceSpec(args[0])
	if err != nil {
		return err
	}
	o.ResourceInfo = resourceInfo

	// Build API path
	APIPathWithoutRoot := fmt.Sprintf("%s/%s", o.ResourceInfo.GroupVersion, o.ResourceInfo.Resource)

	// Only add namespace path for namespaced resources
	if o.ResourceInfo.Namespaced && !o.AllNamespaces {
		namespace, nsErr := o.GetNamespace()
		if nsErr != nil {
			return nsErr
		}
		APIPathWithoutRoot = fmt.Sprintf("%s/namespaces/%s/%s", o.ResourceInfo.GroupVersion, namespace, o.ResourceInfo.Resource)
	}

	// If a specific name is provided, append it to the path
	if o.Name != "" {
		APIPathWithoutRoot = fmt.Sprintf("%s/%s", APIPathWithoutRoot, o.Name)
	}

	// Determine if this is a core resource (no group or empty group)
	if o.ResourceInfo.Group == "" {
		o.APIPath = fmt.Sprintf("/api/%s", APIPathWithoutRoot)
	} else {
		o.APIPath = fmt.Sprintf("/apis/%s", APIPathWithoutRoot)
	}

	// Add label selector as query parameter if provided
	if o.LabelSelector != "" {
		o.APIPath = fmt.Sprintf("%s?labelSelector=%s", o.APIPath, url.QueryEscape(o.LabelSelector))
	}

	return nil
}

func (o *GetOptions) parseResourcesFromBytes(bodyBytes []byte) ([]*unstructured.Unstructured, error) {
	// If a specific name was requested, the API returns a single resource, not a list
	if o.Name != "" {
		var resource unstructured.Unstructured
		err := json.Unmarshal(bodyBytes, &resource)
		if err != nil {
			return nil, fmt.Errorf("error deserializing the body into unstructured.Unstructured: %w", err)
		}
		return []*unstructured.Unstructured{&resource}, nil
	}

	// Otherwise, parse as a list
	var list unstructured.UnstructuredList
	err := json.Unmarshal(bodyBytes, &list)
	if err != nil {
		return nil, fmt.Errorf("error deserializing the body into unstructured.UnstructuredList: %w", err)
	}

	// Convert unstructured objects to slice of pointers
	var unstructuredObjects []*unstructured.Unstructured
	for i := range list.Items {
		unstructuredObjects = append(unstructuredObjects, &list.Items[i])
	}

	return unstructuredObjects, nil
}

// sortResourcesByCreationTime sorts resources with availability by creation timestamp (newest first)
func sortResourcesByCreationTime(resources []*ResourceWithAvailability) {
	sort.Slice(resources, func(i, j int) bool {
		timeI := resources[i].Resource.GetCreationTimestamp().Time
		timeJ := resources[j].Resource.GetCreationTimestamp().Time

		// If both have timestamps, compare them (newer first)
		if !timeI.IsZero() && !timeJ.IsZero() {
			if timeI.After(timeJ) {
				return true
			}
			if timeJ.After(timeI) {
				return false
			}
			// If timestamps are equal, sort by name for stable ordering
			return resources[i].Resource.GetName() < resources[j].Resource.GetName()
		}

		// If only one has a timestamp, prioritize the one with timestamp
		if !timeI.IsZero() && timeJ.IsZero() {
			return true
		}
		if timeI.IsZero() && !timeJ.IsZero() {
			return false
		}

		// If neither has a timestamp, sort by name for stable ordering
		return resources[i].Resource.GetName() < resources[j].Resource.GetName()
	})
}

func (o *GetOptions) Run() error {
	var k8sResources []*unstructured.Unstructured
	var kubearchiveResources []*unstructured.Unstructured
	var k8sNotFound, kubearchiveNotFound bool

	// Get resources from Kubernetes API (only if --in-cluster is true)
	if o.InCluster {
		bodyBytes, apiErr := o.GetFromAPI(Kubernetes, o.APIPath)
		if apiErr != nil {
			if apiErr.StatusCode != http.StatusNotFound {
				return apiErr
			}
			k8sNotFound = true
		} else {
			resources, parseErr := o.parseResourcesFromBytes(bodyBytes)
			if parseErr != nil {
				return &APIError{
					StatusCode: 200,
					URL:        "Kubernetes API",
					Message:    fmt.Sprintf("error parsing resources from the cluster: %v", parseErr),
					Body:       string(bodyBytes),
				}
			}
			k8sResources = resources
		}
	} else {
		k8sNotFound = true
	}

	// Get resources from KubeArchive API (only if --archived is true)
	if o.Archived {
		bodyBytes, apiErr := o.GetFromAPI(KubeArchive, o.APIPath)
		if apiErr != nil {
			// If KubeArchive fails with authentication error, don't fall back to just Kubernetes
			if apiErr.StatusCode == http.StatusUnauthorized ||
				strings.Contains(apiErr.Message, "empty authorization bearer token given") ||
				strings.Contains(apiErr.Message, "authentication failed") {
				return fmt.Errorf("KubeArchive authentication required: %s", apiErr.Message)
			}
			// If we have k8s resources, continue with those regardless of the error type
			if len(k8sResources) > 0 {
				kubearchiveNotFound = true
			} else {
				// Only return error if we don't have any k8s resources
				if apiErr.StatusCode != http.StatusNotFound {
					return apiErr
				}
				kubearchiveNotFound = true
			}
		} else {
			resources, parseErr := o.parseResourcesFromBytes(bodyBytes)
			if parseErr != nil {
				// If we have k8s resources, continue with those
				if len(k8sResources) > 0 {
					kubearchiveNotFound = true
				} else {
					return &APIError{
						StatusCode: 200,
						URL:        "KubeArchive API",
						Message:    fmt.Sprintf("error parsing resources from KubeArchive: %v", parseErr),
						Body:       string(bodyBytes),
					}
				}
			} else {
				kubearchiveResources = resources
			}
		}
	} else {
		kubearchiveNotFound = true
	}

	// Handle case where both APIs returned not found
	if k8sNotFound && kubearchiveNotFound {
		if o.Name != "" {
			if o.InCluster && o.Archived {
				return fmt.Errorf("resource not found in Kubernetes or KubeArchive")
			} else if o.InCluster {
				return fmt.Errorf("resource not found in Kubernetes cluster")
			} else if o.Archived {
				return fmt.Errorf("resource not found in KubeArchive")
			}
		}
		if o.InCluster && o.Archived {
			return fmt.Errorf("no resources found in Kubernetes or KubeArchive")
		} else if o.InCluster {
			return fmt.Errorf("no resources found in Kubernetes cluster")
		} else if o.Archived {
			return fmt.Errorf("no resources found in KubeArchive")
		}
	}

	// Build resource availability map
	resourceMap := make(map[string]*ResourceWithAvailability)

	// Add Kubernetes resources
	for _, obj := range k8sResources {
		key := string(obj.GetUID())
		resourceMap[key] = &ResourceWithAvailability{
			Resource:  obj,
			InCluster: true,
			Archived:  false,
		}
	}

	// Add KubeArchive resources
	for _, obj := range kubearchiveResources {
		key := string(obj.GetUID())
		if existing, exists := resourceMap[key]; exists {
			// Resource exists in both - mark as archived too
			existing.Archived = true
		} else {
			// Resource only in archive
			resourceMap[key] = &ResourceWithAvailability{
				Resource:  obj,
				InCluster: false,
				Archived:  true,
			}
		}
	}

	// Convert to slice for printing
	finalResources := make([]*ResourceWithAvailability, 0, len(resourceMap))
	for _, obj := range resourceMap {
		finalResources = append(finalResources, obj)
	}

	if len(finalResources) == 0 {
		if o.Name != "" {
			if o.InCluster && o.Archived {
				return fmt.Errorf("resource not found in Kubernetes or KubeArchive")
			} else if o.InCluster {
				return fmt.Errorf("resource not found in Kubernetes cluster")
			} else if o.Archived {
				return fmt.Errorf("resource not found in KubeArchive")
			}
		}
		if o.AllNamespaces {
			return fmt.Errorf("no resources found")
		} else {
			namespace, nsErr := o.GetNamespace()
			if nsErr != nil {
				return nsErr
			}
			return fmt.Errorf("no resources found in %s namespace", namespace)
		}
	}

	return o.printResources(finalResources)
}

func (o *GetOptions) printResources(resources []*ResourceWithAvailability) error {
	// Sort resources by creation timestamp (newest first)
	sortResourcesByCreationTime(resources)

	if o.OutputFormat != nil && *o.OutputFormat != "" {
		// For JSON/YAML output, just print the resources without availability info
		printer, printerErr := o.JSONYamlPrintFlags.ToPrinter(*o.OutputFormat)
		if printerErr != nil {
			return printerErr
		}
		list := &unstructured.UnstructuredList{
			Object: map[string]interface{}{
				"kind":       "List",
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"resourceVersion": "",
				},
			},
		}

		for _, resourceWithAvailability := range resources {
			list.Items = append(list.Items, *resourceWithAvailability.Resource)
		}

		if printErr := printer.PrintObj(list, o.Out); printErr != nil {
			return printErr
		}
	} else {
		// Custom table output with availability columns
		return o.printCustomTable(resources)
	}

	return nil
}

func (o *GetOptions) printCustomTable(resources []*ResourceWithAvailability) error {
	w := tabwriter.NewWriter(o.Out, 0, 0, 3, ' ', 0)
	defer w.Flush()

	// Print header
	fmt.Fprintln(w, "NAME\tIN-CLUSTER\tARCHIVED\tAGE")

	// Print each resource
	for _, resourceWithAvailability := range resources {
		obj := resourceWithAvailability.Resource
		name := obj.GetName()

		// Format availability columns
		inCluster := "no"
		if resourceWithAvailability.InCluster {
			inCluster = "yes"
		}

		archived := "no"
		if resourceWithAvailability.Archived {
			archived = "yes"
		}

		// Calculate age
		age := "<unknown>"
		if !obj.GetCreationTimestamp().Time.IsZero() {
			age = duration.HumanDuration(time.Since(obj.GetCreationTimestamp().Time))
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, inCluster, archived, age)
	}

	return nil
}
