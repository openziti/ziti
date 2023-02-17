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
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/event"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"path"
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
}

func (self *BoltDbFsm) Init() error {
	log := pfxlog.Logger()
	log.Info("initializing fsm")

	if _, err := os.Stat(self.dbPath); err == nil {
		backup := self.dbPath + ".previous"
		log.Infof("moving previous db to %v", backup)
		err := os.Rename(self.dbPath, backup)
		if err != nil {
			return err
		}
	}

	var err error
	self.db, err = db.Open(self.dbPath)
	if err != nil {
		return err
	}

	return nil
}

func (self *BoltDbFsm) GetDb() boltz.Db {
	return self.db
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
