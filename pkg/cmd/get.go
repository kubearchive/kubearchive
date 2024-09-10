// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
)

type GetOptions struct {
	AllNamespaces  bool
	IsCoreResource bool
	APIPath        string
	Resource       string
	GroupVersion   string
	Token          string

	KubeArchiveHost string

	RESTConfig *rest.Config
	kubeFlags  *genericclioptions.ConfigFlags
}

func NewGetOptions() *GetOptions {
	return &GetOptions{
		kubeFlags:       genericclioptions.NewConfigFlags(true),
		KubeArchiveHost: "https://localhost:8081",
	}
}

func NewGetCmd() *cobra.Command {
	o := NewGetOptions()

	cmd := &cobra.Command{
		Use:   "get [GROUPVERSION] [RESOURCE]",
		Short: "Command to get resources from KubeArchive",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			err = o.Complete(args)
			if err != nil {
				return fmt.Errorf("error completing the args: %w", err)
			}

			err = o.Run()
			if err != nil {
				return fmt.Errorf("error running the command: %w", err)
			}

			return nil
		},
	}

	o.kubeFlags.AddFlags(cmd.Flags())
	cmd.Flags().BoolVarP(&o.AllNamespaces, "all-namespaces", "A", o.AllNamespaces, "If present, list the requested object(s) across all namespaces. Namespace in current context is ignored even if specified with --namespace.")
	cmd.Flags().StringVar(&o.KubeArchiveHost, "kubearchive-host", o.KubeArchiveHost, fmt.Sprintf("Host where the KubeArchive API Server is listening. Defaults to '%s'", o.KubeArchiveHost))

	return cmd
}

func (o *GetOptions) Complete(args []string) error {
	o.GroupVersion = args[0]
	if strings.HasPrefix(o.GroupVersion, "v1") {
		o.IsCoreResource = true
	}

	o.Resource = args[1]
	APIPathWithoutRoot := fmt.Sprintf("%s/%s", o.GroupVersion, o.Resource)
	if *o.kubeFlags.Namespace != "" {
		APIPathWithoutRoot = fmt.Sprintf("%s/namespaces/%s/%s", o.GroupVersion, *o.kubeFlags.Namespace, o.Resource)
	}

	o.APIPath = fmt.Sprintf("/apis/%s", APIPathWithoutRoot)
	if o.IsCoreResource {
		o.APIPath = fmt.Sprintf("/api/%s", APIPathWithoutRoot)
	}

	config, err := o.kubeFlags.ToRESTConfig()
	if err != nil {
		return fmt.Errorf("error creating the REST configuration: %w", err)
	}

	o.RESTConfig = config

	return nil
}

func (o *GetOptions) getResources(host string) ([]unstructured.Unstructured, error) {
	url := fmt.Sprintf("%s%s", host, o.APIPath)

	client, err := rest.HTTPClientFor(o.RESTConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating the HTTP client from the REST config: %w", err)
	}
	response, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("error on GET to '%s': %w", url, err)
	}
	defer response.Body.Close()

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("error deserializing the body: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET to '%s' returned with code '%d' and body: %s", url, response.StatusCode, string(bodyBytes))
	}

	var list unstructured.UnstructuredList
	err = json.Unmarshal(bodyBytes, &list)
	if err != nil {
		return nil, fmt.Errorf("error deserializing the body into unstructured.UnstructuredList: %w", err)
	}

	return list.Items, nil
}

func (o *GetOptions) getKubeArchiveResources() ([]unstructured.Unstructured, error) {
	o.RESTConfig.CAData = nil      // Remove CA data from the Kubeconfig
	o.RESTConfig.CAFile = "ca.crt" // This expects you to have extracted the CA, see DEVELOPMENT.md
	o.RESTConfig.BearerToken = *o.kubeFlags.BearerToken

	return o.getResources(o.KubeArchiveHost)
}

func (o *GetOptions) Run() error {
	clusterResources, err := o.getResources(o.RESTConfig.Host)
	if err != nil {
		return fmt.Errorf("error retrieving resources from the cluster: %w", err)
	}

	kubearchiveResources, err := o.getKubeArchiveResources()
	if err != nil {
		return fmt.Errorf("error retrieving resources from the KubeArchive API: %w", err)
	}
	var objs []unstructured.Unstructured
	objs = append(objs, clusterResources...)
	objs = append(objs, kubearchiveResources...)

	for ix := range objs {
		fmt.Println(objs[ix].GetName())
	}

	return nil
}
