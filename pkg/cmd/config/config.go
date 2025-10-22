// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package config

import (
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
)

type API int

const (
	Kubernetes API = iota
	KubeArchive
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
}

type KACLICommand interface {
	Complete() error
	AddFlags(flags *pflag.FlagSet)
	GetFromAPI(api API, path string) ([]byte, error)
	GetNamespace() (string, error)
	ResolveResourceSpec(resourceSpec string) (*ResourceInfo, error)
}

type KAOptions struct {
	host            string
	tlsInsecure     bool
	certificatePath string
	kubeFlags       *genericclioptions.ConfigFlags
	K8sRESTConfig   *rest.Config
	K9eRESTConfig   *rest.Config
	discoveryClient DiscoveryInterface
}

// NewKAOptions loads config from env vars and sets defaults
func NewKAOptions() *KAOptions {
	opts := &KAOptions{
		host:            "https://localhost:8081",
		certificatePath: "",
		kubeFlags:       genericclioptions.NewConfigFlags(true),
	}
	if v := os.Getenv("KUBECTL_PLUGIN_KA_HOST"); v != "" {
		opts.host = v
	}
	if v := os.Getenv("KUBECTL_PLUGIN_KA_CERT_PATH"); v != "" {
		opts.certificatePath = v
	}
	if v := os.Getenv("KUBECTL_PLUGIN_KA_TLS_INSECURE"); v != "" {
		opts.tlsInsecure, _ = strconv.ParseBool(v)
	}
	return opts
}

// GetFromAPI HTTP GET request to the given endpoint
func (opts *KAOptions) GetFromAPI(api API, path string) ([]byte, error) {
	var restConfig *rest.Config
	var host string

	switch api {
	case Kubernetes:
		restConfig = opts.K8sRESTConfig
		host = opts.K8sRESTConfig.Host
	case KubeArchive:
		restConfig = opts.K9eRESTConfig
		host = opts.host
	}

	client, err := rest.HTTPClientFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating the HTTP client from the REST config: %w", err)
	}
	url := fmt.Sprintf("%s%s", host, path)
	response, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("error on GET to '%s': %w", url, err)
	}
	defer response.Body.Close()

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("error deserializing the body: %w", err)
	}

	if response.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unable to get '%s': unauthorized", url)
	}

	if response.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("unable to get '%s': not found", url)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unable to get '%s': unknown error: %s (%d)", url, string(bodyBytes), response.StatusCode)
	}

	return bodyBytes, nil
}

// GetCertificateData get the certificate from a file path if set
func (opts *KAOptions) getCertificateData() ([]byte, error) {
	if opts.certificatePath != "" {
		certData, err := os.ReadFile(opts.certificatePath)
		if err == nil {
			// Successfully loaded local certificate
			return certData, nil
		}

		return nil, fmt.Errorf("failed to load certificate from path and no secret info available: %w", err)
	}

	return nil, nil
}

// GetNamespace get the provided namespace or the namespace used in kubeconfig context
func (opts *KAOptions) GetNamespace() (string, error) {
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

// Complete resolve the final values considering the values of the kubectl builtin flags
func (opts *KAOptions) Complete() error {
	restConfig, err := opts.kubeFlags.ToRESTConfig()
	if err != nil {
		return fmt.Errorf("error creating the REST configuration: %w", err)
	}
	opts.K8sRESTConfig = restConfig

	client, err := opts.kubeFlags.ToDiscoveryClient()
	if err != nil {
		return fmt.Errorf("failed to create discovery client: %w", err)
	}
	opts.discoveryClient = client

	certData, err := opts.getCertificateData()
	if err != nil {
		return fmt.Errorf("failed to get certificate data: %w", err)
	}

	var token string
	if opts.kubeFlags.BearerToken != nil && *opts.kubeFlags.BearerToken != "" {
		token = *opts.kubeFlags.BearerToken
	} else if t := os.Getenv("KUBECTL_PLUGIN_KA_TOKEN"); t != "" {
		token = t
	} else {
		token = opts.K8sRESTConfig.BearerToken
	}

	opts.K9eRESTConfig = &rest.Config{
		Host:        opts.host,
		BearerToken: token,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: opts.tlsInsecure,
		},
	}
	if certData != nil {
		opts.K9eRESTConfig.CAData = certData
		opts.K9eRESTConfig.Insecure = false
	}

	return nil
}

// AddFlags adds all archive-related flags to the given flag set
func (opts *KAOptions) AddFlags(flags *pflag.FlagSet) {
	opts.kubeFlags.AddFlags(flags)
	flags.StringVar(&opts.host, "host", opts.host, "host where the KubeArchive API Server is listening.")
	flags.BoolVar(&opts.tlsInsecure, "kubearchive-insecure-skip-tls-verify", opts.tlsInsecure, "Allow insecure requests to the KubeArchive API.")
	flags.StringVar(&opts.certificatePath, "kubearchive-certificate-authority", opts.certificatePath, "Path to the certificate authority file.")
}

// ResolveResourceSpec resolves a resource specification using Kubernetes discovery API
func (opts *KAOptions) ResolveResourceSpec(resourceSpec string) (*ResourceInfo, error) {
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
func (opts *KAOptions) findResourceInfo(resourceName, requestedVersion, requestedGroup string) (*ResourceInfo, error) {
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
func (opts *KAOptions) matchesResource(apiResource v1.APIResource, resourceName string) bool {
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

// selectPreferredResource selects the preferred version from multiple candidates
func (opts *KAOptions) selectPreferredResource(discoveryClient DiscoveryInterface, candidates []*ResourceInfo) (*ResourceInfo, error) {
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
func (opts *KAOptions) getPreferredVersion(discoveryClient DiscoveryInterface, group string) (string, error) {
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
