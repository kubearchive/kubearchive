// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"

	"github.com/kubearchive/kubearchive/pkg/cmd/config"
)

// DiscoveryInterface defines the methods we need from the discovery client
type DiscoveryInterface interface {
	ServerGroups() (*v1.APIGroupList, error)
	ServerGroupsAndResources() ([]*v1.APIGroup, []*v1.APIResourceList, error)
}

// ResourceInfo holds the resolved resource information from discovery
type ResourceInfo struct {
	Resource     string
	Version      string
	Group        string
	GroupVersion string
	Kind         string
	Namespaced   bool
}

// KARetrieverOptions implements KARetrieverCommand for get/logs commands and KACLICommand for the config command
type KARetrieverOptions struct {
	host               string
	tlsInsecure        bool
	certificatePath    string
	token              string
	certData           []byte
	kubeFlags          *genericclioptions.ConfigFlags
	k8sRESTConfig      *rest.Config
	k9eRESTConfig      *rest.Config
	discoveryClient    DiscoveryInterface
	connectivityTester config.ConnectivityTester
}

// NewKARetrieverOptionsNoEnv creates KARetrieverOptions without env vars
func NewKARetrieverOptionsNoEnv() *KARetrieverOptions {
	return &KARetrieverOptions{
		kubeFlags:          genericclioptions.NewConfigFlags(true),
		connectivityTester: config.NewDefaultConnectivityTester(),
	}
}

// NewKARetrieverOptions creates KARetrieverOptions for retriever commands (get/logs) - loads env vars
func NewKARetrieverOptions() *KARetrieverOptions {
	opts := &KARetrieverOptions{
		host:               "", // No default host - must be configured
		certificatePath:    "",
		kubeFlags:          genericclioptions.NewConfigFlags(true),
		connectivityTester: config.NewDefaultConnectivityTester(),
	}

	// Load environment variables for retriever commands
	if v := os.Getenv("KUBECTL_PLUGIN_KA_HOST"); v != "" {
		opts.host = v
	}
	if v := os.Getenv("KUBECTL_PLUGIN_KA_CERT_PATH"); v != "" {
		opts.certificatePath = v
	}
	if v := os.Getenv("KUBECTL_PLUGIN_KA_TLS_INSECURE"); v != "" {
		opts.tlsInsecure, _ = strconv.ParseBool(v)
	}
	if v := os.Getenv("KUBECTL_PLUGIN_KA_TOKEN"); v != "" {
		opts.token = v
	}

	return opts
}

// CompleteK8sConfig implements KACLICommand interface for Kubernetes configuration
func (opts *KARetrieverOptions) CompleteK8sConfig() error {
	// Load Kubernetes REST Config
	restConfig, err := opts.kubeFlags.ToRESTConfig()
	if err != nil {
		return fmt.Errorf("error creating the REST configuration: %w", err)
	}
	opts.k8sRESTConfig = restConfig
	return nil
}

// GetK8sRESTConfig returns the Kubernetes REST configuration
func (opts *KARetrieverOptions) GetK8sRESTConfig() *rest.Config {
	return opts.k8sRESTConfig
}

// CompleteRetriever implements the full retriever workflow for get/logs commands
func (opts *KARetrieverOptions) CompleteRetriever() error {
	// First complete the basic configuration
	if err := opts.CompleteK8sConfig(); err != nil {
		return err
	}

	client, err := opts.kubeFlags.ToDiscoveryClient()
	if err != nil {
		return fmt.Errorf("failed to create discovery client: %w", err)
	}
	opts.discoveryClient = client

	if opts.kubeFlags.BearerToken != nil && *opts.kubeFlags.BearerToken != "" {
		opts.token = *opts.kubeFlags.BearerToken
	}

	if opts.token == "" && opts.k8sRESTConfig.BearerToken != "" {
		opts.token = opts.k8sRESTConfig.BearerToken
	}

	if opts.certificatePath != "" {
		expandedCertPath, certData, errCert := config.LoadCertData(opts.certificatePath)
		if errCert != nil {
			return errCert
		}
		opts.certificatePath = expandedCertPath
		opts.certData = certData
	}

	// Check env+cli setup is ok
	if err = opts.testConnectivity(); err != nil {
		// If it isn't, try to reconfigure the current cluster
		configManager := config.NewFileConfigManager()
		if err = opts.loadPersistentConfig(configManager); err != nil {
			fmt.Printf("failed to load persistent configuration: %s\n", err)
		}

		// If still not working
		if err = opts.testConnectivity(); err != nil {
			fmt.Println("No working KubeArchive configuration found.")
			fmt.Println()

			ns, errNs := opts.GetNamespace()
			if errNs != nil {
				ns = "default"
			}
			interactiveSetup := config.NewInteractiveSetup(configManager, ns, opts.connectivityTester)

			confirmation, errPrompt := config.PromptForConfirmation("Do you want to setup the configuration for the current connected cluster?", config.DefaultYes)
			if errPrompt != nil {
				return err
			}
			if !confirmation {
				return fmt.Errorf("setup configuration stopped by the user")
			}
			err = interactiveSetup.RunSetup()
			if err != nil {
				return fmt.Errorf("interactive setup failed: %w", err)
			}
			err = opts.loadPersistentConfig(configManager)
			if err != nil {
				return err
			}
		}
	}

	if err = opts.testConnectivity(); err != nil {
		return err
	}
	return opts.setK9eRESTConfig()
}

func (opts *KARetrieverOptions) testConnectivity() error {
	if opts.host == "" {
		return fmt.Errorf("no host provided")
	} else if opts.token == "" {
		return fmt.Errorf("no token provided")
	} else if err := opts.connectivityTester.TestKubeArchiveConnectivity(opts.host, opts.tlsInsecure, opts.token, opts.certData); err != nil {
		return err
	}
	return nil
}

// AddRetrieverFlags adds all archive-related flags to the given flag set
func (opts *KARetrieverOptions) AddRetrieverFlags(flags *pflag.FlagSet) {
	opts.AddK8sFlags(flags)
	flags.StringVar(&opts.host, "host", opts.host, "host where the KubeArchive API Server is listening.")
	flags.BoolVar(&opts.tlsInsecure, "kubearchive-insecure-skip-tls-verify", opts.tlsInsecure, "Allow insecure requests to the KubeArchive API.")
	flags.StringVar(&opts.certificatePath, "kubearchive-certificate-authority", opts.certificatePath, "Path to the certificate authority file.")
}

func (opts *KARetrieverOptions) AddK8sFlags(flags *pflag.FlagSet) {
	opts.kubeFlags.AddFlags(flags)
}

// GetFromAPI retrieves data from either Kubernetes or KubeArchive API
func (opts *KARetrieverOptions) GetFromAPI(api API, path string) ([]byte, *APIError) {
	var restConfig *rest.Config
	var baseURL string

	switch api {
	case Kubernetes:
		restConfig = opts.k8sRESTConfig
		baseURL = restConfig.Host
	case KubeArchive:
		restConfig = opts.k9eRESTConfig
		baseURL = restConfig.Host
	}

	// Create HTTP client using the REST config
	client, err := rest.HTTPClientFor(restConfig)
	if err != nil {
		return nil, &APIError{
			StatusCode: 500,
			URL:        path,
			Message:    fmt.Sprintf("error creating the HTTP client from the REST config: %v", err),
			Body:       "",
		}
	}

	// Build full URL
	fullURL := baseURL + path

	// Create request
	response, err := client.Get(fullURL)
	if err != nil {
		return nil, &APIError{
			StatusCode: 500,
			URL:        fullURL,
			Message:    fmt.Sprintf("error on GET to '%s': %v", fullURL, err),
			Body:       "",
		}
	}

	defer response.Body.Close()
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, &APIError{
			StatusCode: response.StatusCode,
			URL:        fullURL,
			Message:    fmt.Sprintf("error reading response body from '%s': %v", fullURL, err),
			Body:       "",
		}
	}

	if response.StatusCode != http.StatusOK {
		message := extractErrorMessage(bodyBytes, response.StatusCode, fullURL)
		return nil, &APIError{
			StatusCode: response.StatusCode,
			URL:        fullURL,
			Message:    message,
			Body:       string(bodyBytes),
		}
	}

	return bodyBytes, nil
}

func extractErrorMessage(body []byte, statusCode int, url string) string {
	// Try to parse Kubernetes Status object for cleaner error messages
	var status struct {
		Message string `json:"message"`
		Reason  string `json:"reason"`
	}

	if err := json.Unmarshal(body, &status); err == nil && status.Message != "" {
		return status.Message
	}

	// Fallback to raw body or generic message
	bodyStr := string(body)
	if bodyStr != "" {
		return fmt.Sprintf("unable to get '%s': %s (%d)", url, bodyStr, statusCode)
	}

	return fmt.Sprintf("unable to get '%s': HTTP %d", url, statusCode)
}

// GetNamespace get the provided namespace or the namespace used in kubeconfig context
func (opts *KARetrieverOptions) GetNamespace() (string, error) {
	if opts.kubeFlags.Namespace != nil && *opts.kubeFlags.Namespace != "" {
		return *opts.kubeFlags.Namespace, nil
	}
	if rawLoader := opts.kubeFlags.ToRawKubeConfigLoader(); rawLoader != nil {
		ns, _, nsErr := rawLoader.Namespace()
		if nsErr != nil {
			return "", fmt.Errorf("error retrieving namespace from kubeconfig context: %w", nsErr)
		}
		opts.kubeFlags.Namespace = &ns
		return ns, nil
	}
	return "", fmt.Errorf("unable to retrieve namespace from kubeconfig context")
}

// ResolveResourceSpec resolves a resource specification using Kubernetes discovery API
func (opts *KARetrieverOptions) ResolveResourceSpec(resourceSpec string) (*ResourceInfo, error) {
	parts := strings.Split(resourceSpec, ".")

	if len(parts) == 0 || parts[0] == "" {
		return nil, fmt.Errorf("resource name cannot be empty")
	}

	if len(parts) > 3 {
		return nil, fmt.Errorf("invalid resource specification format: %s", resourceSpec)
	}

	resourceName := parts[0]
	var requestedVersion, requestedGroup string

	switch len(parts) {
	case 1:
		// resource only - need to discover both version and group
	case 2:
		// resource.group - need to discover the version for the group
		requestedGroup = parts[1]
	case 3:
		// resource.version.group - everything specified
		requestedVersion = parts[1]
		requestedGroup = parts[2]
	}

	// Find the resource using discovery
	return opts.findResourceInfo(resourceName, requestedVersion, requestedGroup)
}

// findResourceInfo uses discovery to find the correct group, version, and kind for a resource
func (opts *KARetrieverOptions) findResourceInfo(resourceName, requestedVersion, requestedGroup string) (*ResourceInfo, error) {
	// Get all API resources
	_, apiResourceLists, err := opts.discoveryClient.ServerGroupsAndResources()
	if err != nil {
		return nil, fmt.Errorf("failed to discover API resources: %w", err)
	}

	var candidates []*ResourceInfo

	// Search through all API groups and versions
	for _, apiResourceList := range apiResourceLists {
		gv, err := schema.ParseGroupVersion(apiResourceList.GroupVersion)
		if err != nil {
			continue
		}

		// If a specific group is requested, skip other groups
		if requestedGroup != "" && gv.Group != requestedGroup {
			continue
		}

		// If a specific version is requested, skip other versions
		if requestedVersion != "" && gv.Version != requestedVersion {
			continue
		}

		// Look for the resource in this group/version
		for _, apiResource := range apiResourceList.APIResources {
			if opts.matchesResource(apiResource, resourceName) {
				resourceInfo := &ResourceInfo{
					Resource:     apiResource.Name, // Use the actual resource name, not the input
					Version:      gv.Version,
					Group:        gv.Group,
					GroupVersion: apiResourceList.GroupVersion,
					Kind:         apiResource.Kind,
					Namespaced:   apiResource.Namespaced,
				}
				candidates = append(candidates, resourceInfo)
			}
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("resource %q not found", resourceName)
	}

	// If we have multiple candidates, prefer the one from the preferred version
	if len(candidates) > 1 {
		return opts.selectPreferredResource(opts.discoveryClient, candidates)
	}

	return candidates[0], nil
}

// matchesResource checks if an API resource matches the requested resource name
func (opts *KARetrieverOptions) matchesResource(apiResource v1.APIResource, resourceName string) bool {
	// Check exact match with resource name
	if apiResource.Name == resourceName {
		return true
	}

	// Check if it matches any of the short names
	for _, shortName := range apiResource.ShortNames {
		if shortName == resourceName {
			return true
		}
	}

	// Check singular name if available
	if apiResource.SingularName != "" && apiResource.SingularName == resourceName {
		return true
	}

	return false
}

// loadPersistentConfig attempts to load configuration from the persistent config file
func (opts *KARetrieverOptions) loadPersistentConfig(configManager config.ConfigManager) error {

	if err := configManager.LoadConfig(opts.k8sRESTConfig); err != nil {
		return err
	}

	// Get cluster-specific configuration
	clusterConfig, err := configManager.GetCurrentClusterConfig()
	if err != nil {
		return err
	}

	if clusterConfig != nil {
		opts.host = clusterConfig.Host
		opts.certificatePath = clusterConfig.CertPath
		opts.tlsInsecure = clusterConfig.TLSInsecure
		opts.token = clusterConfig.Token
		if opts.token == "" {
			opts.token = opts.k8sRESTConfig.BearerToken
		}
		if opts.certificatePath != "" {
			expandedCertPath, certData, err := config.LoadCertData(opts.certificatePath)
			if err != nil {
				return err
			}
			opts.certificatePath = expandedCertPath
			opts.certData = certData
		}
	}

	return nil
}

// setK9eRESTConfig completes the configuration setup with token resolution and k9eRESTConfig creation
func (opts *KARetrieverOptions) setK9eRESTConfig() error {

	opts.k9eRESTConfig = &rest.Config{
		Host: opts.host,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: opts.tlsInsecure,
		},
	}

	opts.k9eRESTConfig.BearerToken = opts.token
	if opts.certData != nil {
		opts.k9eRESTConfig.CAData = opts.certData
		opts.k9eRESTConfig.Insecure = false
	}

	return nil
}

// selectPreferredResource selects the preferred version from multiple candidates
func (opts *KARetrieverOptions) selectPreferredResource(discoveryClient DiscoveryInterface, candidates []*ResourceInfo) (*ResourceInfo, error) {
	// Group candidates by API group
	groupMap := make(map[string][]*ResourceInfo)
	for _, candidate := range candidates {
		groupMap[candidate.Group] = append(groupMap[candidate.Group], candidate)
	}

	// For each group, find the preferred version
	for group, groupCandidates := range groupMap {
		if len(groupCandidates) == 1 {
			return groupCandidates[0], nil
		}

		// Get the preferred version for this group
		preferredVersion, err := opts.getPreferredVersion(discoveryClient, group)
		if err != nil {
			// If we can't determine preferred version, just use the first one
			//lint:ignore nilerr fallback to the first apiresource we don't want to fail if there is no preferred
			return groupCandidates[0], nil
		}

		// Find the candidate with the preferred version
		for _, candidate := range groupCandidates {
			if candidate.Version == preferredVersion {
				return candidate, nil
			}
		}
	}

	// Fallback to first candidate
	return candidates[0], nil
}

// getPreferredVersion gets the preferred version for a given API group
func (opts *KARetrieverOptions) getPreferredVersion(discoveryClient DiscoveryInterface, group string) (string, error) {
	if group == "" {
		// Core API group always uses v1
		return "v1", nil
	}

	// Get group information
	groups, err := discoveryClient.ServerGroups()
	if err != nil {
		return "", fmt.Errorf("failed to get server groups: %w", err)
	}

	for _, apiGroup := range groups.Groups {
		if apiGroup.Name == group {
			if len(apiGroup.Versions) > 0 {
				return apiGroup.PreferredVersion.Version, nil
			}
		}
	}

	return "", fmt.Errorf("group %q not found", group)
}
