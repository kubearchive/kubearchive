// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package files

import (
	"errors"
	"fmt"

	"github.com/fsnotify/fsnotify"
)

// contains the logic that tells fsnotify.Watcher what to do when an fsnotify.Event is received.
type LoopFunc func(*fsnotify.Watcher)

// terminates an fsnotify.Watcher and cleans up any resources that it has open.
type UpdateStopper func() error

// noopUpdateStopper is used by UpdateOnPaths when it has to return an UpdateStopper if the fsnotify.Watcher could not
// be started.
func noopUpdateStopper() error {
	return nil
}

// Creates an fsnotify.Watcher that will execute loop on a separate Goroutine. If the watcher cannot be created or if
// any path provided cannot be watched, it returns an error. The UpdatedStopper that is returned will always be callable
// and will never be nil.
func UpdateOnPaths(loop LoopFunc, paths ...string) (UpdateStopper, error) {
	if len(paths) < 1 {
		return noopUpdateStopper, fmt.Errorf("at least one path must be provided to watch on")
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return noopUpdateStopper, err
	}
	go loop(watcher)
	for _, path := range paths {
		err = watcher.Add(path)
		if err != nil {
			closeErr := watcher.Close()
			if closeErr != nil {
				return noopUpdateStopper, errors.Join(err, closeErr)
			}
			return noopUpdateStopper, err
		}
	}
	return watcher.Close, nil
}
