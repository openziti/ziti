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

package env

import (
	"crypto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/event"
	"github.com/openziti/ziti/controller/model"
	"go.etcd.io/bbolt"
)

const (
	SessionRemovedType = int32(edge_ctrl_pb.ContentType_SessionRemovedType)

	ApiSessionHeartbeatType = int32(edge_ctrl_pb.ContentType_ApiSessionHeartbeatType)
	ApiSessionRemovedType   = int32(edge_ctrl_pb.ContentType_ApiSessionRemovedType)
	ApiSessionAddedType     = int32(edge_ctrl_pb.ContentType_ApiSessionAddedType)
	ApiSessionUpdatedType   = int32(edge_ctrl_pb.ContentType_ApiSessionUpdatedType)
	RequestClientReSyncType = int32(edge_ctrl_pb.ContentType_RequestClientReSyncType)
	DataStateType           = int32(edge_ctrl_pb.ContentType_DataStateType)
	ValidateDataStateType   = int32(edge_ctrl_pb.ContentType_ValidateDataStateRequestType)
	DataStateChangeSetType  = int32(edge_ctrl_pb.ContentType_DataStateChangeSetType)

	ServerHelloType = int32(edge_ctrl_pb.ContentType_ServerHelloType)
	ClientHelloType = int32(edge_ctrl_pb.ContentType_ClientHelloType)

	EnrollmentCertsResponseType             = int32(edge_ctrl_pb.ContentType_EnrollmentCertsResponseType)
	EnrollmentExtendRouterRequestType       = int32(edge_ctrl_pb.ContentType_EnrollmentExtendRouterRequestType)
	EnrollmentExtendRouterVerifyRequestType = int32(edge_ctrl_pb.ContentType_EnrollmentExtendRouterVerifyRequestType)
)

type RouterSyncCache struct {
}

// The Broker delegates Ziti Edge events to a RouterSyncStrategy. Handling the details of which events to watch
// and dealing with casting arguments to their proper concrete types.
type Broker struct {
	ae                  *AppEnv
	sessionChunkSize    int
	apiSessionChunkSize int
	routerMsgBufferSize int
	routerSyncStrategy  RouterSyncStrategy
}

func NewBroker(ae *AppEnv, synchronizer RouterSyncStrategy) *Broker {
	broker := &Broker{
		ae:                  ae,
		routerSyncStrategy:  synchronizer,
		sessionChunkSize:    100,
		apiSessionChunkSize: 100,
		routerMsgBufferSize: 100,
	}

	broker.ae.GetStores().Session.AddEntityEventListenerF(broker.routerSyncStrategy.SessionDeleted, boltz.EntityDeletedAsync)
	broker.ae.GetStores().ApiSession.AddEntityEventListenerF(broker.routerSyncStrategy.ApiSessionDeleted, boltz.EntityDeletedAsync)
	broker.ae.GetStores().ApiSession.GetEventsEmitter().AddListener(db.EventFullyAuthenticated, broker.apiSessionFullyAuthenticated)
	broker.ae.GetStores().ApiSessionCertificate.AddEntityEventListenerF(broker.apiSessionCertificateCreated, boltz.EntityCreatedAsync)
	broker.ae.GetStores().ApiSessionCertificate.AddEntityEventListenerF(broker.apiSessionCertificateDeleted, boltz.EntityDeletedAsync)

	ae.HostController.GetNetwork().AddRouterPresenceHandler(broker)

	//updates controller store on leader, store update trigger strategy in all controllers
	ae.HostController.GetEventDispatcher().AddClusterEventHandler(broker)

	return broker
}

func (broker *Broker) ValidateRouterDataModel() []error {
	return broker.routerSyncStrategy.Validate()
}

func (broker *Broker) GetRouterDataModel() *common.RouterDataModel {
	return broker.routerSyncStrategy.GetRouterDataModel()
}

func (broker *Broker) AcceptClusterEvent(clusterEvent *event.ClusterEvent) {
	if clusterEvent.EventType == event.ClusterLeadershipGained {
		broker.ae.Managers.Controller.PeersConnected(clusterEvent.Peers, false)
	}

	if broker.ae.HostController.IsRaftLeader() {
		if clusterEvent.EventType == event.ClusterPeerConnected {
			broker.ae.Managers.Controller.PeersConnected(clusterEvent.Peers, true)
		}

		if clusterEvent.EventType == event.ClusterPeerDisconnected {
			broker.ae.Managers.Controller.PeersDisconnected(clusterEvent.Peers)
		}
	}
}

func (broker *Broker) GetReceiveHandlers() []channel.TypedReceiveHandler {
	return broker.routerSyncStrategy.GetReceiveHandlers()
}

func (broker *Broker) RouterConnected(router *model.Router) {
	go func() {
		fingerprint := ""
		if router != nil && router.Fingerprint != nil {
			fingerprint = *router.Fingerprint
		}

		log := pfxlog.Logger().WithField("routerId", router.Id).WithField("routerName", router.Name).WithField("routerFingerprint", fingerprint)

		if fingerprint == "" {
			log.Errorf("router without fingerprints connecting [id: %s], ignoring", router.Id)
			return
		}

		if edgeRouter, _ := broker.ae.Managers.EdgeRouter.ReadOneByFingerprint(fingerprint); edgeRouter != nil {
			log.Infof("broker detected edge router with id %s connecting", router.Id)
			broker.routerSyncStrategy.RouterConnected(edgeRouter, router)
		} else {
			log.Debugf("broker detected non-edge router with id %s connecting", router.Id)
		}

	}()
}

func (broker *Broker) RouterDisconnected(r *model.Router) {
	go func() {
		pfxlog.Logger().WithField("routerId", r.Id).
			WithField("routerName", r.Name).
			WithField("routerFingerprint", r.Fingerprint).
			Infof("broker detected router with id %s disconnecting", r.Id)
		broker.routerSyncStrategy.RouterDisconnected(r)
	}()
}

func (broker *Broker) apiSessionFullyAuthenticated(args ...interface{}) {
	var apiSession *db.ApiSession
	if len(args) == 1 {
		apiSession, _ = args[0].(*db.ApiSession)
	}

	if apiSession == nil {
		pfxlog.Logger().Error("during broker apiSessionFullyAuthenticated could not cast arg[0] to *persistence.ApiSession")
		return
	}
	go broker.routerSyncStrategy.ApiSessionAdded(apiSession)
}

func (broker *Broker) apiSessionCertificateCreated(entity *db.ApiSessionCertificate) {
	go broker.apiSessionCertificateHandler(false, entity)
}

func (broker *Broker) apiSessionCertificateDeleted(entity *db.ApiSessionCertificate) {
	go broker.apiSessionCertificateHandler(true, entity)
}

func (broker *Broker) apiSessionCertificateHandler(delete bool, apiSessionCert *db.ApiSessionCertificate) {
	var apiSession *db.ApiSession
	var err error
	err = broker.ae.GetDb().View(func(tx *bbolt.Tx) error {
		apiSession, err = broker.ae.GetStores().ApiSession.LoadById(tx, apiSessionCert.ApiSessionId)
		return err
	})

	if err != nil {
		// If it's not found, it's because it was deleted, which is expected when the cert was deleted via session delete cascade
		if !delete || !boltz.IsErrNotFoundErr(err) {
			pfxlog.Logger().WithError(err).Error("could not process API Session certificate event, failed to query for parent API Session")
		}
		return
	}

	go broker.routerSyncStrategy.ApiSessionUpdated(apiSession, apiSessionCert)
}

func (broker *Broker) IsEdgeRouterOnline(id string) bool {
	state := broker.GetEdgeRouterState(id)
	return state.IsOnline
}

func (broker *Broker) GetEdgeRouterState(id string) RouterStateValues {
	return broker.routerSyncStrategy.GetEdgeRouterState(id)
}

func (broker *Broker) Stop() {
	broker.routerSyncStrategy.Stop()
}

func (broker *Broker) GetPublicKeys() map[string]crypto.PublicKey {
	return broker.routerSyncStrategy.GetPublicKeys()
}
