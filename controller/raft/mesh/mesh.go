package mesh

import (
	"github.com/hashicorp/raft"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/identity"
	"github.com/openziti/transport/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"net"
	"sync"
	"time"
)

const (
	PeerIdHeader   = 10
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
}

func (self *Peer) HandleClose(channel.Channel) {
	self.mesh.RemovePeer(self)
}

func (self *Peer) ContentType() int32 {
	return RaftConnectType
}

func (self *Peer) HandleReceive(m *channel.Message, ch channel.Channel) {
	go func() {
		response := channel.NewMessage(channel.ContentTypeResultType, nil)
		response.Headers[PeerIdHeader] = []byte(self.mesh.raftId)
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
	id, _ := response.GetStringHeader(PeerIdHeader)
	self.Id = raft.ServerID(id)

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

// Mesh provides the networking layer to raft
type Mesh interface {
	raft.StreamLayer

	channel.UnderlayAcceptor

	// GetOrConnectPeer returns a peer for the given address. If a peer has already been established,
	// it will be returned, otherwise a new connection will be established
	GetOrConnectPeer(address string, timeout time.Duration) (*Peer, error)
}

func New(id *identity.TokenId, raftId raft.ServerID, raftAddr raft.ServerAddress, bindHandler channel.BindHandler) Mesh {
	return &impl{
		id:       id,
		raftId:   raftId,
		raftAddr: raftAddr,
		netAddr: &meshAddr{
			network: "mesh",
			addr:    string(raftAddr),
		},
		Peers:       map[string]*Peer{},
		closeNotify: make(chan struct{}),
		raftAccepts: make(chan net.Conn),
		bindHandler: bindHandler,
	}
}

type impl struct {
	id          *identity.TokenId
	raftId      raft.ServerID
	raftAddr    raft.ServerAddress
	netAddr     net.Addr
	Peers       map[string]*Peer
	lock        sync.RWMutex
	closeNotify chan struct{}
	closed      concurrenz.AtomicBoolean
	raftAccepts chan net.Conn
	bindHandler channel.BindHandler
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
		logrus.Infof("existing peer found for %v, returning", address)
		return peer, nil
	}
	logrus.Infof("creating new peer for %v, returning", address)

	addr, err := transport.ParseAddress(address)
	if err != nil {
		logrus.WithError(err).WithField("address", address).Error("failed to parse address")
		return nil, err
	}

	headers := map[int32][]byte{
		PeerIdHeader:       []byte(self.raftId),
		PeerAddrHeader:     []byte(self.raftAddr),
		channel.TypeHeader: []byte(ChannelTypeMesh),
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

func (self *impl) AddPeer(peer *Peer) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.Peers[peer.Address] = peer
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
		id := string(ch.Underlay().Headers()[PeerIdHeader])
		addr := string(ch.Underlay().Headers()[PeerAddrHeader])

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
