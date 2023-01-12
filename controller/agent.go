package controller

import (
	"fmt"
	"io"
	"net"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/fabric/controller/raft"
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
	binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_RaftJoinRequestType), self.agentOpRaftJoin)
	binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_RaftRemoveRequestType), self.agentOpRaftRemove)
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

func (self *Controller) agentOpRaftJoin(m *channel.Message, ch channel.Channel) {
	addr, found := m.GetStringHeader(AgentAddrHeader)
	if !found {
		handler_common.SendOpResult(m, ch, "raft.join", "address not supplied", false)
		return
	}

	id, found := m.GetStringHeader(AgentIdHeader)
	if !found {
		peerId, err := self.raftController.Mesh.GetPeerId(addr, 15*time.Second)
		if err != nil {
			errMsg := fmt.Sprintf("id not supplied and unable to retrieve [%v]", err.Error())
			handler_common.SendOpResult(m, ch, "raft.join", errMsg, false)
			return
		}
		id = peerId
	}

	isVoter, found := m.GetBoolHeader(AgentIsVoterHeader)
	if !found {
		isVoter = true
	}

	req := &raft.JoinRequest{
		Addr:    addr,
		Id:      id,
		IsVoter: isVoter,
	}

	if err := self.raftController.Join(req); err != nil {
		handler_common.SendOpResult(m, ch, "raft.join", err.Error(), false)
		return
	}
	handler_common.SendOpResult(m, ch, "raft.join", fmt.Sprintf("success, added %v at %v to cluster", id, addr), true)
}

func (self *Controller) agentOpRaftRemove(m *channel.Message, ch channel.Channel) {
	//id := string(m.Body)

	// TODO: make this work like Join where we test if we're bootstrapped yet
	//if err := self.raftController.HandleRemove(); err != nil {
	//	return err
	//}
	// _, err := c.WriteString("success\n")
	var addr string

	id, found := m.GetStringHeader(AgentIdHeader)
	if !found {
		addr, found = m.GetStringHeader(AgentAddrHeader)
		if !found {
			handler_common.SendOpResult(m, ch, "raft.leave", "address or id not supplied", false)
			return
		}
		peerId, err := self.raftController.Mesh.GetPeerId(addr, 15*time.Second)
		if err != nil {
			errMsg := fmt.Sprintf("id not supplied and unable to retrieve from %s [%v]", addr, err.Error())
			handler_common.SendOpResult(m, ch, "raft.leave", errMsg, false)
			return
		}
		id = peerId
	}

	req := &raft.RemoveRequest{
		Id: id,
	}

	if err := self.raftController.HandleRemove(req); err != nil {
		handler_common.SendOpResult(m, ch, "raft.leave", err.Error(), false)
		return
	}
	handler_common.SendOpResult(m, ch, "raft.leave", fmt.Sprintf("success, removed %v at %v from cluster", id, addr), true)
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
