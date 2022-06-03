package model

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/edge_common"
	"github.com/openziti/edge/pb/edge_cmd_pb"
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/network"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"strings"
)

type CreateEdgeTerminatorCmd struct {
	Env    Env
	Entity *network.Terminator
}

func (self *CreateEdgeTerminatorCmd) Apply() error {
	createCmd := &command.CreateEntityCommand[*network.Terminator]{
		Creator:        self.Env.GetManagers().Terminator,
		Entity:         self.Entity,
		PostCreateHook: self.validateTerminatorIdentity,
	}
	return self.Env.GetManagers().Terminator.ApplyCreate(createCmd)
}

func (self *CreateEdgeTerminatorCmd) validateTerminatorIdentity(tx *bbolt.Tx, terminator *network.Terminator) error {
	session, err := self.getTerminatorSession(tx, terminator, "")
	if err != nil {
		return err
	}

	if terminator.GetIdentity() == "" {
		return nil
	}

	identityTerminators, err := self.Env.GetStores().Terminator.GetTerminatorsInIdentityGroup(tx, terminator.GetId())
	for _, otherTerminator := range identityTerminators {
		otherSession, err := self.getTerminatorSession(tx, otherTerminator, "sibling ")
		if err != nil {
			return err
		}
		if otherSession != nil {
			if otherSession.ApiSession.IdentityId != session.ApiSession.IdentityId {
				return errors.Errorf("sibling terminator %v with shared identity %v belongs to different identity", terminator.GetId(), terminator.GetIdentity())
			}
		}
	}

	return nil
}

type terminator interface {
	GetId() string
	GetIdentity() string
	GetBinding() string
	GetAddress() string
}

func (self *CreateEdgeTerminatorCmd) getTerminatorSession(tx *bbolt.Tx, terminator terminator, context string) (*persistence.Session, error) {
	if terminator.GetBinding() != edge_common.EdgeBinding {
		return nil, errors.Errorf("%vterminator %v with identity %v is not edge terminator. Can't share identity", context, terminator.GetId(), terminator.GetIdentity())
	}

	addressParts := strings.Split(terminator.GetAddress(), ":")
	if len(addressParts) != 2 {
		return nil, errors.Errorf("%vterminator %v with identity %v is not edge terminator. Can't share identity", context, terminator.GetId(), terminator.GetIdentity())
	}

	if addressParts[0] != "hosted" {
		return nil, errors.Errorf("%vterminator %v with identity %v is not edge terminator. Can't share identity", context, terminator.GetId(), terminator.GetIdentity())
	}

	sessionToken := addressParts[1]
	session, err := self.Env.GetStores().Session.LoadOneByToken(tx, sessionToken)
	if err != nil {
		pfxlog.Logger().Warnf("sibling terminator %v with shared identity %v has invalid session token %v", terminator.GetId(), terminator.GetIdentity(), sessionToken)
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
	return self.Env.GetManagers().Terminator.Marshall(self.Entity)
}

func (self *CreateEdgeTerminatorCmd) Decode(env Env, msg *edge_cmd_pb.CreateEdgeTerminatorCommand) error {
	var err error
	self.Env = env
	self.Entity, err = env.GetManagers().Terminator.Unmarshall(msg.TerminatorData)
	return err
}
