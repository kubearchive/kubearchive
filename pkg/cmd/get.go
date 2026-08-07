// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/yaml"
)

const serverMaxLimit = 1000

type GetOptions struct {
	genericiooptions.IOStreams
	KARetrieverCommand
	AllNamespaces          bool
	Count                  bool
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
	PageSize               int
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
	cmd.Flags().BoolVar(&o.Count, "count", false, "Only print the count of matching resources (KubeArchive only).")
	cmd.Flags().StringVarP(&o.LabelSelector, "selector", "l", o.LabelSelector, "Selector (label query) to filter on, supports '=', '==', '!=', 'in', 'notin'.(e.g. -l key1=value1,key2=value2,key3 in (value3)). Matching objects must satisfy all of the specified label constraints.")
	cmd.Flags().BoolVar(&o.InCluster, "in-cluster", true, "Include resources from the Kubernetes cluster.")
	cmd.Flags().BoolVar(&o.Archived, "archived", true, "Include resources from KubeArchive.")
	cmd.Flags().IntVar(&o.Limit, "limit", o.Limit, "Maximum number of resources to return (default 100, 0 for all).")
	cmd.Flags().IntVar(&o.PageSize, "page-size", 0, "Number of resources per KubeArchive API request (default: min(limit, 1000), max: 1000).")
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
	if o.Count {
		params.Set("count", "true")
	} else {
		params.Set("limit", fmt.Sprintf("%d", o.pageSize()))
	}
	if !o.After.IsZero() {
		params.Set("creationTimestampAfter", o.After.Format(time.RFC3339))
	}
	if o.Before.Before(time.Now()) {
		params.Set("creationTimestampBefore", o.Before.Format(time.RFC3339))
	}

	return params
}

func (o *GetOptions) pageSize() int {
	if o.PageSize > 0 {
		return min(o.PageSize, serverMaxLimit)
	}
	if o.Limit == 0 || o.Limit > serverMaxLimit {
		return serverMaxLimit
	}
	return o.Limit
}

func (o *GetOptions) handleCountResponse(bodyBytes []byte, apiErr *APIError) error {
	if apiErr != nil {
		return apiErr
	}
	var countResponse struct {
		Count int64 `json:"count"`
	}
	if err := json.Unmarshal(bodyBytes, &countResponse); err != nil {
		return fmt.Errorf("error parsing count response: %w", err)
	}
	fmt.Fprintf(o.Out, "%d\n", countResponse.Count)
	return nil
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

	// --count only works with KubeArchive (Kubernetes API doesn't support server-side counting)
	if o.Count {
		if o.Name != "" {
			return fmt.Errorf("cannot use --count with a specific resource name")
		}
		if !o.Archived {
			return fmt.Errorf("--count requires --archived (Kubernetes API does not support server-side counting)")
		}
		if flags.Changed("limit") {
			return fmt.Errorf("cannot use --count with --limit")
		}
		o.InCluster = false
	}

	// Validate limit
	if o.Limit < 0 {
		return fmt.Errorf("limit must be 0 (all) or a positive number")
	}

	// Validate page size
	if o.PageSize < 0 || o.PageSize > serverMaxLimit {
		return fmt.Errorf("page-size must be between 0 and %d (0 means automatic)", serverMaxLimit)
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

// mergeSortedResources merges new resources into an existing sorted slice with
// deduplication by UID. No limit is applied.
func mergeSortedResources(
	init []*ResourceWithAvailability,
	resources []*unstructured.Unstructured,
	fromK8s bool) []*ResourceWithAvailability {

	result := make([]*ResourceWithAvailability, len(init), len(resources)+len(init))
	copy(result, init)

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

		result = slices.Insert(result, idx, resourceWithAvailability)
	}

	return result
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

// extractWatermark returns the creationTimestamp of the oldest resource in a
// newest-first sorted slice. Returns zero time if the slice is empty.
func extractWatermark(resources []*unstructured.Unstructured) time.Time {
	if len(resources) == 0 {
		return time.Time{}
	}
	return resources[len(resources)-1].GetCreationTimestamp().Time
}

// splitByWatermark partitions resources into those at or above the watermark
// and those below it. Input must be sorted newest-first.
func splitByWatermark(resources []*unstructured.Unstructured, watermark time.Time) (above, below []*unstructured.Unstructured) {
	if watermark.IsZero() {
		return resources, nil
	}
	idx := sort.Search(len(resources), func(i int) bool {
		return resources[i].GetCreationTimestamp().Time.Before(watermark)
	})
	return resources[:idx], resources[idx:]
}

type resourceStreamer interface {
	emitBatch(resources []*ResourceWithAvailability) int
	close() error
	count() int
	oldest() time.Time
}

// tableStreamer manages streaming table output with consistent headers
type tableStreamer struct {
	writer        *tabwriter.Writer
	headerDone    bool
	emittedCount  int
	limit         int
	oldestEmitted time.Time
}

func newTableStreamer(out io.Writer, limit int) *tableStreamer {
	return &tableStreamer{
		writer: tabwriter.NewWriter(out, 0, 0, 3, ' ', 0),
		limit:  limit,
	}
}

// emitBatch writes resources to the tabwriter without flushing, returning the
// number of resources actually written. Resources beyond the limit are skipped.
func (ts *tableStreamer) emitBatch(resources []*ResourceWithAvailability) int {
	if len(resources) == 0 {
		return 0
	}

	if !ts.headerDone {
		fmt.Fprintln(ts.writer, "NAME\tIN-CLUSTER\tARCHIVED\tAGE")
		ts.headerDone = true
	}

	emitted := 0
	for _, rwa := range resources {
		if ts.limit > 0 && ts.emittedCount >= ts.limit {
			break
		}

		obj := rwa.Resource
		inCluster := "no"
		if rwa.InCluster {
			inCluster = "yes"
		}
		archived := "no"
		if rwa.Archived {
			archived = "yes"
		}
		age := "<unknown>"
		if !obj.GetCreationTimestamp().Time.IsZero() {
			age = duration.HumanDuration(time.Since(obj.GetCreationTimestamp().Time))
		}

		fmt.Fprintf(ts.writer, "%s\t%-10s\t%-8s\t%s\n", obj.GetName(), inCluster, archived, age)
		ts.emittedCount++
		ts.oldestEmitted = obj.GetCreationTimestamp().Time
		emitted++
	}

	ts.writer.Flush()

	return emitted
}

func (ts *tableStreamer) close() error {
	return ts.writer.Flush()
}

func (ts *tableStreamer) count() int        { return ts.emittedCount }
func (ts *tableStreamer) oldest() time.Time { return ts.oldestEmitted }

type jsonStreamer struct {
	writer        io.Writer
	headerDone    bool
	closed        bool
	emittedCount  int
	limit         int
	oldestEmitted time.Time
}

func newJSONStreamer(out io.Writer, limit int) *jsonStreamer {
	return &jsonStreamer{writer: out, limit: limit}
}

func (js *jsonStreamer) emitBatch(resources []*ResourceWithAvailability) int {
	emitted := 0
	for _, rwa := range resources {
		if js.limit > 0 && js.emittedCount >= js.limit {
			break
		}

		if !js.headerDone {
			fmt.Fprint(js.writer, "{\n    \"apiVersion\": \"v1\",\n    \"items\": [\n")
			js.headerDone = true
		}

		itemBytes, err := json.MarshalIndent(rwa.Resource.Object, "        ", "    ")
		if err != nil {
			continue
		}

		if js.emittedCount > 0 {
			fmt.Fprint(js.writer, ",\n")
		}
		fmt.Fprintf(js.writer, "        %s", string(itemBytes))

		js.emittedCount++
		js.oldestEmitted = rwa.Resource.GetCreationTimestamp().Time
		emitted++
	}
	return emitted
}

func (js *jsonStreamer) close() error {
	if js.closed || !js.headerDone {
		return nil
	}
	js.closed = true
	_, err := fmt.Fprint(js.writer, "\n    ],\n    \"kind\": \"List\",\n    \"metadata\": {\n        \"resourceVersion\": \"\"\n    }\n}\n")
	return err
}

func (js *jsonStreamer) count() int        { return js.emittedCount }
func (js *jsonStreamer) oldest() time.Time { return js.oldestEmitted }

type yamlStreamer struct {
	writer        io.Writer
	headerDone    bool
	closed        bool
	emittedCount  int
	limit         int
	oldestEmitted time.Time
}

func newYAMLStreamer(out io.Writer, limit int) *yamlStreamer {
	return &yamlStreamer{writer: out, limit: limit}
}

func (ys *yamlStreamer) emitBatch(resources []*ResourceWithAvailability) int {
	emitted := 0
	for _, rwa := range resources {
		if ys.limit > 0 && ys.emittedCount >= ys.limit {
			break
		}

		if !ys.headerDone {
			fmt.Fprint(ys.writer, "apiVersion: v1\nitems:\n")
			ys.headerDone = true
		}

		itemBytes, err := yaml.Marshal(rwa.Resource.Object)
		if err != nil {
			continue
		}

		lines := strings.Split(strings.TrimRight(string(itemBytes), "\n"), "\n")
		for i, line := range lines {
			if i == 0 {
				fmt.Fprintf(ys.writer, "- %s\n", line)
			} else {
				fmt.Fprintf(ys.writer, "  %s\n", line)
			}
		}

		ys.emittedCount++
		ys.oldestEmitted = rwa.Resource.GetCreationTimestamp().Time
		emitted++
	}
	return emitted
}

func (ys *yamlStreamer) close() error {
	if ys.closed || !ys.headerDone {
		return nil
	}
	ys.closed = true
	_, err := fmt.Fprint(ys.writer, "kind: List\nmetadata:\n  resourceVersion: \"\"\n")
	return err
}

func (ys *yamlStreamer) count() int        { return ys.emittedCount }
func (ys *yamlStreamer) oldest() time.Time { return ys.oldestEmitted }

func (o *GetOptions) newResourceStreamer() (resourceStreamer, error) {
	switch o.OutputFormat {
	case "":
		return newTableStreamer(o.Out, o.Limit), nil
	case "json":
		return newJSONStreamer(o.Out, o.Limit), nil
	case "yaml":
		return newYAMLStreamer(o.Out, o.Limit), nil
	default:
		_, err := o.JSONYamlPrintFlags.ToPrinter(o.OutputFormat)
		return nil, err
	}
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

func (o *GetOptions) fetchK8sResources() ([]*unstructured.Unstructured, bool, error) {
	if !o.InCluster {
		return nil, true, nil
	}
	bodyBytes, apiErr := o.GetFromAPI(Kubernetes, o.APIPath)
	if apiErr != nil {
		if apiErr.StatusCode != http.StatusNotFound {
			return nil, false, apiErr
		}
		return nil, true, nil
	}
	resources, parseErr := o.parseResourcesFromBytes(bodyBytes)
	if parseErr != nil {
		return nil, false, &APIError{
			StatusCode: 200,
			URL:        "Kubernetes API",
			Message:    fmt.Sprintf("error parsing resources from the cluster: %v", parseErr),
			Body:       string(bodyBytes),
		}
	}
	return resources, false, nil
}

func (o *GetOptions) kubearchiveAPIURL(basePath string, queryParams url.Values) string {
	if encoded := queryParams.Encode(); encoded != "" {
		return fmt.Sprintf("%s?%s", basePath, encoded)
	}
	return basePath
}

func (o *GetOptions) checkKubeArchiveAuthError(apiErr *APIError) error {
	if apiErr == nil {
		return nil
	}
	if apiErr.StatusCode == http.StatusUnauthorized ||
		strings.Contains(apiErr.Message, "empty authorization bearer token given") ||
		strings.Contains(apiErr.Message, "authentication failed") {
		return fmt.Errorf("KubeArchive authentication required: %s", apiErr.Message)
	}
	return nil
}

func (o *GetOptions) noResourcesFoundError(k8sNotFound, kubearchiveNotFound bool) error {
	if k8sNotFound && kubearchiveNotFound {
		if o.Name != "" {
			if o.InCluster && o.Archived {
				return fmt.Errorf("resource not found in Kubernetes or KubeArchive")
			} else if o.InCluster {
				return fmt.Errorf("resource not found in Kubernetes cluster")
			}
			return fmt.Errorf("resource not found in KubeArchive")
		}
		if o.InCluster && o.Archived {
			return fmt.Errorf("no resources found in Kubernetes or KubeArchive")
		} else if o.InCluster {
			return fmt.Errorf("no resources found in Kubernetes cluster")
		}
		return fmt.Errorf("no resources found in KubeArchive")
	}

	if o.Name != "" {
		if o.InCluster && o.Archived {
			return fmt.Errorf("resource not found in Kubernetes or KubeArchive")
		} else if o.InCluster {
			return fmt.Errorf("resource not found in Kubernetes cluster")
		}
		return fmt.Errorf("resource not found in KubeArchive")
	}
	if o.AllNamespaces {
		return fmt.Errorf("no resources found")
	}
	namespace, nsErr := o.GetNamespace()
	if nsErr != nil {
		return nsErr
	}
	return fmt.Errorf("no resources found in %s namespace", namespace)
}

func (o *GetOptions) Run() error {
	if o.Count {
		return o.runCount()
	}

	k8sResources, k8sNotFound, err := o.fetchK8sResources()
	if err != nil {
		return err
	}

	return o.runStreaming(k8sResources, k8sNotFound)
}

func (o *GetOptions) runCount() error {
	basePath := o.APIPath
	if strings.Contains(basePath, "?") {
		basePath = strings.SplitN(basePath, "?", 2)[0]
	}
	queryParams := url.Values{}
	for k, v := range o.kubearchiveQueryParams {
		queryParams[k] = v
	}
	bodyBytes, apiErr := o.GetFromAPI(KubeArchive, o.kubearchiveAPIURL(basePath, queryParams))
	if authErr := o.checkKubeArchiveAuthError(apiErr); authErr != nil {
		return authErr
	}
	return o.handleCountResponse(bodyBytes, apiErr)
}

func (o *GetOptions) runStreaming(k8sResources []*unstructured.Unstructured, k8sNotFound bool) error {
	streamer, err := o.newResourceStreamer()
	if err != nil {
		return err
	}
	defer streamer.close()

	sort.Slice(k8sResources, func(i, j int) bool {
		return k8sResources[i].GetCreationTimestamp().Time.After(
			k8sResources[j].GetCreationTimestamp().Time)
	})

	remaining := k8sResources
	kubearchiveNotFound := !o.Archived
	var moreInCluster, moreArchived bool

	if o.Archived {
		basePath := o.APIPath
		if strings.Contains(basePath, "?") {
			basePath = strings.SplitN(basePath, "?", 2)[0]
		}
		queryParams := url.Values{}
		for k, v := range o.kubearchiveQueryParams {
			queryParams[k] = v
		}

		for {
			bodyBytes, apiErr := o.GetFromAPI(KubeArchive, o.kubearchiveAPIURL(basePath, queryParams))
			if authErr := o.checkKubeArchiveAuthError(apiErr); authErr != nil {
				return authErr
			}

			if apiErr != nil {
				if streamer.count() > 0 || len(remaining) > 0 {
					kubearchiveNotFound = true
				} else if apiErr.StatusCode != http.StatusNotFound {
					return apiErr
				} else {
					kubearchiveNotFound = true
				}
				break
			}

			pageResources, parseErr := o.parseResourcesFromBytes(bodyBytes)
			if parseErr != nil {
				if streamer.count() > 0 || len(remaining) > 0 {
					kubearchiveNotFound = true
				} else {
					return &APIError{
						StatusCode: 200,
						URL:        "KubeArchive API",
						Message:    fmt.Sprintf("error parsing resources from KubeArchive: %v", parseErr),
						Body:       string(bodyBytes),
					}
				}
				break
			}

			lastContinueToken := getContinueToken(bodyBytes)

			if len(pageResources) == 0 {
				if lastContinueToken == "" {
					break
				}
				queryParams.Set("continue", lastContinueToken)
				continue
			}

			watermark := extractWatermark(pageResources)

			above, below := splitByWatermark(remaining, watermark)
			remaining = below

			batch := mergeSortedResources(nil, pageResources, false)
			batch = mergeSortedResources(batch, above, true)

			emitted := streamer.emitBatch(batch)
			if emitted < len(batch) {
				for _, r := range batch[emitted:] {
					moreInCluster = moreInCluster || r.InCluster
					moreArchived = moreArchived || r.Archived
				}
			}

			if o.Limit > 0 && streamer.count() >= o.Limit {
				if lastContinueToken != "" {
					moreArchived = true
				}
				break
			}
			if lastContinueToken == "" {
				break
			}

			queryParams.Set("continue", lastContinueToken)
			if o.Limit > 0 {
				rem := o.Limit - streamer.count()
				queryParams.Set("limit", fmt.Sprintf("%d", min(rem, o.pageSize())))
			}
		}
	}

	// Emit remaining K8s resources not yet settled by any watermark
	if len(remaining) > 0 {
		finalBatch := make([]*ResourceWithAvailability, 0, len(remaining))
		for _, r := range remaining {
			finalBatch = append(finalBatch, &ResourceWithAvailability{Resource: r, InCluster: true})
		}
		emitted := streamer.emitBatch(finalBatch)
		if emitted < len(remaining) {
			moreInCluster = true
		}
	}

	if streamer.count() == 0 {
		return o.noResourcesFoundError(k8sNotFound, kubearchiveNotFound)
	}

	if err := streamer.close(); err != nil {
		return err
	}

	return o.printPaginationMessage(streamer.count(), streamer.oldest(), moreInCluster, moreArchived)
}

// printPaginationMessage prints a message indicating results were trimmed and suggests the next command
func (o *GetOptions) printPaginationMessage(count int, oldestTimestamp time.Time, moreInCluster, moreArchived bool) error {
	if !moreInCluster && !moreArchived {
		return nil
	}

	if o.ResourceInfo == nil {
		return fmt.Errorf("error generating command for getting the next page: no resource info")
	}

	if oldestTimestamp.IsZero() {
		return nil
	}

	var nextCmd strings.Builder
	nextCmd.WriteString("kubectl ka get ")

	if o.ResourceInfo.Group == "" {
		nextCmd.WriteString(o.ResourceInfo.Resource)
	} else {
		fmt.Fprintf(&nextCmd, "%s.%s.%s", o.ResourceInfo.Resource, o.ResourceInfo.Version, o.ResourceInfo.Group)
	}

	if o.ResourceInfo.Namespaced && !o.AllNamespaces {
		namespace, _ := o.GetNamespace()
		fmt.Fprintf(&nextCmd, " --namespace %s", namespace)
	} else if o.AllNamespaces {
		nextCmd.WriteString(" --all-namespaces")
	}

	if o.LabelSelector != "" {
		fmt.Fprintf(&nextCmd, " --selector '%s'", o.LabelSelector)
	}

	fmt.Fprintf(&nextCmd, " --limit %d", o.Limit)

	// Subtract 1 nanosecond to ensure we don't include the same resource again
	// Use UTC to match how Kubernetes stores creation timestamps, ensuring correct string comparison in the DB
	beforeTimestamp := oldestTimestamp.Add(-1 * time.Nanosecond).UTC()
	fmt.Fprintf(&nextCmd, " --before %s", beforeTimestamp.Format(time.RFC3339Nano))

	if !o.After.IsZero() {
		fmt.Fprintf(&nextCmd, " --after %s", o.After.Format(time.RFC3339))
	}

	if moreArchived && !moreInCluster {
		nextCmd.WriteString(" --archived")
	} else if moreInCluster && !moreArchived {
		nextCmd.WriteString(" --in-cluster")
	}

	if o.OutputFormat != "" {
		fmt.Fprintf(&nextCmd, " --output %s", o.OutputFormat)
	}

	fmt.Fprintf(o.ErrOut, "\nResults are trimmed to %d, to get the next page of elements, run:\n  %s\n", count, nextCmd.String())
	return nil
}

