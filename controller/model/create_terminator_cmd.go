package model

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/pb/cmd_pb"
	"github.com/openziti/ziti/common/pb/edge_cmd_pb"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/network"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
	"strings"
)

type CreateEdgeTerminatorCmd struct {
	Env     Env
	Entity  *network.Terminator
	Context *change.Context
}

func (self *CreateEdgeTerminatorCmd) Apply(ctx boltz.MutateContext) error {
	createCmd := &command.CreateEntityCommand[*network.Terminator]{
		Creator:        self.Env.GetManagers().Terminator,
		Entity:         self.Entity,
		PostCreateHook: self.validateTerminatorIdentity,
		Context:        self.Context,
	}
	return self.Env.GetManagers().Terminator.ApplyCreate(createCmd, ctx)
}

func (self *CreateEdgeTerminatorCmd) validateTerminatorIdentity(ctx boltz.MutateContext, terminator *network.Terminator) error {
	tx := ctx.Tx()

	if terminator.GetInstanceId() == "" {
		return nil
	}

	identityTerminators, err := self.Env.GetStores().Terminator.GetTerminatorsInIdentityGroup(tx, terminator.GetId())
	if err != nil {
		return err
	}

	for _, otherTerminator := range identityTerminators {
		if otherTerminator.HostId != terminator.HostId {
			pfxlog.Logger().WithFields(logrus.Fields{
				"terminatorId":       terminator.GetId(),
				"siblingId":          otherTerminator.GetId(),
				"instanceId":         terminator.InstanceId,
				"terminatorIdentity": terminator.HostId,
				"existingIdentity":   otherTerminator.HostId,
			}).Warn("validation of terminator failed, shared identity belongs to different identity")
			return errors.Errorf("sibling terminator %v with shared identity %v belongs to different identity", otherTerminator.GetId(), terminator.GetInstanceId())
		}
	}

	return nil
}

func (self *CreateEdgeTerminatorCmd) GetChangeContext() *change.Context {
	return self.Context
}

type terminator interface {
	GetId() string
	GetInstanceId() string
	GetBinding() string
	GetAddress() string
}

func (self *CreateEdgeTerminatorCmd) getTerminatorSession(tx *bbolt.Tx, terminator terminator, context string) (*db.Session, error) {
	if terminator.GetBinding() != common.EdgeBinding {
		return nil, errors.Errorf("%vterminator %v with identity %v is not edge terminator. Can't share identity", context, terminator.GetId(), terminator.GetInstanceId())
	}

	addressParts := strings.Split(terminator.GetAddress(), ":")
	if len(addressParts) != 2 {
		return nil, errors.Errorf("%vterminator %v with identity %v is not edge terminator. Can't share identity", context, terminator.GetId(), terminator.GetInstanceId())
	}

	if addressParts[0] != "hosted" {
		return nil, errors.Errorf("%vterminator %v with identity %v is not edge terminator. Can't share identity", context, terminator.GetId(), terminator.GetInstanceId())
	}

	sessionToken := addressParts[1]
	session, err := self.Env.GetStores().Session.LoadOneByToken(tx, sessionToken)
	if err != nil {
		pfxlog.Logger().Warnf("sibling terminator %v with shared identity %v has invalid session token %v", terminator.GetId(), terminator.GetInstanceId(), sessionToken)
		return nil, nil
	}

	if session.ApiSession == nil {
		apiSession, err := self.Env.GetStores().ApiSession.LoadOneById(tx, session.ApiSessionId)
		if err != nil {
			return nil, err
		}
		session.ApiSession = apiSession
	}

	return session, nil
}

func (self *CreateEdgeTerminatorCmd) Encode() ([]byte, error) {
	terminatorData, err := self.Env.GetManagers().Terminator.Marshall(self.Entity)
	if err != nil {
		return nil, err
	}
	cmd := &edge_cmd_pb.CreateEdgeTerminatorCommand{
		TerminatorData: terminatorData,
		Ctx:            ContextToProtobuf(self.Context),
	}
	return cmd_pb.EncodeProtobuf(cmd)
}

func (self *CreateEdgeTerminatorCmd) Decode(env Env, msg *edge_cmd_pb.CreateEdgeTerminatorCommand) error {
	var err error
	self.Env = env
	self.Entity, err = env.GetManagers().Terminator.Unmarshall(msg.TerminatorData)
	self.Context = ProtobufToContext(msg.Ctx)
	return err
}
