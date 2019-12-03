/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/netfoundry/ziti-foundation/common/version"
	"github.com/netfoundry/ziti-edge/edge/controller/model"
	"github.com/netfoundry/ziti-edge/edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/edge/migration"
	"github.com/netfoundry/ziti-edge/edge/pb/edge_ctrl_pb"
	"github.com/netfoundry/ziti-fabric/fabric/controller/network"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"bytes"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/kataras/go-events"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"strings"
	"sync"
	"time"
)

type Broker struct {
	ae         *AppEnv
	events     map[events.EventEmmiter]map[events.EventName][]events.Listener
	clusterMap *clusterMap
}

type clusterMap struct {
	internalClusterMap *sync.Map
}

const (
	SessionRemovedType = int32(edge_ctrl_pb.ContentType_SessionRemovedType)
	SessionAddedType   = int32(edge_ctrl_pb.ContentType_SessionAddedType)
	SessionUpdatedType = int32(edge_ctrl_pb.ContentType_SessionUpdatedType)

	ApiSessionHeartbeatType = int32(edge_ctrl_pb.ContentType_ApiSessionHeartbeatType)
	ApiSessionRemovedType   = int32(edge_ctrl_pb.ContentType_ApiSessionRemovedType)
	ApiSessionAddedType     = int32(edge_ctrl_pb.ContentType_ApiSessionAddedType)
	ApiSessionUpdatedType   = int32(edge_ctrl_pb.ContentType_ApiSessionUpdatedType)

	ServerHelloType = int32(edge_ctrl_pb.ContentType_ServerHelloType)
	ClientHelloType = int32(edge_ctrl_pb.ContentType_ClientHelloType)
	ErrorType       = int32(edge_ctrl_pb.ContentType_ErrorType)
)

func (m *clusterMap) getEntryMap(clusterId string) *sync.Map {
	if value, ok := m.internalClusterMap.Load(clusterId); ok {
		edgeRouterMap, ok := value.(*sync.Map)

		if !ok {
			panic("could not convert edge router map to *sync.Map")
		}

		return edgeRouterMap
	}
	return nil
}

func (m *clusterMap) AddEntry(clusterId string, edgeRouterEntry *edgeRouterEntry) {
	edgeRouterMap := m.getEntryMap(clusterId)

	if edgeRouterMap == nil {
		edgeRouterMap = &sync.Map{}
		m.internalClusterMap.Store(clusterId, edgeRouterMap)
	}

	edgeRouterMap.Store(edgeRouterEntry.EdgeRouter.Id, edgeRouterEntry)
}

func (m *clusterMap) GetEntries(clusterId string) []*edgeRouterEntry {
	var entries []*edgeRouterEntry

	entryMap := m.getEntryMap(clusterId)

	if entryMap == nil {
		return nil
	}

	entryMap.Range(func(_, vi interface{}) bool {
		if entry, ok := vi.(*edgeRouterEntry); ok {
			entries = append(entries, entry)
		} else {
			panic("cluster/edge router map contains a non *edgeRouterEntry value")
		}

		return true
	})

	return entries
}

func (m *clusterMap) GetOnlineEntries(clusterId string) []*edgeRouterEntry {
	var entries []*edgeRouterEntry

	for _, entry := range m.GetEntries(clusterId) {
		if !entry.Channel.IsClosed() {
			entries = append(entries, entry)
		}
	}

	return entries
}

func (m *clusterMap) RemoveEntry(clusterId, edgeRouterId string) {
	if v, ok := m.internalClusterMap.Load(clusterId); ok {
		if edgeRouterMap, ok := v.(*sync.Map); ok {
			edgeRouterMap.Delete(edgeRouterId)
		}
	}

}

func (m *clusterMap) RangeEdgeRouterEntries(f func(entries *edgeRouterEntry) bool) {
	m.internalClusterMap.Range(func(clusterId, value interface{}) bool {
		if edgeRouterMap, ok := value.(*sync.Map); ok {
			edgeRouterMap.Range(func(edgeRouterId, value interface{}) bool {
				if edgeRouterEntry, ok := value.(*edgeRouterEntry); ok {
					return f(edgeRouterEntry)
				}
				pfxlog.Logger().Panic("could not convert edge router entry")
				return false
			})
		}
		return true
	})
}

type edgeRouterEntry struct {
	EdgeRouter *model.EdgeRouter
	Channel    channel2.Channel
}

func listener2async(l events.Listener) events.Listener {
	return func(args ...interface{}) {
		go l(args...)
	}
}

func NewBroker(ae *AppEnv) *Broker {
	b := &Broker{
		ae: ae,
		clusterMap: &clusterMap{
			internalClusterMap: &sync.Map{},
		},
	}

	b.events = map[events.EventEmmiter]map[events.EventName][]events.Listener{
		b.ae.GetStores().Session: {
			boltz.EventDelete: []events.Listener{
				b.sessionDeleteEventHandler,
			},
			boltz.EventCreate: []events.Listener{
				b.sessionCreateEventHandler,
			},
		},
		b.ae.GetStores().ApiSession: {
			boltz.EventCreate: []events.Listener{
				b.apiSessionCreateEventHandler,
			},
			boltz.EventDelete: []events.Listener{
				b.apiSessionDeleteEventHandler,
			},
		},
	}

	b.registerEventHandlers()

	ae.HostController.GetNetwork().AddRouterPresenceHandler(b)

	return b
}

func (b *Broker) AddEdgeRouter(ch channel2.Channel, edgeRouter *model.EdgeRouter) {
	if edgeRouter.ClusterId == "" {
		pfxlog.Logger().Errorf("adding connecting edge router failed, edge router [%s] did not have a cluster id [%s]", edgeRouter.Id, edgeRouter.ClusterId)
		return
	}

	edgeRouterEntry := &edgeRouterEntry{
		EdgeRouter: edgeRouter,
		Channel:    ch,
	}

	b.clusterMap.AddEntry(edgeRouter.ClusterId, edgeRouterEntry)

	sessionMsg, err := b.getCurrentSessions()

	if err != nil {
		pfxlog.Logger().WithField("cause", err).Error("could not get current sessions")
		return
	}

	pfxlog.Logger().Infof("sending edge router session updates [%d]", len(sessionMsg.ApiSessions))

	if buf, err := proto.Marshal(sessionMsg); err == nil {
		if err = ch.Send(channel2.NewMessage(ApiSessionAddedType, buf)); err != nil {
			pfxlog.Logger().WithField("cause", err).Error("error sending session added")
		}
	} else {
		pfxlog.Logger().WithField("cause", err).Error("error sending session added, could not marshal message content")
	}

	networkSessionMsg, err := b.getCurrentStateNetworkSessions(edgeRouter.ClusterId)

	if err != nil {
		pfxlog.Logger().WithField("cause", err).Errorf("could not get current network sessions for cluster id [%s]", edgeRouter.ClusterId)
		return
	}

	pfxlog.Logger().Infof("sending edge router network session updates [%d]", len(networkSessionMsg.Sessions))

	if buf, err := proto.Marshal(networkSessionMsg); err == nil {
		if err = ch.Send(channel2.NewMessage(SessionAddedType, buf)); err != nil {
			pfxlog.Logger().WithField("cause", err).Error("error sending network session added")
		}
	} else {
		pfxlog.Logger().WithField("cause", err).Error("error sending network session added, could not marshal message content")
	}

	pfxlog.Logger().Infof("edge router connection finalized and synchronized [%s] [%s]", edgeRouter.Id, edgeRouter.Name)
}

func (b *Broker) getEventDetails(args []interface{}) *migration.CrudEventDetails {
	if args != nil && args[0] != nil {
		if ed, ok := args[0].(*migration.CrudEventDetails); ok {
			return ed
		}
	} else {
		log := pfxlog.Logger()
		log.Warn("event fired without the expected CrudEventDetails pointer as args[0]")
	}

	return nil
}

func (b *Broker) apiSessionCreateEventHandler(args ...interface{}) {
	var apiSession *persistence.ApiSession
	if len(args) == 1 {
		apiSession, _ = args[0].(*persistence.ApiSession)
	}

	if apiSession == nil {
		log := pfxlog.Logger()
		log.Error("could not cast event args to event details")
		return
	}

	b.sendApiSessionCreates(apiSession)
}

func (b *Broker) sendApiSessionCreates(apiSessions ...*persistence.ApiSession) {
	apiSessionMsg := &edge_ctrl_pb.ApiSessionAdded{}

	apiSessionMsg.IsFullState = false

	for _, session := range apiSessions {
		fingerprints, err := b.getApiSessionFingerprints(session.IdentityId)
		if err != nil {
			pfxlog.Logger().WithError(err).Errorf("could not get session fingerprints")
			return
		}

		apiSessionMsg.ApiSessions = append(apiSessionMsg.ApiSessions, &edge_ctrl_pb.ApiSession{
			Token:            session.Token,
			CertFingerprints: fingerprints,
		})
	}

	byteMsg, err := proto.Marshal(apiSessionMsg)

	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not marshal api session added message")
		return
	}

	channelMsg := channel2.NewMessage(ApiSessionAddedType, byteMsg)

	b.sendToAllEdgeRouters(channelMsg)
}

func (b *Broker) apiSessionUpdateEventHandler(args ...interface{}) {
	var apiSession *persistence.ApiSession
	if len(args) == 1 {
		apiSession, _ = args[0].(*persistence.ApiSession)
	}

	if apiSession == nil {
		log := pfxlog.Logger()
		log.Error("could not cast event args to event details")
		return
	}

	b.sendApiSessionUpdates(apiSession)
}

func (b *Broker) sendApiSessionUpdates(sessions ...*persistence.ApiSession) {
	apiSessionMsg := &edge_ctrl_pb.ApiSessionUpdated{}

	for _, session := range sessions {
		fingerprints, err := b.getApiSessionFingerprints(session.IdentityId)
		if err != nil {
			pfxlog.Logger().WithError(err).Errorf("could not get session fingerprints")
			return
		}

		apiSessionMsg.ApiSessions = append(apiSessionMsg.ApiSessions, &edge_ctrl_pb.ApiSession{
			Token:            session.Token,
			CertFingerprints: fingerprints,
		})
	}

	byteMsg, err := proto.Marshal(apiSessionMsg)

	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not marshal session updated message")
		return
	}

	channelMsg := channel2.NewMessage(ApiSessionUpdatedType, byteMsg)

	b.sendToAllEdgeRouters(channelMsg)
}

func (b *Broker) sendToAllEdgeRouters(msg *channel2.Message) {
	b.clusterMap.RangeEdgeRouterEntries(func(edgeRouterEntry *edgeRouterEntry) bool {
		err := edgeRouterEntry.Channel.Send(msg)

		if err != nil {
			pfxlog.Logger().WithError(err).Errorf("could send session added message")
		}

		return true
	})
}

func (b *Broker) apiSessionDeleteEventHandler(args ...interface{}) {
	var apiSession *persistence.ApiSession
	if len(args) == 1 {
		apiSession, _ = args[0].(*persistence.ApiSession)
	}

	if apiSession == nil {
		log := pfxlog.Logger()
		log.Error("could not cast event args to event details")
		return
	}

	b.sendApiSessionDeletes(apiSession)
}

func (b *Broker) sendApiSessionDeletes(sessions ...*persistence.ApiSession) {
	apiSessionMsg := &edge_ctrl_pb.ApiSessionRemoved{}

	for _, session := range sessions {

		apiSessionMsg.Tokens = append(apiSessionMsg.Tokens, session.Token)
	}

	byteMsg, err := proto.Marshal(apiSessionMsg)

	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not marshal session removed message")
		return
	}

	channelMsg := channel2.NewMessage(ApiSessionRemovedType, byteMsg)

	b.sendToAllEdgeRouters(channelMsg)
}

func (b *Broker) syncIdentitiesEventHandler(eventName events.EventName, args ...interface{}) {
	ed := b.getEventDetails(args)

	if ed == nil {
		log := pfxlog.Logger()
		log.Error("could not cast event args to event details")
		return
	}

	log := pfxlog.Logger()

	err := b.ae.GetDbProvider().GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		for _, e := range ed.Entities {
			if eventName == migration.EventCreate || eventName == migration.EventUpdate {
				log.Debugf("creating bolt identity to mirror gorm identity: %v", e.GetId())
				gormIdentity, ok := e.(*migration.Identity)
				if !ok {
					return errors.Errorf("unable to cast event entity to Identity. id: %v, type: %v", e.GetId(), reflect.TypeOf(e))
				}
				identity := &persistence.Identity{
					BaseEdgeEntityImpl: persistence.BaseEdgeEntityImpl{
						Id: gormIdentity.ID,
						EdgeEntityFields: persistence.EdgeEntityFields{
							CreatedAt: *gormIdentity.CreatedAt,
							UpdatedAt: *gormIdentity.UpdatedAt,
							Tags:      *gormIdentity.Tags,
							Migrate:   true,
						},
					},
					Name:           *gormIdentity.Name,
					IsDefaultAdmin: *gormIdentity.IsDefaultAdmin,
					IsAdmin:        *gormIdentity.IsAdmin,
				}
				if eventName == migration.EventCreate {
					if err := b.ae.GetStores().Identity.Create(ctx, identity); err != nil {
						return err
					}
				} else {
					if err := b.ae.GetStores().Identity.Update(ctx, identity, nil); err != nil {
						return err
					}
				}
			}
			if eventName == migration.EventDelete {
				log.Debugf("deleting bolt identity to mirror gorm identity: %v", e.GetId())
				if err := b.ae.GetStores().Identity.DeleteById(ctx, e.GetId()); err != nil {
					return err
				}
			}
		}
		return nil
	})

	if err != nil {
		log.Errorf("Failure while syncing identities from gorm to bolt: %+v", err)
	}
}

func (b *Broker) sessionDeleteEventHandler(args ...interface{}) {
	var session *persistence.Session
	if len(args) == 1 {
		session, _ = args[0].(*persistence.Session)
	}

	if session == nil {
		log := pfxlog.Logger()
		log.Error("could not cast event args to event details")
		return
	}

	b.sendSessionDeletes(session)
}

func (b *Broker) sendSessionDeletes(sessions ...*persistence.Session) {
	removedByClusterId := map[string]*edge_ctrl_pb.SessionRemoved{}

	for _, session := range sessions {
		service, err := b.ae.Handlers.Service.HandleRead(session.ServiceId)
		if err != nil {
			log := pfxlog.Logger()
			log.WithField("cause", err).Error("could not send network session removed, could not find service")
			continue
		}

		for _, clusterId := range service.Clusters {
			var ok bool
			if _, ok = removedByClusterId[clusterId]; !ok {
				removedByClusterId[clusterId] = &edge_ctrl_pb.SessionRemoved{}
			}

			removedByClusterId[clusterId].Tokens = append(removedByClusterId[clusterId].Tokens, session.Token)
		}
	}

	for clusterId, nsr := range removedByClusterId {
		entries := b.clusterMap.GetOnlineEntries(clusterId)
		for _, e := range entries {

			if buf, err := proto.Marshal(nsr); err == nil {
				if err = e.Channel.Send(channel2.NewMessage(SessionRemovedType, buf)); err != nil {
					pfxlog.Logger().WithField("cause", err).Error("error sending network session removed")
				}
			} else {
				pfxlog.Logger().WithField("cause", err).Error("error sending network session removed, could not marshal message content")
			}
		}
	}
}

func (b *Broker) sessionCreateEventHandler(args ...interface{}) {
	var session *persistence.Session
	if len(args) == 1 {
		session, _ = args[0].(*persistence.Session)
	}

	if session == nil {
		log := pfxlog.Logger()
		log.Error("could not cast event args to event details")
		return
	}

	b.sendSessionCreates(session)
}

func (b *Broker) sendSessionCreates(sessions ...*persistence.Session) {
	addByClusterId := map[string]*edge_ctrl_pb.SessionAdded{}

	for _, session := range sessions {
		service, err := b.ae.Handlers.Service.HandleRead(session.ServiceId)
		if err != nil {
			log := pfxlog.Logger()
			log.WithField("cause", err).Error("could not send network session added, could not find service")
			continue
		}

		for _, clusterId := range service.Clusters {
			var ok bool
			if _, ok = addByClusterId[clusterId]; !ok {
				addByClusterId[clusterId] = &edge_ctrl_pb.SessionAdded{}
			}

			fps, err := b.getActiveFingerprints(session.Id)

			if err != nil {
				pfxlog.Logger().Errorf("could not obtain a fingerprint for the api session [%s] and session [%s]", session.ApiSessionId, session.Id)
				continue
			}

			svc, err := b.modelServiceToProto(service)

			if err != nil {
				pfxlog.Logger().Errorf("could not convert service [%s] to proto: %s", service.Id, err)
				continue
			}

			addByClusterId[clusterId].Sessions = append(addByClusterId[clusterId].Sessions, &edge_ctrl_pb.Session{
				Token:            session.Token,
				Service:          svc,
				CertFingerprints: fps,
				Hosting:          session.IsHosting,
			})
		}
	}

	for clusterId, nsr := range addByClusterId {
		entries := b.clusterMap.GetOnlineEntries(clusterId)
		for _, e := range entries {

			if buf, err := proto.Marshal(nsr); err == nil {
				if err = e.Channel.Send(channel2.NewMessage(SessionAddedType, buf)); err != nil {
					pfxlog.Logger().WithField("cause", err).Error("error sending network session added")
				}
			} else {
				pfxlog.Logger().WithField("cause", err).Error("error sending network session added, could not marshal message content")
			}
		}
	}
}

func (b *Broker) modelServiceToProto(service *model.Service) (*edge_ctrl_pb.Service, error) {
	return &edge_ctrl_pb.Service{
		Name:            service.Name,
		Id:              service.Id,
		EndpointAddress: service.EndpointAddress,
		EgressRouter:    service.EgressRouter,
	}, nil
}

func (b *Broker) registerEventHandlers() {
	for s, enls := range b.events {
		for en, ls := range enls {
			for _, l := range ls {
				s.AddListener(en, l)
			}
		}
	}
}

func (b *Broker) getCurrentSessions() (*edge_ctrl_pb.ApiSessionAdded, error) {
	ret := &edge_ctrl_pb.ApiSessionAdded{
		IsFullState: true,
	}

	sessions, err := b.ae.GetHandlers().ApiSession.HandleQuery("true limit none")
	if err != nil {
		return nil, err
	}

	log := pfxlog.Logger()
	for _, session := range sessions.ApiSessions {
		sessionProto, err := b.modelApiSessionToProto(session.Token, session.IdentityId)

		if err != nil {
			log.Error(err)
			continue
		}

		ret.ApiSessions = append(ret.ApiSessions, sessionProto)
	}

	return ret, nil
}

func (b *Broker) getCurrentStateNetworkSessions(clusterId string) (*edge_ctrl_pb.SessionAdded, error) {
	ret := &edge_ctrl_pb.SessionAdded{
		IsFullState: true,
	}

	query := fmt.Sprintf(`anyOf(service.clusters.id) = "%v" limit none`, clusterId)
	result, err := b.ae.Handlers.Session.HandleQuery(query)

	if err != nil {
		return nil, err
	}

	log := pfxlog.Logger()
	for _, session := range result.Sessions {
		nu, err := b.modelSessionToProto(session)

		if err != nil {
			log.Error(err)
			continue
		}

		ret.Sessions = append(ret.Sessions, nu)
	}

	return ret, nil
}

func (b *Broker) getActiveFingerprints(sessionId string) ([]string, error) {
	certs, err := b.ae.Handlers.Session.HandleReadSessionCerts(sessionId)
	if err != nil {
		return nil, err
	}
	var ret []string

	now := time.Now()
	for _, c := range certs {
		if now.After(c.ValidFrom) && now.Before(c.ValidTo) {
			ret = append(ret, c.Fingerprint)
		}
	}

	return ret, nil
}

func (b *Broker) getApiSessionFingerprints(identityId string) ([]string, error) {
	fingerprintsMap := map[string]bool{}

	err := b.ae.Handlers.Identity.HandleCollectAuthenticators(identityId, func(entity model.BaseModelEntity) error {
		authenticator, ok := entity.(*model.Authenticator)

		if !ok {
			return fmt.Errorf("unexpected type %v when converting base model entity to authenticator", reflect.TypeOf(entity))
		}

		for _, authPrint := range authenticator.Fingerprints() {
			fingerprintsMap[authPrint] = true
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	var fingerprints []string
	for fingerprint, _ := range fingerprintsMap {
		fingerprints = append(fingerprints, fingerprint)
	}

	return fingerprints, nil
}

func (b *Broker) modelApiSessionToProto(token, identityId string) (*edge_ctrl_pb.ApiSession, error) {
	fingerprints, err := b.getApiSessionFingerprints(identityId)
	if err != nil {
		return nil, err
	}

	return &edge_ctrl_pb.ApiSession{
		Token:            token,
		CertFingerprints: fingerprints,
	}, nil
}

func (b *Broker) modelSessionToProto(ns *model.Session) (*edge_ctrl_pb.Session, error) {
	service, err := b.ae.Handlers.Service.HandleRead(ns.ServiceId)
	if err != nil {
		return nil, fmt.Errorf("could not convert to session proto, could not find service: %s", err)
	}

	apiSession, err := b.ae.Handlers.ApiSession.HandleRead(ns.ApiSessionId)
	if err != nil {
		return nil, fmt.Errorf("could not convert to network session proto, could not find session: %s", err)
	}

	fps, err := b.getActiveFingerprints(ns.Id)

	if err != nil {
		return nil, fmt.Errorf("could not get fingerprints for network session: %s", err)
	}

	svc, err := b.modelServiceToProto(service)

	if err != nil {
		return nil, fmt.Errorf("could not convert service [%s] to proto: %s", service.Id, err)
	}

	return &edge_ctrl_pb.Session{
		Token:            ns.Token,
		SessionToken:     apiSession.Token,
		Service:          svc,
		CertFingerprints: fps,
		Hosting:          ns.IsHosting,
	}, nil
}

func (b *Broker) GetOnlineEdgeRouter(id string) *model.EdgeRouter {
	var edgeRouter *model.EdgeRouter

	b.clusterMap.RangeEdgeRouterEntries(func(edgeRouterEntry *edgeRouterEntry) bool {
		if edgeRouterEntry.EdgeRouter.GetId() == id {
			edgeRouter = edgeRouterEntry.EdgeRouter
			return false
		}
		return true
	})

	return edgeRouter
}

func (b *Broker) ClusterHasEdgeRouterOnline(clusterId string) bool {
	entries := b.clusterMap.GetOnlineEntries(clusterId)
	return len(entries) > 0
}

func (b *Broker) GetOnlineEdgeRoutersByCluster(clusterId string) []*model.EdgeRouter {
	var ret []*model.EdgeRouter

	for _, entry := range b.clusterMap.GetOnlineEntries(clusterId) {
		ret = append(ret, entry.EdgeRouter)
	}

	return ret
}

func (b *Broker) RouterConnected(r *network.Router) {
	go func() {
		fp := formatFingerprint(r.Fingerprint)
		edgeRouter, _ := b.ae.Handlers.EdgeRouter.HandleReadOneByFingerprint(fp)

		// not an edge router
		if edgeRouter == nil || edgeRouter.Id == "" {
			return
		}

		b.sendHello(r, edgeRouter, fp)
	}()

}

func (b *Broker) sendHello(r *network.Router, edgeRouter *model.EdgeRouter, fingerprint string) {
	serverHello := &edge_ctrl_pb.ServerHello{
		Version: version.GetVersion(),
	}

	if buf, err := proto.Marshal(serverHello); err == nil {
		if waitCh, err := r.Control.SendAndWait(channel2.NewMessage(ServerHelloType, buf)); err == nil {

			select {
			case resp := <-waitCh:
				if resp.ContentType == ClientHelloType {
					respHello := &edge_ctrl_pb.ClientHello{}
					if err := proto.Unmarshal(resp.Body, respHello); err == nil {
						pfxlog.Logger().
							WithField("version", respHello.Version).
							WithField("hostname", respHello.Hostname).
							WithField("protocols", respHello.Protocols).
							WithField("data", respHello.Data).
							Info("edge router responded with client hello")

						protocols := map[string]string{}
						for _, p := range respHello.Protocols {
							ingressUrl := fmt.Sprintf("%s://%s", p, respHello.Hostname)
							protocols[p] = ingressUrl
						}

						//in memory only
						edgeRouter.Hostname = &respHello.Hostname
						edgeRouter.EdgeRouterProtocols = protocols

						//todo: restrict version?
						pfxlog.Logger().Infof("edge router connecting with version [%s] to controller with version [%s]", respHello.Version, version.GetVersion())

						b.AddEdgeRouter(r.Control, edgeRouter)
					} else {
						pfxlog.Logger().WithField("cause", err).Error("could not unmarshal clientHello after serverHello")
						return
					}
				}

				if resp.ContentType == ErrorType {
					respErr := &edge_ctrl_pb.Error{}
					if err := proto.Unmarshal(resp.Body, respErr); err == nil {
						pfxlog.Logger().WithField("cause", respErr.Cause).WithField("code", respErr.Code).WithField("message", respErr.Message).
							Error("client responded with error after serverHello")
						return
					} else {
						pfxlog.Logger().WithField("cause", err).Error("could not unmarshal error from client after serverHello")
						return
					}

				}

			case <-time.After(5 * time.Second):
				pfxlog.Logger().Error("timeout - waiting for clientHello from edge router")
			}

		} else {
			pfxlog.Logger().WithField("cause", err).Error("could not send serverHello message for edge router")
			return
		}
	}
}

func (b *Broker) RouterDisconnected(r *network.Router) {
	go func() {
		fp := formatFingerprint(r.Fingerprint)
		edgeRouter, _ := b.ae.Handlers.EdgeRouter.HandleReadOneByFingerprint(fp)

		// not an edge router
		if edgeRouter == nil || edgeRouter.Id == "" {
			return
		}

		b.clusterMap.RemoveEntry(edgeRouter.ClusterId, edgeRouter.Id)
	}()
}

func formatFingerprint(fp string) string {
	fp = strings.ToUpper(fp)

	return insertNth(fp, 2, ":")
}

func insertNth(s string, n int, i string) string {
	rs := []rune(i)
	var buffer bytes.Buffer
	var n_1 = n - 1
	var l_1 = len(s) - 1
	for i, cr := range s {
		buffer.WriteRune(cr)
		if i%n == n_1 && i != l_1 {
			for _, r := range rs {
				buffer.WriteRune(r)
			}
		}
	}
	return buffer.String()
}
