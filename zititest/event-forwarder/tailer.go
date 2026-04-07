/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package main

import (
	"bufio"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/michaelquigley/pfxlog"
)

// fileTailer follows a file, reading new lines as they appear. It handles
// log rotation by detecting when the file's inode changes (rename + recreate)
// and reopening from the beginning of the new file. Lines are delivered to the
// Lines channel.
//
// Unlike inotify-based tailers, this uses simple polling, which avoids known
// race conditions in inotify-based libraries during file rotation.
type fileTailer struct {
	path     string
	Lines    chan string
	pollRate time.Duration
	done     chan struct{}
}

// newFileTailer creates a tailer that follows the given path. It starts reading
// from the end of the current file (like tail -f). Call stop() to shut down.
func newFileTailer(path string) *fileTailer {
	return &fileTailer{
		path:     path,
		Lines:    make(chan string, 1024),
		pollRate: 250 * time.Millisecond,
		done:     make(chan struct{}),
	}
}

// start begins tailing in a background goroutine.
func (t *fileTailer) start() error {
	f, err := os.Open(t.path)
	if err != nil {
		return err
	}

	// Seek to end so we only read new content.
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		f.Close()
		return err
	}

	go t.run(f)
	return nil
}

// stop signals the tailer to shut down.
func (t *fileTailer) stop() {
	close(t.done)
}

func (t *fileTailer) run(f *os.File) {
	log := pfxlog.Logger()
	defer close(t.Lines)
	defer f.Close()

	reader := bufio.NewReader(f)
	var currentInode uint64
	if info, err := f.Stat(); err == nil {
		currentInode = fileInode(info)
	}

	ticker := time.NewTicker(t.pollRate)
	defer ticker.Stop()

	for {
		// Read all available complete lines.
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if len(line) > 0 {
					// Partial line (no trailing newline yet). Push it back by
					// re-creating the reader at the current position so we
					// re-read the partial data next poll.
					pos, _ := f.Seek(0, io.SeekCurrent)
					rewind := int64(len(line))
					f.Seek(pos-rewind, io.SeekStart)
					reader.Reset(f)
				}
				break
			}
			// Trim the trailing newline.
			line = line[:len(line)-1]
			if len(line) > 0 {
				select {
				case t.Lines <- line:
				case <-t.done:
					return
				}
			}
		}

		// Wait for the next poll or shutdown.
		select {
		case <-t.done:
			return
		case <-ticker.C:
		}

		// Check if the file was rotated (inode changed).
		newInfo, err := os.Stat(t.path)
		if err != nil {
			// File may not exist briefly during rotation.
			continue
		}

		newInode := fileInode(newInfo)
		if newInode != currentInode && currentInode != 0 {
			log.Infof("file rotated (inode %d -> %d), reopening %s", currentInode, newInode, t.path)
			// Drain any remaining lines from the old file.
			t.drainFile(f, reader)
			f.Close()

			newF, err := os.Open(t.path)
			if err != nil {
				log.WithError(err).Warn("failed to reopen after rotation, waiting...")
				// Wait for the file to reappear.
				f, currentInode = t.waitForFile()
				if f == nil {
					return // done was closed
				}
				reader = bufio.NewReader(f)
				continue
			}

			f = newF
			reader = bufio.NewReader(f)
			currentInode = newInode
		}
	}
}

// drainFile reads any remaining complete lines from the old file before closing it.
func (t *fileTailer) drainFile(f *os.File, reader *bufio.Reader) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = line[:len(line)-1]
		if len(line) > 0 {
			select {
			case t.Lines <- line:
			case <-t.done:
				return
			}
		}
	}
}

// waitForFile polls until the file appears at the path or done is closed.
func (t *fileTailer) waitForFile() (*os.File, uint64) {
	log := pfxlog.Logger()
	for {
		select {
		case <-t.done:
			return nil, 0
		case <-time.After(500 * time.Millisecond):
		}

		f, err := os.Open(t.path)
		if err != nil {
			continue
		}

		info, err := f.Stat()
		if err != nil {
			f.Close()
			continue
		}

		log.Infof("file reappeared: %s", t.path)
		return f, fileInode(info)
	}
}

// fileInode extracts the inode number from a FileInfo.
func fileInode(info os.FileInfo) uint64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Ino
	}
	return 0
}
