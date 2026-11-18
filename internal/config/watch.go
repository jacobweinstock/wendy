package config

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watch reloads the catalog whenever a file in one of the loader's
// directories changes, and calls fn with each new catalog. A directory that
// does not exist yet is watched through its parent, so creating it (as the
// starter config does) is noticed too. Watch blocks until ctx is done.
func (l *Loader) Watch(ctx context.Context, fn func(*Catalog)) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	watched := map[string]bool{}
	add := func() {
		for _, dir := range l.Dirs {
			target := dir
			if _, err := os.Stat(dir); err != nil {
				target = filepath.Dir(dir)
			}
			if !watched[target] && w.Add(target) == nil {
				watched[target] = true
			}
		}
	}
	add()

	var timer *time.Timer
	fire := make(chan struct{}, 1)
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if !l.relevant(ev.Name) {
				continue
			}
			// Editors write files in several events; reload once they settle.
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(200*time.Millisecond, func() {
				select {
				case fire <- struct{}{}:
				default:
				}
			})
		case <-fire:
			add()
			fn(l.Load())
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			// An error such as a queue overflow can mean lost events, so
			// reload rather than miss a change.
			if err != nil {
				add()
				fn(l.Load())
			}
		}
	}
}

func (l *Loader) relevant(name string) bool {
	for _, dir := range l.Dirs {
		if name == dir || filepath.Dir(name) == dir {
			return true
		}
	}
	return false
}
