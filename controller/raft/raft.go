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
	"encoding/json"
	"fmt"
	"github.com/openziti/transport/v2"
	"os"
	"path"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/raft/mesh"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type Config struct {
	Recover               bool
	DataDir               string
	MinClusterSize        uint32
	AdvertiseAddress      string
	BootstrapMembers      []string
	CommandHandlerOptions struct {
		MaxQueueSize uint16
		MaxWorkers   uint16
	}
}

func NewController(id *identity.TokenId, version string, config *Config, metricsRegistry metrics.Registry, migrationMgr MigrationManager) *Controller {
	result := &Controller{
		Id:              id,
		Config:          config,
		metricsRegistry: metricsRegistry,
		indexTracker:    NewIndexTracker(),
		version:         version,
		migrationMgr:    migrationMgr,
	}
	return result
}

// Controller manages RAFT related state and operations
type Controller struct {
	Id              *identity.TokenId
	Config          *Config
	Mesh            mesh.Mesh
	Raft            *raft.Raft
	Fsm             *BoltDbFsm
	bootstrapped    atomic.Bool
	clusterLock     sync.Mutex
	servers         []raft.Server
	metricsRegistry metrics.Registry
	closeNotify     <-chan struct{}
	indexTracker    IndexTracker
	version         string
	migrationMgr    MigrationManager
}

// GetRaft returns the managed raft instance
func (self *Controller) GetRaft() *raft.Raft {
	return self.Raft
}

// GetMesh returns the related Mesh instance
func (self *Controller) GetMesh() mesh.Mesh {
	return self.Mesh
}

// GetDb returns the DB instance
func (self *Controller) GetDb() boltz.Db {
	return self.Fsm.GetDb()
}

// IsLeader returns true if the current node is the RAFT leader
func (self *Controller) IsLeader() bool {
	return self.Raft.State() == raft.Leader
}

// GetLeaderAddr returns the current leader address, which may be blank if there is no leader currently
func (self *Controller) GetLeaderAddr() string {
	addr, _ := self.Raft.LeaderWithID()
	return string(addr)
}

// Dispatch dispatches the given command to the current leader. If the current node is the leader, the command
// will be applied and the result returned
func (self *Controller) Dispatch(cmd command.Command) error {
	log := pfxlog.Logger()
	if validatable, ok := cmd.(command.Validatable); ok {
		if err := validatable.Validate(); err != nil {
			return err
		}
	}

	if self.Mesh.IsReadOnly() {
		return errors.New("unable to execute command. In a readonly state: different versions detected in cluster")
	}

	if self.IsLeader() {
		_, err := self.applyCommand(cmd)
		return err
	}

	log.WithField("cmd", reflect.TypeOf(cmd)).WithField("dest", self.GetLeaderAddr()).Info("forwarding command")

	peer, err := self.GetMesh().GetOrConnectPeer(self.GetLeaderAddr(), 5*time.Second)
	if err != nil {
		return err
	}

	encoded, err := cmd.Encode()
	if err != nil {
		return err
	}

	msg := channel.NewMessage(NewLogEntryType, encoded)
	result, err := msg.WithTimeout(5 * time.Second).SendForReply(peer.Channel)
	if err != nil {
		return err
	}

	if result.ContentType == SuccessResponseType {
		idx, found := result.GetUint64Header(IndexHeader)
		if found {
			if err = self.indexTracker.WaitForIndex(idx, time.Now().Add(5*time.Second)); err != nil {
				return err
			}
		}
		return nil
	}

	if result.ContentType == ErrorResponseType {
		errCode, found := result.GetUint32Header(HeaderErrorCode)
		if found && errCode == ErrorCodeApiError {
			return self.decodeApiError(result.Body)
		}
		return errors.New(string(result.Body))
	}

	return errors.Errorf("unexpected response type %v", result.ContentType)
}

func (self *Controller) decodeApiError(data []byte) error {
	m := map[string]interface{}{}
	if err := json.Unmarshal(data, &m); err != nil {
		pfxlog.Logger().Warnf("invalid api error encoding, unable to decode: %v", string(data))
		return errors.New(string(data))
	}

	apiErr := &errorz.ApiError{}

	if code, ok := m["code"]; ok {
		if apiErr.Code, ok = code.(string); !ok {
			pfxlog.Logger().Warnf("invalid api error encoding, invalid code, not string: %v", string(data))
			return errors.New(string(data))
		}
	} else {
		pfxlog.Logger().Warnf("invalid api error encoding, no code: %v", string(data))
		return errors.New(string(data))
	}

	if status, ok := m["status"]; ok {
		statusStr := fmt.Sprintf("%v", status)
		statusInt, err := strconv.Atoi(statusStr)
		if err != nil {
			pfxlog.Logger().Warnf("invalid api error encoding, invalid code, not int: %v", string(data))
			return errors.New(string(data))
		}
		apiErr.Status = statusInt
	} else {
		pfxlog.Logger().Warnf("invalid api error encoding, no status: %v", string(data))
		return errors.New(string(data))
	}

	if message, ok := m["message"]; ok {
		if apiErr.Message, ok = message.(string); !ok {
			pfxlog.Logger().Warnf("invalid api error encoding, no message: %v", string(data))
			return errors.New(string(data))
		}
	} else {
		pfxlog.Logger().Warnf("invalid api error encoding, invalid message, not string: %v", string(data))
		return errors.New(string(data))
	}

	if cause, ok := m["cause"]; ok {
		if strCause, ok := cause.(string); ok {
			apiErr.Cause = errors.New(strCause)
		} else if objCause, ok := cause.(map[string]interface{}); ok {
			apiErr.Cause = self.parseFieldError(objCause)
			if apiErr.Cause == nil {
				if b, err := json.Marshal(objCause); err == nil {
					apiErr.Cause = errors.New(string(b))
				} else {
					apiErr.Cause = errors.New(fmt.Sprintf("%+v", objCause))
				}
			}
		} else {
			pfxlog.Logger().Warnf("invalid api error encoding, no cause: %v", string(data))
			return errors.New(string(data))
		}
	}

	return apiErr
}

func (self *Controller) parseFieldError(m map[string]any) *errorz.FieldError {
	var fieldError *errorz.FieldError
	field, ok := m["field"]
	if !ok {
		return nil
	}

	fieldStr, ok := field.(string)
	if !ok {
		return nil
	}

	fieldError = &errorz.FieldError{
		FieldName:  fieldStr,
		FieldValue: m["value"],
	}

	if reason, ok := m["reason"]; ok {
		if reasonStr, ok := reason.(string); ok {
			fieldError.Reason = reasonStr
		}
	}

	return fieldError
}

// applyCommand encodes the command and passes it to ApplyEncodedCommand
func (self *Controller) applyCommand(cmd command.Command) (uint64, error) {
	encoded, err := cmd.Encode()
	if err != nil {
		return 0, err
	}
	return self.ApplyEncodedCommand(encoded)
}

// ApplyEncodedCommand applies the command to the RAFT distributed log
func (self *Controller) ApplyEncodedCommand(encoded []byte) (uint64, error) {
	val, idx, err := self.ApplyWithTimeout(encoded, 5*time.Second)
	if err != nil {
		return 0, err
	}
	if err, ok := val.(error); ok {
		return 0, err
	}
	if val != nil {
		cmd, err := self.Fsm.decoders.Decode(encoded)
		if err != nil {
			logrus.WithError(err).Error("failed to unmarshal command which returned non-nil, non-error value")
			return 0, err
		}
		pfxlog.Logger().WithField("cmdType", reflect.TypeOf(cmd)).Error("command return non-nil, non-error value")
	}
	return idx, nil
}

// ApplyWithTimeout applies the given command to the RAFT distributed log with the given timeout
func (self *Controller) ApplyWithTimeout(log []byte, timeout time.Duration) (interface{}, uint64, error) {
	f := self.Raft.Apply(log, timeout)
	if err := f.Error(); err != nil {
		return nil, 0, err
	}
	return f.Response(), f.Index(), nil
}

// Init sets up the Mesh and Raft instances
func (self *Controller) Init() error {
	raftConfig := self.Config

	if err := os.MkdirAll(raftConfig.DataDir, 0700); err != nil {
		logrus.WithField("dir", raftConfig.DataDir).WithError(err).Error("failed to initialize data directory")
		return err
	}

	hclLogger := NewHcLogrusLogger()

	localAddr := raft.ServerAddress(raftConfig.AdvertiseAddress)
	conf := raft.DefaultConfig()
	conf.SnapshotThreshold = 10
	conf.LocalID = raft.ServerID(self.Id.Token)
	conf.NoSnapshotRestoreOnStart = false
	conf.Logger = hclLogger

	// Create the log store and stable store.
	raftBoltFile := path.Join(raftConfig.DataDir, "raft.db")
	boltDbStore, err := raftboltdb.NewBoltStore(raftBoltFile)
	if err != nil {
		logrus.WithError(err).Error("failed to initialize raft bolt storage")
		return err
	}

	snapshotsDir := raftConfig.DataDir
	snapshotStore, err := raft.NewFileSnapshotStoreWithLogger(snapshotsDir, 5, hclLogger)
	if err != nil {
		logrus.WithField("snapshotDir", snapshotsDir).WithError(err).Errorf("failed to initialize raft snapshot store in: '%v'", snapshotsDir)
		return err
	}

	bindHandler := func(binding channel.Binding) error {
		binding.AddTypedReceiveHandler(NewCommandHandler(self))
		binding.AddTypedReceiveHandler(NewJoinHandler(self))
		binding.AddTypedReceiveHandler(NewRemoveHandler(self))
		return nil
	}

	self.Mesh = mesh.New(self.Id, self.version, conf.LocalID, localAddr, channel.BindHandlerF(bindHandler))
	self.Fsm = NewFsm(raftConfig.DataDir, command.GetDefaultDecoders(), self.indexTracker)

	if err = self.Fsm.Init(); err != nil {
		return errors.Wrap(err, "failed to init FSM")
	}

	transport := raft.NewNetworkTransportWithLogger(self.Mesh, 3, 10*time.Second, hclLogger)

	if raftConfig.Recover {
		err := raft.RecoverCluster(conf, self.Fsm, boltDbStore, boltDbStore, snapshotStore, transport, raft.Configuration{
			Servers: []raft.Server{
				{ID: conf.LocalID, Address: localAddr},
			},
		})
		if err != nil {
			return errors.Wrap(err, "failed to recover cluster")
		}

		logrus.Info("raft configuration reset to only include local node. exiting.")
		os.Exit(0)
	}

	r, err := raft.NewRaft(conf, self.Fsm, boltDbStore, boltDbStore, snapshotStore, transport)
	if err != nil {
		return errors.Wrap(err, "failed to initialise raft")
	}
	self.Fsm.initialized.Store(true)
	self.Raft = r

	return nil
}

func (self *Controller) Bootstrap() error {
	if self.Raft.LastIndex() > 0 {
		logrus.Info("raft already bootstrapped")
		self.bootstrapped.Store(true)
	} else {
		logrus.Infof("waiting for cluster size: %v", self.Config.MinClusterSize)
		req := &JoinRequest{
			Addr:    string(self.Mesh.GetAdvertiseAddr()),
			Id:      self.Id.Token,
			IsVoter: true,
		}
		if err := self.Join(req); err != nil {
			return err
		}
	}
	return nil
}

// Join adds the given node to the raft cluster
func (self *Controller) Join(req *JoinRequest) error {
	log := pfxlog.Logger()
	self.clusterLock.Lock()
	defer self.clusterLock.Unlock()

	if req.Id == "" {
		return errors.Errorf("invalid server id '%v'", req.Id)
	}

	if req.Addr == "" {
		return errors.Errorf("invalid server addr '%v' for servier %v", req.Addr, req.Id)
	}

	if self.bootstrapped.Load() || self.GetRaft().LastIndex() > 0 {
		return self.HandleJoin(req)
	}

	suffrage := raft.Voter
	if !req.IsVoter {
		suffrage = raft.Nonvoter
	}

	self.servers = append(self.servers, raft.Server{
		ID:       raft.ServerID(req.Id),
		Address:  raft.ServerAddress(req.Addr),
		Suffrage: suffrage,
	})

	bootstrapMembers := map[string]struct{}{}
	for _, bootstrapMember := range self.Config.BootstrapMembers {
		_, err := transport.ParseAddress(bootstrapMember)
		if err != nil {
			panic(errors.Wrapf(err, "unable to parse address for bootstrap member [%v]", bootstrapMember))
		}
		bootstrapMembers[bootstrapMember] = struct{}{}
	}

	for len(bootstrapMembers) > 0 {
		hasErrs := false
		for bootstrapMember := range bootstrapMembers {
			if id, err := self.Mesh.GetPeerId(bootstrapMember, time.Second*5); err != nil {
				pfxlog.Logger().WithError(err).Errorf("unable to get id for bootstrap member [%v]", bootstrapMember)
				hasErrs = true
			} else {
				self.servers = append(self.servers, raft.Server{
					Suffrage: raft.Voter,
					ID:       raft.ServerID(id),
					Address:  raft.ServerAddress(bootstrapMember),
				})
				delete(bootstrapMembers, bootstrapMember)
			}
		}
		if hasErrs {
			time.Sleep(time.Second * 5)
		}
	}

	votingCount := uint32(0)
	for _, server := range self.servers {
		if server.Suffrage == raft.Voter {
			votingCount++
		}
	}

	if votingCount >= self.Config.MinClusterSize {
		log.Infof("min cluster member count met, bootstrapping cluster")
		f := self.GetRaft().BootstrapCluster(raft.Configuration{Servers: self.servers})
		if err := f.Error(); err != nil {
			return errors.Wrapf(err, "failed to bootstrap cluster")
		}
		self.bootstrapped.Store(true)
		log.Info("raft cluster bootstrap complete")
		if err := self.migrationMgr.TryInitializeRaftFromBoltDb(); err != nil {
			panic(err)
		}
	}

	return nil
}

// RemoveServer removes the node specified by the given id from the raft cluster
func (self *Controller) RemoveServer(id string) error {
	req := &RemoveRequest{
		Id: id,
	}

	return self.HandleRemove(req)
}

type MigrationManager interface {
	TryInitializeRaftFromBoltDb() error
	InitializeRaftFromBoltDb(srcDb string) error
}
