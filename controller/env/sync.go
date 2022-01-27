/*
	Copyright NetFoundry, Inc.

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

package env

import (
	"github.com/openziti/channel"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/common"
	"sync"
)

// Aliased type for router strategies
type RouterSyncStrategyType string

// Aliased type for router sync status
type RouterSyncStatus string

const (
	RouterSyncNew        RouterSyncStatus = "SYNC_NEW"         //connection accepted but no strategy actions have been taken
	RouterSyncQueued     RouterSyncStatus = "SYNC_QUEUED"      //connection handed to strategy, but not processed
	RouterSyncHello      RouterSyncStatus = "SYNC_HELLO"       //connection is beginning hello cycle
	RouterSyncHelloWait  RouterSyncStatus = "SYNC_HELLO_WAIT"  //hello received from router, but there are too many synchronizing routers
	RouterSyncResyncWait RouterSyncStatus = "SYNC_RESYNC_WAIT" //router requested a resync, in queue
	RouterSynInProgress  RouterSyncStatus = "SYNC_IN_PROGRESS" //hello finished, starting to send state
	RouterSyncDone       RouterSyncStatus = "SYNC_DONE"        //initial state sent

	//Error states
	RouterSyncUnknown      RouterSyncStatus = "SYNC_UNKNOWN"       //the router is currently unknown
	RouterSyncDisconnected RouterSyncStatus = "SYNC_DISCONNECTED"  //strategy was disconnected before finishing
	RouterSyncHelloTimeout RouterSyncStatus = "SYNC_HELLO_TIMEOUT" //sync failed due to a hello timeout.
	RouterSyncError        RouterSyncStatus = "SYNC_ERROR"         //sync failed due to an unexpected error

	//msg headers
	SyncStrategyTypeHeader  = 1013
	SyncStrategyStateHeader = 1014
)

// RouterSyncStrategy handles the life cycle of an Edge Router connecting to the controller, synchronizing
// any upfront state and then maintaining state after that.
type RouterSyncStrategy interface {
	Type() RouterSyncStrategyType
	GetEdgeRouterState(id string) RouterStateValues
	Stop()
	RouterConnectionHandler
	RouterSynchronizerEventHandler
}

// RouterConnectionHandler is responsible for handling router connect/disconnect for synchronizing state.
// This is intended for API Session but additional state is possible. Implementations may bind additional
// handlers to the channel.
type RouterConnectionHandler interface {
	RouterConnected(edgeRouter *model.EdgeRouter, router *network.Router)
	RouterDisconnected(router *network.Router)
	GetReceiveHandlers() []channel.TypedReceiveHandler
}

// RouterSynchronizerEventHandler is responsible for keeping Edge Routers up to date on API Sessions
type RouterSynchronizerEventHandler interface {
	ApiSessionAdded(apiSession *persistence.ApiSession)
	ApiSessionUpdated(apiSession *persistence.ApiSession, apiSessionCert *persistence.ApiSessionCertificate)
	ApiSessionDeleted(apiSession *persistence.ApiSession)

	SessionDeleted(session *persistence.Session)
}

// RouterState provides a thread save mechanism to access and set router status information that may be influx
// due to reouter connection/disconnection.
type RouterState interface {
	SetIsOnline(isOnline bool)
	IsOnline() bool

	SetHostname(hostname string)
	Hostname() string

	SetProtocols(protocols map[string]string)
	Protocols() map[string]string

	SetSyncStatus(status RouterSyncStatus)
	SyncStatus() RouterSyncStatus

	SetVersionInfo(versionInfo common.VersionInfo)
	GetVersionInfo() common.VersionInfo

	Values() RouterStateValues
}

var _ RouterState = &LockingRouterState{}

type RouterStateValues struct {
	IsOnline    bool
	Hostname    string
	Protocols   map[string]string
	SyncStatus  RouterSyncStatus
	VersionInfo common.VersionInfo
}

func NewRouterStatusValues() RouterStateValues {
	return RouterStateValues{
		IsOnline:   false,
		Hostname:   "",
		Protocols:  map[string]string{},
		SyncStatus: RouterSyncUnknown,
		VersionInfo: common.VersionInfo{
			Version:   "",
			Revision:  "",
			BuildDate: "",
			OS:        "",
			Arch:      "",
		},
	}
}

type LockingRouterState struct {
	internal RouterStateValues
	lock     sync.Mutex
}

func NewLockingRouterStatus() *LockingRouterState {
	return &LockingRouterState{
		internal: NewRouterStatusValues(),
		lock:     sync.Mutex{},
	}
}

func (r *LockingRouterState) Values() RouterStateValues {
	r.lock.Lock()
	defer r.lock.Unlock()

	ret := r.internal

	ret.Protocols = map[string]string{}

	for k, v := range r.internal.Protocols {
		ret.Protocols[k] = v
	}

	return ret
}

func (r *LockingRouterState) SetIsOnline(isOnline bool) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.internal.IsOnline = isOnline
}

func (r *LockingRouterState) IsOnline() bool {
	r.lock.Lock()
	defer r.lock.Unlock()

	return r.internal.IsOnline
}

func (r *LockingRouterState) SetHostname(hostname string) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.internal.Hostname = hostname
}

func (r *LockingRouterState) Hostname() string {
	r.lock.Lock()
	defer r.lock.Unlock()

	return r.internal.Hostname
}

func (r *LockingRouterState) SetProtocols(protocols map[string]string) {
	r.lock.Lock()
	defer r.lock.Unlock()

	newProtocols := map[string]string{}

	for k, v := range protocols {
		newProtocols[k] = v
	}

	r.internal.Protocols = newProtocols
}

func (r *LockingRouterState) Protocols() map[string]string {
	r.lock.Lock()
	defer r.lock.Unlock()

	//to return empty, not nil
	copy := map[string]string{}

	for k, v := range r.internal.Protocols {
		copy[k] = v
	}

	return copy
}

func (r *LockingRouterState) SetSyncStatus(syncStatus RouterSyncStatus) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.internal.SyncStatus = syncStatus
}

func (r *LockingRouterState) SyncStatus() RouterSyncStatus {
	r.lock.Lock()
	defer r.lock.Unlock()

	return r.internal.SyncStatus
}

func (r *LockingRouterState) SetVersionInfo(versionInfo common.VersionInfo) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.internal.VersionInfo = versionInfo
}

func (r *LockingRouterState) GetVersionInfo() common.VersionInfo {
	r.lock.Lock()
	defer r.lock.Unlock()

	return r.internal.VersionInfo
}
