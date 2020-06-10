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
	"bytes"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/kataras/go-events"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/build"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/storage/boltz"
	"strings"
	"sync"
	"time"
)

type Broker struct {
	ae            *AppEnv
	events        map[events.EventEmmiter]map[events.EventName][]events.Listener
	edgeRouterMap *edgeRouterMap
}

type edgeRouterMap struct {
	internalMap *sync.Map
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

func (m *edgeRouterMap) AddEntry(edgeRouterEntry *edgeRouterEntry) {
	m.internalMap.Store(edgeRouterEntry.EdgeRouter.Id, edgeRouterEntry)
}

func (m *edgeRouterMap) GetEntry(edgeRouterId string) *edgeRouterEntry {
	val, found := m.internalMap.Load(edgeRouterId)
	if !found {
		return nil
	}
	return val.(*edgeRouterEntry)
}

func (m *edgeRouterMap) GetOnlineEntries() []*edgeRouterEntry {
	var entries []*edgeRouterEntry

	m.internalMap.Range(func(_, vi interface{}) bool {
		if entry, ok := vi.(*edgeRouterEntry); ok && !entry.Channel.IsClosed() {
			entries = append(entries, entry)
		} else {
			panic("edge router map contains a non *edgeRouterEntry value")
		}

		return true
	})

	return entries
}

func (m *edgeRouterMap) RemoveEntry(edgeRouterId string) {
	m.internalMap.Delete(edgeRouterId)
}

func (m *edgeRouterMap) RangeEdgeRouterEntries(f func(entries *edgeRouterEntry) bool) {
	m.internalMap.Range(func(edgeRouterId, value interface{}) bool {
		if edgeRouterEntry, ok := value.(*edgeRouterEntry); ok {
			return f(edgeRouterEntry)
		}
		pfxlog.Logger().Panic("could not convert edge router entry")
		return false
	})
}

type edgeRouterEntry struct {
	EdgeRouter *model.EdgeRouter
	Channel    channel2.Channel
}

func NewBroker(ae *AppEnv) *Broker {
	b := &Broker{
		ae: ae,
		edgeRouterMap: &edgeRouterMap{
			internalMap: &sync.Map{},
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
	edgeRouterEntry := &edgeRouterEntry{
		EdgeRouter: edgeRouter,
		Channel:    ch,
	}

	b.edgeRouterMap.AddEntry(edgeRouterEntry)

	sessionMsg, err := b.getCurrentSessions()

	if err != nil {
		pfxlog.Logger().WithError(err).Error("could not get current sessions")
		return
	}

	pfxlog.Logger().Infof("sending edge router session updates [%d]", len(sessionMsg.ApiSessions))

	if buf, err := proto.Marshal(sessionMsg); err == nil {
		if err = ch.Send(channel2.NewMessage(ApiSessionAddedType, buf)); err != nil {
			pfxlog.Logger().WithError(err).Error("error sending session added")
		}
	} else {
		pfxlog.Logger().WithError(err).Error("error sending session added, could not marshal message content")
	}

	networkSessionMsg, err := b.getCurrentStateNetworkSessions(edgeRouter.Id)

	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not get current network sessions for edge router id [%s]", edgeRouter.Id)
		return
	}

	pfxlog.Logger().Infof("sending edge router network session updates [%d]", len(networkSessionMsg.Sessions))

	if buf, err := proto.Marshal(networkSessionMsg); err == nil {
		if err = ch.Send(channel2.NewMessage(SessionAddedType, buf)); err != nil {
			pfxlog.Logger().WithError(err).Error("error sending network session added")
		}
	} else {
		pfxlog.Logger().WithError(err).Error("error sending network session added, could not marshal message content")
	}

	pfxlog.Logger().Infof("edge router connection finalized and synchronized [%s] [%s]", edgeRouter.Id, edgeRouter.Name)
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

func (b *Broker) sendApiSessionCreates(apiSession *persistence.ApiSession) {
	apiSessionMsg := &edge_ctrl_pb.ApiSessionAdded{}

	apiSessionMsg.IsFullState = false

	fingerprints, err := b.getApiSessionFingerprints(apiSession.IdentityId)
	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not get session fingerprints")
		return
	}

	apiSessionMsg.ApiSessions = append(apiSessionMsg.ApiSessions, &edge_ctrl_pb.ApiSession{
		Token:            apiSession.Token,
		CertFingerprints: fingerprints,
	})

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

func (b *Broker) sendApiSessionUpdates(apiSession *persistence.ApiSession) {
	apiSessionMsg := &edge_ctrl_pb.ApiSessionUpdated{}

	fingerprints, err := b.getApiSessionFingerprints(apiSession.IdentityId)
	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not get session fingerprints")
		return
	}

	apiSessionMsg.ApiSessions = append(apiSessionMsg.ApiSessions, &edge_ctrl_pb.ApiSession{
		Token:            apiSession.Token,
		CertFingerprints: fingerprints,
	})

	byteMsg, err := proto.Marshal(apiSessionMsg)

	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not marshal session updated message")
		return
	}

	channelMsg := channel2.NewMessage(ApiSessionUpdatedType, byteMsg)

	b.sendToAllEdgeRouters(channelMsg)
}

func (b *Broker) sendToAllEdgeRouters(msg *channel2.Message) {
	b.edgeRouterMap.RangeEdgeRouterEntries(func(edgeRouterEntry *edgeRouterEntry) bool {
		if err := edgeRouterEntry.Channel.Send(msg); err != nil {
			pfxlog.Logger().WithError(err).Errorf("could not send session added message")
		}
		return true
	})
}

func (b *Broker) sendToAllEdgeRoutersForSession(sessionId string, msg *channel2.Message) {
	edgeRouterList, err := b.ae.Handlers.EdgeRouter.ListForSession(sessionId)
	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not get edge routers for session [%s]", sessionId)
		return
	}

	for _, edgeRouter := range edgeRouterList.EdgeRouters {
		edgeRouterEntry := b.edgeRouterMap.GetEntry(edgeRouter.Id)
		if edgeRouterEntry != nil && !edgeRouterEntry.Channel.IsClosed() {
			if err := edgeRouterEntry.Channel.Send(msg); err != nil {
				pfxlog.Logger().WithError(err).Errorf("could not send session added message")
			}
		}
	}
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

func (b *Broker) sendApiSessionDeletes(apiSession *persistence.ApiSession) {
	apiSessionMsg := &edge_ctrl_pb.ApiSessionRemoved{}
	apiSessionMsg.Tokens = append(apiSessionMsg.Tokens, apiSession.Token)

	byteMsg, err := proto.Marshal(apiSessionMsg)

	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not marshal session removed message")
		return
	}

	channelMsg := channel2.NewMessage(ApiSessionRemovedType, byteMsg)

	b.sendToAllEdgeRouters(channelMsg)
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

func (b *Broker) sendSessionDeletes(session *persistence.Session) {
	sessionsRemoved := &edge_ctrl_pb.SessionRemoved{}
	sessionsRemoved.Tokens = append(sessionsRemoved.Tokens, session.Token)

	if buf, err := proto.Marshal(sessionsRemoved); err == nil {
		msg := channel2.NewMessage(SessionRemovedType, buf)
		// can't use sendToAllEdgeRoutersForSession b/c the session is gone
		b.sendToAllEdgeRouters(msg)
	} else {
		pfxlog.Logger().WithError(err).Error("error sending session removed, could not marshal message content")
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

func (b *Broker) sendSessionCreates(session *persistence.Session) {
	sessionAdded := &edge_ctrl_pb.SessionAdded{}

	service, err := b.ae.Handlers.EdgeService.Read(session.ServiceId)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("could not send network session added, could not find service")
		return
	}

	fps, err := b.getActiveFingerprints(session.Id)

	if err != nil {
		pfxlog.Logger().Errorf("could not obtain a fingerprint for the api session [%s] and session [%s]", session.ApiSessionId, session.Id)
		return
	}

	svc, err := b.modelServiceToProto(service)

	if err != nil {
		pfxlog.Logger().Errorf("could not convert service [%s] to proto: %s", service.Id, err)
		return
	}

	sessionType := edge_ctrl_pb.SessionType_Dial
	if session.Type == persistence.SessionTypeBind {
		sessionType = edge_ctrl_pb.SessionType_Bind
	}

	sessionAdded.Sessions = append(sessionAdded.Sessions, &edge_ctrl_pb.Session{
		Token:            session.Token,
		Service:          svc,
		CertFingerprints: fps,
		Type:             sessionType,
	})

	if buf, err := proto.Marshal(sessionAdded); err == nil {
		msg := channel2.NewMessage(SessionAddedType, buf)
		b.sendToAllEdgeRoutersForSession(session.Id, msg)
	} else {
		pfxlog.Logger().WithError(err).Error("error sending network session added, could not marshal message content")
	}
}

func (b *Broker) modelServiceToProto(service *model.Service) (*edge_ctrl_pb.Service, error) {
	return &edge_ctrl_pb.Service{
		Name: service.Name,
		Id:   service.Id,
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

	sessions, err := b.ae.GetHandlers().ApiSession.Query("true limit none")
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

func (b *Broker) getCurrentStateNetworkSessions(edgeRouterId string) (*edge_ctrl_pb.SessionAdded, error) {
	ret := &edge_ctrl_pb.SessionAdded{
		IsFullState: true,
	}

	result, err := b.ae.Handlers.Session.ListSessionsForEdgeRouter(edgeRouterId)

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
	certs, err := b.ae.Handlers.Session.ReadSessionCerts(sessionId)
	if err != nil {
		return nil, err
	}
	var ret []string

	now := time.Now()
	for _, c := range certs {
		if (now.Equal(c.ValidFrom) || now.After(c.ValidFrom)) && now.Before(c.ValidTo) {
			ret = append(ret, c.Fingerprint)
		}
	}

	return ret, nil
}

func (b *Broker) getApiSessionFingerprints(identityId string) ([]string, error) {
	fingerprintsMap := map[string]struct{}{}

	err := b.ae.Handlers.Identity.CollectAuthenticators(identityId, func(authenticator *model.Authenticator) error {
		for _, authPrint := range authenticator.Fingerprints() {
			fingerprintsMap[authPrint] = struct{}{}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	var fingerprints []string
	for fingerprint := range fingerprintsMap {
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
	service, err := b.ae.Handlers.EdgeService.Read(ns.ServiceId)
	if err != nil {
		return nil, fmt.Errorf("could not convert to session proto, could not find service: %s", err)
	}

	apiSession, err := b.ae.Handlers.ApiSession.Read(ns.ApiSessionId)
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

	sessionType := edge_ctrl_pb.SessionType_Dial
	if ns.Type == persistence.SessionTypeBind {
		sessionType = edge_ctrl_pb.SessionType_Bind
	}

	return &edge_ctrl_pb.Session{
		Token:            ns.Token,
		SessionToken:     apiSession.Token,
		Service:          svc,
		CertFingerprints: fps,
		Type:             sessionType,
	}, nil
}

func (b *Broker) GetOnlineEdgeRouter(id string) *model.EdgeRouter {
	result, found := b.edgeRouterMap.internalMap.Load(id)
	if !found {
		return nil
	}

	entry, ok := result.(*edgeRouterEntry)
	if !ok || entry.Channel.IsClosed() {
		return nil
	}
	return entry.EdgeRouter
}

func (b *Broker) RouterConnected(r *network.Router) {
	go func() {
		if r.Fingerprint == nil {
			return
		}

		fp := formatFingerprint(*r.Fingerprint)
		edgeRouter, _ := b.ae.Handlers.EdgeRouter.ReadOneByFingerprint(fp)

		// not an edge router
		if edgeRouter == nil || edgeRouter.Id == "" {
			return
		}

		b.sendHello(r, edgeRouter, fp)
	}()
}

func (b *Broker) sendHello(r *network.Router, edgeRouter *model.EdgeRouter, fingerprint string) {
	serverVersion := build.GetBuildInfo().GetVersion()
	serverHello := &edge_ctrl_pb.ServerHello{
		Version: serverVersion,
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
						pfxlog.Logger().Infof("edge router connecting with version [%s] to controller with version [%s]", respHello.Version, serverVersion)

						b.AddEdgeRouter(r.Control, edgeRouter)
					} else {
						pfxlog.Logger().WithError(err).Error("could not unmarshal clientHello after serverHello")
						return
					}
				}

				if resp.ContentType == ErrorType {
					respErr := &edge_ctrl_pb.Error{}
					if err := proto.Unmarshal(resp.Body, respErr); err == nil {
						pfxlog.Logger().WithField("cause", respErr.Cause).WithField("code", respErr.Code).WithField("message", respErr.Message).
							Error("client responded with error after serverHello")
						return
					}
					pfxlog.Logger().WithError(err).Error("could not unmarshal error from client after serverHello")
					return
				}

			case <-time.After(5 * time.Second):
				pfxlog.Logger().Error("timeout - waiting for clientHello from edge router")
			}

		} else {
			pfxlog.Logger().WithError(err).Error("could not send serverHello message for edge router")
			return
		}
	}
}

func (b *Broker) RouterDisconnected(r *network.Router) {
	go func() {
		if r.Fingerprint == nil {
			return
		}
		fp := formatFingerprint(*r.Fingerprint)
		edgeRouter, _ := b.ae.Handlers.EdgeRouter.ReadOneByFingerprint(fp)

		// not an edge router
		if edgeRouter == nil || edgeRouter.Id == "" {
			return
		}

		b.edgeRouterMap.RemoveEntry(edgeRouter.Id)
	}()
}

func formatFingerprint(fp string) string {
	fp = strings.ToUpper(fp)

	return insertNth(fp, 2, ":")
}

func insertNth(s string, n int, i string) string {
	rs := []rune(i)
	var buffer bytes.Buffer
	var nMinus1 = n - 1
	var lenMinus1 = len(s) - 1
	for i, cr := range s {
		buffer.WriteRune(cr)
		if i%n == nMinus1 && i != lenMinus1 {
			for _, r := range rs {
				buffer.WriteRune(r)
			}
		}
	}
	return buffer.String()
}
