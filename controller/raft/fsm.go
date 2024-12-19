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
	"bytes"
	"compress/gzip"
	"fmt"
	"github.com/hashicorp/raft"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/db"
	event2 "github.com/openziti/ziti/controller/event"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
	"io"
	"os"
	"path"
	"sync/atomic"
)

func NewFsm(dataDir string, decoders command.Decoders, indexTracker IndexTracker, eventDispatcher event2.Dispatcher) *BoltDbFsm {
	return &BoltDbFsm{
		decoders:        decoders,
		dbPath:          path.Join(dataDir, "ctrl-ha.db"),
		indexTracker:    indexTracker,
		eventDispatcher: eventDispatcher,
	}
}

type BoltDbFsm struct {
	db              boltz.Db
	dbPath          string
	decoders        command.Decoders
	indexTracker    IndexTracker
	eventDispatcher event2.Dispatcher
	currentState    atomic.Pointer[raft.Configuration]
	index           uint64
}

func (self *BoltDbFsm) Init() error {
	log := pfxlog.Logger()
	log.WithField("dbPath", self.dbPath).Info("initializing fsm")

	var err error
	self.db, err = db.Open(self.dbPath)
	if err != nil {
		return err
	}

	index, err := self.loadCurrentIndex()
	if err != nil {
		return err
	}
	self.index = index
	self.indexTracker.NotifyOfIndex(index)

	return nil
}

func (self *BoltDbFsm) GetDb() boltz.Db {
	return self.db
}

func (self *BoltDbFsm) loadCurrentIndex() (uint64, error) {
	return self.loadDbIndex(self.db)
}

func (self *BoltDbFsm) loadDbIndex(zitiDb boltz.Db) (uint64, error) {
	var result uint64
	err := zitiDb.View(func(tx *bbolt.Tx) error {
		result = db.LoadCurrentRaftIndex(tx)
		return nil
	})
	return result, err
}

func (self *BoltDbFsm) updateIndexInTx(tx *bbolt.Tx, index uint64) error {
	raftBucket := boltz.GetOrCreatePath(tx, db.RootBucket, db.MetadataBucket)
	raftBucket.SetInt64(db.FieldRaftIndex, int64(index), nil)
	return raftBucket.GetError()
}

func (self *BoltDbFsm) updateIndex(index uint64) {
	err := self.db.Update(nil, func(ctx boltz.MutateContext) error {
		return self.updateIndexInTx(ctx.Tx(), index)
	})
	if err != nil {
		pfxlog.Logger().WithError(err).Error("unable to update raft index in database")
	}
}

func (self *BoltDbFsm) GetCurrentState(raft *raft.Raft) (uint64, *raft.Configuration) {
	currentState := self.currentState.Load()
	if currentState == nil {
		if err := raft.GetConfiguration().Error(); err != nil {
			pfxlog.Logger().WithError(err).Error("error getting configuration future")
		}
		cfg := raft.GetConfiguration().Configuration()
		currentState = &cfg
		self.currentState.CompareAndSwap(nil, currentState)
	}
	return self.indexTracker.Index(), currentState
}

func (self *BoltDbFsm) StoreConfiguration(index uint64, configuration raft.Configuration) {
	self.currentState.Store(&configuration)
	evt := event2.NewClusterEvent(event2.ClusterMembersChanged)
	evt.Index = index
	for _, srv := range configuration.Servers {
		evt.Peers = append(evt.Peers, &event2.ClusterPeer{
			Id:   string(srv.ID),
			Addr: string(srv.Address),
		})
	}
	self.eventDispatcher.AcceptClusterEvent(evt)
}

func (self *BoltDbFsm) Apply(log *raft.Log) interface{} {
	logger := pfxlog.Logger().WithField("index", log.Index)
	if log.Type == raft.LogCommand {
		defer self.indexTracker.NotifyOfIndex(log.Index)

		if log.Index <= self.index {
			logger.Debug("skipping replay of command")
			return nil
		}

		self.index = log.Index

		if len(log.Data) >= 4 {
			cmd, err := self.decoders.Decode(log.Data)
			if err != nil {
				logger.WithError(err).Error("failed to create command")
				return err
			}

			logger.Infof("apply log with type %T", cmd)
			changeCtx := cmd.GetChangeContext()
			if changeCtx == nil {
				changeCtx = change.New().SetSourceType("unattributed").SetChangeAuthorType(change.AuthorTypeUnattributed)
			}
			changeCtx.RaftIndex = log.Index

			ctx := changeCtx.NewMutateContext()
			ctx.AddPreCommitAction(func(ctx boltz.MutateContext) error {
				return self.updateIndexInTx(ctx.Tx(), log.Index)
			})

			if err = cmd.Apply(ctx); err != nil {
				logger.WithError(err).Error("applying log resulted in error")
				// if this errored, assume that we haven't updated the index in the db
				self.updateIndex(log.Index)
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

	buf := &bytes.Buffer{}
	gzWriter := gzip.NewWriter(buf)
	id, err := self.db.SnapshotToWriter(gzWriter)
	if err != nil {
		return nil, err
	}

	if err = gzWriter.Close(); err != nil {
		return nil, errors.Wrap(err, "error finishing gz compression of raft snapshot")
	}

	logrus.WithField("id", id).WithField("index", self.indexTracker.Index()).Info("creating snapshot")

	return &boltSnapshot{
		snapshotId:   id,
		snapshotData: buf.Bytes(),
	}, nil
}

func (self *BoltDbFsm) Restore(snapshot io.ReadCloser) error {
	var currentSnapshotId string
	var currentIndex uint64

	if self.db != nil {
		snapshotId, _ := self.db.GetSnapshotId()
		if snapshotId != nil {
			currentSnapshotId = *snapshotId
		}
		currentIndex, _ = self.loadCurrentIndex()
	}

	logrus.Info("restoring from snapshot")

	tmpPath := self.dbPath + ".stage"
	if err := self.restoreSnapshotDbFile(tmpPath, snapshot); err != nil {
		return err
	}

	newSnapshotId, newIndex, err := self.GetSnapshotMetadata(tmpPath)
	if err != nil {
		return err
	}

	log := pfxlog.Logger().
		WithField("currentSnapshotId", currentSnapshotId).
		WithField("newSnapshotId", newSnapshotId).
		WithField("currentIndex", currentIndex).
		WithField("newIndex", newIndex)

	if newIndex > currentIndex {
		log.Info("new index is greater than current index, restoring snapshot")
	} else {
		log.Info("snapshot index is <= current index, no need to restore snapshot")
		if err = os.Remove(tmpPath); err != nil {
			pfxlog.Logger().WithError(err).WithField("path", tmpPath).Error("failed to remove temporary snapshot db file")
		}
		return nil
	}

	if self.db != nil {
		if err := self.db.Close(); err != nil {
			return err
		}
	}

	backup := self.dbPath + ".backup"
	if err = os.Rename(self.dbPath, backup); err != nil {
		return fmt.Errorf("failed to copy existing db to backup path (%w)", err)
	}

	if err = os.Rename(tmpPath, self.dbPath); err != nil {
		return fmt.Errorf("failed to copy snapshot db to primary db location (%w)", err)
	}

	// if we're not initializing from a snapshot at startup, restart
	if self.indexTracker.Index() > 0 {
		log.Info("restored snapshot to initialized system, restart required. exiting")
		os.Exit(0)
	}

	self.db, err = db.Open(self.dbPath)
	if err == nil {
		log.Info("restored snapshot to uninitialized system, ok to continue")
	}
	return err
}

func (self *BoltDbFsm) restoreSnapshotDbFile(path string, snapshot io.ReadCloser) error {
	dbFile, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	success := false

	defer func() {
		if err = dbFile.Close(); err != nil {
			pfxlog.Logger().WithError(err).WithField("path", path).Error("failed to close temporary snapshot db file")
		}

		if !success {
			if err = os.Remove(path); err != nil {
				pfxlog.Logger().WithError(err).WithField("path", path).Error("failed to remove temporary snapshot db file")
			}
		}
	}()

	gzReader, err := gzip.NewReader(snapshot)
	if err != nil {
		return errors.Wrapf(err, "unable to create gz reader for reading raft snapshot during restore")
	}

	if _, err = io.Copy(dbFile, gzReader); err != nil {
		return err
	}

	success = true
	return nil
}

func (self *BoltDbFsm) GetSnapshotMetadata(path string) (string, uint64, error) {
	newDb, err := db.Open(path)
	if err != nil {
		return "", 0, err
	}

	defer func() {
		if err = newDb.Close(); err != nil {
			pfxlog.Logger().WithError(err).WithField("path", path).Error("error closing snapshot db")
		}
	}()

	snapshotIdP, err := newDb.GetSnapshotId()
	if err != nil {
		return "", 0, err
	}
	var snapshotId string
	if snapshotIdP != nil {
		snapshotId = *snapshotIdP
	}

	idx, err := self.loadDbIndex(newDb)
	if err != nil {
		return "", 0, err
	}

	return snapshotId, idx, nil
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
