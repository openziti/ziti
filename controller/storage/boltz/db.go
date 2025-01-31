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

package boltz

import (
	"bytes"
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	Metadata      = "meta"
	SnapshotId    = "snapshotId"
	ResetTimeline = "resetTimeline"
	TimelineId    = "timelineId"
)

type TimelineMode string

const (
	TimelineModeDefault     TimelineMode = "default"
	TimelineModeInitIfEmpty TimelineMode = "initIfEmpty"
	TimelineModeForceReset  TimelineMode = "forceReset"
)

func (t TimelineMode) forceResetTimeline(timelineId *string) bool {
	if t == TimelineModeForceReset {
		return true
	}
	if t == TimelineModeInitIfEmpty && timelineId == nil {
		return true
	}
	return false
}

type Db interface {
	io.Closer
	Update(ctx MutateContext, fn func(ctx MutateContext) error) error
	Batch(ctx MutateContext, fn func(ctx MutateContext) error) error
	View(fn func(tx *bbolt.Tx) error) error
	RootBucket(tx *bbolt.Tx) (*bbolt.Bucket, error)

	// GetDefaultSnapshotPath returns the default location for a snapshot created now
	GetDefaultSnapshotPath() string

	// Snapshot makes a copy of the bolt file at the given location
	Snapshot(path string) (string, string, error)

	// SnapshotInTx makes a copy of the bolt file at the given location, using an existing tx
	SnapshotInTx(tx *bbolt.Tx, path string) (string, string, error)

	StreamToWriter(w io.Writer) error

	// GetSnapshotId returns the id of the last snapshot created/restored
	GetSnapshotId() (*string, error)

	// RestoreSnapshot will replace the existing DB with the given snapshot
	// This operation is not allowed to fail, and will thus panic if the snapshot cannot be restored
	RestoreSnapshot(snapshotData []byte)

	// RestoreFromReader will replace the existing DB with the given snapshot
	// This operation is not allowed to fail, and will thus panic if the snapshot cannot be restored
	RestoreFromReader(snapshot io.Reader)

	// AddRestoreListener adds a callback which will be invoked asynchronously when a snapshot is restored
	AddRestoreListener(listener func())

	// AddTxCompleteListener adds a listener which is called all tx processing is complete, including
	// post-commit hooks
	AddTxCompleteListener(listener func(ctx MutateContext))

	// GetTimelineId returns the timeline id
	GetTimelineId(mode TimelineMode, idF func() (string, error)) (string, error)
}

type DbImpl struct {
	rootBucket          string
	reloadLock          sync.RWMutex
	db                  *bbolt.DB
	restoreListeners    concurrenz.CopyOnWriteSlice[func()]
	txCompleteListeners concurrenz.CopyOnWriteSlice[func(ctx MutateContext)]
}

func Open(path string, rootBucket string) (*DbImpl, error) {
	result := &DbImpl{
		rootBucket: rootBucket,
	}
	if err := result.Open(path); err != nil {
		return nil, err
	}

	return result, nil
}

func (self *DbImpl) Open(path string) error {
	// Only wait 1 second if database file can't be locked, as it most likely means another controller is running
	options := *bbolt.DefaultOptions
	options.Timeout = time.Second

	var err error
	if self.db, err = bbolt.Open(path, 0600, &options); err != nil {
		return errors.Wrapf(err, "unable to open controller database [%s]", path)
	}

	return nil
}

func (self *DbImpl) Close() error {
	return self.db.Close()
}

func (self *DbImpl) AddTxCompleteListener(listener func(ctx MutateContext)) {
	self.txCompleteListeners.Append(listener)
}

func (self *DbImpl) Update(ctx MutateContext, fn func(ctx MutateContext) error) error {
	if ctx == nil {
		ctx = NewMutateContext(context.Background())
	}

	if ctx.Tx() == nil {
		self.reloadLock.RLock()
		defer self.reloadLock.RUnlock()

		defer ctx.setTx(nil)

		return self.db.Update(func(tx *bbolt.Tx) error {
			ctx.setTx(tx)
			if err := fn(ctx); err != nil {
				return err
			}
			if err := ctx.runPreCommitActions(); err != nil {
				return err
			}

			txCompleteListeners := self.txCompleteListeners.Value()
			if txCompleteListeners != nil {
				tx.OnCommit(func() {
					for _, listener := range txCompleteListeners {
						listener(ctx)
					}
				})
			}

			return nil
		})
	}

	return fn(ctx)
}

func (self *DbImpl) Batch(ctx MutateContext, fn func(ctx MutateContext) error) error {
	if ctx == nil {
		ctx = NewMutateContext(context.Background())
	}

	if ctx.Tx() == nil {
		self.reloadLock.RLock()
		defer self.reloadLock.RUnlock()

		defer ctx.setTx(nil)

		return self.db.Batch(func(tx *bbolt.Tx) error {
			ctx.setTx(tx)
			if err := fn(ctx); err != nil {
				return err
			}
			return ctx.runPreCommitActions()
		})
	}

	return fn(ctx)
}

func (self *DbImpl) View(fn func(tx *bbolt.Tx) error) error {
	self.reloadLock.RLock()
	defer self.reloadLock.RUnlock()
	return self.db.View(fn)
}

func (self *DbImpl) Stats() bbolt.Stats {
	self.reloadLock.RLock()
	defer self.reloadLock.RUnlock()

	return self.db.Stats()
}

func (self *DbImpl) RootBucket(tx *bbolt.Tx) (*bbolt.Bucket, error) {
	self.reloadLock.RLock()
	defer self.reloadLock.RUnlock()

	rootBucket := tx.Bucket([]byte(self.rootBucket))
	if rootBucket == nil {
		return nil, errors.Errorf("db missing root bucket [%v]", self.rootBucket)
	}
	return rootBucket, nil
}

func (self *DbImpl) GetDefaultSnapshotPath() string {
	path := self.db.Path()
	path += "-" + time.Now().Format("20060102-150405")
	return path
}

func (self *DbImpl) Snapshot(path string) (string, string, error) {
	var actualPath string
	var snapshotId string
	err := self.View(func(tx *bbolt.Tx) error {
		var err error
		actualPath, snapshotId, err = self.SnapshotInTx(tx, path)
		return err
	})
	if err != nil {
		return "", "", err
	}
	return actualPath, snapshotId, nil
}

func (self *DbImpl) SnapshotInTx(tx *bbolt.Tx, path string) (string, string, error) {
	self.reloadLock.RLock()
	defer self.reloadLock.RUnlock()

	now := time.Now()
	dateStr := now.Format("20060102")
	timeStr := now.Format("150405")

	path = strings.ReplaceAll(path, "__DATE__", dateStr)
	path = strings.ReplaceAll(path, "__TIME__", timeStr)
	path = strings.ReplaceAll(path, "__DB_DIR__", filepath.Dir(self.db.Path()))
	path = strings.ReplaceAll(path, "__DB_FILE__", filepath.Base(self.db.Path()))
	path = strings.ReplaceAll(path, "DATE", dateStr)
	path = strings.ReplaceAll(path, "TIME", timeStr)
	path = strings.ReplaceAll(path, "DB_DIR", filepath.Dir(self.db.Path()))
	path = strings.ReplaceAll(path, "DB_FILE", filepath.Base(self.db.Path()))

	pfxlog.Logger().WithField("path", path).Info("snapshotting database to file")

	if err := tx.CopyFile(path, 0600); err != nil {
		return "", "", err
	}

	snapshotId, err := self.MarkAsSnapshot(path)
	if err != nil {
		if rmErr := os.Remove(path); rmErr != nil {
			pfxlog.Logger().WithError(rmErr).Error("failed to removed snapshot after failing to mark snapshot")
		}
		return "", "", fmt.Errorf("failed to update snapshot metadata: %w", err)
	}

	return path, snapshotId, nil
}

func (self *DbImpl) StreamToWriter(w io.Writer) error {
	self.reloadLock.RLock()
	defer self.reloadLock.RUnlock()

	return self.db.View(func(tx *bbolt.Tx) error {
		_, err := tx.WriteTo(w)
		return err
	})
}

func (self *DbImpl) RestoreSnapshot(snapshot []byte) {
	r := bytes.NewBuffer(snapshot)
	self.RestoreFromReader(r)
}

func (self *DbImpl) RestoreFromReader(snapshot io.Reader) {
	snapshotPath, err := self.persistSnapshot(snapshot)
	if err != nil {
		panic(err)
	}

	self.reloadLock.Lock()
	defer self.reloadLock.Unlock()

	dbPath := self.db.Path()

	if err = self.Close(); err != nil {
		panic(errors.Wrap(err, "unable to close current database while applying snapshot"))
	}

	backupPath := dbPath + ".previous"
	if err = os.Rename(dbPath, backupPath); err != nil {
		panic(errors.Wrapf(err, "unable to rename current db file [%v] to [%v]", dbPath, backupPath))
	}

	if err = os.Rename(snapshotPath, dbPath); err != nil {
		panic(errors.Wrapf(err, "unable to rename new db snapshot file [%v] to [%v]", snapshotPath, dbPath))
	}

	if err = self.Open(dbPath); err != nil {
		panic(err)
	}

	for _, listener := range self.restoreListeners.Value() {
		go listener()
	}
}

func (self *DbImpl) persistSnapshot(snapshot io.Reader) (string, error) {
	tmpPath := self.db.Path() + ".snapshot." + uuid.NewString()
	f, err := os.Create(tmpPath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create snapshot file [%v]", tmpPath)
	}
	_, err = io.Copy(f, snapshot)
	if err != nil {
		if closeErr := f.Close(); closeErr != nil {
			return "", errors.Wrapf(errorz.MultipleErrors{err, closeErr}, "unable to write snapshot data to file [%v]", tmpPath)
		}
		return "", errors.Wrapf(err, "unable to write snapshot data to file [%v]", tmpPath)
	}
	if err = f.Close(); err != nil {
		return "", errors.Wrapf(err, "unable to close db snapshot file after write [%v]", tmpPath)
	}
	return tmpPath, nil
}

func (self *DbImpl) AddRestoreListener(f func()) {
	self.restoreListeners.Append(f)
}

func (self *DbImpl) MarkAsSnapshot(path string) (string, error) {
	db, err := Open(path, self.rootBucket)
	if err != nil {
		return "", err
	}

	defer func() {
		_ = db.Close()
	}()

	snapshotId := uuid.NewString()

	err = db.Update(nil, func(ctx MutateContext) error {
		tx := ctx.Tx()
		b := GetOrCreatePath(tx, Metadata)
		b.SetString(SnapshotId, snapshotId, nil)
		b.SetBool(ResetTimeline, true, nil)
		return b.GetError()
	})

	if err != nil {
		return "", fmt.Errorf("error setting snapshot properties %w", err)
	}

	pfxlog.Logger().Infof("set snapshot id at %s to %s", path, snapshotId)
	return snapshotId, nil
}

func (self *DbImpl) GetSnapshotId() (*string, error) {
	var snapshotId *string
	err := self.View(func(tx *bbolt.Tx) error {
		if b := Path(tx, Metadata); b != nil {
			snapshotId = b.GetString(SnapshotId)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snapshotId, nil
}

func (self *DbImpl) GetTimelineId(mode TimelineMode, idF func() (string, error)) (string, error) {
	timelineId := ""
	err := self.Update(nil, func(ctx MutateContext) error {
		b := GetOrCreatePath(ctx.Tx(), Metadata)
		if b.HasError() {
			return b.Err
		}
		resetRequired := b.GetBoolWithDefault(ResetTimeline, false)
		idPointer := b.GetString(TimelineId)
		pfxlog.Logger().Infof("checking timeline id. reset required? %v timelineId: %s", resetRequired, func() string {
			if idPointer != nil {
				return *idPointer
			}
			return "nil"
		}())

		if resetRequired || mode.forceResetTimeline(idPointer) {
			id, err := idF()
			if err != nil {
				return err
			}
			timelineId = id
			b.SetString(TimelineId, id, nil)
			b.SetBool(ResetTimeline, false, nil)

			oldTimelineId := ""
			if idPointer != nil {
				oldTimelineId = *idPointer
			}
			pfxlog.Logger().Infof("updated timeline id %s -> %s", oldTimelineId, timelineId)
			return b.GetError()
		}
		if idPointer != nil {
			timelineId = *idPointer
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return timelineId, nil

}
