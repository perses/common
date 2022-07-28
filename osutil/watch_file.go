package osutil

import (
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"path/filepath"
)

func WatchFile(filename string, callback func()) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Write == fsnotify.Write && event.Name == filename {
					callback()
				}
			case err := <-watcher.Errors:
				if err != nil {
					logrus.Errorf("Unable to watch the file %s: %s", filename, err)
				}
			}
		}
	}()
	err = watcher.Add(filepath.Dir(filename))
	return err
}
