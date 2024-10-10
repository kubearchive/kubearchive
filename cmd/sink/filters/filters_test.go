// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	toolsWatch "k8s.io/client-go/tools/watch"
	sourcev1 "knative.dev/eventing/pkg/apis/sources/v1"
)

func newMockWatcher(ch <-chan watch.Event) func(metav1.ListOptions) (watch.Interface, error) {
	return func(options metav1.ListOptions) (watch.Interface, error) {
		return watch.MockWatcher{
			StopFunc:       func() {},
			ResultChanFunc: func() <-chan watch.Event { return ch },
		}, nil
	}
}

func TestHandleUpdates(t *testing.T) {
	f := NewFilters(fake.NewSimpleClientset())
	buf := bytes.NewBuffer(make([]byte, 0))
	defer t.Log(buf.String())
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

	cases := []struct {
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
	for i, testCase := range cases {
		ch := make(chan watch.Event)
		watcher := newMockWatcher(ch)
		retryWatcher, err := toolsWatch.NewRetryWatcher("1", &cache.ListWatch{WatchFunc: watcher})
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
			Data: testCase.data,
		}
		event := watch.Event{Type: watch.Modified, Object: &cm}
		ch <- event
		wg.Add(1)
		go func(rw *toolsWatch.RetryWatcher, filter *Filters) {
			defer wg.Done()
			filter.handleUpdates(rw)
		}(retryWatcher, f)
		// we need to wait before calling retryWatcher.Stop to make sure that it sends a watch.Event first
		time.Sleep(time.Millisecond * 500)
		retryWatcher.Stop()
		// wait until the filters update before checking the result of the update
		wg.Wait()
		assert.Equal(t, testCase.archiveSize, len(f.archive))
		assert.Equal(t, testCase.deleteSize, len(f.delete))
		assert.Equal(t, testCase.archiveOnDeleteSize, len(f.archiveOnDelete))
	}
}
