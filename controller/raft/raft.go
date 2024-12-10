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
	"crypto/x509"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"github.com/michaelquigley/pfxlog"
	"github.com/mitchellh/mapstructure"
	"github.com/openziti/channel/v3"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/rate"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/common/pb/cmd_pb"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/config"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/event"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/peermsg"
	"github.com/openziti/ziti/controller/raft/mesh"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"os"
	"path"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

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
	GetCommandRateLimiterConfig() command.RateLimiterConfig
	GetRaftConfig() *config.RaftConfig
	GetMetricsRegistry() metrics.Registry
	GetEventDispatcher() event.Dispatcher
	GetCloseNotify() <-chan struct{}
	GetHelloHeaderProviders() []mesh.HeaderProvider
}

func NewController(env Env, migrationMgr MigrationManager) *Controller {
	result := &Controller{
		env:                env,
		Config:             env.GetRaftConfig(),
		indexTracker:       NewIndexTracker(),
		migrationMgr:       migrationMgr,
		clusterEvents:      make(chan raft.Observation, 16),
		commandRateLimiter: command.NewRateLimiter(env.GetCommandRateLimiterConfig(), env.GetMetricsRegistry(), env.GetCloseNotify()),
		errorMappers:       map[string]func(map[string]any) error{},
	}
	result.initErrorMappers()
	return result
}

// Controller manages RAFT related state and operations
type Controller struct {
	clusterId                  concurrenz.AtomicValue[string]
	env                        Env
	Config                     *config.RaftConfig
	Mesh                       mesh.Mesh
	Raft                       *raft.Raft
	Fsm                        *BoltDbFsm
	bootstrapped               atomic.Bool
	clusterLock                sync.Mutex
	closeNotify                <-chan struct{}
	indexTracker               IndexTracker
	migrationMgr               MigrationManager
	clusterStateChangeHandlers concurrenz.CopyOnWriteSlice[func(event ClusterEvent, state ClusterState)]
	isLeader                   atomic.Bool
	clusterEvents              chan raft.Observation
	commandRateLimiter         rate.RateLimiter
	errorMappers               map[string]func(map[string]any) error
}

func (self *Controller) GetNodeId() *identity.TokenId {
	return self.env.GetId()
}

func (self *Controller) GetClusterId() string {
	return self.clusterId.Load()
}

func (self *Controller) GetVersionProvider() versions.VersionProvider {
	return self.env.GetVersionProvider()
}

func (self *Controller) GetEventDispatcher() event.Dispatcher {
	return self.env.GetEventDispatcher()
}

func (self *Controller) GetListenerHeaders() map[int32][]byte {
	return map[int32][]byte{
		mesh.ClusterIdHeader: []byte(self.clusterId.Load()),
		mesh.PeerAddrHeader:  []byte(self.Config.AdvertiseAddress.String()),
	}
}

func (self *Controller) initErrorMappers() {
	self.errorMappers[fmt.Sprintf("%T", &boltz.RecordNotFoundError{})] = self.parseBoltzNotFoundError
	self.errorMappers[fmt.Sprintf("%T", &errorz.FieldError{})] = self.parseFieldError
}

func (self *Controller) RegisterClusterEventHandler(f func(event ClusterEvent, state ClusterState)) {
	if self.isLeader.Load() {
		f(ClusterEventLeadershipGained, newClusterState(true, !self.Mesh.IsReadOnly()))
	}
	self.clusterStateChangeHandlers.Append(f)
}

func (self *Controller) InitEnv(env model.Env) error {
	model.RegisterCommand(env, &InitClusterIdCmd{}, &cmd_pb.InitClusterIdCommand{})
	clusterId, err := db.LoadClusterId(env.GetDb())
	if err != nil {
		return err
	}
	self.clusterId.Store(clusterId)
	return nil
}

// GetRaft returns the managed raft instance
func (self *Controller) GetRaft() *raft.Raft {
	return self.Raft
}

// GetMesh returns the related Mesh instance
func (self *Controller) GetMesh() mesh.Mesh {
	return self.Mesh
}

func (self *Controller) GetRateLimiter() rate.RateLimiter {
	return self.commandRateLimiter
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

func (self *Controller) IsLeaderless() bool {
	return self.GetLeaderAddr() == ""
}

func (self *Controller) IsBootstrapped() bool {
	return self.bootstrapped.Load() || self.GetRaft().LastIndex() > 0
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

	if self.GetLeaderAddr() == "" {
		return apierror.NewClusterHasNoLeaderError()
	}

	log.WithField("cmd", reflect.TypeOf(cmd)).WithField("dest", self.GetLeaderAddr()).Debug("forwarding command")

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

	if cause, ok := m["cause"]; ok && cause != nil {
		if strCause, ok := cause.(string); ok {
			apiErr.Cause = errors.New(strCause)
		} else if objCause, ok := cause.(map[string]any); ok {
			if parser := self.getErrorParser(m); parser != nil {
				pfxlog.Logger().Info("parser found for cause type")
				apiErr.Cause = parser(objCause)
			} else {
				pfxlog.Logger().Info("no parser found for cause type")
			}

			if apiErr.Cause == nil {
				apiErr.Cause = self.fallbackMarshallError(objCause)
			}
		} else {
			pfxlog.Logger().Warnf("invalid api error encoding, no cause: %v", string(data))
			return errors.New(string(data))
		}
	}

	return apiErr
}

func (self *Controller) parseFieldError(m map[string]any) error {
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

	if reason, ok := m["message"]; ok {
		if reasonStr, ok := reason.(string); ok {
			fieldError.Reason = reasonStr
		}
	} else if reason, ok := m["reason"]; ok {
		if reasonStr, ok := reason.(string); ok {
			fieldError.Reason = reasonStr
		}
	}

	return fieldError
}

func (self *Controller) parseBoltzNotFoundError(m map[string]any) error {
	result := &boltz.RecordNotFoundError{}
	err := mapstructure.Decode(m, result)
	if err != nil {
		multi := errorz.MultipleErrors{}
		multi = append(multi, fmt.Errorf("unable to decode RecordNotFoundError (%w)", err))
		multi = append(multi, self.fallbackMarshallError(m))
		return multi
	}
	return result
}

func (self *Controller) fallbackMarshallError(m map[string]any) error {
	if b, err := json.Marshal(m); err == nil {
		return errors.New(string(b))
	}
	return errors.New(fmt.Sprintf("%+v", m))
}

func (self *Controller) getErrorParser(m map[string]any) func(map[string]any) error {
	causeType, ok := m["causeType"]
	if !ok {
		pfxlog.Logger().Info("no causetype defined for error parser")
		return nil
	}

	causeTypeStr, ok := causeType.(string)
	if !ok {
		pfxlog.Logger().Info("causetype not string")
		return nil
	}

	pfxlog.Logger().Infof("causetype %s", causeTypeStr)

	return self.errorMappers[causeTypeStr]
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
	returnValue := atomic.Value{}
	index := atomic.Uint64{}
	err := self.commandRateLimiter.RunRateLimited(func() error {
		f := self.Raft.Apply(log, timeout)
		if err := f.Error(); err != nil {
			return err
		}

		if response := f.Response(); response != nil {
			returnValue.Store(response)
		}
		index.Store(f.Index())
		return nil
	})

	if err != nil {
		if errors.Is(err, raft.ErrNotLeader) {
			noLeaderErr := apierror.NewClusterHasNoLeaderError()
			noLeaderErr.Cause = err
			err = noLeaderErr
		}

		return nil, 0, err
	}

	return returnValue.Load(), index.Load(), nil
}

// Init sets up the Mesh and Raft instances
func (self *Controller) Init() error {
	self.validateCert()

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
	self.Configure(raftConfig, conf)

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

	helloHeaderProviders := self.env.GetHelloHeaderProviders()

	self.Mesh = mesh.New(self, localAddr, helloHeaderProviders)
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
	self.ConfigureReloadable(raftConfig, &rc)
	if err = r.ReloadConfig(rc); err != nil {
		return errors.Wrap(err, "error reloading raft configuration")
	}

	self.Raft = r
	self.addEventsHandlers()
	self.ObserveLeaderChanges()

	return nil
}

func (self *Controller) Configure(ctrlConfig *config.RaftConfig, conf *raft.Config) {
	if ctrlConfig.SnapshotThreshold != nil {
		conf.SnapshotThreshold = uint64(*ctrlConfig.SnapshotThreshold)
	}

	if ctrlConfig.SnapshotInterval != nil {
		conf.SnapshotInterval = *ctrlConfig.SnapshotInterval
	}

	if ctrlConfig.TrailingLogs != nil {
		conf.TrailingLogs = uint64(*ctrlConfig.TrailingLogs)
	}

	if ctrlConfig.MaxAppendEntries != nil {
		conf.MaxAppendEntries = int(*ctrlConfig.MaxAppendEntries)
	}

	if ctrlConfig.CommitTimeout != nil {
		conf.CommitTimeout = *ctrlConfig.CommitTimeout
	}

	conf.ElectionTimeout = ctrlConfig.ElectionTimeout
	conf.HeartbeatTimeout = ctrlConfig.HeartbeatTimeout
	conf.LeaderLeaseTimeout = ctrlConfig.LeaderLeaseTimeout

	if ctrlConfig.LogLevel != nil {
		conf.LogLevel = *ctrlConfig.LogLevel
	}

	conf.Logger = ctrlConfig.Logger
}

func (self *Controller) ConfigureReloadable(ctrlConfig *config.RaftConfig, conf *raft.ReloadableConfig) {
	if ctrlConfig.SnapshotThreshold != nil {
		conf.SnapshotThreshold = uint64(*ctrlConfig.SnapshotThreshold)
	}

	if ctrlConfig.SnapshotInterval != nil {
		conf.SnapshotInterval = *ctrlConfig.SnapshotInterval
	}

	if ctrlConfig.TrailingLogs != nil {
		conf.TrailingLogs = uint64(*ctrlConfig.TrailingLogs)
	}

	conf.ElectionTimeout = ctrlConfig.ElectionTimeout
	conf.HeartbeatTimeout = ctrlConfig.HeartbeatTimeout
}

func (self *Controller) validateCert() {
	var certs []*x509.Certificate
	for _, cert := range self.env.GetId().ServerCert() {
		certs = append(certs, cert.Leaf)
	}
	if _, err := mesh.ExtractSpiffeId(certs); err != nil {
		logrus.WithError(err).Fatal("controller cert must have Subject Alternative Name URI of form spiffe://<trust domain>/controller/<controller id>")
	}
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
		if err := self.migrationMgr.ValidateMigrationEnvironment(); err != nil {
			return err
		}

		req := &cmd_pb.AddPeerRequest{
			Addr:    string(self.Mesh.GetAdvertiseAddr()),
			Id:      self.env.GetId().Token,
			IsVoter: true,
		}

		if err := self.Join(req); err != nil {
			return err
		}

		start := time.Now()
		firstCheckPassed := false
		for {
			// make sure this is in a reasonably steady state by waiting a bit longer and checking twice
			if self.isLeader.Load() {
				if firstCheckPassed {
					break
				} else {
					firstCheckPassed = true
				}
			}
			if time.Since(start) > time.Second*10 {
				return fmt.Errorf("node did not bootstrap in time")
			}
			time.Sleep(100 * time.Millisecond)
		}

		go self.addConfiguredInitialMembers()

		self.clusterId.Store(uuid.NewString())
		pfxlog.Logger().WithField("clusterId", self.clusterId.Load()).Info("cluster id initialized")
		return self.Dispatch(&InitClusterIdCmd{
			ClusterId:      self.clusterId.Load(),
			raftController: self,
		})
	}
	return nil
}

func (self *Controller) addConfiguredInitialMembers() {
	for _, bootstrapMember := range self.Config.InitialMembers {
		_, err := transport.ParseAddress(bootstrapMember)
		if err != nil {
			pfxlog.Logger().WithError(err).Errorf("unable to parse address for bootstrap member [%v], it will be ignored", bootstrapMember)
			continue
		}

		go self.retryBootstrapMember(bootstrapMember)
	}
}

func (self *Controller) retryBootstrapMember(bootstrapMember string) {
	ticker := time.NewTicker(6 * time.Second)
	defer ticker.Stop()

	for {
		if id, addr, err := self.Mesh.GetPeerInfo(bootstrapMember, time.Second*5); err != nil {
			pfxlog.Logger().WithError(err).Errorf("unable to get id for bootstrap member [%v], will retry", bootstrapMember)
		} else {
			req := &cmd_pb.AddPeerRequest{
				Addr:    string(addr),
				Id:      string(id),
				IsVoter: true,
			}

			if err = self.Join(req); err == nil {
				pfxlog.Logger().WithError(err).Errorf("error adding bootstrap member [%s], stopping attempts to join it to the cluster", bootstrapMember)
				return
			}
		}
		<-ticker.C
	}
}

// Join adds the given node to the raft cluster
func (self *Controller) Join(req *cmd_pb.AddPeerRequest) error {
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

	return self.tryBootstrap(raft.Server{
		ID:       raft.ServerID(req.Id),
		Address:  raft.ServerAddress(req.Addr),
		Suffrage: suffrage,
	})
}

func (self *Controller) tryBootstrap(servers ...raft.Server) error {
	log := pfxlog.Logger()

	log.Infof("bootstrapping cluster")
	f := self.GetRaft().BootstrapCluster(raft.Configuration{Servers: servers})
	if err := f.Error(); err != nil {
		return errors.Wrapf(err, "failed to bootstrap cluster")
	}
	self.bootstrapped.Store(true)
	log.Info("raft cluster bootstrap complete")

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
			self.newClusterEvent(event.ClusterLeadershipGained, self.Mesh.GetAllPeersForEvent())
		case ClusterEventLeadershipLost:
			self.newClusterEvent(event.ClusterLeadershipLost, nil)
		case ClusterEventReadOnly:
			self.newClusterEvent(event.ClusterStateReadOnly, nil)
		case ClusterEventReadWrite:
			self.newClusterEvent(event.ClusterStateReadWrite, nil)
		default:
			pfxlog.Logger().Errorf("unhandled cluster event type: %v", evt)
		}
	})
}

func (self *Controller) newClusterEvent(eventType event.ClusterEventType, peers []*event.ClusterPeer) {
	evt := event.NewClusterEvent(eventType)
	evt.Peers = peers
	self.env.GetEventDispatcher().AcceptClusterEvent(evt)
}

type MigrationManager interface {
	ValidateMigrationEnvironment() error
	TryInitializeRaftFromBoltDb() error
	InitializeRaftFromBoltDb(srcDb string) error
}

type InitClusterIdCmd struct {
	ClusterId      string `json:"clusterId"`
	raftController *Controller
}

func (self *InitClusterIdCmd) Apply(ctx boltz.MutateContext) error {
	self.raftController.clusterId.Store(self.ClusterId)
	return db.InitClusterId(self.raftController.Fsm.GetDb(), ctx, self.ClusterId)
}

func (self *InitClusterIdCmd) Encode() ([]byte, error) {
	cmd := &cmd_pb.InitClusterIdCommand{
		ClusterId: self.ClusterId,
	}
	return cmd_pb.EncodeProtobuf(cmd)
}

func (self *InitClusterIdCmd) Decode(env model.Env, msg *cmd_pb.InitClusterIdCommand) error {
	self.ClusterId = msg.ClusterId
	self.raftController = env.GetManagers().Dispatcher.(*Controller)
	return nil
}

func (self *InitClusterIdCmd) GetChangeContext() *change.Context {
	return change.New().SetChangeAuthorType(change.AuthorTypeController)
}
