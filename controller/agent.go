package controller

import (
	"fmt"
	"github.com/openziti/ziti/common/pb/cmd_pb"
	"google.golang.org/protobuf/proto"
	"io"
	"net"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/channel/v3/protobufs"
	"github.com/openziti/ziti/common/handler_common"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	AgentAppId byte = 1

	AgentIdHeader         = 10
	AgentAddrHeader       = 11
	AgentIsVoterHeader    = 12
	AgentSnapshotFileName = 13
)

func (self *Controller) RegisterAgentBindHandler(bindHandler channel.BindHandler) {
	self.agentBindHandlers = append(self.agentBindHandlers, bindHandler)
}

func (self *Controller) bindAgentChannel(binding channel.Binding) error {
	binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_SnapshotDbRequestType), self.agentOpSnapshotDb)
	binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_RaftListMembersRequestType), self.agentOpRaftList)
	binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_RaftAddPeerRequestType), self.agentOpRaftAddPeer)
	binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_RaftRemovePeerRequestType), self.agentOpRaftRemovePeer)
	binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_RaftTransferLeadershipRequestType), self.agentOpRaftTransferLeadership)
	binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_RaftInitFromDb), self.agentOpInitFromDb)
	binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_RaftInit), self.agentOpInit)

	for _, bh := range self.agentBindHandlers {
		if err := binding.Bind(bh); err != nil {
			return err
		}
	}
	return nil
}

func (self *Controller) HandleCustomAgentAsyncOp(conn net.Conn) error {
	logrus.Debug("received agent operation request")

	appIdBuf := []byte{0}
	_, err := io.ReadFull(conn, appIdBuf)
	if err != nil {
		return err
	}
	appId := appIdBuf[0]

	if appId != AgentAppId {
		logrus.WithField("appId", appId).Debug("invalid app id on agent request")
		return errors.New("invalid operation for controller")
	}

	options := channel.DefaultOptions()
	options.ConnectTimeout = time.Second
	listener := channel.NewExistingConnListener(self.config.Id, conn, nil)
	_, err = channel.NewChannel("agent", listener, channel.BindHandlerF(self.bindAgentChannel), options)
	return err
}

func (self *Controller) agentOpSnapshotDb(m *channel.Message, ch channel.Channel) {
	fileName, _ := m.GetStringHeader(AgentSnapshotFileName)

	log := pfxlog.Logger()
	if path, err := self.network.SnapshotDatabaseToFile(fileName); err != nil {
		log.WithError(err).Error("failed to snapshot db")
		handler_common.SendOpResult(m, ch, "db.snapshot", err.Error(), false)
	} else {
		handler_common.SendOpResult(m, ch, "db.snapshot", path, true)
	}
}

func (self *Controller) agentOpRaftList(m *channel.Message, ch channel.Channel) {
	if self.raftController == nil {
		handler_common.SendOpResult(m, ch, "cluster.list", "controller not running in clustered mode", false)
		return
	}

	members, err := self.raftController.ListMembers()
	if err != nil {
		handler_common.SendOpResult(m, ch, "cluster.list", err.Error(), false)
	}

	result := &mgmt_pb.RaftMemberListResponse{}
	for _, member := range members {
		result.Members = append(result.Members, &mgmt_pb.RaftMember{
			Id:          member.Id,
			Addr:        member.Addr,
			IsVoter:     member.Voter,
			IsLeader:    member.Leader,
			Version:     member.Version,
			IsConnected: member.Connected,
		})
	}

	if err = protobufs.MarshalTyped(result).ReplyTo(m).WithTimeout(time.Second).Send(ch); err != nil {
		pfxlog.Logger().WithError(err).Error("failure sending raft member list response")
	}
}

func (self *Controller) agentOpRaftAddPeer(m *channel.Message, ch channel.Channel) {
	if self.raftController == nil {
		handler_common.SendOpResult(m, ch, "cluster.add-peer", "controller not running in clustered mode", false)
		return
	}

	if !self.raftController.IsBootstrapped() {
		self.agentOpRaftJoinCluster(m, ch)
		return
	}

	addr, found := m.GetStringHeader(AgentAddrHeader)
	if !found {
		handler_common.SendOpResult(m, ch, "cluster.add-peer", "address not supplied", false)
		return
	}

	id, found := m.GetStringHeader(AgentIdHeader)
	if !found {
		peerId, peerAddr, err := self.raftController.Mesh.GetPeerInfo(addr, 15*time.Second)
		if err != nil {
			errMsg := fmt.Sprintf("id not supplied and unable to retrieve [%v]", err.Error())
			handler_common.SendOpResult(m, ch, "cluster.add-peer", errMsg, false)
			return
		}
		id = string(peerId)
		addr = string(peerAddr)
	}

	isVoter, found := m.GetBoolHeader(AgentIsVoterHeader)
	if !found {
		isVoter = true
	}

	req := &cmd_pb.AddPeerRequest{
		Addr:    addr,
		Id:      id,
		IsVoter: isVoter,
	}

	if err := self.raftController.Join(req); err != nil {
		handler_common.SendOpResult(m, ch, "cluster.add-peer", err.Error(), false)
		return
	}
	handler_common.SendOpResult(m, ch, "cluster.add-peer", fmt.Sprintf("success, added %v at %v to cluster", id, addr), true)
}

func (self *Controller) agentOpRaftJoinCluster(m *channel.Message, ch channel.Channel) {
	if self.raftController == nil {
		handler_common.SendOpResult(m, ch, "cluster.join", "controller not running in clustered mode", false)
		return
	}

	if self.raftController.IsBootstrapped() {
		handler_common.SendOpResult(m, ch, "cluster.join",
			"Local instance is already initialized. Only uninitialized nodes may be joined to a cluster. ",
			false)
		return
	}

	addr, found := m.GetStringHeader(AgentAddrHeader)
	if !found {
		handler_common.SendOpResult(m, ch, "cluster.join", "address not supplied", false)
		return
	}

	isVoter, found := m.GetBoolHeader(AgentIsVoterHeader)
	if !found {
		isVoter = true
	}

	req := &cmd_pb.AddPeerRequest{
		Addr:    self.raftController.Config.AdvertiseAddress.String(),
		Id:      self.config.Id.Token,
		IsVoter: isVoter,
	}

	if err := self.raftController.ForwardToAddr(addr, req); err != nil {
		handler_common.SendOpResult(m, ch, "cluster.join", err.Error(), false)
		return
	}

	handler_common.SendOpResult(m, ch, "cluster.join", "success, added self to cluster", true)
}

func (self *Controller) agentOpRaftRemovePeer(m *channel.Message, ch channel.Channel) {
	if self.raftController == nil {
		handler_common.SendOpResult(m, ch, "cluster.remove-peer", "controller not running in clustered mode", false)
		return
	}

	id, found := m.GetStringHeader(AgentIdHeader)
	if !found {
		handler_common.SendOpResult(m, ch, "cluster.remove-peer", "id not supplied", false)
		return
	}

	req := &cmd_pb.RemovePeerRequest{
		Id: id,
	}

	if err := self.raftController.HandleRemovePeer(req); err != nil {
		handler_common.SendOpResult(m, ch, "cluster.remove-peer", err.Error(), false)
		return
	}
	handler_common.SendOpResult(m, ch, "cluster.remove-peer", fmt.Sprintf("success, removed %v from cluster", id), true)
}

func (self *Controller) agentOpRaftTransferLeadership(m *channel.Message, ch channel.Channel) {
	if self.raftController == nil {
		handler_common.SendOpResult(m, ch, "cluster.transfer-leadership", "controller not running in clustered mode", false)
		return
	}

	id, _ := m.GetStringHeader(AgentIdHeader)
	req := &cmd_pb.TransferLeadershipRequest{
		Id: id,
	}

	if err := self.raftController.HandleTransferLeadership(req); err != nil {
		handler_common.SendOpResult(m, ch, "cluster.transfer-leadership", err.Error(), false)
		return
	}
	handler_common.SendOpResult(m, ch, "cluster.transfer-leadership", "success", true)
}

func (self *Controller) agentOpInitFromDb(m *channel.Message, ch channel.Channel) {
	if self.raftController == nil {
		handler_common.SendOpResult(m, ch, "cluster.init-from-db", "controller not running in clustered mode", false)
		return
	}

	sourceDbPath := string(m.Body)
	if len(sourceDbPath) == 0 {
		handler_common.SendOpResult(m, ch, "cluster.init-from-db", "source db not supplied", false)
		return
	}

	if err := self.InitializeRaftFromBoltDb(sourceDbPath); err != nil {
		handler_common.SendOpResult(m, ch, "cluster.init-from-db", err.Error(), false)
		return
	}
	handler_common.SendOpResult(m, ch, "cluster.init-from-db", fmt.Sprintf("success, initialized from [%v]", sourceDbPath), true)
}

func (self *Controller) agentOpInit(m *channel.Message, ch channel.Channel) {
	if self.raftController == nil {
		handler_common.SendOpResult(m, ch, "init.edge", "controller not running in clustered mode", false)
		return
	}

	log := pfxlog.Logger().WithField("channel", ch.LogicalName())

	request := &mgmt_pb.InitRequest{}
	if err := proto.Unmarshal(m.Body, request); err != nil {
		log.WithError(err).Error("unable to parse InitRequest, closing channel")
		if err = ch.Close(); err != nil {
			log.WithError(err).Error("error closing mgmt channel")
		}
		return
	}

	if err := self.env.Managers.Identity.InitializeDefaultAdmin(request.Username, request.Password, request.Name); err != nil {
		handler_common.SendOpResult(m, ch, "init.edge", err.Error(), false)
	} else {
		handler_common.SendOpResult(m, ch, "init.edge", "success", true)
	}
}
