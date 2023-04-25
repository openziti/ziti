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
	"github.com/hashicorp/raft"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/change"
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/event"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
	"io"
	"os"
	"path"
)

const (
	bucketName = "raft"
	fieldIndex = "index"
)

func NewFsm(dataDir string, decoders command.Decoders, indexTracker IndexTracker, eventDispatcher event.Dispatcher) *BoltDbFsm {
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
	eventDispatcher event.Dispatcher
	currentState    *raft.Configuration
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
	var result uint64
	err := self.db.View(func(tx *bbolt.Tx) error {
		if raftBucket := boltz.Path(tx, db.RootBucket, bucketName); raftBucket != nil {
			if val := raftBucket.GetInt64(fieldIndex); val != nil {
				result = uint64(*val)
			}
		}
		return nil
	})
	return result, err
}

func (self *BoltDbFsm) updateIndexInTx(tx *bbolt.Tx, index uint64) error {
	raftBucket := boltz.GetOrCreatePath(tx, db.RootBucket, bucketName)
	raftBucket.SetInt64(fieldIndex, int64(index), nil)
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
	if self.currentState == nil {
		if err := raft.GetConfiguration().Error(); err != nil {
			pfxlog.Logger().WithError(err).Error("error getting configuration future")
		}
		cfg := raft.GetConfiguration().Configuration()
		self.currentState = &cfg
	}
	return self.indexTracker.Index(), self.currentState
}

func (self *BoltDbFsm) StoreConfiguration(index uint64, configuration raft.Configuration) {
	self.currentState = &configuration
	evt := event.NewClusterEvent(event.ClusterMembersChanged)
	evt.Index = index
	for _, srv := range configuration.Servers {
		evt.Peers = append(evt.Peers, &event.ClusterPeer{
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
				changeCtx = change.New().SetSource("untracked")
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
	if err != nil {
		return err
	}

	gzReader, err := gzip.NewReader(snapshot)
	if err != nil {
		return errors.Wrapf(err, "unable to create gz reader for reading raft snapshot during restore")
	}

	_, err = io.Copy(dbFile, gzReader)
	if err != nil {
		_ = os.Remove(self.dbPath)
		if renameErr := os.Rename(backup, self.dbPath); renameErr != nil {
			logrus.WithError(renameErr).Error("failed to move ziti db back to original location")
		}
		return err
	}

	// if we're not initializing from a snapshot at startup, restart
	if self.indexTracker.Index() > 0 {
		os.Exit(0)
	}

	self.db, err = db.Open(self.dbPath)
	return err
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
