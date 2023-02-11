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
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/versions"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/raft"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/identity"
	"github.com/openziti/transport/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	PeerAddrHeader = 11

	RaftConnectType = 2048
	RaftDataType    = 2049

	ChannelTypeMesh = "ctrl.mesh"
)

type Peer struct {
	mesh     *impl
	Id       raft.ServerID
	Address  string
	Channel  channel.Channel
	RaftConn *raftPeerConn
	Version  *versions.VersionInfo
}

func (self *Peer) HandleClose(channel.Channel) {
	self.mesh.RemovePeer(self)
}

func (self *Peer) ContentType() int32 {
	return RaftConnectType
}

func (self *Peer) HandleReceive(m *channel.Message, _ channel.Channel) {
	go func() {
		response := channel.NewResult(true, "")
		response.ReplyTo(m)

		if err := response.WithTimeout(5 * time.Second).Send(self.Channel); err != nil {
			logrus.WithError(err).Error("failed to send connect response")
		} else {
			select {
			case self.mesh.raftAccepts <- self.RaftConn:
			case <-self.mesh.closeNotify:
			}
		}
	}()
}

func (self *Peer) Connect(timeout time.Duration) error {
	msg := channel.NewMessage(RaftConnectType, nil)
	response, err := msg.WithTimeout(timeout).SendForReply(self.Channel)
	if err != nil {
		return err
	}
	result := channel.UnmarshalResult(response)
	if !result.Success {
		return errors.Errorf("connect failed: %v", result.Message)
	}

	logrus.Infof("connected peer %v at %v", self.Id, self.Address)

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

// Mesh provides the networking layer to raft
type Mesh interface {
	raft.StreamLayer

	channel.UnderlayAcceptor

	// GetOrConnectPeer returns a peer for the given address. If a peer has already been established,
	// it will be returned, otherwise a new connection will be established
	GetOrConnectPeer(address string, timeout time.Duration) (*Peer, error)
	IsReadOnly() bool

	GetPeerId(address string, timeout time.Duration) (string, error)
	GetAdvertiseAddr() raft.ServerAddress
	GetPeers() map[string]*Peer

	RegisterClusterStateHandler(f func(state ClusterState))
	Init(bindHandler channel.BindHandler)
}

func New(id *identity.TokenId, version versions.VersionProvider, raftAddr raft.ServerAddress) Mesh {
	versionEncoded, err := version.EncoderDecoder().Encode(version.AsVersionInfo())
	if err != nil {
		panic(err)
	}

	return &impl{
		id:       id,
		raftAddr: raftAddr,
		netAddr: &meshAddr{
			network: "mesh",
			addr:    string(raftAddr),
		},
		Peers:          map[string]*Peer{},
		closeNotify:    make(chan struct{}),
		raftAccepts:    make(chan net.Conn),
		version:        version,
		versionEncoded: versionEncoded,
	}
}

type impl struct {
	id                   *identity.TokenId
	raftAddr             raft.ServerAddress
	netAddr              net.Addr
	Peers                map[string]*Peer
	lock                 sync.RWMutex
	closeNotify          chan struct{}
	closed               atomic.Bool
	raftAccepts          chan net.Conn
	bindHandler          channel.BindHandler
	version              versions.VersionProvider
	versionEncoded       []byte
	readonly             atomic.Bool
	clusterStateHandlers concurrenz.CopyOnWriteSlice[func(state ClusterState)]
}

func (self *impl) RegisterClusterStateHandler(f func(state ClusterState)) {
	self.clusterStateHandlers.Append(f)
}

func (self *impl) Init(bindHandler channel.BindHandler) {
	if self.bindHandler == nil {
		self.bindHandler = bindHandler
	}
}

func (self *impl) GetAdvertiseAddr() raft.ServerAddress {
	return self.raftAddr
}

func (self *impl) Close() error {
	if self.closed.CompareAndSwap(false, true) {
		close(self.closeNotify)
	}
	return nil
}

func (self *impl) Addr() net.Addr {
	return self.netAddr
}

func (self *impl) Accept() (net.Conn, error) {
	select {
	case conn := <-self.raftAccepts:
		return conn, nil
	case <-self.closeNotify:
		return nil, errors.New("closed")
	}
}

func (self *impl) Dial(address raft.ServerAddress, timeout time.Duration) (net.Conn, error) {
	logrus.Infof("dialing %v", address)
	peer, err := self.GetOrConnectPeer(string(address), timeout)
	if err != nil {
		return nil, err
	}
	if err := peer.Connect(timeout); err != nil {
		return nil, err
	}
	return peer.RaftConn, nil
}

func (self *impl) GetOrConnectPeer(address string, timeout time.Duration) (*Peer, error) {
	if address == "" {
		return nil, errors.New("cannot get peer for empty address")
	}
	if peer := self.GetPeer(raft.ServerAddress(address)); peer != nil {
		logrus.Debugf("existing peer found for %v, returning", address)
		return peer, nil
	}
	logrus.Infof("creating new peer for %v, returning", address)

	addr, err := transport.ParseAddress(address)
	if err != nil {
		logrus.WithError(err).WithField("address", address).Error("failed to parse address")
		return nil, err
	}

	headers := map[int32][]byte{
		channel.HelloVersionHeader: self.versionEncoded,
		channel.TypeHeader:         []byte(ChannelTypeMesh),
		PeerAddrHeader:             []byte(self.raftAddr),
	}

	dialer := channel.NewClassicDialer(self.id, addr, headers)
	dialOptions := channel.DefaultOptions()
	dialOptions.ConnectOptions.ConnectTimeout = timeout

	peer := &Peer{
		mesh:    self,
		Address: address,
	}

	bindHandler := channel.BindHandlerF(func(binding channel.Binding) error {
		if err := self.bindHandler.BindChannel(binding); err != nil {
			return err
		}

		peer.Channel = binding.GetChannel()

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

		peer.Version = versionInfo
		peer.RaftConn = newRaftPeerConn(peer, self.netAddr)

		binding.AddTypedReceiveHandler(peer)
		binding.AddTypedReceiveHandler(peer.RaftConn)
		binding.AddCloseHandler(peer)

		return nil
	})

	if _, err = channel.NewChannel(ChannelTypeMesh, dialer, bindHandler, channel.DefaultOptions()); err != nil {
		return nil, errors.Wrapf(err, "unable to dial %v", address)
	}

	self.AddPeer(peer)
	return peer, nil
}

func (self *impl) GetPeerId(address string, timeout time.Duration) (string, error) {
	log := pfxlog.Logger().WithField("address", address)
	addr, err := transport.ParseAddress(address)
	if err != nil {
		log.WithError(err).Error("failed to parse address")
		return "", err
	}

	c, err := addr.Dial("test", self.id, timeout, nil)
	if err != nil {
		log.WithError(err).Error("failed to dial address")
		return "", err
	}

	defer func() {
		if err = c.Close(); err != nil {
			log.WithError(err).Error("failed to close peer connection while getting peer id")
		}
	}()

	return self.extractPeerId(address, c.PeerCertificates())
}

func (self *impl) extractPeerId(peerAddr string, certs []*x509.Certificate) (string, error) {
	if len(certs) == 0 {
		return "", errors.Errorf("no certificates for peer at %v", peerAddr)
	}

	leaf := certs[0]
	for _, uri := range leaf.URIs {
		if uri.Scheme == "spiffe" {
			return strings.TrimPrefix(uri.Path, "/controller/"), nil
		}
	}

	return "", errors.New("no controller SPIFFE ID found in peer certificates")
}

func (self *impl) AddPeer(peer *Peer) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.Peers[peer.Address] = peer
	self.updateClusterState()
	logrus.Infof("added peer at %v", peer.Address)
}

func (self *impl) GetPeer(addr raft.ServerAddress) *Peer {
	self.lock.RLock()
	defer self.lock.RUnlock()
	return self.Peers[string(addr)]
}

func (self *impl) RemovePeer(peer *Peer) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	delete(self.Peers, peer.Address)
	self.updateClusterState()
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

		if err := binding.Bind(self.bindHandler); err != nil {
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

		peer.Version = versionInfo

		peer.RaftConn = newRaftPeerConn(peer, self.netAddr)
		binding.AddTypedReceiveHandler(peer)
		binding.AddTypedReceiveHandler(peer.RaftConn)
		binding.AddCloseHandler(peer)
		return nil
	})

	_, err := channel.NewChannelWithUnderlay(ChannelTypeMesh, underlay, bindHandler, channel.DefaultOptions())
	if err != nil {
		return err
	}

	self.AddPeer(peer)
	logrus.Infof("connected peer %v at %v", peer.Id, peer.Address)

	return nil
}

func (self *impl) GetPeers() map[string]*Peer {
	self.lock.Lock()
	defer self.lock.Unlock()

	result := map[string]*Peer{}

	for k, v := range self.Peers {
		result[k] = v
	}

	return result
}

func (self *impl) IsReadOnly() bool {
	return self.readonly.Load()
}
