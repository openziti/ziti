package model

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/common"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/pb/edge_cmd_pb"
	"github.com/openziti/fabric/controller/change"
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/pb/cmd_pb"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
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
	session, err := self.getTerminatorSession(tx, terminator, "")
	if err != nil {
		return err
	}

	if terminator.GetInstanceId() == "" {
		return nil
	}

	identityTerminators, err := self.Env.GetStores().Terminator.GetTerminatorsInIdentityGroup(tx, terminator.GetId())
	if err != nil {
		return err
	}

	for _, otherTerminator := range identityTerminators {
		otherSession, err := self.getTerminatorSession(tx, otherTerminator, "sibling ")
		if err != nil {
			return err
		}
		if otherSession != nil {
			if otherSession.ApiSession.IdentityId != session.ApiSession.IdentityId {
				return errors.Errorf("sibling terminator %v with shared identity %v belongs to different identity", terminator.GetId(), terminator.GetInstanceId())
			}
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

func (self *CreateEdgeTerminatorCmd) getTerminatorSession(tx *bbolt.Tx, terminator terminator, context string) (*persistence.Session, error) {
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
