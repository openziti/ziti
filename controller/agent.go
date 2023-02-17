package controller

import (
	"fmt"
	"github.com/openziti/fabric/pb/cmd_pb"
	"io"
	"net"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/fabric/handler_common"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	AgentAppId byte = 1

	AgentIdHeader      = 10
	AgentAddrHeader    = 11
	AgentIsVoterHeader = 12
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
	log := pfxlog.Logger()
	if err := self.network.SnapshotDatabase(); err != nil {
		log.WithError(err).Error("failed to snapshot db")
		handler_common.SendOpResult(m, ch, "db.snapshot", err.Error(), false)
	} else {
		handler_common.SendOpResult(m, ch, "db.snapshot", "", true)
	}
}

func (self *Controller) agentOpRaftList(m *channel.Message, ch channel.Channel) {
	members, err := self.raftController.ListMembers()
	if err != nil {
		handler_common.SendOpResult(m, ch, "raft.list", err.Error(), false)
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
	addr, found := m.GetStringHeader(AgentAddrHeader)
	if !found {
		handler_common.SendOpResult(m, ch, "raft.join", "address not supplied", false)
		return
	}

	id, found := m.GetStringHeader(AgentIdHeader)
	if !found {
		peerId, peerAddr, err := self.raftController.Mesh.GetPeerInfo(addr, 15*time.Second)
		if err != nil {
			errMsg := fmt.Sprintf("id not supplied and unable to retrieve [%v]", err.Error())
			handler_common.SendOpResult(m, ch, "raft.join", errMsg, false)
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

func (self *Controller) agentOpRaftRemovePeer(m *channel.Message, ch channel.Channel) {
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
	sourceDbPath := string(m.Body)
	if len(sourceDbPath) == 0 {
		handler_common.SendOpResult(m, ch, "raft.initFromDb", "source db not supplied", false)
		return
	}

	if err := self.InitializeRaftFromBoltDb(sourceDbPath); err != nil {
		handler_common.SendOpResult(m, ch, "raft.initFromDb", err.Error(), false)
		return
	}
	handler_common.SendOpResult(m, ch, "raft.initFromDb", fmt.Sprintf("success, initialized from [%v]", sourceDbPath), true)
}
