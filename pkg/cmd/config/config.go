// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
)

type ArchiveOptions struct {
	Host            string
	Token           string
	TLSInsecure     bool
	CertificatePath string
	KubeFlags       *genericclioptions.ConfigFlags
}

// NewArchiveOptions loads config from env vars and sets defaults
func NewArchiveOptions() *ArchiveOptions {
	opts := &ArchiveOptions{
		Host:            "https://localhost:8081",
		CertificatePath: "",
		KubeFlags:       genericclioptions.NewConfigFlags(true),
	}
	if v := os.Getenv("KUBECTL_PLUGIN_ARCHIVE_HOST"); v != "" {
		opts.Host = v
	}
	if v := os.Getenv("KUBECTL_PLUGIN_ARCHIVE_CERT_PATH"); v != "" {
		opts.CertificatePath = v
	}
	if v := os.Getenv("KUBECTL_PLUGIN_ARCHIVE_TLS_INSECURE"); v != "" {
		opts.TLSInsecure, _ = strconv.ParseBool(v)
	}
	return opts
}

// GetCertificateData get the certificate from a file path if set
func (opts *ArchiveOptions) GetCertificateData() ([]byte, error) {

	if opts.CertificatePath != "" {
		// Try to load the local certificate file
		certData, err := os.ReadFile(opts.CertificatePath)
		if err == nil {
			// Successfully loaded local certificate
			return certData, nil
		}

		return nil, fmt.Errorf("failed to load certificate from path and no secret info available: %w", err)
	}

	// No certificate available, so skipped
	return nil, nil
}

// Complete resolve the final values considering the values of the kubectl builtin flags
func (opts *ArchiveOptions) Complete(restConfig *rest.Config) error {

	// Handle token precedence:
	// 1. --token flag
	// 2. If KUBECTL_PLUGIN_ARCHIVE_TOKEN
	// 3. kubeconfig token
	if opts.KubeFlags.BearerToken != nil && *opts.KubeFlags.BearerToken != "" {
		opts.Token = *opts.KubeFlags.BearerToken
	} else if token := os.Getenv("KUBECTL_PLUGIN_ARCHIVE_TOKEN"); token != "" {
		opts.Token = token
	} else {
		opts.Token = restConfig.BearerToken
	}

	return nil
}

// AddFlags adds all archive-related flags to the given flag set
func (opts *ArchiveOptions) AddFlags(flags *pflag.FlagSet) {
	opts.KubeFlags.AddFlags(flags)
	flags.StringVar(&opts.Host, "host", opts.Host, "Host where the KubeArchive API Server is listening.")
	flags.BoolVar(&opts.TLSInsecure, "kubearchive-insecure-skip-tls-verify", opts.TLSInsecure, "Allow insecure requests to the KubeArchive API.")
	flags.StringVar(&opts.CertificatePath, "kubearchive-certificate-authority", opts.CertificatePath, "Path to the certificate authority file.")
}
