// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	"github.com/kubearchive/kubearchive/pkg/constants"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"
	toolsWatch "k8s.io/client-go/tools/watch"
)

// nolint:staticcheck
func TestMain(m *testing.M) {
	m.Run()
}

func newMockWatcher(t testing.TB, ch <-chan watch.Event) func(metav1.ListOptions) (watch.Interface, error) {
	t.Helper()
	return func(options metav1.ListOptions) (watch.Interface, error) {
		return watch.MockWatcher{
			StopFunc:       func() {},
			ResultChanFunc: func() <-chan watch.Event { return ch },
		}, nil
	}
}

func TestHandleUpdates(t *testing.T) {
	unstruct := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubearchive.org/v1",
			"kind":       "SinkFilter",
			"metadata": map[string]interface{}{
				"namespace": constants.KubeArchiveNamespace,
				"name":      constants.SinkFilterResourceName,
			},
		},
	}
	f := NewFilters(fake.NewSimpleDynamicClient(runtime.NewScheme(), unstruct))
	fooCfg := []kubearchiveapi.KubeArchiveConfigResource{
		{
			Selector: kubearchiveapi.APIVersionKind{
				APIVersion: "batch/v1",
				Kind:       "Job",
			},
			ArchiveWhen:     "has(status.startTime)",
			DeleteWhen:      "has(status.completionTime)",
			ArchiveOnDelete: "metadata.name.startsWith('tmp')",
		},
	}
	barCfg := []kubearchiveapi.KubeArchiveConfigResource{
		{
			Selector: kubearchiveapi.APIVersionKind{
				APIVersion: "batch/v1",
				Kind:       "Job",
			},
			ArchiveWhen: "has(status.startTime)",
			DeleteWhen:  "has(status.completionTime)",
		},
	}
	bazCfg := []kubearchiveapi.KubeArchiveConfigResource{
		{
			Selector: kubearchiveapi.APIVersionKind{
				APIVersion: "batch/v1",
				Kind:       "Job",
			},
			ArchiveWhen: "has(status.startTime)",
		},
	}

	tests := []struct {
		name                string
		data                map[string][]kubearchiveapi.KubeArchiveConfigResource
		eventType           watch.EventType
		archiveSize         int
		deleteSize          int
		archiveOnDeleteSize int
	}{
		{
			name:                "Empty Map",
			data:                make(map[string][]kubearchiveapi.KubeArchiveConfigResource),
			eventType:           watch.Added,
			archiveSize:         0,
			deleteSize:          0,
			archiveOnDeleteSize: 0,
		},
		{
			name: "Many namespaces with filters",
			data: map[string][]kubearchiveapi.KubeArchiveConfigResource{
				"foo": fooCfg,
				"bar": barCfg,
				"baz": bazCfg,
			},
			eventType:           watch.Modified,
			archiveSize:         3,
			deleteSize:          2,
			archiveOnDeleteSize: 1,
		},
		{
			name:                "Delete event removes all filters",
			data:                make(map[string][]kubearchiveapi.KubeArchiveConfigResource),
			eventType:           watch.Deleted,
			archiveSize:         0,
			deleteSize:          0,
			archiveOnDeleteSize: 0,
		},
		{
			name:                "Error event does not update filters",
			data:                make(map[string][]kubearchiveapi.KubeArchiveConfigResource),
			eventType:           watch.Error,
			archiveSize:         0,
			deleteSize:          0,
			archiveOnDeleteSize: 0,
		},
		{
			name:                "Bookmark event does not update filters",
			data:                make(map[string][]kubearchiveapi.KubeArchiveConfigResource),
			eventType:           watch.Bookmark,
			archiveSize:         0,
			deleteSize:          0,
			archiveOnDeleteSize: 0,
		},
	}
	var wg sync.WaitGroup
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan watch.Event)
			t.Cleanup(func() { close(ch) })
			watcher := newMockWatcher(t, ch)
			retryWatcher, err := toolsWatch.NewRetryWatcher("1", &cache.ListWatch{WatchFunc: watcher})
			if err != nil {
				assert.FailNow(t, "Could not create a watcher")
			}
			sf := kubearchiveapi.SinkFilter{
				TypeMeta: metav1.TypeMeta{
					Kind:       "SinkFilter",
					APIVersion: "kubearchive.org/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            constants.SinkFilterResourceName,
					Namespace:       constants.KubeArchiveNamespace,
					ResourceVersion: fmt.Sprintf("%d", i),
				},
				Spec: kubearchiveapi.SinkFilterSpec{
					Namespaces: map[string][]kubearchiveapi.KubeArchiveConfigResource{},
				},
			}
			sf.Spec.Namespaces = tt.data
			event := watch.Event{Type: watch.Modified, Object: &sf}
			ch <- event
			wg.Add(1)
			go func(rw *toolsWatch.RetryWatcher, filter *Filters) {
				filter.handleUpdates(rw)
				wg.Done()
			}(retryWatcher, f)
			// we need to wait before calling retryWatcher.Stop to make sure that it sends a watch.Event first
			time.Sleep(time.Millisecond * 500)
			retryWatcher.Stop()
			// wait until the filters update before checking the result of the update
			wg.Wait()
			assert.Equal(t, tt.archiveSize, len(f.archive))
			assert.Equal(t, tt.deleteSize, len(f.delete))
			assert.Equal(t, tt.archiveOnDeleteSize, len(f.archiveOnDelete))
		})
	}
}

func TestChangeGlobalFilters(t *testing.T) {
	filters := NewFilters(nil)
	fh, err := os.Open("testdata/sf.yaml")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fh.Close() })

	fileBytes, err := io.ReadAll(fh)
	if err != nil {
		t.Fatal(err)
	}

	var unstructuredData map[string]interface{}
	err = yaml.Unmarshal(fileBytes, &unstructuredData)
	if err != nil {
		t.Fatal(err)
	}

	obj := &unstructured.Unstructured{Object: unstructuredData}
	bytes, err := obj.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	sf := &kubearchiveapi.SinkFilter{}

	if err = json.Unmarshal(bytes, sf); err != nil {
		t.Fatal(err)
	}

	err = filters.changeFilters(sf.Spec.Namespaces)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		resource    unstructured.Unstructured
		mustArchive bool
		mustDelete  bool
	}{
		{
			name: "pod is archived because of local filters",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "pods-archive",
						"labels": map[string]string{
							"local-filter": "fake",
						},
					},
				},
			},
			mustArchive: true,
			mustDelete:  false,
		},
		{
			name: "pod is archived because of global filters",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "pods-archive",
						"labels": map[string]string{
							"global-filter": "fake",
						},
					},
				},
			},
			mustArchive: true,
			mustDelete:  false,
		},
		{
			name: "pod is not archived",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "pods-archive",
					},
				},
			},
			mustArchive: false,
			mustDelete:  false,
		},
		{
			name: "cronjob is archived because global filters",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "batch/v1",
					"kind":       "CronJob",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "pods-archive",
					},
				},
			},
			mustArchive: true,
			mustDelete:  false,
		},
		{
			name: "job is archived because global filters and has a start time",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "batch/v1",
					"kind":       "Job",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "pods-archive",
					},
					"status": map[string]string{
						"startTime": "fake-time",
					},
				},
			},
			mustArchive: true,
			mustDelete:  false,
		},
		{
			name: "job is deleted because global filters and has a completion time",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "batch/v1",
					"kind":       "Job",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "pods-archive",
					},
					"status": map[string]string{
						"completionTime": "fake-time",
					},
				},
			},
			mustArchive: true,
			mustDelete:  true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			resourceMustArchive := filters.MustArchive(context.Background(), &testCase.resource)
			resourceMustDelete := filters.MustDelete(context.Background(), &testCase.resource)

			assert.Equal(t, testCase.mustArchive, resourceMustArchive, "archive should match")
			assert.Equal(t, testCase.mustDelete, resourceMustDelete, "delete should match")
		})
	}

}

func TestIsConfigured(t *testing.T) {
	filters := NewFilters(nil)
	fh, err := os.Open("testdata/sf.yaml")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fh.Close() })

	fileBytes, err := io.ReadAll(fh)
	if err != nil {
		t.Fatal(err)
	}

	var unstructuredData map[string]interface{}
	err = yaml.Unmarshal(fileBytes, &unstructuredData)
	if err != nil {
		t.Fatal(err)
	}

	obj := &unstructured.Unstructured{Object: unstructuredData}
	bytes, err := obj.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	sf := &kubearchiveapi.SinkFilter{}

	if err = json.Unmarshal(bytes, sf); err != nil {
		t.Fatal(err)
	}

	err = filters.changeFilters(sf.Spec.Namespaces)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		resource     unstructured.Unstructured
		isConfigured bool
	}{
		{
			name: "pod is configured",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "pods-archive",
					},
				},
			},
			isConfigured: true,
		},
		{
			name: "cronjob is configured",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "batch/v1",
					"kind":       "CronJob",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "pods-archive",
					},
				},
			},
			isConfigured: true,
		},
		{
			name: "Deployment is not configured",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "pods-archive",
					},
				},
			},
			isConfigured: false,
		},
		{
			name: "pod is not configured namespace is not monitored",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "not-monitored",
					},
				},
			},
			isConfigured: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			isConfigured := filters.IsConfigured(context.Background(), &testCase.resource)

			assert.Equal(t, testCase.isConfigured, isConfigured)
		})
	}
}

func TestMustArchive(t *testing.T) {
	filters := NewFilters(nil)
	fh, err := os.Open("testdata/sf-archive.yaml")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fh.Close() })

	fileBytes, err := io.ReadAll(fh)
	if err != nil {
		t.Fatal(err)
	}

	var unstructuredData map[string]interface{}
	err = yaml.Unmarshal(fileBytes, &unstructuredData)
	if err != nil {
		t.Fatal(err)
	}

	obj := &unstructured.Unstructured{Object: unstructuredData}
	bytes, err := obj.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	sf := &kubearchiveapi.SinkFilter{}

	if err = json.Unmarshal(bytes, sf); err != nil {
		t.Fatal(err)
	}

	err = filters.changeFilters(sf.Spec.Namespaces)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		resource    unstructured.Unstructured
		mustArchive bool
	}{
		{
			name: "archive pod",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "pods-archive",
					},
				},
			},
			mustArchive: true,
		},
		{
			name: "archive cronjob",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "batch/v1",
					"kind":       "CronJob",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "pods-archive",
					},
				},
			},
			mustArchive: true,
		},
		{
			name: "do not archive Deployment",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "pods-archive",
					},
				},
			},
			mustArchive: false,
		},
		{
			name: "do not archive pod namespace is not monitored",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "not-monitored",
					},
				},
			},
			mustArchive: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			archive := filters.MustArchive(context.Background(), &testCase.resource)

			assert.Equal(t, testCase.mustArchive, archive)
		})
	}
}

func TestMustDelete(t *testing.T) {
	filters := NewFilters(nil)
	fh, err := os.Open("testdata/sf-delete.yaml")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fh.Close() })

	fileBytes, err := io.ReadAll(fh)
	if err != nil {
		t.Fatal(err)
	}

	var unstructuredData map[string]interface{}
	err = yaml.Unmarshal(fileBytes, &unstructuredData)
	if err != nil {
		t.Fatal(err)
	}

	obj := &unstructured.Unstructured{Object: unstructuredData}
	bytes, err := obj.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	sf := &kubearchiveapi.SinkFilter{}

	if err = json.Unmarshal(bytes, sf); err != nil {
		t.Fatal(err)
	}

	err = filters.changeFilters(sf.Spec.Namespaces)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		resource    unstructured.Unstructured
		mustArchive bool
	}{
		{
			name: "archive pod",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "pods-archive",
					},
				},
			},
			mustArchive: true,
		},
		{
			name: "archive cronjob",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "batch/v1",
					"kind":       "CronJob",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "pods-archive",
					},
				},
			},
			mustArchive: true,
		},
		{
			name: "do not archive Deployment",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "pods-archive",
					},
				},
			},
			mustArchive: false,
		},
		{
			name: "do not archive pod namespace is not monitored",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "not-monitored",
					},
				},
			},
			mustArchive: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			archive := filters.MustDelete(context.Background(), &testCase.resource)

			assert.Equal(t, testCase.mustArchive, archive)
		})
	}
}

func TestMustArchiveOnDelete(t *testing.T) {
	filters := NewFilters(nil)
	fh, err := os.Open("testdata/sf-archive-on-delete.yaml")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fh.Close() })

	fileBytes, err := io.ReadAll(fh)
	if err != nil {
		t.Fatal(err)
	}

	var unstructuredData map[string]interface{}
	err = yaml.Unmarshal(fileBytes, &unstructuredData)
	if err != nil {
		t.Fatal(err)
	}

	obj := &unstructured.Unstructured{Object: unstructuredData}
	bytes, err := obj.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	sf := &kubearchiveapi.SinkFilter{}

	if err = json.Unmarshal(bytes, sf); err != nil {
		t.Fatal(err)
	}

	err = filters.changeFilters(sf.Spec.Namespaces)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		resource    unstructured.Unstructured
		mustArchive bool
	}{
		{
			name: "archive pod",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "pods-archive",
					},
				},
			},
			mustArchive: true,
		},
		{
			name: "archive cronjob",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "batch/v1",
					"kind":       "CronJob",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "pods-archive",
					},
				},
			},
			mustArchive: true,
		},
		{
			name: "do not archive Deployment",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "pods-archive",
					},
				},
			},
			mustArchive: false,
		},
		{
			name: "do not archive pod namespace is not monitored",
			resource: unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"name":      "busybox",
						"namespace": "not-monitored",
					},
				},
			},
			mustArchive: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			archive := filters.MustArchiveOnDelete(context.Background(), &testCase.resource)

			assert.Equal(t, testCase.mustArchive, archive)
		})
	}
}
