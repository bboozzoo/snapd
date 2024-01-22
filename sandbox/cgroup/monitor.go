// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2022 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package cgroup

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/snapcore/snapd/logger"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/osutil/inotify"
)

// inotifyWatcher manages the inotify watcher, allowing to have a single watch descriptor open
type inotifyWatcher struct {
	// The watch object
	wd *inotify.Watcher
	// This is used to ensure that the watcher goroutine is launched only once
	doOnce sync.Once
	// This channel is used to add a new CGroup that has to be checked
	addWatchChan chan *groupToWatch
	// Contains the list of groups to monitor, to detect when they have been deleted
	groupList []*groupToWatch
	// Contains the list of paths being monitored by the inotify watcher.
	// The paths monitored are both the actual leaf path and the parent
	// directory. See discussion in addWatch for details on why the parent
	// needs to be tracked as well.
	pathList map[string]uint
}

// Contains the data corresponding to a CGroup that must be watched to detect
// when it is destroyed
type groupToWatch struct {
	// This is the CGroup identifier
	name string
	// This contains a hash map of folders to monitor. When all of them have been
	// deleted, the CGroup has been destroyed and there are no processes running
	folders map[string]struct{}
	// This channel is used to notify to the requester that this CGroup has been
	// destroyed. The watcher writes the CGroup identifier on it; this way, the
	// same channel can be shared to monitor several CGroups.
	channel chan<- string
}

var currentWatcher *inotifyWatcher = newInotifyWatcher()

func newInotifyWatcher(ctx context.Context) *inotifyWatcher {
	return &inotifyWatcher{
		pathList:     make(map[string]uint),
		addWatchChan: make(chan *groupToWatch),
	}
}

func (gtw *groupToWatch) sendClosedNotification() {
	go func() {
		defer func() {
			if err := recover(); err != nil {
				logger.Noticef("cannot send closed notification for %s", gtw.name)
			}
		}()
		gtw.channel <- gtw.name
	}()
}

// Close stops the watcher and waits for it to finish.
func (iw *inotifyWatcher) Close() {
	iw.done()
	<-iw.doneChan
	for p := range iw.pathList {
		iw.removePath(p)
	}
	iw.wd.Close()
}

func (iw *inotifyWatcher) addWatch(newWatch *groupToWatch) {
	for fullPath := range newWatch.folders {
		// It's not possible to use inotify.InDeleteSelf in /sys/fs because it
		// isn't triggered, so we must monitor the parent folder and use InDelete
		basePath := filepath.Dir(fullPath)
		if _, exists := iw.pathList[basePath]; !exists {
			iw.pathList[basePath] = 0
			if err := iw.wd.AddWatch(basePath, inotify.InDelete); err != nil {
				target := &os.PathError{}
				if !errors.As(err, &target) {
					logger.Noticef("cannot add watch for path %s: %s", target.Path, target.Err)
				}
				delete(iw.pathList, basePath)
				continue
			}
		}

		// bump for the base path, since we're relying on a watch being added there
		iw.pathList[basePath]++
		// bump on the actual path
		iw.pathList[fullPath]++

		if !osutil.FileExists(fullPath) {
			// the path is gone by now
			delete(newWatch.folders, fullPath)
			iw.removePath(fullPath)
		}
	}

	if len(newWatch.folders) == 0 {
		newWatch.sendClosedNotification()
	} else {
		iw.groupList = append(iw.groupList, newWatch)
	}

	iw.notifyObserver(newWatch)

	logger.Debugf("watches after add: %v", iw.pathList)
}

func (iw *inotifyWatcher) removePath(fullPath string) {
	cnt, ok := iw.pathList[fullPath]

	if !ok {
		// path we are not watching
		return
	}

	cnt--

	if cnt > 0 {
		// still references to this path
		iw.pathList[fullPath] = cnt
		return
	}

	logger.Debugf("removing watch for %s", fullPath)

	delete(iw.pathList, fullPath)

	// we are only adding watch on inotify for the parent dir path, see
	// addWatch about the details
	parent := filepath.Dir(fullPath)

	iw.pathList[parent]--

	if iw.pathList[parent] == 0 {
		if err := iw.wd.RemoveWatch(parent); err != nil {
			logger.Noticef("cannot remove watch: %v", err)
		}

		delete(iw.pathList, parent)
	}

	logger.Debugf("watches after remove: %v", iw.pathList)
}

// processDeletedPath checks if the received path corresponds to the passed
// CGroup, removing it from the list of folders being watched in that CGroup if
// needed. It returns true if there remain folders to be monitored in that CGroup,
// or false if all the folders of that CGroup have been deleted.
func (iw *inotifyWatcher) processDeletedPath(watch *groupToWatch, deletedPath string) (keepWatching bool) {
	if _, ok := watch.folders[deletedPath]; ok {
		// if the folder name is in the list of folders to monitor, decrement the
		// parent's usage counter, and remove it from the list of folders to watch
		// in this CGroup (by not adding it to tmpFolders)
		iw.removePath(deletedPath)
		delete(watch.folders, deletedPath)

	}
	if len(watch.folders) == 0 {
		// if all the files/folders of this CGroup have been deleted, notify the
		// client that it is done.
		watch.sendClosedNotification()
		return false
	}
	return true
}

func (iw *inotifyWatcher) watcherMainLoop() {
	for {
		select {
		case event := <-iw.wd.Event:
			if event.Mask&inotify.InDelete == 0 {
				continue
			}
			var newGroupList []*groupToWatch
			for _, watch := range iw.groupList {
				if iw.processDeletedPath(watch, event.Name) {
					newGroupList = append(newGroupList, watch)
				}
			}
			iw.groupList = newGroupList
		case newWatch := <-iw.addWatchChan:
			iw.addWatch(newWatch)
		}
	}
}

// MonitorDelete monitors the specified paths for deletion.
// Once all of them have been deleted, it pushes the specified name through the channel.
func (iw *inotifyWatcher) monitorDelete(folders []string, name string, channel chan<- string) (err error) {
	iw.doOnce.Do(func() {
		iw.wd, err = inotify.NewWatcher()
		if err != nil {
			return
		}
		go iw.watcherMainLoop()
	})
	if err != nil {
		return err
	}
	if iw.wd == nil {
		return errors.New("cannot initialise Inotify.")
	}

	foldersMap := make(map[string]struct{}, len(folders))
	for _, f := range folders {
		foldersMap[f] = struct{}{}
	}

	iw.addWatchChan <- &groupToWatch{
		name:    name,
		folders: foldersMap,
		channel: channel,
	}
	return nil
}

// MonitorSnapEnded monitors the running instances of a snap. Once all
// instances of the snap have stopped, its name is pushed through the supplied
// channel. This allows the caller to use the same channel to monitor several snaps.
func MonitorSnapEnded(snapName string, channel chan<- string) error {
	options := InstancePathsOptions{
		ReturnCGroupPath: true,
	}
	paths, err := InstancePathsOfSnap(snapName, options)
	if err != nil {
		return err
	}
	return currentWatcher.monitorDelete(paths, snapName, channel)
}
