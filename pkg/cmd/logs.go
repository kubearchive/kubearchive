// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"encoding/json"
	"errors"
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
	Resource      string
	GroupVersion  string
	LabelSelector string
}

var logsLong = `Print the logs for a container in a pod or specified resource from KubeArchive.
If the provided resource, directly or indirectly, owns a pod, and all the resources in the
chain are archived, you can also retrieve the logs of one of the containers in the chain
(the latest one, if not specified).`

var logsExample = `
# Return logs from pod nginx with only one container
kubectl archive logs nginx

# Return logs from pod nginx by specifying the container name
kubectl archive logs nginx -c my-container

# Return logs from a resource different from a pod. It will pick one of the pod's logs.
kubectl archive logs apps/v1 deployments nginx

# Return logs from a custom resource
kubectl archive logs tekton.dev/v1 taskrun my-task
`

func NewLogsOptions() *LogsOptions {
	return &LogsOptions{
		KACLICommand: config.NewKAOptions(),
	}
}

func NewLogCmd() *cobra.Command {
	o := NewLogsOptions()

	cmd := &cobra.Command{
		Use:     "logs [GROUPVERSION] [RESOURCE] [NAME]",
		Short:   "Command to get logs resources from KubeArchive",
		Long:    logsLong,
		Example: logsExample,
		Args:    cobra.RangeArgs(0, 3),
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
	switch len(args) {
	case 0:
		if o.LabelSelector == "" {
			return errors.New("specify a resource or use -l/--selector")
		}

		o.GroupVersion = "v1"
		o.Resource = "pods"
	case 1:
		if o.LabelSelector != "" {
			return errors.New("can't specify resource AND a label selector, use just one")
		}

		o.GroupVersion = "v1"
		o.Resource = "pods"
		o.Name = args[0]
	case 2:
		if o.LabelSelector == "" {
			return errors.New("invalid number of arguments")
		}

		o.GroupVersion = args[0]
		o.Resource = args[1]
	case 3:
		if o.LabelSelector != "" {
			return errors.New("can't specify resource AND a label selector, use just one")
		}

		o.GroupVersion = args[0]
		o.Resource = args[1]
		o.Name = args[2]
	default:
		return errors.New("error scanning arguments")
	}

	return nil
}

func (o *LogsOptions) Run(cmd *cobra.Command) error {
	apiPrefix := "/apis"
	if strings.HasPrefix(o.GroupVersion, "v1") {
		apiPrefix = "/api"
	}

	ns, nsErr := o.GetNamespace()
	if nsErr != nil {
		return fmt.Errorf("error getting namespace: %w", nsErr)
	}

	names := make([]string, 0)
	if o.LabelSelector != "" {
		apiPath := fmt.Sprintf("%s/%s/namespaces/%s/%s?labelSelector=%s", apiPrefix, o.GroupVersion, ns, o.Resource, url.QueryEscape(o.LabelSelector))
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
		apiPath := fmt.Sprintf("%s/%s/namespaces/%s/%s/%s/log", apiPrefix, o.GroupVersion, ns, o.Resource, name)
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
