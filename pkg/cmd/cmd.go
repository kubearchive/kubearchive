// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/spf13/pflag"
	"k8s.io/client-go/rest"
)

type API int

const (
	Kubernetes API = iota
	KubeArchive
)

type APIError struct {
	StatusCode int
	URL        string
	Message    string
	Body       string
}

func (e *APIError) Error() string {
	return e.Message
}

// KACLICommand defines the base interface for all kubectl-ka commands
type KACLICommand interface {
	CompleteK8sConfig() error
	GetK8sRESTConfig() *rest.Config
	AddK8sFlags(flags *pflag.FlagSet)
	GetNamespace() (string, error)
}

// KARetrieverCommand defines the interface for commands that retrieve data from KubeArchive API
type KARetrieverCommand interface {
	KACLICommand
	CompleteRetriever() error
	AddRetrieverFlags(flags *pflag.FlagSet)
	GetFromAPI(api API, path string) ([]byte, *APIError)
	ResolveResourceSpec(resourceSpec string) (*ResourceInfo, error)
}
