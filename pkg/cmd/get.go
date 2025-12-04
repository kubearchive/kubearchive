// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

type GetOptions struct {
	genericiooptions.IOStreams
	KARetrieverCommand
	AllNamespaces          bool
	APIPath                string
	ResourceInfo           *ResourceInfo
	Name                   string
	LabelSelector          string
	OutputFormat           string
	JSONYamlPrintFlags     *genericclioptions.JSONYamlPrintFlags
	IsValidOutput          bool
	InCluster              bool
	Archived               bool
	Limit                  int
	After                  time.Time
	Before                 time.Time
	kubearchiveQueryParams url.Values
}

// ResourceWithAvailability tracks a resource and its availability in different APIs
type ResourceWithAvailability struct {
	Resource  *unstructured.Unstructured
	InCluster bool
	Archived  bool
}

// KubeArchiveResponse represents the response structure from KubeArchive API
type KubeArchiveResponse struct {
	Kind       string                      `json:"kind"`
	APIVersion string                      `json:"apiVersion"`
	Metadata   map[string]interface{}      `json:"metadata"`
	Items      []unstructured.Unstructured `json:"items"`
}

// getContinueToken extracts the continue token from KubeArchive API response metadata
func getContinueToken(bodyBytes []byte) string {
	var response KubeArchiveResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return ""
	}
	if response.Metadata != nil {
		if continueToken, ok := response.Metadata["continue"].(string); ok {
			return continueToken
		}
	}
	return ""
}

func NewGetOptions() *GetOptions {
	return &GetOptions{
		OutputFormat:       "",
		JSONYamlPrintFlags: genericclioptions.NewJSONYamlPrintFlags(),
		IOStreams: genericiooptions.IOStreams{
			In:     os.Stdin,
			Out:    os.Stdout,
			ErrOut: os.Stderr,
		},
		KARetrieverCommand: NewKARetrieverOptions(),
		Limit:              100, // Default limit as per API
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
			if err := o.Complete(cmd.Flags(), args); err != nil {
				return err
			}
			return o.Run()
		},
	}

	cmd.Flags().BoolVarP(&o.AllNamespaces, "all-namespaces", "A", o.AllNamespaces, "If present, list the requested object(s) across all namespaces. Namespace in current context is ignored even if specified with --namespace.")
	cmd.Flags().StringVarP(&o.LabelSelector, "selector", "l", o.LabelSelector, "Selector (label query) to filter on, supports '=', '==', '!=', 'in', 'notin'.(e.g. -l key1=value1,key2=value2,key3 in (value3)). Matching objects must satisfy all of the specified label constraints.")
	cmd.Flags().BoolVar(&o.InCluster, "in-cluster", true, "Include resources from the Kubernetes cluster.")
	cmd.Flags().BoolVar(&o.Archived, "archived", true, "Include resources from KubeArchive.")
	cmd.Flags().IntVar(&o.Limit, "limit", o.Limit, "Maximum number of resources to return (default 100, max 1000).")
	cmd.Flags().TimeVar(&o.After, "after", time.Time{}, []string{time.RFC3339}, "Only return resources created after this timestamp (RFC3339 format, e.g., 2023-01-01T12:00:00Z).")
	cmd.Flags().TimeVar(&o.Before, "before", time.Now().Add(1*time.Hour), []string{time.RFC3339}, "Only return resources created before this timestamp (RFC3339 format, e.g., 2023-12-31T12:00:00Z).")
	o.AddRetrieverFlags(cmd.Flags())
	o.JSONYamlPrintFlags.AddFlags(cmd)
	cmd.Flags().StringVarP(&o.OutputFormat, "output", "o", o.OutputFormat, fmt.Sprintf(`Output format. One of: (%s).`, strings.Join(o.JSONYamlPrintFlags.AllowedFormats(), ", ")))

	return cmd
}

// buildKubeArchiveQueryParams creates query parameters for KubeArchive API
func (o *GetOptions) buildKubeArchiveQueryParams() url.Values {
	// Parse existing query parameters from base API path
	var params url.Values
	if strings.Contains(o.APIPath, "?") {
		parts := strings.SplitN(o.APIPath, "?", 2)
		var err error
		params, err = url.ParseQuery(parts[1])
		if err != nil {
			params = url.Values{}
		}
	} else {
		params = url.Values{}
	}

	// Add KubeArchive-specific parameters
	params.Set("limit", fmt.Sprintf("%d", o.Limit))
	if !o.After.IsZero() {
		params.Set("creationTimestampAfter", o.After.Format(time.RFC3339))
	}
	if !o.Before.Before(time.Now()) {
		params.Set("creationTimestampBefore", o.Before.Format(time.RFC3339))
	}

	return params
}

func (o *GetOptions) Complete(flags *pflag.FlagSet, args []string) error {
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
	if flags.Changed("archived") && o.Archived {
		o.InCluster = false
	}
	if flags.Changed("in-cluster") && o.InCluster {
		o.Archived = false
	}

	// Validate limit
	if o.Limit < 1 || o.Limit > 1000 {
		return fmt.Errorf("limit must be between 1 and 1000")
	}

	// Validate timestamp order
	if o.Before.Before(o.After) || o.Before.Equal(o.After) {
		return fmt.Errorf("--before must be after --after")
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

	// Pre-compute KubeArchive query parameters
	o.kubearchiveQueryParams = o.buildKubeArchiveQueryParams()

	return nil
}

// insertSortedResourcesOptimized inserts resources into a sorted slice with deduplication and limit handling
func (o *GetOptions) insertSortedResourcesOptimized(
	init []*ResourceWithAvailability,
	resources []*unstructured.Unstructured,
	fromK8s bool) ([]*ResourceWithAvailability, bool, bool) {

	result := make([]*ResourceWithAvailability, len(init), min(len(resources)+len(init), o.Limit))
	copy(result, init)

	var k8sTrimmed, k9eTrimmed bool

	for _, resource := range resources {

		idx, found := slices.BinarySearchFunc(result, resource, cmpResource)

		if found {
			if fromK8s {
				result[idx].InCluster = true
			} else {
				result[idx].Archived = true
			}
			continue
		}

		resourceWithAvailability := &ResourceWithAvailability{
			Resource:  resource,
			InCluster: fromK8s,
			Archived:  !fromK8s,
		}

		// Insert at the correct position
		if idx < cap(result) {
			if len(result) == cap(result) {
				// If the slice is full we are going to trim the last element
				k8sTrimmed = k8sTrimmed || result[len(result)-1].InCluster
				k9eTrimmed = k9eTrimmed || result[len(result)-1].Archived
			} else {
				// Increase the length
				result = append(result, nil)
			}
			copy(result[idx+1:], result[idx:])
			result[idx] = resourceWithAvailability

		} else {
			// Not inserted, so resources are trimmed
			k8sTrimmed = k8sTrimmed || fromK8s
			k9eTrimmed = k9eTrimmed || !fromK8s
		}
	}

	return result, k8sTrimmed, k9eTrimmed
}

// cmpResource
func cmpResource(existing *ResourceWithAvailability, target *unstructured.Unstructured) int {
	existingTime := existing.Resource.GetCreationTimestamp().Time
	targetTime := target.GetCreationTimestamp().Time

	if targetTime.After(existingTime) {
		return 1
	} else if targetTime.Before(existingTime) {
		return -1
	} else if target.GetUID() == existing.Resource.GetUID() {
		return 0
	}
	// If timestamps are equal, but not UUIDs, sort by name
	return strings.Compare(target.GetName(), existing.Resource.GetName())
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

func (o *GetOptions) Run() error {
	var sortedResources []*ResourceWithAvailability
	var k8sTrimmed, k8sTrimmedInMerge, kubeArchiveTrimmed bool
	var k8sNotFound, kubearchiveNotFound bool
	var kubearchiveContinueToken string

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
			// Process Kubernetes resources: filter by timestamp, sort, and limit
			sortedResources, k8sTrimmed, _ = o.insertSortedResourcesOptimized(make([]*ResourceWithAvailability, 0), resources, true)
		}
	} else {
		k8sNotFound = true
	}

	// Get resources from KubeArchive API (only if --archived is true)
	if o.Archived {
		// Build KubeArchive API path using pre-computed query parameters
		var kubearchiveAPIPath string
		basePath := o.APIPath
		if strings.Contains(basePath, "?") {
			basePath = strings.SplitN(basePath, "?", 2)[0]
		}
		if o.kubearchiveQueryParams.Encode() != "" {
			kubearchiveAPIPath = fmt.Sprintf("%s?%s", basePath, o.kubearchiveQueryParams.Encode())
		} else {
			kubearchiveAPIPath = basePath
		}

		bodyBytes, apiErr := o.GetFromAPI(KubeArchive, kubearchiveAPIPath)
		if apiErr != nil {
			// If KubeArchive fails with authentication error, don't fall back to just Kubernetes
			if apiErr.StatusCode == http.StatusUnauthorized ||
				strings.Contains(apiErr.Message, "empty authorization bearer token given") ||
				strings.Contains(apiErr.Message, "authentication failed") {
				return fmt.Errorf("KubeArchive authentication required: %s", apiErr.Message)
			}
			// If we have k8s resources, continue with those regardless of the error type
			if len(sortedResources) > 0 {
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
				if len(sortedResources) > 0 {
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
				kubearchiveContinueToken = getContinueToken(bodyBytes)
				// Merge KubeArchive resources efficiently with existing sorted list
				sortedResources, k8sTrimmedInMerge, kubeArchiveTrimmed = o.insertSortedResourcesOptimized(sortedResources, resources, false)
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

	if len(sortedResources) == 0 {
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

	// Print resources
	err := o.printResources(sortedResources)
	if err != nil {
		return err
	}

	// If we have more in-cluster resources and no continue token, suggest --in-cluster
	moreInCluster := k8sTrimmed || k8sTrimmedInMerge
	// If we have a continue token or more archived-only resources, suggest --archived
	moreArchived := kubearchiveContinueToken != "" || kubeArchiveTrimmed

	return o.printPaginationMessage(sortedResources, moreInCluster, moreArchived)
}

// printPaginationMessage prints a message indicating results were trimmed and suggests the next command
func (o *GetOptions) printPaginationMessage(resources []*ResourceWithAvailability, moreInCluster, moreArchived bool) error {
	if !moreInCluster && !moreArchived {
		return nil
	}

	// Check if ResourceInfo is available
	if o.ResourceInfo == nil {
		return fmt.Errorf("error generating command for getting the next page: no resource info")
	}

	// Get the timestamp of the oldest resource shown
	oldestResource := resources[len(resources)-1]
	oldestTimestamp := oldestResource.Resource.GetCreationTimestamp().Time
	if oldestTimestamp.IsZero() {
		return nil // Can't generate pagination command without timestamp
	}

	// Build the next command
	var nextCmd strings.Builder
	nextCmd.WriteString("kubectl ka get ")

	// Add resource type
	if o.ResourceInfo.Group == "" {
		nextCmd.WriteString(o.ResourceInfo.Resource)
	} else {
		nextCmd.WriteString(fmt.Sprintf("%s.%s.%s", o.ResourceInfo.Resource, o.ResourceInfo.Version, o.ResourceInfo.Group))
	}

	// Add namespace if applicable
	if o.ResourceInfo.Namespaced && !o.AllNamespaces {
		namespace, _ := o.GetNamespace()
		nextCmd.WriteString(fmt.Sprintf(" --namespace %s", namespace))
	} else if o.AllNamespaces {
		nextCmd.WriteString(" --all-namespaces")
	}

	// Add label selector if provided
	if o.LabelSelector != "" {
		nextCmd.WriteString(fmt.Sprintf(" --selector '%s'", o.LabelSelector))
	}

	// Add limit
	nextCmd.WriteString(fmt.Sprintf(" --limit %d", o.Limit))

	// Add before timestamp (set to just before the oldest resource's timestamp to avoid including it)
	// Subtract 1 nanosecond to ensure we don't include the same resource again
	beforeTimestamp := oldestTimestamp.Add(-1 * time.Nanosecond)
	nextCmd.WriteString(fmt.Sprintf(" --before %s", beforeTimestamp.Format(time.RFC3339Nano)))

	// Add after timestamp if originally provided
	if !o.After.IsZero() {
		nextCmd.WriteString(fmt.Sprintf(" --after %s", o.After.Format(time.RFC3339)))
	}

	// Add appropriate flags based on where more resources are likely to be found
	if moreArchived && !moreInCluster {
		nextCmd.WriteString(" --archived")
	} else if moreInCluster && !moreArchived {
		nextCmd.WriteString(" --in-cluster")
	}

	// Add output format if specified
	if o.OutputFormat != "" {
		nextCmd.WriteString(fmt.Sprintf(" --output %s", o.OutputFormat))
	}

	fmt.Fprintf(o.ErrOut, "\nResults are trimmed to %d, to get the next page of elements, run:\n  %s\n", len(resources), nextCmd.String())
	return nil
}

func (o *GetOptions) printResources(resources []*ResourceWithAvailability) error {

	if o.OutputFormat != "" {
		// For JSON/YAML output, just print the resources without availability info
		printer, printerErr := o.JSONYamlPrintFlags.ToPrinter(o.OutputFormat)
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
