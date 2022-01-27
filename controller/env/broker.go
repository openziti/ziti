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
	"github.com/kataras/go-events"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	SessionRemovedType = int32(edge_ctrl_pb.ContentType_SessionRemovedType)

	ApiSessionHeartbeatType = int32(edge_ctrl_pb.ContentType_ApiSessionHeartbeatType)
	ApiSessionRemovedType   = int32(edge_ctrl_pb.ContentType_ApiSessionRemovedType)
	ApiSessionAddedType     = int32(edge_ctrl_pb.ContentType_ApiSessionAddedType)
	ApiSessionUpdatedType   = int32(edge_ctrl_pb.ContentType_ApiSessionUpdatedType)
	RequestClientReSyncType = int32(edge_ctrl_pb.ContentType_RequestClientReSyncType)

	ServerHelloType = int32(edge_ctrl_pb.ContentType_ServerHelloType)
	ClientHelloType = int32(edge_ctrl_pb.ContentType_ClientHelloType)

	EnrollmentCertsResponseType             = int32(edge_ctrl_pb.ContentType_EnrollmentCertsResponseType)
	EnrollmentExtendRouterRequestType       = int32(edge_ctrl_pb.ContentType_EnrollmentExtendRouterRequestType)
	EnrollmentExtendRouterVerifyRequestType = int32(edge_ctrl_pb.ContentType_EnrollmentExtendRouterVerifyRequestType)
)

// The Broker delegates Ziti Edge events to a RouterSyncStrategy. Handling the details of which events to watch
// and dealing with casting arguments to their proper concrete types.
type Broker struct {
	ae                  *AppEnv
	events              map[events.EventEmmiter]map[events.EventName][]events.Listener
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

	broker.ae.GetStores().Session.AddListener(boltz.EventDelete, broker.sessionDeleted)
	broker.ae.GetStores().ApiSession.AddListener(persistence.EventFullyAuthenticated, broker.apiSessionFullyAuthenticated)
	broker.ae.GetStores().ApiSession.AddListener(boltz.EventDelete, broker.apiSessionDeleted)
	broker.ae.GetStores().ApiSessionCertificate.AddListener(boltz.EventCreate, broker.apiSessionCertificateCreated)
	broker.ae.GetStores().ApiSessionCertificate.AddListener(boltz.EventDelete, broker.apiSessionCertificateDeleted)

	ae.HostController.GetNetwork().AddRouterPresenceHandler(broker)

	return broker
}

func (broker *Broker) GetReceiveHandlers() []channel.TypedReceiveHandler {
	return broker.routerSyncStrategy.GetReceiveHandlers()
}

func (broker *Broker) RouterConnected(router *network.Router) {
	go func() {
		log := pfxlog.Logger().WithField("routerId", router.Id).WithField("routerName", router.Name).WithField("routerFingerprint", router.Fingerprint)

		//check connection status, if already connected, ignore as it will be disconnected shortly
		if broker.ae.IsEdgeRouterOnline(router.Id) {
			log.Errorf("duplicate router connection detected [id: %s], ignoring", router.Id)
			return
		}

		if router.Fingerprint == nil {
			log.Errorf("router without fingerprints connecting [id: %s], ignoring", router.Id)
			return
		}

		if edgeRouter, _ := broker.ae.Handlers.EdgeRouter.ReadOneByFingerprint(*router.Fingerprint); edgeRouter != nil {
			pfxlog.Logger().WithField("routerId", router.Id).
				WithField("routerName", router.Name).
				WithField("routerFingerprint", router.Fingerprint).
				Infof("broker detected edge router with id %s connecting", router.Id)
			broker.routerSyncStrategy.RouterConnected(edgeRouter, router)
		} else {
			log.Debugf("broker detected non-edge router with id %s connecting", router.Id)
		}

	}()
}

func (broker *Broker) RouterDisconnected(r *network.Router) {
	// if disconnected but, by id it is still connected then it may have been a dupe
	// router connecting and being disconnected
	if !broker.ae.HostController.GetNetwork().ConnectedRouter(r.Id) {
		go func() {
			pfxlog.Logger().WithField("routerId", r.Id).
				WithField("routerName", r.Name).
				WithField("routerFingerprint", r.Fingerprint).
				Infof("broker detected router with id %s disconnecting", r.Id)
			broker.routerSyncStrategy.RouterDisconnected(r)
		}()
	}
}

func (broker *Broker) apiSessionFullyAuthenticated(args ...interface{}) {
	var apiSession *persistence.ApiSession
	if len(args) == 1 {
		apiSession, _ = args[0].(*persistence.ApiSession)
	}

	if apiSession == nil {
		pfxlog.Logger().Error("during broker apiSessionFullyAuthenticated could not cast arg[0] to *persistence.ApiSession")
		return
	}
	go broker.routerSyncStrategy.ApiSessionAdded(apiSession)
}

func (broker *Broker) apiSessionDeleted(args ...interface{}) {
	var apiSession *persistence.ApiSession
	if len(args) == 1 {
		apiSession, _ = args[0].(*persistence.ApiSession)
	}

	if apiSession == nil {
		pfxlog.Logger().Error("during broker apiSessionDeleted could not cast arg[0] to *persistence.ApiSession")
		return
	}

	go broker.routerSyncStrategy.ApiSessionDeleted(apiSession)
}

func (broker *Broker) sessionDeleted(args ...interface{}) {
	var session *persistence.Session
	if len(args) == 1 {
		session, _ = args[0].(*persistence.Session)
	}

	if session == nil {
		pfxlog.Logger().Error("during broker sessionDeleted could not cast arg[0] to *persistence.Session")
		return
	}

	go broker.routerSyncStrategy.SessionDeleted(session)
}

func (broker *Broker) apiSessionCertificateCreated(args ...interface{}) {
	go broker.apiSessionCertificateHandler(false, args...)
}

func (broker *Broker) apiSessionCertificateDeleted(args ...interface{}) {
	go broker.apiSessionCertificateHandler(true, args...)
}

func (broker *Broker) apiSessionCertificateHandler(delete bool, args ...interface{}) {
	var apiSessionCert *persistence.ApiSessionCertificate
	if len(args) == 1 {
		apiSessionCert, _ = args[0].(*persistence.ApiSessionCertificate)
	}

	if apiSessionCert == nil {
		pfxlog.Logger().Error("during broker apiSessionCertificateEvent could not cast arg[0] to *persistence.ApiSessionCertificate")
		return
	}
	var apiSession *persistence.ApiSession
	var err error
	err = broker.ae.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		apiSession, err = broker.ae.GetStores().ApiSession.LoadOneById(tx, apiSessionCert.ApiSessionId)
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
