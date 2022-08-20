package raft

import (
	"github.com/hashicorp/raft"
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
	"io"
	"io/ioutil"
	"os"
	"sync/atomic"
)

func NewFsm(dataDir string, decoders command.Decoders) *BoltDbFsm {
	return &BoltDbFsm{
		dataDir:  dataDir,
		decoders: decoders,
		dbPath:   dataDir + "ctrl.db",
	}
}

type BoltDbFsm struct {
	db          boltz.Db
	dataDir     string
	dbPath      string
	decoders    command.Decoders
	env         atomic.Value
	initialized concurrenz.AtomicBoolean
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
	self.initialized.Set(true)
}

func (self *BoltDbFsm) GetDb() boltz.Db {
	return self.db
}

func (self *BoltDbFsm) Apply(log *raft.Log) interface{} {
	if log.Type == raft.LogCommand {
		if len(log.Data) >= 4 {
			cmd, err := self.decoders.Decode(log.Data)
			if err != nil {
				logrus.WithError(err).Error("failed to create command")
				return err
			}

			logrus.Infof("apply log with type %T", cmd)

			if err = cmd.Apply(); err != nil {
				logrus.WithError(err).Error("applying log resulted in error")
			}

			return err
		} else {
			return errors.Errorf("log data contained invalid message type. data: %+v", log.Data)
		}
	}
	return nil
}

func (self *BoltDbFsm) Snapshot() (raft.FSMSnapshot, error) {
	logrus.Info("creating snapshot")

	file, err := ioutil.TempFile(self.dataDir, "raft-*.snapshot.db")
	if err != nil {
		return nil, err
	}

	err = self.db.View(func(tx *bbolt.Tx) error {
		_, err = tx.WriteTo(file)
		return err
	})

	if err != nil {
		return nil, err
	}

	return &boltSnapshot{path: file.Name()}, nil
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

	if self.initialized.Get() {
		// figure out what happens when we restore a snapshot on startup, vs during runtime
		os.Exit(0)
	}

	return self.Init()
}

type boltSnapshot struct {
	path string
}

func (self *boltSnapshot) Persist(sink raft.SnapshotSink) error {
	file, err := os.Open(self.path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	_, err = io.Copy(sink, file)
	return err
}

func (self *boltSnapshot) Release() {
	if err := os.Remove(self.path); err != nil {
		logrus.WithError(err).WithField("path", self.path).Error("failed to remove bolt snapshot")
	}
}
