// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package config

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
)

type API int

const (
	Kubernetes API = iota
	KubeArchive
)

type KACLICommand interface {
	Complete() error
	AddFlags(flags *pflag.FlagSet)
	GetFromAPI(api API, path string) ([]byte, error)
	GetNamespace() (string, error)
}

type KAOptions struct {
	host            string
	tlsInsecure     bool
	certificatePath string
	kubeFlags       *genericclioptions.ConfigFlags
	K8sRESTConfig   *rest.Config
	K9eRESTConfig   *rest.Config
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
