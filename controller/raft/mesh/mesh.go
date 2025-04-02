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

package mesh

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/ziti/controller/event"
	"math/rand"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/raft"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/identity"
	"github.com/openziti/transport/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	PeerAddrHeader     = 11
	SigningCertHeader  = 12
	ApiAddressesHeader = 13
	RaftConnIdHeader   = 14
	ClusterIdHeader    = 15

	RaftConnectType    = 2048
	RaftDataType       = 2049
	RaftDisconnectType = 2052

	ChannelTypeMesh = "ctrl.mesh"
)

type Peer struct {
	mesh          *impl
	Id            raft.ServerID
	Address       string
	Channel       channel.Channel
	RaftConns     concurrenz.CopyOnWriteMap[uint32, *raftPeerConn]
	Version       *versions.VersionInfo
	SigningCerts  []*x509.Certificate
	ApiAddresses  map[string][]event.ApiAddress
	raftPeerIdGen uint32
}

func (self *Peer) nextRaftPeerId() uint32 {
	// dialing peers use odd ids, dialed peers use even, so we
	// shouldn't get any conflict
	return atomic.AddUint32(&self.raftPeerIdGen, 2)
}

func (self *Peer) newRaftPeerConn(id uint32) *raftPeerConn {
	result := &raftPeerConn{
		id:          id,
		peer:        self,
		localAddr:   self.mesh.netAddr,
		readTimeout: newDeadline(),
		readC:       make(chan []byte, 16),
		closeNotify: make(chan struct{}),
	}
	self.RaftConns.Put(id, result)
	return result
}

func (self *Peer) HandleClose(channel.Channel) {
	conns := self.RaftConns.AsMap()
	self.RaftConns.Clear()
	for _, v := range conns {
		v.close()
	}
	self.mesh.PeerDisconnected(self)
}

func (self *Peer) handleReceiveConnect(m *channel.Message, ch channel.Channel) {
	go func() {
		log := pfxlog.Logger().WithField("peerId", ch.Id())
		log.Info("received connect request from raft peer")

		id, ok := m.GetUint32Header(RaftConnIdHeader)
		if !ok {
			response := channel.NewResult(false, "no conn id in connect request")
			response.ReplyTo(m)

			if err := response.WithTimeout(5 * time.Second).Send(self.Channel); err != nil {
				log.WithError(err).Error("failed to send raft peer connect error response")
			}
			return
		}

		if peerConn := self.RaftConns.Get(id); peerConn != nil {
			response := channel.NewResult(false, "duplicate conn id in connect request")
			response.ReplyTo(m)

			if err := response.WithTimeout(5 * time.Second).Send(self.Channel); err != nil {
				log.WithError(err).Error("failed to send raft peer connect error response")
			}
			return
		}

		response := channel.NewResult(true, "")
		response.ReplyTo(m)

		if err := response.WithTimeout(5 * time.Second).Send(self.Channel); err != nil {
			log.WithError(err).Error("failed to send raft peer connect response")
		} else {
			conn := self.newRaftPeerConn(id)
			select {
			case self.mesh.raftAccepts <- conn:
				log.Info("raft peer connection sent to listener")
			case <-self.mesh.closeNotify:
				log.Info("unable to send raft peer connection to listener, listener closed")
			}
		}
	}()
}

func (self *Peer) handleReceiveDisconnect(m *channel.Message, ch channel.Channel) {
	go func() {
		log := pfxlog.ContextLogger(ch.Label())

		id, ok := m.GetUint32Header(RaftConnIdHeader)
		if !ok {
			response := channel.NewResult(false, "no conn id in disconnect request")
			response.ReplyTo(m)

			if err := response.WithTimeout(5 * time.Second).Send(self.Channel); err != nil {
				log.WithError(err).Error("failed to send raft peer connect error response")
			}
			return
		}

		if conn := self.RaftConns.Get(id); conn != nil {
			conn.close()
		}

		response := channel.NewResult(true, "")
		response.ReplyTo(m)

		if err := response.WithTimeout(5 * time.Second).Send(self.Channel); err != nil {
			log.WithError(err).Error("failed to send close response, closing channel")
			if closeErr := self.Channel.Close(); closeErr != nil {
				pfxlog.Logger().WithError(closeErr).WithField("ch", self.Channel.Label()).Error("failed to close channel")
			}
		}

		log.Infof("received disconnect, disconnected peer %v at %v", self.Id, self.Address)
	}()
}

func (self *Peer) handleReceiveData(m *channel.Message, ch channel.Channel) {
	id, ok := m.GetUint32Header(RaftConnIdHeader)
	if !ok {
		pfxlog.Logger().WithField("peerId", ch.Id()).Error("no conn id in data request")
		return
	}

	conn := self.RaftConns.Get(id)
	if conn == nil {
		pfxlog.Logger().WithField("peerId", ch.Id()).
			WithField("connId", id).Error("invalid conn id in data request")
		return
	}

	conn.HandleReceive(m, ch)
}

func (self *Peer) Connect(timeout time.Duration) (net.Conn, error) {
	log := pfxlog.Logger().WithField("peerId", string(self.Id)).WithField("address", self.Address)
	log.Info("sending connect msg to raft peer")

	id := self.nextRaftPeerId()
	msg := channel.NewMessage(RaftConnectType, nil)
	msg.Headers.PutUint32Header(RaftConnIdHeader, id)

	response, err := msg.WithTimeout(timeout).SendForReply(self.Channel)
	if err != nil {
		log.WithError(err).Error("failed to send connect message to raft peer, closing channel")
		if closeErr := self.Channel.Close(); closeErr != nil {
			log.WithError(closeErr).Error("failed to close raft peer channel")
		}
		return nil, err
	}
	result := channel.UnmarshalResult(response)
	if !result.Success {
		log.WithError(err).Error("non-success response to raft peer connect message, closing channel")
		if closeErr := self.Channel.Close(); closeErr != nil {
			log.WithError(closeErr).Error("failed to close raft peer channel")
		}
		return nil, errors.Errorf("raft peer connect failed: %v", result.Message)
	}

	log.Info("raft peer connected")

	return self.newRaftPeerConn(id), nil
}

func (self *Peer) closeRaftConn(peerConn *raftPeerConn, timeout time.Duration) error {
	isCurrentPeer := self.RaftConns.DeleteIf(func(key uint32, val *raftPeerConn) bool {
		return key == peerConn.id && val == peerConn
	})

	peerConn.close()

	log := pfxlog.Logger().WithField("peerId", self.Id)
	if !isCurrentPeer {
		log.Info("closed peer connection is not current connection, not sending disconnect message")
		return nil
	}

	log.Info("closed peer connection is current connection, sending disconnect message")

	msg := channel.NewMessage(RaftDisconnectType, nil)
	msg.Headers.PutUint32Header(RaftConnIdHeader, peerConn.id)

	response, err := msg.WithTimeout(timeout).SendForReply(self.Channel)
	if err != nil {
		log.WithError(err).Error("failed to send disconnect msg response, closing channel")
		if closeErr := self.Channel.Close(); closeErr != nil {
			log.WithError(closeErr).Error("failed to close channel")
		}
		return err
	}
	result := channel.UnmarshalResult(response)
	if !result.Success {
		log.WithError(err).Error("result from disconnect was not success, closing channel")
		if closeErr := self.Channel.Close(); closeErr != nil {
			log.WithError(closeErr).Error("failed to close channel")
		}
		return errors.Errorf("close failed: %v", result.Message)
	}

	logrus.Infof("disconnected peer %v at %v", self.Id, self.Address)

	return nil
}

type meshAddr struct {
	network string
	addr    string
}

func (self meshAddr) Network() string {
	return self.network
}

func (self meshAddr) String() string {
	return self.addr
}

type ClusterState uint32

const (
	ClusterReadWrite ClusterState = 0
	ClusterReadOnly  ClusterState = 1
)

type Env interface {
	GetNodeId() *identity.TokenId
	GetClusterId() string
	GetVersionProvider() versions.VersionProvider
	GetEventDispatcher() event.Dispatcher
	IsPeerMember(id string) bool
	IsLeader() bool
}

// Mesh provides the networking layer to raft
type Mesh interface {
	raft.StreamLayer

	channel.UnderlayAcceptor

	// GetOrConnectPeer returns a peer for the given address. If a peer has already been established,
	// it will be returned, otherwise a new connection will be established
	GetOrConnectPeer(address string, timeout time.Duration) (*Peer, error)
	IsReadOnly() bool

	GetPeerInfo(address string, timeout time.Duration) (raft.ServerID, raft.ServerAddress, error)
	GetAdvertiseAddr() raft.ServerAddress
	GetPeers() map[string]*Peer

	RegisterClusterStateHandler(f func(state ClusterState))
	Init(bindHandler channel.BindHandler)
}

func New(env Env, raftAddr raft.ServerAddress, helloHeaderProviders []HeaderProvider) Mesh {
	versionEncoded, err := env.GetVersionProvider().EncoderDecoder().Encode(env.GetVersionProvider().AsVersionInfo())
	if err != nil {
		panic(err)
	}

	return &impl{
		env:      env,
		nodeId:   env.GetNodeId(),
		raftAddr: raftAddr,
		netAddr: &meshAddr{
			network: "mesh",
			addr:    string(raftAddr),
		},
		Peers:                map[string]*Peer{},
		closeNotify:          make(chan struct{}),
		raftAccepts:          make(chan *raftPeerConn),
		version:              env.GetVersionProvider(),
		versionEncoded:       versionEncoded,
		eventDispatcher:      env.GetEventDispatcher(),
		helloHeaderProviders: helloHeaderProviders,
	}
}

type impl struct {
	env                  Env
	nodeId               *identity.TokenId
	raftAddr             raft.ServerAddress
	netAddr              net.Addr
	Peers                map[string]*Peer
	lock                 sync.RWMutex
	closeNotify          chan struct{}
	closed               atomic.Bool
	raftAccepts          chan *raftPeerConn
	bindHandler          concurrenz.AtomicValue[channel.BindHandler]
	version              versions.VersionProvider
	versionEncoded       []byte
	readonly             atomic.Bool
	clusterStateHandlers concurrenz.CopyOnWriteSlice[func(state ClusterState)]
	eventDispatcher      event.Dispatcher
	helloHeaderProviders []HeaderProvider
}

func (self *impl) RegisterClusterStateHandler(f func(state ClusterState)) {
	self.clusterStateHandlers.Append(f)
}

func (self *impl) Init(bindHandler channel.BindHandler) {
	if self.bindHandler.Load() == nil {
		self.bindHandler.Store(bindHandler)
	}
}

func (self *impl) GetAdvertiseAddr() raft.ServerAddress {
	return self.raftAddr
}

func (self *impl) Close() error {
	if self.closed.CompareAndSwap(false, true) {
		close(self.closeNotify)
	}

	for _, p := range self.GetPeers() {
		if err := p.Channel.Close(); err != nil {
			pfxlog.Logger().WithError(err).Error("failed to close ctrl mesh peer channel")
		}
	}

	return nil
}

func (self *impl) Addr() net.Addr {
	return self.netAddr
}

func (self *impl) Accept() (net.Conn, error) {
	select {
	case conn := <-self.raftAccepts:
		pfxlog.Logger().WithField("peerId", conn.peer.Id).Info("new raft peer connection return to raft layer")
		return conn, nil
	case <-self.closeNotify:
		pfxlog.Logger().Error("return error from raft peer mesh listener accept, listener closed")
		return nil, errors.New("raft peer listener closed")
	}
}

func (self *impl) Dial(address raft.ServerAddress, timeout time.Duration) (net.Conn, error) {
	if self.closed.Load() {
		return nil, errors.New("ctrl mesh is closed")
	}

	log := pfxlog.Logger().WithField("address", address)
	log.Info("dialing raft peer channel")
	peer, err := self.GetOrConnectPeer(string(address), timeout)
	if err != nil {
		log.WithError(err).Error("unable to get or connect raft peer channel")
		return nil, err
	}

	log.WithField("peerId", peer.Id).Info("invoking raft connect on established peer channel")

	return peer.Connect(timeout)
}

func (self *impl) GetOrConnectPeer(address string, timeout time.Duration) (*Peer, error) {
	log := pfxlog.Logger().WithField("address", address)

	if address == "" {
		return nil, errors.New("cannot get raft peer for empty address")
	}

	if peer := self.GetPeer(raft.ServerAddress(address)); peer != nil {
		log.Debug("existing new raft peer channel found for address")
		return peer, nil
	}

	log.Info("establishing new raft peer channel")

	addr, err := transport.ParseAddress(address)
	if err != nil {
		return nil, err
	}

	tlsCert := self.nodeId.ServerCert()
	var serverCert []byte
	if len(tlsCert) != 0 && len(tlsCert[0].Certificate) != 0 {
		serverCert = tlsCert[0].Certificate[0]
	}

	headers := map[int32][]byte{
		channel.HelloVersionHeader: self.versionEncoded,
		channel.TypeHeader:         []byte(ChannelTypeMesh),
		PeerAddrHeader:             []byte(self.raftAddr),
		SigningCertHeader:          serverCert,
		ClusterIdHeader:            []byte(self.env.GetClusterId()),
	}

	for _, headerProvider := range self.helloHeaderProviders {
		headerProvider.Apply(headers)
	}

	dialer := channel.NewClassicDialer(channel.DialerConfig{
		Identity: self.nodeId,
		Endpoint: addr,
		Headers:  headers,
		TransportConfig: transport.Configuration{
			transport.KeyProtocol: "ziti-ctrl",
		},
	})
	dialOptions := channel.DefaultOptions()
	dialOptions.ConnectOptions.ConnectTimeout = timeout

	peer := &Peer{
		mesh:          self,
		Address:       address,
		raftPeerIdGen: 1,
	}

	bindHandler := channel.BindHandlerF(func(binding channel.Binding) error {
		if self.bindHandler.Load() == nil {
			return errors.New("bindHandler not initialized, cannot initialize new channels")
		}
		if err = self.bindHandler.Load().BindChannel(binding); err != nil {
			return err
		}

		peer.Channel = binding.GetChannel()

		if err = self.validateConnection(peer.Channel); err != nil {
			return err
		}

		underlay := binding.GetChannel().Underlay()
		id, err := self.extractPeerId(underlay.GetRemoteAddr().String(), underlay.Certificates())
		if err != nil {
			return err
		}

		peer.Id = raft.ServerID(id)

		versionEncoded, found := peer.Channel.Underlay().Headers()[channel.HelloVersionHeader]
		if !found {
			return errors.New("no version header supplied in hello response, can't bind peer")
		}
		versionInfo, err := self.version.EncoderDecoder().Decode(versionEncoded)
		if err != nil {
			return errors.Wrap(err, "can't decode version from returned from peer")
		}

		if apiAddressBytes, found := peer.Channel.Underlay().Headers()[ApiAddressesHeader]; found {
			err := json.Unmarshal(apiAddressBytes, &peer.ApiAddresses)
			if err != nil {
				pfxlog.Logger().WithError(err).Error("could not unmarshal api address header")
			}
		}

		peer.Version = versionInfo
		peer.SigningCerts = []*x509.Certificate{underlay.Certificates()[0]}

		binding.AddReceiveHandlerF(RaftDataType, peer.handleReceiveData)
		binding.AddReceiveHandlerF(RaftConnectType, peer.handleReceiveConnect)
		binding.AddReceiveHandlerF(RaftDisconnectType, peer.handleReceiveDisconnect)
		binding.AddCloseHandler(peer)

		return self.PeerConnected(peer, true)
	})

	if _, err = channel.NewChannel(ChannelTypeMesh, dialer, bindHandler, channel.DefaultOptions()); err != nil {
		// introduce random delay in case ctrls are dialing each other and closing each other's connections
		time.Sleep(time.Duration(rand.Intn(250)+1) * time.Millisecond)
		return nil, errors.Wrapf(err, "error dialing peer %v", address)
	}

	log.WithField("peerId", peer.Id).Info("established new raft peer channel")

	return peer, nil
}

func (self *impl) validateConnection(ch channel.Channel) error {
	if err := self.checkClusterIds(ch); err != nil {
		return err
	}

	return self.checkCerts(ch)
}

func (self *impl) checkClusterIds(ch channel.Channel) error {
	clusterId := string(ch.Underlay().Headers()[ClusterIdHeader])
	if clusterId != "" && self.env.GetClusterId() != "" && clusterId != self.env.GetClusterId() {
		return fmt.Errorf("local cluster id %s doesn't match peer cluster id %s", self.env.GetClusterId(), clusterId)
	}
	return nil
}

func (self *impl) checkCerts(ch channel.Channel) error {
	certs := ch.Underlay().Certificates()
	if len(certs) == 0 {
		return errors.New("unable to validate peer connection, no certs presented")
	}

	for _, cert := range ch.Underlay().Certificates() {
		if _, err := self.env.GetNodeId().CaPool().VerifyToRoot(cert); err == nil {
			return nil
		}
	}

	return errors.New("unable to validate peer connection, no certs presented matched the CA for this node")
}

func (self *impl) GetPeerInfo(address string, timeout time.Duration) (raft.ServerID, raft.ServerAddress, error) {
	log := pfxlog.Logger().WithField("address", address)
	addr, err := transport.ParseAddress(address)
	if err != nil {
		log.WithError(err).Error("failed to parse address")
		return "", "", err
	}

	headers := map[int32][]byte{
		channel.HelloVersionHeader: self.versionEncoded,
		channel.TypeHeader:         []byte(ChannelTypeMesh),
		PeerAddrHeader:             []byte(self.raftAddr),
		ClusterIdHeader:            []byte(self.env.GetClusterId()),
	}

	for _, headerProvider := range self.helloHeaderProviders {
		headerProvider.Apply(headers)
	}

	dialer := channel.NewClassicDialer(channel.DialerConfig{
		Identity: self.nodeId,
		Endpoint: addr,
		Headers:  headers,
		TransportConfig: transport.Configuration{
			transport.KeyProtocol: "ziti-ctrl",
		},
	})
	dialOptions := channel.DefaultOptions()
	dialOptions.ConnectOptions.ConnectTimeout = timeout

	var peerId raft.ServerID
	var peerAddr raft.ServerAddress

	markerErr := errors.New("closing, after peer information extracted")

	bindHandler := channel.BindHandlerF(func(binding channel.Binding) error {
		underlay := binding.GetChannel().Underlay()
		id, err := self.extractPeerId(underlay.GetRemoteAddr().String(), underlay.Certificates())
		if err != nil {
			return err
		}

		if err = self.validateConnection(binding.GetChannel()); err != nil {
			return err
		}

		peerId = raft.ServerID(id)
		peerAddr = raft.ServerAddress(underlay.Headers()[PeerAddrHeader])

		return markerErr
	})

	if _, err = channel.NewChannel(ChannelTypeMesh, dialer, bindHandler, channel.DefaultOptions()); err != markerErr {
		return "", "", errors.Wrapf(err, "unable to dial %v", address)
	}

	if peerAddr == "" {
		return "", "", errors.Errorf("peer at %v did not supply advertise address", addr)
	}

	return peerId, peerAddr, nil
}

func (self *impl) extractPeerId(peerAddr string, certs []*x509.Certificate) (string, error) {
	if len(certs) == 0 {
		return "", errors.Errorf("no certificates for peer at %v", peerAddr)
	}

	return ExtractSpiffeId(certs)
}

func ExtractSpiffeId(certs []*x509.Certificate) (string, error) {
	if len(certs) > 0 {
		leaf := certs[0]
		for _, uri := range leaf.URIs {
			if uri.Scheme == "spiffe" && strings.HasPrefix(uri.Path, "/controller/") {
				return strings.TrimPrefix(uri.Path, "/controller/"), nil
			}
		}
	}

	return "", errors.New("invalid controller certificate, no controller SPIFFE ID in cert")
}

func (self *impl) PeerConnected(peer *Peer, dial bool) error {
	self.lock.Lock()
	if self.Peers[peer.Address] != nil {
		defer self.lock.Unlock()
		return fmt.Errorf("connection from peer %v @ %v already present", peer.Id, peer.Address)
	}

	self.Peers[peer.Address] = peer
	self.updateClusterState()
	self.lock.Unlock()

	pfxlog.Logger().WithField("peerId", peer.Id).
		WithField("peerAddr", peer.Address).
		Info("peer connected")

	evt := event.NewClusterEvent(event.ClusterPeerConnected)
	evt.Peers = self.GetEventPeerList(peer)

	self.eventDispatcher.AcceptClusterEvent(evt)

	if !dial {
		srcAddr := ""
		dstAddr := ""
		if ch := peer.Channel; ch != nil {
			srcAddr = ch.Underlay().GetRemoteAddr().String()
			dstAddr = ch.Underlay().GetLocalAddr().String()
		}
		connectEvent := &event.ConnectEvent{
			Namespace: event.ConnectEventNS,
			SrcType:   event.ConnectSourcePeer,
			DstType:   event.ConnectDestinationController,
			SrcId:     string(peer.Id),
			SrcAddr:   srcAddr,
			DstId:     self.nodeId.Token,
			DstAddr:   dstAddr,
			Timestamp: time.Now(),
		}

		self.eventDispatcher.AcceptConnectEvent(connectEvent)
	}

	return nil
}

func (self *impl) GetEventPeerList(peers ...*Peer) []*event.ClusterPeer {
	if len(peers) == 0 {
		return nil
	}
	var result []*event.ClusterPeer
	for _, peer := range peers {
		result = append(result, &event.ClusterPeer{
			Id:           string(peer.Id),
			Addr:         peer.Address,
			Version:      peer.Version.Version,
			ServerCert:   peer.SigningCerts,
			ApiAddresses: peer.ApiAddresses,
		})
	}
	return result
}

func (self *impl) GetPeer(addr raft.ServerAddress) *Peer {
	self.lock.RLock()
	defer self.lock.RUnlock()
	return self.Peers[string(addr)]
}

func (self *impl) PeerDisconnected(peer *Peer) {
	self.lock.Lock()
	currentPeer := self.Peers[peer.Address]
	if currentPeer == nil || currentPeer != peer {
		self.lock.Unlock()
		return
	}

	delete(self.Peers, peer.Address)
	self.updateClusterState()
	self.lock.Unlock()

	pfxlog.Logger().WithField("peerId", peer.Id).
		WithField("peerAddr", peer.Address).
		Info("peer disconnected")

	evt := event.NewClusterEvent(event.ClusterPeerDisconnected)
	evt.Peers = self.GetEventPeerList(peer)

	self.eventDispatcher.AcceptClusterEvent(evt)
}

func (self *impl) updateClusterState() {
	readOnlyPrevious := self.readonly.Load()

	log := pfxlog.Logger()
	readOnly := false
	for _, p := range self.Peers {
		if self.version.Version() != p.Version.Version {
			if !readOnlyPrevious {
				log.Infof("peer %v has version %v, not matching local version %v, entering read-only mode", p.Id, p.Version, self.version)
			}
			readOnly = true
		}
	}

	self.readonly.Store(readOnly)

	if readOnlyPrevious != readOnly {
		for _, handler := range self.clusterStateHandlers.Value() {
			if readOnly {
				handler(ClusterReadOnly)
			} else {
				log.Info("cluster back at uniform version, exiting read-only mode")
				handler(ClusterReadWrite)
			}
		}
	}
}

func (self *impl) AcceptUnderlay(underlay channel.Underlay) error {
	log := pfxlog.Logger()
	log.Info("started")
	defer log.Warn("exited")

	peer := &Peer{
		mesh: self,
	}

	bindHandler := channel.BindHandlerF(func(binding channel.Binding) error {
		ch := binding.GetChannel()
		addr := string(ch.Underlay().Headers()[PeerAddrHeader])

		id, err := self.extractPeerId(underlay.GetRemoteAddr().String(), underlay.Certificates())
		if err != nil {
			return err
		}

		if id == "" || addr == "" {
			_ = ch.Close()
			return errors.Errorf("connection didn't provide id '%v' or address '%v', closing connection", id, addr)
		}

		bh := self.bindHandler.Load()
		if bh == nil {
			return errors.New("bindHandler not initialized, can't accept controller connection")
		}
		if err = binding.Bind(bh); err != nil {
			_ = ch.Close()
			return errors.Wrapf(err, "error while binding channel from id '%v' or address '%v', closing connection", id, addr)
		}

		peer.Id = raft.ServerID(id)
		peer.Address = addr
		peer.Channel = ch

		versionEncoded, found := ch.Underlay().Headers()[channel.HelloVersionHeader]
		if !found {
			return errors.New("no version header supplied in hello, can't bind peer")
		}

		versionInfo, err := self.version.EncoderDecoder().Decode(versionEncoded)
		if err != nil {
			return errors.Wrap(err, "can't decode version from returned from dialing peer")
		}

		certHeader, found := ch.Underlay().Headers()[SigningCertHeader]
		if found {
			if cert, err := x509.ParseCertificate(certHeader); err == nil {
				peer.SigningCerts = append(peer.SigningCerts, cert)
			}
		}

		apiAddressesHeader, found := ch.Underlay().Headers()[ApiAddressesHeader]
		if found {
			if err := json.Unmarshal(apiAddressesHeader, &peer.ApiAddresses); err != nil {
				pfxlog.Logger().WithError(err).Error("could not parse peer api addresses header")
			}
		}

		if err = self.validateConnection(peer.Channel); err != nil {
			return err
		}

		peer.Version = versionInfo
		peer.SigningCerts = []*x509.Certificate{underlay.Certificates()[0]}

		binding.AddReceiveHandlerF(RaftDataType, peer.handleReceiveData)
		binding.AddReceiveHandlerF(RaftConnectType, peer.handleReceiveConnect)
		binding.AddReceiveHandlerF(RaftDisconnectType, peer.handleReceiveDisconnect)
		binding.AddCloseHandler(peer)

		if self.env.IsLeader() && !self.env.IsPeerMember(id) {
			time.AfterFunc(time.Minute, func() {
				if !self.env.IsPeerMember(id) && !binding.GetChannel().IsClosed() {
					logger := pfxlog.Logger().WithField("peer", peer.Id)
					logger.Info("disconnecting non-member peer after 1 minute")
					if err := binding.GetChannel().Close(); err != nil {
						log.WithError(err).Error("error closing channel to non-member peer")
					}

					evt := event.NewClusterEvent(event.ClusterPeerNotMember)
					evt.Peers = self.GetEventPeerList(peer)
					self.env.GetEventDispatcher().AcceptClusterEvent(evt)
				}
			})
		}

		return self.PeerConnected(peer, false)
	})

	_, err := channel.NewChannelWithUnderlay(ChannelTypeMesh, underlay, bindHandler, channel.DefaultOptions())
	if err != nil {
		// introduce random delay in case ctrls are dialing each other and closing each other's connections
		time.Sleep(time.Duration(rand.Intn(250)+1) * time.Millisecond)

		return err
	}

	logrus.Infof("connected peer %v at %v", peer.Id, peer.Address)

	return nil
}

func (self *impl) GetPeers() map[string]*Peer {
	self.lock.RLock()
	defer self.lock.RUnlock()

	result := map[string]*Peer{}

	for k, v := range self.Peers {
		result[k] = v
	}

	return result
}

func (self *impl) IsReadOnly() bool {
	return self.readonly.Load()
}

type HeaderProvider interface {
	Apply(map[int32][]byte)
}

type HeaderProviderFunc func(map[int32][]byte)

func (self HeaderProviderFunc) Apply(headers map[int32][]byte) {
	self(headers)
}
