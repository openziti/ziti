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
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"io"
	"os"
	"sync"
	"time"
)

const (
	Metadata   = "meta"
	SnapshotId = "snapshotId"
)

type Db interface {
	io.Closer
	Update(fn func(tx *bbolt.Tx) error) error
	Batch(fn func(tx *bbolt.Tx) error) error
	View(fn func(tx *bbolt.Tx) error) error
	RootBucket(tx *bbolt.Tx) (*bbolt.Bucket, error)

	// Snapshot makes a copy of the bolt file
	Snapshot(tx *bbolt.Tx) error

	// SnapshotToMemory writes a snapshot of the database state to a memory buffer.
	// The snapshot has a UUID generated and stored at rootBucket/snapshotId
	// The snapshot id and snapshot are returned
	SnapshotToMemory() (string, []byte, error)

	// GetSnapshotId returns the id of the last snapshot created/restored
	GetSnapshotId() (*string, error)

	// RestoreSnapshot will replace the existing DB with the given snapshot
	// This operation is not allowed to fail, and will thus panic if the snapshot cannot be restored
	RestoreSnapshot(snapshotData []byte)

	// AddRestoreListener adds a callback which will be invoked asynchronously when a snapshot is restored
	AddRestoreListener(listener func())
}

type DbImpl struct {
	rootBucket       string
	reloadLock       sync.RWMutex
	db               *bbolt.DB
	restoreListeners concurrenz.CopyOnWriteSlice[func()]
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

func (self *DbImpl) Update(fn func(tx *bbolt.Tx) error) error {
	self.reloadLock.RLock()
	defer self.reloadLock.RUnlock()

	return self.db.Update(fn)
}

func (self *DbImpl) Batch(fn func(tx *bbolt.Tx) error) error {
	self.reloadLock.RLock()
	defer self.reloadLock.RUnlock()

	return self.db.Batch(fn)
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

func (self *DbImpl) Snapshot(tx *bbolt.Tx) error {
	self.reloadLock.RLock()
	defer self.reloadLock.RUnlock()

	path := self.db.Path()
	path += "-" + time.Now().Format("20060102-150405")

	_, err := os.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	} else {
		pfxlog.Logger().Infof("bolt db backup already made: %v", path)
		return nil
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer func() {
		if err = file.Close(); err != nil {
			pfxlog.Logger().Errorf("failed to close backup database file %v (%v)", path, err)
		}
	}()

	_, err = tx.WriteTo(file)
	if err != nil {
		pfxlog.Logger().Infof("created bolt db backup: %v", path)
	}
	return err
}

func (self *DbImpl) RestoreSnapshot(snapshot []byte) {
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

func (self *DbImpl) persistSnapshot(snapshot []byte) (string, error) {
	tmpPath := self.db.Path() + ".snapshot." + uuid.NewString()
	f, err := os.Create(tmpPath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create snapshot file [%v]", tmpPath)
	}
	_, err = f.Write(snapshot)
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

func (self *DbImpl) SnapshotToMemory() (string, []byte, error) {
	buf := &bytes.Buffer{}
	snapshotId := uuid.NewString()
	err := self.Update(func(tx *bbolt.Tx) error {
		b := GetOrCreatePath(tx, Metadata)
		b.SetString(SnapshotId, snapshotId, nil)
		if b.HasError() {
			return b.GetError()
		}
		_, err := tx.WriteTo(buf)
		return err
	})
	if err != nil {
		return "", nil, err
	}
	return snapshotId, buf.Bytes(), nil
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
