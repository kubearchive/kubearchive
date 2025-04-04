// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	toolsWatch "k8s.io/client-go/tools/watch"
	sourcev1 "knative.dev/eventing/pkg/apis/sources/v1"
)

// nolint:staticcheck
func TestMain(m *testing.M) {
	globalKey = "kubearchive"
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
	f := NewFilters(fake.NewSimpleClientset())
	fooCfg := []kubearchiveapi.KubeArchiveConfigResource{
		{
			Selector: sourcev1.APIVersionKindSelector{
				APIVersion: "batch/v1",
				Kind:       "Job",
			},
			ArchiveWhen:     "has(status.startTime)",
			DeleteWhen:      "has(status.completionTime)",
			ArchiveOnDelete: "metadata.name.startsWith('tmp')",
		},
	}
	fooCfgBytes, err := yaml.Marshal(fooCfg)
	if err != nil {
		assert.FailNow(t, "Could not marshall KubeArchiveConfig")
	}
	barCfg := []kubearchiveapi.KubeArchiveConfigResource{
		{
			Selector: sourcev1.APIVersionKindSelector{
				APIVersion: "batch/v1",
				Kind:       "Job",
			},
			ArchiveWhen: "has(status.startTime)",
			DeleteWhen:  "has(status.completionTime)",
		},
	}
	barCfgBytes, err := yaml.Marshal(barCfg)
	if err != nil {
		assert.FailNow(t, "Could not marshall KubeArchiveConfig")
	}
	bazCfg := []kubearchiveapi.KubeArchiveConfigResource{
		{
			Selector: sourcev1.APIVersionKindSelector{
				APIVersion: "batch/v1",
				Kind:       "Job",
			},
			ArchiveWhen: "has(status.startTime)",
		},
	}
	bazCfgBytes, err := yaml.Marshal(bazCfg)
	if err != nil {
		assert.FailNow(t, "Could not marshall KubeArchiveConfig")
	}

	tests := []struct {
		name                string
		data                map[string]string
		eventType           watch.EventType
		archiveSize         int
		deleteSize          int
		archiveOnDeleteSize int
	}{
		{
			name:                "Empty Map",
			data:                make(map[string]string),
			eventType:           watch.Added,
			archiveSize:         0,
			deleteSize:          0,
			archiveOnDeleteSize: 0,
		},
		{
			name: "Many namespaces with filters",
			data: map[string]string{
				"foo": string(fooCfgBytes),
				"bar": string(barCfgBytes),
				"baz": string(bazCfgBytes),
			},
			eventType:           watch.Modified,
			archiveSize:         3,
			deleteSize:          2,
			archiveOnDeleteSize: 1,
		},
		{
			name:                "Delete event removes all filters",
			data:                make(map[string]string),
			eventType:           watch.Deleted,
			archiveSize:         0,
			deleteSize:          0,
			archiveOnDeleteSize: 0,
		},
		{
			name:                "Error event does not update filters",
			data:                make(map[string]string),
			eventType:           watch.Error,
			archiveSize:         0,
			deleteSize:          0,
			archiveOnDeleteSize: 0,
		},
		{
			name:                "Bookmark event does not update filters",
			data:                make(map[string]string),
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
			retryWatcher, err := toolsWatch.NewRetryWatcherWithContext(t.Context(), "1", &cache.ListWatch{WatchFunc: watcher})
			if err != nil {
				assert.FailNow(t, "Could not create a watcher")
			}
			cm := corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ConfigMap",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "sink-filters",
					Namespace:       "kubearchive",
					ResourceVersion: fmt.Sprintf("%d", i),
				},
				Data: tt.data,
			}
			event := watch.Event{Type: watch.Modified, Object: &cm}
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
	fh, err := os.Open("testdata/cm.yaml")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fh.Close() })

	fileBytes, err := io.ReadAll(fh)
	if err != nil {
		t.Fatal(err)
	}

	var cm corev1.ConfigMap
	err = yaml.Unmarshal(fileBytes, &cm)
	if err != nil {
		t.Fatal(err)
	}

	err = filters.changeFilters(cm.Data)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name           string
		resource       unstructured.Unstructured
		sinkFilterPath string
		mustArchive    bool
		mustDelete     bool
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
			sinkFilterPath: "testdata/cm.yaml",
			mustArchive:    true,
			mustDelete:     false,
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
			sinkFilterPath: "testdata/cm.yaml",
			mustArchive:    true,
			mustDelete:     false,
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
			sinkFilterPath: "testdata/cm.yaml",
			mustArchive:    false,
			mustDelete:     false,
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
			sinkFilterPath: "testdata/cm.yaml",
			mustArchive:    true,
			mustDelete:     false,
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
			sinkFilterPath: "testdata/cm.yaml",
			mustArchive:    true,
			mustDelete:     false,
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
			sinkFilterPath: "testdata/cm.yaml",
			mustArchive:    true,
			mustDelete:     true,
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
