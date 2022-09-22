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

package raft

import (
	"github.com/hashicorp/raft"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"sync/atomic"
)

func NewFsm(dataDir string, decoders command.Decoders, indexTracker IndexTracker) *BoltDbFsm {
	return &BoltDbFsm{
		dataDir:      dataDir,
		decoders:     decoders,
		dbPath:       dataDir + "ctrl.db",
		indexTracker: indexTracker,
	}
}

type BoltDbFsm struct {
	db           boltz.Db
	dataDir      string
	dbPath       string
	decoders     command.Decoders
	initialized  atomic.Bool
	indexTracker IndexTracker
}

func (self *BoltDbFsm) Init() error {
	var err error
	self.db, err = db.Open(self.dbPath)
	if err != nil {
		return err
	}

	return nil
}

func (self *BoltDbFsm) RaftInitialized() {
	self.initialized.Store(true)
}

func (self *BoltDbFsm) GetDb() boltz.Db {
	return self.db
}

func (self *BoltDbFsm) Apply(log *raft.Log) interface{} {
	logger := pfxlog.Logger()
	if log.Type == raft.LogCommand {
		defer self.indexTracker.NotifyOfIndex(log.Index)

		if len(log.Data) >= 4 {
			cmd, err := self.decoders.Decode(log.Data)
			if err != nil {
				logger.WithError(err).Error("failed to create command")
				return err
			}

			logger.Infof("[%v] apply log with type %T", log.Index, cmd)

			if err = cmd.Apply(); err != nil {
				logger.WithError(err).Error("applying log resulted in error")
			}

			return err
		} else {
			return errors.Errorf("log data contained invalid message type. data: %+v", log.Data)
		}
	}
	return nil
}

func (self *BoltDbFsm) Snapshot() (raft.FSMSnapshot, error) {
	logrus.Debug("creating snapshot")

	id, data, err := self.db.SnapshotToMemory()
	if err != nil {
		return nil, err
	}

	logrus.WithField("id", id).WithField("index", self.indexTracker.Index()).Info("creating snapshot")

	return &boltSnapshot{
		snapshotId:   id,
		snapshotData: data,
	}, nil
}

func (self *BoltDbFsm) Restore(snapshot io.ReadCloser) error {
	if self.db != nil {
		if err := self.db.Close(); err != nil {
			return err
		}
	}

	logrus.Info("restoring from snapshot")

	backup := self.dbPath + ".backup"
	err := os.Rename(self.dbPath, backup)
	if err != nil {
		return err
	}

	dbFile, err := os.OpenFile(self.dbPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
	_, err = io.Copy(dbFile, snapshot)
	if err != nil {
		_ = os.Remove(self.dbPath)
		if renameErr := os.Rename(backup, self.dbPath); renameErr != nil {
			logrus.WithError(renameErr).Error("failed to move ziti db back to original location")
		}
		return err
	}

	if self.initialized.Load() {
		// figure out what happens when we restore a snapshot on startup, vs during runtime
		os.Exit(0)
	}

	return self.Init()
}

type boltSnapshot struct {
	snapshotId   string
	snapshotData []byte
}

func (self *boltSnapshot) Persist(sink raft.SnapshotSink) error {
	_, err := sink.Write(self.snapshotData)
	return err
}

func (self *boltSnapshot) Release() {
	self.snapshotData = nil
}
