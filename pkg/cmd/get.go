// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/client-go/rest"
)

type GetOptions struct {
	AllNamespaces bool
	APIPath       string
	Resource      string
	GroupVersion  string
	Token         string

	KubeArchiveHost string

	RESTConfig         *rest.Config
	kubeFlags          *genericclioptions.ConfigFlags
	OutputFormat       *string
	JSONYamlPrintFlags *genericclioptions.JSONYamlPrintFlags
	IsValidOutput      bool
	genericiooptions.IOStreams
}

func NewGetOptions() *GetOptions {
	outputFormat := ""
	return &GetOptions{
		kubeFlags:          genericclioptions.NewConfigFlags(true),
		KubeArchiveHost:    "https://localhost:8081",
		OutputFormat:       &outputFormat,
		JSONYamlPrintFlags: genericclioptions.NewJSONYamlPrintFlags(),
		IOStreams: genericiooptions.IOStreams{
			In:     os.Stdin,
			Out:    os.Stdout,
			ErrOut: os.Stderr,
		},
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
	cmd.Flags().StringVarP(o.OutputFormat, "output", "o", *o.OutputFormat, fmt.Sprintf(`Output format. One of: (%s).`, strings.Join(o.JSONYamlPrintFlags.AllowedFormats(), ", ")))
	o.JSONYamlPrintFlags.AddFlags(cmd)

	return cmd
}

func (o *GetOptions) Complete(args []string) error {
	o.GroupVersion = args[0]

	o.Resource = args[1]
	APIPathWithoutRoot := fmt.Sprintf("%s/%s", o.GroupVersion, o.Resource)
	if *o.kubeFlags.Namespace != "" {
		APIPathWithoutRoot = fmt.Sprintf("%s/namespaces/%s/%s", o.GroupVersion, *o.kubeFlags.Namespace, o.Resource)
	}

	o.APIPath = fmt.Sprintf("/apis/%s", APIPathWithoutRoot)
	if strings.HasPrefix(o.GroupVersion, "v1") {
		o.APIPath = fmt.Sprintf("/api/%s", APIPathWithoutRoot)
	}

	config, err := o.kubeFlags.ToRESTConfig()
	if err != nil {
		return fmt.Errorf("error creating the REST configuration: %w", err)
	}

	o.RESTConfig = config

	return nil
}

func (o *GetOptions) getResources(host string) ([]runtime.Object, error) {
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

	// Convert unstructured objects to runtime.Object
	var runtimeObjects []runtime.Object
	for _, item := range list.Items {
		runtimeObjects = append(runtimeObjects, &item)
	}

	return runtimeObjects, nil
}

func (o *GetOptions) getKubeArchiveResources() ([]runtime.Object, error) {
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
	var objs []runtime.Object
	objs = append(objs, clusterResources...)
	objs = append(objs, kubearchiveResources...)

	// Handle JSON/YAML output differently - create a List object
	if o.OutputFormat != nil && *o.OutputFormat != "" {
		printer, printerErr := o.JSONYamlPrintFlags.ToPrinter(*o.OutputFormat)
		if printerErr != nil {
			return fmt.Errorf("error getting printer: %w", printerErr)
		}
		// Create a List containing all objects
		list := &unstructured.UnstructuredList{
			Object: map[string]interface{}{
				"kind":       "List",
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"resourceVersion": "",
				},
			},
		}

		for _, obj := range objs {
			if unstructuredObj, ok := obj.(*unstructured.Unstructured); ok {
				list.Items = append(list.Items, *unstructuredObj)
			}
		}

		// Print the list once
		if printErr := printer.PrintObj(list, o.Out); printErr != nil {
			return fmt.Errorf("error printing list: %w", printErr)
		}
	} else {
		// For table output, print each object individually
		printer := printers.NewTablePrinter(printers.PrintOptions{})
		tabWriter := printers.GetNewTabWriter(o.Out)
		defer tabWriter.Flush()
		for _, obj := range objs {
			if printErr := printer.PrintObj(obj, tabWriter); printErr != nil {
				return fmt.Errorf("error printing object: %w", printErr)
			}
		}
	}

	return nil
}
