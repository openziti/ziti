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

package model

import (
	"github.com/michaelquigley/pfxlog"
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/pb/edge_cmd_pb"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/event"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/network"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
	"time"
)

func NewControllerManager(env Env) *ControllerManager {
	manager := &ControllerManager{
		baseEntityManager: newBaseEntityManager[*Controller, *db.Controller](env, env.GetStores().Controller),
	}
	manager.impl = manager

	network.RegisterManagerDecoder[*Controller](env.GetHostController().GetNetwork().Managers, manager)

	return manager
}

type ControllerManager struct {
	baseEntityManager[*Controller, *db.Controller]
}

func (self *ControllerManager) newModelEntity() *Controller {
	return &Controller{}
}

func (self *ControllerManager) Create(entity *Controller, ctx *change.Context) error {
	return network.DispatchCreate[*Controller](self, entity, ctx)
}

func (self *ControllerManager) ApplyCreate(cmd *command.CreateEntityCommand[*Controller], ctx boltz.MutateContext) error {
	_, err := self.createEntity(cmd.Entity, ctx)
	return err
}

func (self *ControllerManager) Update(entity *Controller, checker fields.UpdatedFields, ctx *change.Context) error {
	return network.DispatchUpdate[*Controller](self, entity, checker, ctx)
}

func (self *ControllerManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*Controller], ctx boltz.MutateContext) error {
	return self.updateEntity(cmd.Entity, cmd.UpdatedFields, ctx)
}

func (self *ControllerManager) Read(id string) (*Controller, error) {
	modelEntity := &Controller{}
	if err := self.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (self *ControllerManager) readInTx(tx *bbolt.Tx, id string) (*Controller, error) {
	modelEntity := &Controller{}
	if err := self.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (self *ControllerManager) ReadByName(name string) (*Controller, error) {
	modelEntity := &Controller{}
	nameIndex := self.env.GetStores().Controller.GetNameIndex()
	if err := self.readEntityWithIndex("name", []byte(name), nameIndex, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (self *ControllerManager) MapControllerNamesToIds(values []string, identityId string) map[string]struct{} {
	var result []string
	if stringz.Contains(values, "all") {
		result = []string{"all"}
	} else {
		for _, val := range values {
			if Controller, _ := self.Read(val); Controller != nil {
				result = append(result, val)
			} else if Controller, _ := self.ReadByName(val); Controller != nil {
				result = append(result, Controller.Id)
			} else {
				pfxlog.Logger().Debugf("user %v submitted %v as a config type of interest, but no matching records found", identityId, val)
			}
		}
	}
	return stringz.SliceToSet(result)
}

func (self *ControllerManager) Marshall(entity *Controller) ([]byte, error) {
	msg := &edge_cmd_pb.Controller{
		Id:           entity.Id,
		Name:         entity.Name,
		Address:      entity.Address,
		CertPem:      entity.CertPem,
		Fingerprint:  entity.Fingerprint,
		IsOnline:     entity.IsOnline,
		LastJoinedAt: timePtrToPb(entity.LastJoinedAt),
	}

	return proto.Marshal(msg)
}

func (self *ControllerManager) Unmarshall(bytes []byte) (*Controller, error) {
	msg := &edge_cmd_pb.Controller{}

	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}

	return &Controller{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: edge_cmd_pb.DecodeTags(msg.Tags),
		},
		Name:         msg.Name,
		Address:      msg.Address,
		CertPem:      msg.CertPem,
		Fingerprint:  msg.Fingerprint,
		IsOnline:     msg.IsOnline,
		LastJoinedAt: pbTimeToTimePtr(msg.LastJoinedAt),
	}, nil
}

func (self *ControllerManager) PeersConnected(peers []*event.ClusterPeer) {
	var controllerIds []string
	err := self.ListWithHandler("", func(tx *bbolt.Tx, ids []string, qmd *models.QueryMetaData) error {
		controllerIds = ids
		return nil
	})

	changeCtx := change.New()
	changeCtx.SetSourceType("raft.peers.connected").
		SetChangeAuthorType(change.AuthorTypeController)

	if err != nil {
		pfxlog.Logger().WithError(err).Error("could not list controllers to handle new peer(s) connection")
		return
	}

	connectFields := fields.UpdatedFieldsMap{
		db.FieldControllerLastJoinedAt: struct{}{},
		db.FieldControllerCertPem:      struct{}{},
		db.FieldControllerFingerprint:  struct{}{},
		db.FieldControllerIsOnline:     struct{}{},
		db.FieldControllerAddress:      struct{}{},
	}

	now := time.Now()

	for _, peer := range peers {
		if stringz.Contains(controllerIds, peer.Id) {
			existing, err := self.Read(peer.Id)

			if err != nil {
				pfxlog.Logger().WithError(err).Error("could not handle new peer(s) connection, existing controller could not be read")
				continue
			}

			existing.Address = peer.Addr
			existing.IsOnline = true
			existing.LastJoinedAt = &now

			if len(peer.ServerCert) > 0 {
				existing.CertPem = nfpem.EncodeToString(peer.ServerCert[0])
				existing.Fingerprint = nfpem.FingerprintFromCertificate(peer.ServerCert[0])
			}

			if err := self.Update(existing, connectFields, changeCtx); err != nil {
				pfxlog.Logger().WithError(err).Error("could not update controller during peer(s) connection")
			}
		} else {
			if len(peer.ServerCert) == 0 {
				pfxlog.Logger().Error("could not create controller during peer(s) connection, no server certificate provided")
				continue
			}

			newController := &Controller{
				BaseEntity: models.BaseEntity{
					Id: peer.Id,
				},
				Address:      peer.Addr,
				IsOnline:     true,
				LastJoinedAt: &now,
			}

			newController.Name = peer.ServerCert[0].Subject.CommonName
			newController.CertPem = nfpem.EncodeToString(peer.ServerCert[0])
			newController.Fingerprint = nfpem.FingerprintFromCertificate(peer.ServerCert[0])

			if err := self.Create(newController, changeCtx); err != nil {
				pfxlog.Logger().WithError(err).Error("could not create controller during peer(s) connection")
				continue
			}
		}
	}
}

func (self *ControllerManager) PeersDisconnected(peers []*event.ClusterPeer) {
	changeCtx := change.New()
	changeCtx.SetSourceType("raft.peers.disconnected").
		SetChangeAuthorType(change.AuthorTypeController)

	disconnectFields := fields.UpdatedFieldsMap{
		db.FieldControllerLastJoinedAt: struct{}{},
		db.FieldControllerIsOnline:     struct{}{},
	}
	for _, peer := range peers {
		controller := &Controller{
			BaseEntity: models.BaseEntity{
				Id: peer.Id,
			},
			IsOnline: false,
		}

		if err := self.Update(controller, disconnectFields, changeCtx); err != nil {
			pfxlog.Logger().WithError(err).Error("could not update controller during peer(s) disconnection")
		}
	}
}
