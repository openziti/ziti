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
	"github.com/openziti/foundation/util/concurrenz"
	"go.etcd.io/bbolt"
	"sync"
	"time"
)

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
)

type edgeRouterMap struct {
	internalMap *sync.Map //edgeRouterId -> edgeRouterEntry
}

type edgeRouterEntry struct {
	EdgeRouter *model.EdgeRouter
	Channel    channel2.Channel
	send       chan *channel2.Message
	stop       chan interface{}
	running    concurrenz.AtomicBoolean
	stopping   concurrenz.AtomicBoolean
}

func newEdgeRouterEntry(router *model.EdgeRouter, ch channel2.Channel, sendBufferSize int) *edgeRouterEntry {
	return &edgeRouterEntry{
		EdgeRouter: router,
		Channel:    ch,
		send:       make(chan *channel2.Message, sendBufferSize),
		stop:       make(chan interface{}, 0),
		running:    concurrenz.AtomicBoolean(0),
		stopping:   concurrenz.AtomicBoolean(0),
	}
}

func (entry *edgeRouterEntry) Start() {

	running := entry.running.Get()
	if !running {
		entry.running.Set(true)
		go entry.run()
	}
}

func (entry *edgeRouterEntry) Stop() {
	stopping := entry.stopping.Get()
	if !stopping {
		entry.stopping.Set(true)
		go func() {
			entry.stop <- struct{}{}
		}()
	}
}

func (entry *edgeRouterEntry) run() {
	for {
		select {
		case <-entry.stop:
			entry.running.Set(false)
			entry.stopping.Set(false)
			return
		case msg := <-entry.send:
			if !entry.Channel.IsClosed() {
				_ = entry.Channel.Send(msg)
			}
		}
	}
}

func (entry *edgeRouterEntry) Send(msg *channel2.Message) {
	entry.send <- msg
}

func (m *edgeRouterMap) AddEntry(edgeRouterEntry *edgeRouterEntry) {
	m.internalMap.Store(edgeRouterEntry.EdgeRouter.Id, edgeRouterEntry)
	edgeRouterEntry.Start()
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
	entry := m.GetEntry(edgeRouterId)
	if entry != nil {
		entry.Stop()
		m.internalMap.Delete(edgeRouterId)
	}
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

type Broker struct {
	ae                  *AppEnv
	events              map[events.EventEmmiter]map[events.EventName][]events.Listener
	edgeRouterMap       *edgeRouterMap
	sessionChunkSize    int
	apiSessionChunkSize int
	routerMsgBufferSize int
}

func NewBroker(ae *AppEnv) *Broker {
	b := &Broker{
		ae: ae,
		edgeRouterMap: &edgeRouterMap{
			internalMap: &sync.Map{},
		},
		sessionChunkSize:    100,
		apiSessionChunkSize: 100,
		routerMsgBufferSize: 100,
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
		b.ae.GetStores().ApiSessionCertificate: {
			boltz.EventCreate: []events.Listener{
				b.apiSessionCertificateEventHandler,
			},
			boltz.EventDelete: []events.Listener{
				b.apiSessionCertificateEventHandler,
			},
		},
	}

	b.registerEventHandlers()

	ae.HostController.GetNetwork().AddRouterPresenceHandler(b)

	return b
}

func (b *Broker) AddEdgeRouter(ch channel2.Channel, edgeRouter *model.EdgeRouter) {
	logger := pfxlog.Logger()
	edgeRouterEntry := newEdgeRouterEntry(edgeRouter, ch, b.routerMsgBufferSize)
	edgeRouterEntry.Start()
	b.edgeRouterMap.AddEntry(edgeRouterEntry)

	//stream so we don't hold hundreds of thousands in memory at once
	beforeFullStreamApiSessionTime := time.Now()
	err := b.streamApiSessions(b.apiSessionChunkSize, func(addedMsg *edge_ctrl_pb.ApiSessionAdded) {
		logger.Infof("sending edge router session updates [%d]", len(addedMsg.ApiSessions))

		if buf, err := proto.Marshal(addedMsg); err == nil {

			beforeApiSessionSend := time.Now()

			msg := channel2.NewMessage(ApiSessionAddedType, buf)
			edgeRouterEntry.Send(msg)

			apiSessionSendDelta := time.Now().Sub(beforeApiSessionSend)
			logger.Debugf("broker api session send timing (micro-sec): %v\n", apiSessionSendDelta.Microseconds())
		} else {
			logger.WithError(err).Error("error sending session added, could not marshal message content")
		}
	})

	fullStreamApiSessionDelta := time.Now().Sub(beforeFullStreamApiSessionTime)
	logger.Debugf("broker api session FULL stream timing (micro-sec): %v\n", fullStreamApiSessionDelta.Microseconds())

	if err != nil {
		logger.WithError(err).Error("could not get stream sessions")
		return
	}

	beforeFullStreamSessionTime := time.Now()
	err = b.streamSessions(edgeRouter.Id, b.sessionChunkSize, func(addedMsg *edge_ctrl_pb.SessionAdded) {
		logger.Infof("sending edge router network session updates [%d]", len(addedMsg.Sessions))
		if buf, err := proto.Marshal(addedMsg); err == nil {
			beforeSessionSend := time.Now()

			msg := channel2.NewMessage(SessionAddedType, buf)
			edgeRouterEntry.Send(msg)

			sessionSendDelta := time.Now().Sub(beforeSessionSend)
			logger.Debugf(fmt.Sprintf("broker session send timing (micro-sec): %v\n", sessionSendDelta.Microseconds()))
		} else {
			logger.WithError(err).Error("error sending network session added, could not marshal message content")
		}
	})
	fullStreamSessionTimeDelta := time.Now().Sub(beforeFullStreamSessionTime)
	logger.Debugf(fmt.Sprintf("broker session FULL stream timing (micro-sec): %v\n", fullStreamSessionTimeDelta.Microseconds()))

	if err != nil {
		logger.WithError(err).Errorf("could not stream current network sessions for edge router id [%s], connected but potentially out of sync", edgeRouter.Id)
		return
	}
	logger.Infof("edge router connection finalized and synchronized [%s] [%s]", edgeRouter.Id, edgeRouter.Name)
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

	fingerprints, err := b.getFingerprints(apiSession.IdentityId, apiSession.Id)
	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not get session fingerprints")
		return
	}

	if len(fingerprints) == 0 {
		return
	}

	apiSessionMsg.ApiSessions = append(apiSessionMsg.ApiSessions, &edge_ctrl_pb.ApiSession{
		Token:            apiSession.Token,
		CertFingerprints: fingerprints,
		Id:               apiSession.Id,
	})

	byteMsg, err := proto.Marshal(apiSessionMsg)

	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not marshal api session added message")
		return
	}

	channelMsg := channel2.NewMessage(ApiSessionAddedType, byteMsg)

	b.sendToAllEdgeRouters(channelMsg)
}

// todo: use this once cert rolling happens
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

	fingerprints, err := b.getFingerprints(apiSession.IdentityId, apiSession.Id)
	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not get api session fingerprints")
		return
	}
	
	if len(fingerprints) == 0 {
		pfxlog.Logger().WithError(err).Debug("api session has no fingerprints, not sending to edge routers")
		return
	}

	apiSessionMsg.ApiSessions = append(apiSessionMsg.ApiSessions, &edge_ctrl_pb.ApiSession{
		Token:            apiSession.Token,
		CertFingerprints: fingerprints,
		Id:               apiSession.Id,
	})

	byteMsg, err := proto.Marshal(apiSessionMsg)

	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not marshal api session updated message")
		return
	}

	channelMsg := channel2.NewMessage(ApiSessionUpdatedType, byteMsg)

	b.sendToAllEdgeRouters(channelMsg)
}

func (b *Broker) sendToAllEdgeRouters(msg *channel2.Message) {
	b.edgeRouterMap.RangeEdgeRouterEntries(func(edgeRouterEntry *edgeRouterEntry) bool {
		edgeRouterEntry.Send(msg)
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
			edgeRouterEntry.Send(msg)
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

	fps, err := b.getFingerprintsByApiSessionId(session.ApiSessionId)

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
		Id:               session.Id,
		Token:            session.Token,
		Service:          svc,
		CertFingerprints: fps,
		Type:             sessionType,
		ApiSessionId:     session.ApiSessionId,
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
		Name:               service.Name,
		Id:                 service.Id,
		EncryptionRequired: service.EncryptionRequired,
	}, nil
}

func (b *Broker) registerEventHandlers() {
	for emitter, eventNameMap := range b.events {
		for en, ls := range eventNameMap {
			for _, l := range ls {
				emitter.AddListener(en, l)
			}
		}
	}
}

func (b *Broker) streamApiSessions(chunkSize int, callback func(addedMsg *edge_ctrl_pb.ApiSessionAdded)) error {
	ret := &edge_ctrl_pb.ApiSessionAdded{
		IsFullState: true,
	}
	logger := pfxlog.Logger()

	return b.ae.GetHandlers().ApiSession.Stream("true", func(apiSession *model.ApiSession, err error) error {
		if err != nil {
			logger.Errorf("error reading api sessions for router: %v", err)
			return nil
		}

		if apiSession == nil {
			//done
			callback(ret)
			return nil
		}

		apiSessionProto, err := b.modelApiSessionToProto(apiSession.Token, apiSession.IdentityId, apiSession.Id)

		if err != nil {
			logger.Errorf("error converting api session to proto: %v", err)
			return nil
		}

		ret.ApiSessions = append(ret.ApiSessions, apiSessionProto)

		if len(ret.ApiSessions) > chunkSize {
			callback(ret)
			ret = &edge_ctrl_pb.ApiSessionAdded{}
		}
		return nil
	})
}

func (b *Broker) streamSessions(edgeRouterId string, chunkSize int, callback func(*edge_ctrl_pb.SessionAdded)) error {
	logger := pfxlog.Logger()

	ret := &edge_ctrl_pb.SessionAdded{
		IsFullState: true,
	}

	return b.ae.Handlers.Session.StreamAll(func(session *model.Session, err error) error {
		if err != nil {
			logger.Errorf("error reading sessions for router [%s]: %v", edgeRouterId, err)
			return nil
		}

		if session == nil {
			//done
			callback(ret)
			return nil
		}

		sessionProto, err := b.modelSessionToProto(session)

		if err != nil {
			logger.Errorf("error converting session to proto: %v", err)
			return nil
		}

		ret.Sessions = append(ret.Sessions, sessionProto)

		if len(ret.Sessions) > chunkSize {
			callback(ret)
			ret = &edge_ctrl_pb.SessionAdded{}
		}

		return nil
	})
}

func (b *Broker) getFingerprints(identityId, apiSessionId string) ([]string, error) {
	identityPrints, err := b.getIdentityAuthenticatorFingerprints(identityId)

	if err != nil {
		return nil, err
	}

	apiSessionPrints, err := b.getApiSessionCertificateFingerprints(apiSessionId)

	if err != nil {
		return nil, err
	}

	for _, apiSessionPrint := range apiSessionPrints {
		identityPrints = append(identityPrints, apiSessionPrint)
	}

	return identityPrints, nil
}

func (b *Broker) getFingerprintsByApiSessionId(apiSessionId string) ([]string, error) {
	apiSession, err := b.ae.GetHandlers().ApiSession.Read(apiSessionId)

	if err != nil {
		return nil, fmt.Errorf("could not query fingerprints by api session id [%s]: %s", apiSessionId, err)
	}

	return b.getFingerprints(apiSession.IdentityId, apiSessionId)
}

func (b *Broker) getIdentityAuthenticatorFingerprints(identityId string) ([]string, error) {
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

func (b *Broker) getApiSessionCertificateFingerprints(apiSessionId string) ([]string, error) {
	apiSessionCerts, err := b.ae.GetHandlers().ApiSessionCertificate.ReadByApiSessionId(apiSessionId)

	if err != nil {
		return nil, err
	}

	var validPrints []string

	now := time.Now()
	for _, apiSessionCert := range apiSessionCerts {
		if apiSessionCert.ValidAfter != nil && now.After(*apiSessionCert.ValidAfter) &&
			apiSessionCert.ValidBefore != nil && now.Before(*apiSessionCert.ValidBefore) {
			validPrints = append(validPrints, apiSessionCert.Fingerprint)
		}
	}

	return validPrints, nil
}

func (b *Broker) modelApiSessionToProto(token, identityId, apiSessionId string) (*edge_ctrl_pb.ApiSession, error) {
	fingerprints, err := b.getFingerprints(identityId, apiSessionId)
	if err != nil {
		return nil, err
	}

	return &edge_ctrl_pb.ApiSession{
		Token:            token,
		CertFingerprints: fingerprints,
		Id:               apiSessionId,
	}, nil
}

func (b *Broker) modelSessionToProto(ns *model.Session) (*edge_ctrl_pb.Session, error) {
	service, err := b.ae.Handlers.EdgeService.Read(ns.ServiceId)
	if err != nil {
		return nil, fmt.Errorf("could not convert to session proto, could not find service: %s", err)
	}

	fps, err := b.getFingerprintsByApiSessionId(ns.ApiSessionId)

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
		Id:               ns.Id,
		Token:            ns.Token,
		Service:          svc,
		CertFingerprints: fps,
		Type:             sessionType,
		ApiSessionId:     ns.ApiSessionId,
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
		if r.Fingerprint != nil {
			if edgeRouter, _ := b.ae.Handlers.EdgeRouter.ReadOneByFingerprint(*r.Fingerprint); edgeRouter != nil {
				b.sendHello(r)
			}
		}
	}()
}

func (b *Broker) sendHello(r *network.Router) {
	serverVersion := build.GetBuildInfo().Version()
	serverHello := &edge_ctrl_pb.ServerHello{
		Version: serverVersion,
	}

	buf, err := proto.Marshal(serverHello)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("could not marshal serverHello")
		return
	}

	if err = r.Control.SendWithTimeout(channel2.NewMessage(ServerHelloType, buf), 10*time.Second); err != nil {
		pfxlog.Logger().WithError(err).Error("timed out sending serverHello message for edge router")
		return
	}
}

func (b *Broker) ReceiveHello(r *network.Router, edgeRouter *model.EdgeRouter, respHello *edge_ctrl_pb.ClientHello) {
	serverVersion := build.GetBuildInfo().Version()

	entry := pfxlog.Logger().
		WithField("hostname", respHello.Hostname).
		WithField("protocols", respHello.Protocols).
		WithField("data", respHello.Data)

	if r.VersionInfo != nil {
		entry.WithField("version", r.VersionInfo.Version).
			WithField("revision", r.VersionInfo.Revision).
			WithField("buildDate", r.VersionInfo.BuildDate).
			WithField("os", r.VersionInfo.OS).
			WithField("arch", r.VersionInfo.Arch)
	}

	entry.Info("edge router responded with client hello")

	protocols := map[string]string{}
	for _, p := range respHello.Protocols {
		ingressUrl := fmt.Sprintf("%s://%s", p, respHello.Hostname)
		protocols[p] = ingressUrl
	}

	edgeRouter.Hostname = &respHello.Hostname
	edgeRouter.EdgeRouterProtocols = protocols
	edgeRouter.VersionInfo = r.VersionInfo

	//todo: restrict version?
	pfxlog.Logger().Infof("edge router connecting with version [%s] to controller with version [%s]", respHello.Version, serverVersion)

	b.AddEdgeRouter(r.Control, edgeRouter)
}

func (b *Broker) RouterDisconnected(r *network.Router) {
	go func() {
		if r.Fingerprint != nil {
			if edgeRouter, _ := b.ae.Handlers.EdgeRouter.ReadOneByFingerprint(*r.Fingerprint); edgeRouter != nil {
				b.edgeRouterMap.RemoveEntry(edgeRouter.Id)
			}
		}
	}()
}

func (b *Broker) apiSessionCertificateEventHandler(args ...interface{}) {
	var apiSessionCert *persistence.ApiSessionCertificate
	if len(args) == 1 {
		apiSessionCert, _ = args[0].(*persistence.ApiSessionCertificate)
	}

	if apiSessionCert == nil {
		log := pfxlog.Logger()
		log.Error("could not cast event args to event details: create/delete api session cert")
		return
	}
	var apiSession *persistence.ApiSession
	var err error
	err = b.ae.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		apiSession, err = b.ae.GetStores().ApiSession.LoadOneById(tx, apiSessionCert.ApiSessionId)
		return err
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Error("could not process API Session certificate creation. Failed to query for parent API Session")
		return
	}

	b.sendApiSessionUpdates(apiSession)
}
