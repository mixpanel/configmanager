package configmap

import (
	"context"
	"os"
	"sync"

	"github.com/mixpanel/configmanager/logger"
	"github.com/mixpanel/configmanager/testutil"

	"github.com/mixpanel/obs/obserr"

	"github.com/fsnotify/fsnotify"
)

type OnFileEvent func(path string) error

type CmWatcher struct {
	// Path to ConfigMap file to watch
	Path string
	// Call whenever there is a change to ConfigMap
	onFileEvent OnFileEvent

	wg      sync.WaitGroup
	watcher *fsnotify.Watcher

	// used for tests
	NotifyCounter *testutil.CallCounter

	logger logger.Logger
}

// NewCmWatcher() creates a new ConfigMap file watcher, which looks for changes to the file and invokes onFileEvent
func NewCmWatcher(path string, onFileEvent OnFileEvent, logger logger.Logger) (*CmWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, obserr.Annotate(err, "Error while creating fsnotify watcher")
	}

	w := &CmWatcher{
		Path:        path,
		onFileEvent: onFileEvent,
		watcher:     watcher,
		logger:      logger,
	}

	return w, nil
}

func NewCmWatcherForTest(path string, onFileEvent OnFileEvent, logger logger.Logger) (*CmWatcher, error) {
	c := testutil.NewCallCounter()
	wrapped := func(p string) error {
		defer c.Incr()
		return onFileEvent(p)
	}

	w, err := NewCmWatcher(path, wrapped, logger)
	if err != nil {
		return nil, err
	}
	w.NotifyCounter = c
	return w, nil
}

// Start() start file watcher
func (w *CmWatcher) Start() error {
	if _, err := os.Stat(w.Path); os.IsNotExist(err) {
		return obserr.Annotate(err, "Path does not exist").Set("Path", w.Path)
	}

	if err := w.watcher.Add(w.Path); err != nil {
		return obserr.Annotate(err, "watcher.Add failed")
	}

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.startWatcher(context.Background())
	}()

	return nil
}

// Stop() stop file watcher
func (w *CmWatcher) Stop() {
	if w == nil {
		return
	}
	w.watcher.Close()
	w.wg.Wait()
}

func (w *CmWatcher) startWatcher(ctx context.Context) {
	// force the callback once to make sure that file is processed in the event
	// that no fsnotify events ever fired.
	if err := w.onFileEvent(w.Path); err != nil {
		w.logger.Warn(
			"initial onFileEvent failed",
			"path", w.Path,
			"err", err,
		)
		// fail open
	}
	logger := w.logger
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Name != w.Path {
				continue
			}
			switch event.Op {
			case fsnotify.Remove, fsnotify.Rename, fsnotify.Chmod:
				w.watcher.Remove(event.Name)
				if err := w.watcher.Add(event.Name); err != nil {
					logger.Warn(
						"error while resetting watch on config file",
						"path", event.Name,
						"err", err,
					)
					continue
				}
				if err := w.onFileEvent(event.Name); err != nil {
					logger.Warn(
						"could not read config file",
						"path", event.Name,
						"err", err,
					)
				}
			case fsnotify.Create, fsnotify.Write:
				if err := w.onFileEvent(event.Name); err != nil {
					logger.Warn(
						"could not read config file",
						"path", event.Name,
						"err", err,
					)
				}
			default:
			}
		case err, ok := <-w.watcher.Errors:
			if err != nil {
				logger.Warn("error while watching config file", err)
			}
			if !ok {
				return
			}
		}
	}
}
