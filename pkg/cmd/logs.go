// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/kubearchive/kubearchive/pkg/cmd/config"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type LogsOptions struct {
	config.KACLICommand
	ContainerName string
	Name          string
	ResourceInfo  *config.ResourceInfo
	LabelSelector string
}

var logsLong = `Print the logs for a container in a pod or specified resource from KubeArchive.
If the provided resource, directly or indirectly, owns a pod, and all the resources in the
chain are archived, you can also retrieve the logs of one of the containers in the chain
(the latest one, if not specified).`

var logsExample = `
# Return logs from pod nginx with only one container
kubectl ka logs nginx
kubectl ka logs pod/nginx

# Return logs from pod nginx by specifying the container name
kubectl ka logs nginx -c my-container
kubectl ka logs pod/nginx -c my-container

# Return logs from a deployment (it will pick one of the pod's logs)
kubectl ka logs deployment/nginx
kubectl ka logs deploy/nginx

# Return logs using label selector
kubectl ka logs pods -l app=nginx
kubectl ka logs deployments -l app=nginx
`

func NewLogsOptions() *LogsOptions {
	return &LogsOptions{
		KACLICommand: config.NewKAOptions(),
	}
}

func NewLogCmd() *cobra.Command {
	o := NewLogsOptions()

	cmd := &cobra.Command{
		Use:     "logs ([RESOURCE[.VERSION[.GROUP]]/NAME] | [RESOURCE[.VERSION[.GROUP]]] | [NAME])",
		Short:   "Command to get logs resources from KubeArchive",
		Long:    logsLong,
		Example: logsExample,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			err = o.Complete(args)
			if err != nil {
				return fmt.Errorf("error completing the args: %w", err)
			}

			err = o.Run(cmd)
			if err != nil {
				return fmt.Errorf("error running the command: %w", err)
			}

			return nil
		},
	}

	o.AddFlags(cmd.Flags())
	cmd.Flags().StringVarP(&o.ContainerName, "container", "c", o.ContainerName, "Name of the container to retrieve the logs from.")
	cmd.Flags().StringVarP(&o.LabelSelector, "selector", "l", o.LabelSelector, "Selector (label query) to filter on, supports '=', '==', '!=', 'in', 'notin'.(e.g. -l key1=value1,key2=value2,key3 in (value3)). Matching objects must satisfy all of the specified label constraints.")

	return cmd
}

func (o *LogsOptions) Complete(args []string) error {
	err := o.KACLICommand.Complete()
	if err != nil {
		return fmt.Errorf("error completing the args: %w", err)
	}

	arg := args[0]

	// Split resource/name format: "deploy/nginx" -> "deploy", "nginx"
	parts := strings.SplitN(arg, "/", 2)
	resourceSpec := "pods"
	switch len(parts) {
	case 1:
		if o.LabelSelector != "" {
			resourceSpec = parts[0]
		} else {
			o.Name = parts[0]
		}
	case 2:
		resourceSpec = parts[0]
		o.Name = parts[1]
	default:
		return fmt.Errorf("invalid resource/name format: %s", arg)
	}

	o.ResourceInfo, err = o.ResolveResourceSpec(resourceSpec)
	if err != nil {
		return fmt.Errorf("error resolving resource spec: %w", err)
	}

	return nil
}

func (o *LogsOptions) Run(cmd *cobra.Command) error {
	apiPrefix := "/apis"
	if o.ResourceInfo.Group == "" {
		apiPrefix = "/api"
	}

	ns, nsErr := o.GetNamespace()
	if nsErr != nil {
		return fmt.Errorf("error getting namespace: %w", nsErr)
	}

	names := make([]string, 0)
	if o.LabelSelector != "" {
		apiPath := fmt.Sprintf("%s/%s/namespaces/%s/%s?labelSelector=%s", apiPrefix, o.ResourceInfo.GroupVersion, ns, o.ResourceInfo.Resource, url.QueryEscape(o.LabelSelector))
		bodyBytes, err := o.GetFromAPI(config.KubeArchive, apiPath)
		if err != nil {
			return fmt.Errorf("error while retrieving resources with labelSelector: %w", err)
		}

		var list unstructured.UnstructuredList
		err = json.Unmarshal(bodyBytes, &list)
		if err != nil {
			return fmt.Errorf("error deserializing the body into unstructured.UnstructuredList: %w", err)
		}

		if len(list.Items) == 0 {
			return fmt.Errorf("no resources found in the %s namespace", ns)
		}

		for _, resource := range list.Items {
			names = append(names, resource.GetName())
		}
	} else {
		names = append(names, o.Name)
	}

	for _, name := range names {
		apiPath := fmt.Sprintf("%s/%s/namespaces/%s/%s/%s/log", apiPrefix, o.ResourceInfo.GroupVersion, ns, o.ResourceInfo.Resource, name)
		if o.ContainerName != "" {
			apiPath = fmt.Sprintf("%s?container=%s", apiPath, o.ContainerName)
		}

		kubearchiveLog, err := o.GetFromAPI(config.KubeArchive, apiPath)
		if err != nil {
			return fmt.Errorf("error retrieving resources from the KubeArchive API: %w", err)
		}

		fmt.Fprintln(cmd.OutOrStdout(), string(kubearchiveLog))
	}

	return nil
}
