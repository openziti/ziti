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
	"crypto/x509"
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
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
	"time"
)

func NewControllerManager(env Env) *ControllerManager {
	manager := &ControllerManager{
		baseEntityManager: newBaseEntityManager[*Controller, *db.Controller](env, env.GetStores().Controller),
	}
	manager.impl = manager

	RegisterManagerDecoder[*Controller](env, manager)

	return manager
}

type ControllerManager struct {
	baseEntityManager[*Controller, *db.Controller]
}

func (self *ControllerManager) newModelEntity() *Controller {
	return &Controller{}
}

func (self *ControllerManager) Create(entity *Controller, ctx *change.Context) error {
	return DispatchCreate[*Controller](self, entity, ctx)
}

func (self *ControllerManager) ApplyCreate(cmd *command.CreateEntityCommand[*Controller], ctx boltz.MutateContext) error {
	_, err := self.createEntity(cmd.Entity, ctx)
	return err
}

func (self *ControllerManager) Update(entity *Controller, checker fields.UpdatedFields, ctx *change.Context) error {
	return DispatchUpdate[*Controller](self, entity, checker, ctx)
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

func (self *ControllerManager) Marshall(entity *Controller) ([]byte, error) {
	msg := &edge_cmd_pb.Controller{
		Id:           entity.Id,
		Name:         entity.Name,
		Address:      entity.CtrlAddress,
		CertPem:      entity.CertPem,
		Fingerprint:  entity.Fingerprint,
		IsOnline:     entity.IsOnline,
		LastJoinedAt: timePtrToPb(entity.LastJoinedAt),
		ApiAddresses: map[string]*edge_cmd_pb.ApiAddressList{},
	}

	for apiKey, instances := range entity.ApiAddresses {
		msg.ApiAddresses[apiKey] = &edge_cmd_pb.ApiAddressList{}
		for _, instance := range instances {
			msg.ApiAddresses[apiKey].Addresses = append(msg.ApiAddresses[apiKey].Addresses, &edge_cmd_pb.ApiAddress{
				Url:     instance.Url,
				Version: instance.Version,
			})
		}
	}

	return proto.Marshal(msg)
}

func (self *ControllerManager) Unmarshall(bytes []byte) (*Controller, error) {
	msg := &edge_cmd_pb.Controller{}

	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}

	controller := &Controller{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: edge_cmd_pb.DecodeTags(msg.Tags),
		},
		Name:         msg.Name,
		CtrlAddress:  msg.Address,
		CertPem:      msg.CertPem,
		Fingerprint:  msg.Fingerprint,
		IsOnline:     msg.IsOnline,
		LastJoinedAt: pbTimeToTimePtr(msg.LastJoinedAt),
		ApiAddresses: map[string][]ApiAddress{},
	}

	for apiKey, instanceList := range msg.ApiAddresses {
		controller.ApiAddresses[apiKey] = nil
		if instanceList != nil {
			for _, instance := range instanceList.Addresses {
				controller.ApiAddresses[apiKey] = append(controller.ApiAddresses[apiKey], ApiAddress{
					Url:     instance.Url,
					Version: instance.Version,
				})
			}
		}
	}

	return controller, nil
}

func (self *ControllerManager) getCurrentAsClusterPeer() *event.ClusterPeer {
	addr, id, version := self.env.GetRaftInfo()
	tlsConfig, _, _ := self.env.GetServerCert()
	var leaderCerts []*x509.Certificate

	for _, certBytes := range tlsConfig.Certificate {
		cert, err := x509.ParseCertificate(certBytes)

		if err == nil {
			leaderCerts = append(leaderCerts, cert)
		}
	}

	apiAddresses, _ := self.env.GetApiAddresses()

	return &event.ClusterPeer{
		Id:           id,
		Addr:         addr,
		Version:      version,
		ServerCert:   leaderCerts,
		ApiAddresses: apiAddresses,
	}
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
		db.FieldControllerLastJoinedAt:      struct{}{},
		db.FieldControllerCertPem:           struct{}{},
		db.FieldControllerFingerprint:       struct{}{},
		db.FieldControllerIsOnline:          struct{}{},
		db.FieldControllerCtrlAddress:       struct{}{},
		db.FieldControllerApiAddresses:      struct{}{},
		db.FieldControllerApiAddressUrl:     struct{}{},
		db.FieldControllerApiAddressVersion: struct{}{},
	}

	now := time.Now()

	selfAsPeer := self.getCurrentAsClusterPeer()
	peerFingerprints := ""
	for _, peer := range peers {
		if len(peer.ServerCert) > 0 {
			fingerprint := nfpem.FingerprintFromCertificate(peer.ServerCert[0])

			if peerFingerprints == "" {
				peerFingerprints = fingerprint
			} else {
				peerFingerprints = peerFingerprints + ", " + fingerprint
			}
		}
	}

	pfxlog.Logger().Infof("acting as leader, updating controllers with peers, self: %s, peers: %s", nfpem.FingerprintFromCertificate(selfAsPeer.ServerCert[0]), peerFingerprints)

	// always as this controller as a "peer" to add or update on the controller list
	peers = append(peers, selfAsPeer)

	for _, peer := range peers {
		if stringz.Contains(controllerIds, peer.Id) {
			existing, err := self.Read(peer.Id)
			if err != nil {
				pfxlog.Logger().WithError(err).Error("could not handle new peer(s) connection, existing controller could not be read")
				continue
			}

			existing.CtrlAddress = peer.Addr
			existing.IsOnline = true
			existing.LastJoinedAt = &now
			existing.ApiAddresses = map[string][]ApiAddress{}

			for apiKey, instances := range peer.ApiAddresses {
				existing.ApiAddresses[apiKey] = nil

				for _, instance := range instances {
					existing.ApiAddresses[apiKey] = append(existing.ApiAddresses[apiKey], ApiAddress{
						Url:     instance.Url,
						Version: instance.Version,
					})
				}
			}

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
				CtrlAddress:  peer.Addr,
				IsOnline:     true,
				LastJoinedAt: &now,
				ApiAddresses: map[string][]ApiAddress{},
			}

			for apiKey, instances := range peer.ApiAddresses {
				newController.ApiAddresses[apiKey] = nil

				for _, instance := range instances {
					newController.ApiAddresses[apiKey] = append(newController.ApiAddresses[apiKey], ApiAddress{
						Url:     instance.Url,
						Version: instance.Version,
					})
				}
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
