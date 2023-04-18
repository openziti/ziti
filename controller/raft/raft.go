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
	"github.com/hashicorp/go-hclog"
	"github.com/openziti/fabric/controller/peermsg"
	"github.com/openziti/fabric/event"
	"github.com/openziti/fabric/pb/cmd_pb"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/versions"
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
	AdvertiseAddress      transport.Address
	BootstrapMembers      []string
	CommandHandlerOptions struct {
		MaxQueueSize uint16
		MaxWorkers   uint16
	}

	SnapshotInterval  *time.Duration
	SnapshotThreshold *uint32
	TrailingLogs      *uint32
	MaxAppendEntries  *uint32

	HeartbeatTimeout   *time.Duration
	ElectionTimeout    *time.Duration
	LeaderLeaseTimeout *time.Duration

	LogLevel *string
	Logger   hclog.Logger
}

func (self *Config) Configure(conf *raft.Config) {
	if self.SnapshotThreshold != nil {
		conf.SnapshotThreshold = uint64(*self.SnapshotThreshold)
	}

	if self.SnapshotInterval != nil {
		conf.SnapshotInterval = *self.SnapshotInterval
	}

	if self.TrailingLogs != nil {
		conf.TrailingLogs = uint64(*self.TrailingLogs)
	}

	if self.MaxAppendEntries != nil {
		conf.MaxAppendEntries = int(*self.MaxAppendEntries)
	}

	if self.ElectionTimeout != nil {
		conf.ElectionTimeout = *self.ElectionTimeout
	}

	if self.HeartbeatTimeout != nil {
		conf.HeartbeatTimeout = *self.HeartbeatTimeout
	}

	if self.LeaderLeaseTimeout != nil {
		conf.LeaderLeaseTimeout = *self.LeaderLeaseTimeout
	}

	if self.LogLevel != nil {
		conf.LogLevel = *self.LogLevel
	}

	conf.Logger = self.Logger
}

func (self *Config) ConfigureReloadable(conf *raft.ReloadableConfig) {
	if self.SnapshotThreshold != nil {
		conf.SnapshotThreshold = uint64(*self.SnapshotThreshold)
	}

	if self.SnapshotInterval != nil {
		conf.SnapshotInterval = *self.SnapshotInterval
	}

	if self.TrailingLogs != nil {
		conf.TrailingLogs = uint64(*self.TrailingLogs)
	}

	if self.ElectionTimeout != nil {
		conf.ElectionTimeout = *self.ElectionTimeout
	}

	if self.HeartbeatTimeout != nil {
		conf.HeartbeatTimeout = *self.HeartbeatTimeout
	}
}

type RouterDispatchCallback func(*raft.Configuration) error

type ClusterEvent uint32

func (self ClusterEvent) String() string {
	switch self {
	case ClusterEventReadOnly:
		return "ClusterEventReadOnly"
	case ClusterEventReadWrite:
		return "ClusterEventReadWrite"
	case ClusterEventLeadershipGained:
		return "ClusterEventLeadershipGained"
	case ClusterEventLeadershipLost:
		return "ClusterEventLeadershipLost"
	default:
		return fmt.Sprintf("UnhandledClusterEventType[%v]", uint32(self))
	}
}

const (
	ClusterEventReadOnly         ClusterEvent = 0
	ClusterEventReadWrite        ClusterEvent = 1
	ClusterEventLeadershipGained ClusterEvent = 2
	ClusterEventLeadershipLost   ClusterEvent = 3

	isLeaderMask    = 0b01
	isReadWriteMask = 0b10
)

type ClusterState uint8

func (c ClusterState) IsLeader() bool {
	return uint8(c)&isLeaderMask == isLeaderMask
}

func (c ClusterState) IsReadWrite() bool {
	return uint8(c)&isReadWriteMask == isReadWriteMask
}

func (c ClusterState) String() string {
	return fmt.Sprintf("ClusterState[isLeader=%v, isReadWrite=%v]", c.IsLeader(), c.IsReadWrite())
}

func newClusterState(isLeader, isReadWrite bool) ClusterState {
	var val uint8
	if isLeader {
		val = val | isLeaderMask
	}
	if isReadWrite {
		val = val | isReadWriteMask
	}
	return ClusterState(val)
}

type Env interface {
	GetId() *identity.TokenId
	GetVersionProvider() versions.VersionProvider
	GetRaftConfig() *Config
	GetMetricsRegistry() metrics.Registry
	GetEventDispatcher() event.Dispatcher
}

func NewController(env Env, migrationMgr MigrationManager) *Controller {
	result := &Controller{
		env:           env,
		Config:        env.GetRaftConfig(),
		indexTracker:  NewIndexTracker(),
		migrationMgr:  migrationMgr,
		clusterEvents: make(chan raft.Observation, 16),
	}
	return result
}

// Controller manages RAFT related state and operations
type Controller struct {
	env                        Env
	Config                     *Config
	Mesh                       mesh.Mesh
	Raft                       *raft.Raft
	Fsm                        *BoltDbFsm
	bootstrapped               atomic.Bool
	clusterLock                sync.Mutex
	servers                    []raft.Server
	closeNotify                <-chan struct{}
	indexTracker               IndexTracker
	migrationMgr               MigrationManager
	clusterStateChangeHandlers concurrenz.CopyOnWriteSlice[func(event ClusterEvent, state ClusterState)]
	isLeader                   atomic.Bool
	clusterEvents              chan raft.Observation
}

func (self *Controller) RegisterClusterEventHandler(f func(event ClusterEvent, state ClusterState)) {
	if self.isLeader.Load() {
		f(ClusterEventLeadershipGained, newClusterState(true, !self.Mesh.IsReadOnly()))
	}
	self.clusterStateChangeHandlers.Append(f)
}

// GetRaft returns the managed raft instance
func (self *Controller) GetRaft() *raft.Raft {
	return self.Raft
}

// GetMesh returns the related Mesh instance
func (self *Controller) GetMesh() mesh.Mesh {
	return self.Mesh
}

func (self *Controller) ConfigureMeshHandlers(bindHandler channel.BindHandler) {
	self.Mesh.Init(bindHandler)
}

// GetDb returns the DB instance
func (self *Controller) GetDb() boltz.Db {
	return self.Fsm.GetDb()
}

// IsLeader returns true if the current node is the RAFT leader
func (self *Controller) IsLeader() bool {
	return self.Raft.State() == raft.Leader
}

func (self *Controller) IsLeaderOrLeaderless() bool {
	return self.IsLeader() || self.GetLeaderAddr() == ""
}

func (self *Controller) IsReadOnlyMode() bool {
	return self.Mesh.IsReadOnly()
}

func (self *Controller) IsDistributed() bool {
	return true
}

// GetLeaderAddr returns the current leader address, which may be blank if there is no leader currently
func (self *Controller) GetLeaderAddr() string {
	addr, _ := self.Raft.LeaderWithID()
	return string(addr)
}

func (self *Controller) GetPeers() map[string]channel.Channel {
	result := map[string]channel.Channel{}
	for k, v := range self.Mesh.GetPeers() {
		result[k] = v.Channel
	}
	return result
}

func (self *Controller) GetCloseNotify() <-chan struct{} {
	return self.closeNotify
}

func (self *Controller) GetMetricsRegistry() metrics.Registry {
	return self.env.GetMetricsRegistry()
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

	msg := channel.NewMessage(int32(cmd_pb.ContentType_NewLogEntryType), encoded)
	result, err := msg.WithTimeout(5 * time.Second).SendForReply(peer.Channel)
	if err != nil {
		return err
	}

	if result.ContentType == int32(cmd_pb.ContentType_SuccessResponseType) {
		idx, found := result.GetUint64Header(int32(peermsg.HeaderIndex))
		if found {
			if err = self.indexTracker.WaitForIndex(idx, time.Now().Add(5*time.Second)); err != nil {
				return err
			}
		}
		return nil
	}

	if result.ContentType == int32(cmd_pb.ContentType_ErrorResponseType) {
		errCode, found := result.GetUint32Header(peermsg.HeaderErrorCode)
		if found && errCode == peermsg.ErrorCodeApiError {
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

	if raftConfig.Logger == nil {
		raftConfig.Logger = NewHcLogrusLogger()
	}

	if err := os.MkdirAll(raftConfig.DataDir, 0700); err != nil {
		logrus.WithField("dir", raftConfig.DataDir).WithError(err).Error("failed to initialize data directory")
		return err
	}

	localAddr := raft.ServerAddress(raftConfig.AdvertiseAddress.String())
	conf := raft.DefaultConfig()
	conf.LocalID = raft.ServerID(self.env.GetId().Token)
	conf.NoSnapshotRestoreOnStart = true
	raftConfig.Configure(conf)

	// Create the log store and stable store.
	raftBoltFile := path.Join(raftConfig.DataDir, "raft.db")
	boltDbStore, err := raftboltdb.NewBoltStore(raftBoltFile)
	if err != nil {
		logrus.WithError(err).Error("failed to initialize raft bolt storage")
		return err
	}

	snapshotsDir := raftConfig.DataDir
	snapshotStore, err := raft.NewFileSnapshotStoreWithLogger(snapshotsDir, 5, raftConfig.Logger)
	if err != nil {
		logrus.WithField("snapshotDir", snapshotsDir).WithError(err).Errorf("failed to initialize raft snapshot store in: '%v'", snapshotsDir)
		return err
	}

	self.Mesh = mesh.New(self.env, localAddr)
	self.Mesh.RegisterClusterStateHandler(func(state mesh.ClusterState) {
		obs := raft.Observation{
			Raft: self.Raft,
			Data: state,
		}
		self.clusterEvents <- obs
	})

	self.Fsm = NewFsm(raftConfig.DataDir, command.GetDefaultDecoders(), self.indexTracker, self.env.GetEventDispatcher())

	if err = self.Fsm.Init(); err != nil {
		return errors.Wrap(err, "failed to init FSM")
	}

	raftTransport := raft.NewNetworkTransportWithLogger(self.Mesh, 3, 10*time.Second, raftConfig.Logger)

	if raftConfig.Recover {
		err := raft.RecoverCluster(conf, self.Fsm, boltDbStore, boltDbStore, snapshotStore, raftTransport, raft.Configuration{
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

	r, err := raft.NewRaft(conf, self.Fsm, boltDbStore, boltDbStore, snapshotStore, raftTransport)
	if err != nil {
		return errors.Wrap(err, "failed to initialise raft")
	}

	rc := r.ReloadableConfig()
	raftConfig.ConfigureReloadable(&rc)
	if err = r.ReloadConfig(rc); err != nil {
		return errors.Wrap(err, "error reloading raft configuration")
	}

	self.Raft = r
	self.addEventsHandlers()
	self.ObserveLeaderChanges()

	return nil
}

func (self *Controller) ObserveLeaderChanges() {
	self.Raft.RegisterObserver(raft.NewObserver(self.clusterEvents, true, func(o *raft.Observation) bool {
		_, ok := o.Data.(raft.RaftState)
		return ok
	}))

	go func() {
		if self.Raft.State() == raft.Leader {
			self.isLeader.Store(true)
			self.handleClusterStateChange(ClusterEventLeadershipGained, newClusterState(true, true))
		}

		isReadWrite := true

		for observation := range self.clusterEvents {
			pfxlog.Logger().Tracef("raft observation received: isLeader: %v, isReadWrite: %v", self.isLeader.Load(), isReadWrite)
			if raftState, ok := observation.Data.(raft.RaftState); ok {
				if raftState == raft.Leader && !self.isLeader.Load() {
					self.isLeader.Store(true)
					self.handleClusterStateChange(ClusterEventLeadershipGained, newClusterState(true, isReadWrite))
				} else if raftState != raft.Leader && self.isLeader.Load() {
					self.isLeader.Store(false)
					self.handleClusterStateChange(ClusterEventLeadershipLost, newClusterState(false, isReadWrite))
				}
			} else if state, ok := observation.Data.(mesh.ClusterState); ok {
				if state == mesh.ClusterReadWrite {
					isReadWrite = true
					self.handleClusterStateChange(ClusterEventReadWrite, newClusterState(self.isLeader.Load(), isReadWrite))
				} else if state == mesh.ClusterReadOnly {
					isReadWrite = false
					self.handleClusterStateChange(ClusterEventReadOnly, newClusterState(self.isLeader.Load(), isReadWrite))
				}
			}

			pfxlog.Logger().Tracef("raft observation processed: isLeader: %v, isReadWrite: %v", self.isLeader.Load(), isReadWrite)
		}
	}()
}

func (self *Controller) handleClusterStateChange(event ClusterEvent, state ClusterState) {
	for _, handler := range self.clusterStateChangeHandlers.Value() {
		handler(event, state)
	}
}

func (self *Controller) Bootstrap() error {
	if self.Raft.LastIndex() > 0 {
		logrus.Info("raft already bootstrapped")
		self.bootstrapped.Store(true)
	} else {
		logrus.Infof("waiting for cluster size: %v", self.Config.MinClusterSize)
		req := &cmd_pb.AddPeerRequest{
			Addr:    string(self.Mesh.GetAdvertiseAddr()),
			Id:      self.env.GetId().Token,
			IsVoter: true,
		}
		if err := self.Join(req); err != nil {
			return err
		}
	}
	return nil
}

// Join adds the given node to the raft cluster
func (self *Controller) Join(req *cmd_pb.AddPeerRequest) error {
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
		return self.HandleAddPeer(req)
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
			if id, addr, err := self.Mesh.GetPeerInfo(bootstrapMember, time.Second*5); err != nil {
				pfxlog.Logger().WithError(err).Errorf("unable to get id for bootstrap member [%v]", bootstrapMember)
				hasErrs = true
			} else {
				self.servers = append(self.servers, raft.Server{
					Suffrage: raft.Voter,
					ID:       id,
					Address:  addr,
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
	req := &cmd_pb.RemovePeerRequest{
		Id: id,
	}

	return self.HandleRemovePeer(req)
}

func (self *Controller) CtrlAddresses() (uint64, []string) {
	ret := make([]string, 0)
	index, cfg := self.Fsm.GetCurrentState(self.Raft)
	for _, srvr := range cfg.Servers {
		ret = append(ret, string(srvr.Address))
	}
	return index, ret
}

func (self *Controller) RenderJsonConfig() (string, error) {
	cfg := self.Raft.ReloadableConfig()
	b, err := json.Marshal(cfg)
	return string(b), err
}

func (self *Controller) addEventsHandlers() {
	self.RegisterClusterEventHandler(func(evt ClusterEvent, state ClusterState) {
		switch evt {
		case ClusterEventLeadershipGained:
			self.env.GetEventDispatcher().AcceptClusterEvent(event.NewClusterEvent(event.ClusterLeadershipGained))
		case ClusterEventLeadershipLost:
			self.env.GetEventDispatcher().AcceptClusterEvent(event.NewClusterEvent(event.ClusterLeadershipLost))
		case ClusterEventReadOnly:
			self.env.GetEventDispatcher().AcceptClusterEvent(event.NewClusterEvent(event.ClusterStateReadOnly))
		case ClusterEventReadWrite:
			self.env.GetEventDispatcher().AcceptClusterEvent(event.NewClusterEvent(event.ClusterStateReadWrite))
		default:
			pfxlog.Logger().Errorf("unhandled cluster event type: %v", evt)
		}
	})
}

type MigrationManager interface {
	TryInitializeRaftFromBoltDb() error
	InitializeRaftFromBoltDb(srcDb string) error
}
