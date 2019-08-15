package configmap

import (
	"context"
	"os"
	"sync"
	"testutil"

	"github.com/mixpanel/obs"
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

	fr obs.FlightRecorder
}

// NewCmWatcher() creates a new ConfigMap file watcher, which looks for changes to the file and invokes onFileEvent
func NewCmWatcher(path string, onFileEvent OnFileEvent, fr obs.FlightRecorder) (*CmWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, obserr.Annotate(err, "Error while creating fsnotify watcher")
	}

	w := &CmWatcher{
		Path:        path,
		onFileEvent: onFileEvent,
		watcher:     watcher,
		fr:          fr,
	}

	return w, nil
}

func NewCmWatcherForTest(path string, onFileEvent OnFileEvent, fr obs.FlightRecorder) (*CmWatcher, error) {
	c := testutil.NewCallCounter()
	wrapped := func(p string) error {
		defer c.Incr()
		return onFileEvent(p)
	}

	w, err := NewCmWatcher(path, wrapped, fr)
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
	fs := w.fr.WithSpan(ctx)

	// force the callback once to make sure that file is processed in the event
	// that no fsnotify events ever fired.
	if err := w.onFileEvent(w.Path); err != nil {
		fs.Warn("initial_on_file_event", "initial onFileEvent failed", obs.Vals{
			"Path": w.Path,
		}.WithError(err))
		// fail open
	}

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
					fs.Warn("error_reset", "error while resetting watch on config file", obs.Vals{
						"Path": event.Name,
					}.WithError(err))
					continue
				}
				if err := w.onFileEvent(event.Name); err != nil {
					fs.Warn("error_read", "could not read config file", obs.Vals{
						"Path": event.Name,
					}.WithError(err))
				}
			case fsnotify.Create, fsnotify.Write:
				if err := w.onFileEvent(event.Name); err != nil {
					fs.Warn("error_read", "could not read config file", obs.Vals{
						"Path": event.Name,
					}.WithError(err))
				}
			default:
				fs.Debug("unhandled_fsnotify", obs.Vals{
					"Path": event.Name,
					"op":   event.Op,
				})
			}
		case err, ok := <-w.watcher.Errors:
			if err != nil {
				fs.Warn("error_watching", "error while watching config file", obs.Vals{}.WithError(err))
			}
			if !ok {
				return
			}
		}
	}
}
