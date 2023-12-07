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
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"github.com/dgryski/dgoogauth"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/pb/edge_cmd_pb"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/network"
	"github.com/pkg/errors"
	"github.com/skip2/go-qrcode"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
	"strings"
)

const (
	WindowSizeTOTP int = 5
)

func NewMfaManager(env Env) *MfaManager {
	manager := &MfaManager{
		baseEntityManager: newBaseEntityManager[*Mfa, *db.Mfa](env, env.GetStores().Mfa),
	}
	manager.impl = manager

	network.RegisterManagerDecoder[*Mfa](env.GetHostController().GetNetwork().Managers, manager)

	return manager
}

type MfaManager struct {
	baseEntityManager[*Mfa, *db.Mfa]
}

func (self *MfaManager) newModelEntity() *Mfa {
	return &Mfa{}
}

func (self *MfaManager) CreateForIdentity(identity *Identity, ctx *change.Context) (string, error) {
	secretBytes := make([]byte, 10)
	_, _ = rand.Read(secretBytes)
	secret := base32.StdEncoding.EncodeToString(secretBytes)

	recoveryCodes, err := self.generateRecoveryCodes()
	if err != nil {
		return "", err
	}

	mfa := &Mfa{
		BaseEntity:    models.BaseEntity{},
		IsVerified:    false,
		IdentityId:    identity.Id,
		Identity:      identity,
		Secret:        secret,
		RecoveryCodes: recoveryCodes,
	}

	err = self.Create(mfa, ctx)
	if err != nil {
		return "", err
	}
	return mfa.Id, err
}

func (self *MfaManager) Create(entity *Mfa, ctx *change.Context) error {
	return network.DispatchCreate[*Mfa](self, entity, ctx)
}

func (self *MfaManager) ApplyCreate(cmd *command.CreateEntityCommand[*Mfa], ctx boltz.MutateContext) error {
	return self.GetDb().Update(ctx, func(ctx boltz.MutateContext) error {
		result := &MfaListResult{manager: self}
		err := self.ListWithTx(ctx.Tx(), fmt.Sprintf(`identity = "%s"`, cmd.Entity.IdentityId), result.collect)

		if err != nil {
			return err
		}

		if len(result.Mfas) > 0 {
			return apierror.NewMfaExistsError()
		}

		_, err = self.createEntityInTx(ctx, cmd.Entity)

		return err
	})
}

func (self *MfaManager) Update(entity *Mfa, checker fields.UpdatedFields, ctx *change.Context) error {
	return network.DispatchUpdate[*Mfa](self, entity, checker, ctx)
}

func (self *MfaManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*Mfa], ctx boltz.MutateContext) error {
	var checker boltz.FieldChecker = self
	if cmd.UpdatedFields != nil {
		checker = &AndFieldChecker{first: self, second: cmd.UpdatedFields}
	}
	return self.updateEntity(cmd.Entity, checker, ctx)
}

func (self *MfaManager) IsUpdated(field string) bool {
	return field == db.FieldMfaIsVerified || field == db.FieldMfaRecoveryCodes
}

func (self *MfaManager) Query(query string) (*MfaListResult, error) {
	result := &MfaListResult{manager: self}
	err := self.ListWithHandler(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *MfaManager) ReadOneByIdentityId(identityId string) (*Mfa, error) {
	query := fmt.Sprintf(`identity = "%s"`, identityId)

	resultList, err := self.Query(query)

	if err != nil {
		return nil, err
	}

	if resultList.Count > 1 {
		return nil, fmt.Errorf("too many MFAs associated to a single identity, expected 1 got %d for identityId %s", resultList.Count, identityId)
	}

	if resultList.Count == 0 {
		return nil, nil
	}

	return resultList.Mfas[0], nil
}

func (self *MfaManager) Verify(mfa *Mfa, code string, ctx *change.Context) (bool, error) {
	//check recovery codes
	for i, recoveryCode := range mfa.RecoveryCodes {
		if recoveryCode == code {
			mfa.RecoveryCodes = append(mfa.RecoveryCodes[:i], mfa.RecoveryCodes[i+1:]...)
			if err := self.Update(mfa, nil, ctx); err != nil {
				return false, err
			}
			return true, nil
		}
	}

	return self.VerifyTOTP(mfa, code)
}

// VerifyTOTP verifies TOTP values only, not recovery codes
func (self *MfaManager) VerifyTOTP(mfa *Mfa, code string) (bool, error) {
	otp := dgoogauth.OTPConfig{
		Secret:     mfa.Secret,
		WindowSize: WindowSizeTOTP,
		UTC:        true,
	}

	return otp.Authenticate(code)
}

func (self *MfaManager) DeleteForIdentity(identity *Identity, code string, ctx *change.Context) error {
	mfa, err := self.ReadOneByIdentityId(identity.Id)

	if err != nil {
		return err
	}

	if mfa == nil {
		return errorz.NewNotFound()
	}

	if mfa.IsVerified {
		//if MFA is enabled require a valid code
		valid, err := self.Verify(mfa, code, ctx)

		if err != nil || !valid {
			return apierror.NewInvalidMfaTokenError()
		}
	}

	if err = self.Delete(mfa.Id, ctx); err != nil {
		return err
	}

	return nil
}

func (self *MfaManager) QrCodePng(mfa *Mfa) ([]byte, error) {
	if mfa.IsVerified {
		return nil, fmt.Errorf("MFA is already verified")
	}

	url := self.GetProvisioningUrl(mfa)

	return qrcode.Encode(url, qrcode.Medium, 256)
}

func (self *MfaManager) GetProvisioningUrl(mfa *Mfa) string {
	otcConfig := &dgoogauth.OTPConfig{
		Secret:     mfa.Secret,
		WindowSize: WindowSizeTOTP,
		UTC:        true,
	}
	return otcConfig.ProvisionURIWithIssuer(mfa.Identity.Name, self.env.GetConfig().Totp.Hostname)
}

func (self *MfaManager) RecreateRecoveryCodes(mfa *Mfa, ctx *change.Context) error {
	newCodes, err := self.generateRecoveryCodes()
	if err != nil {
		return err
	}

	mfa.RecoveryCodes = newCodes

	return self.Update(mfa, nil, ctx)
}

func (self *MfaManager) generateRecoveryCodes() ([]string, error) {
	recoveryCodes := []string{}

	for i := 0; i < 20; i++ {
		backupBytes := make([]byte, 8)
		if _, err := rand.Read(backupBytes); err != nil {
			return nil, err
		}
		backupStr := base32.StdEncoding.EncodeToString(backupBytes)
		backupCode := strings.Replace(backupStr, "=", "", -1)[:6]
		recoveryCodes = append(recoveryCodes, backupCode)
	}

	return recoveryCodes, nil
}

func (self *MfaManager) Marshall(entity *Mfa) ([]byte, error) {
	tags, err := edge_cmd_pb.EncodeTags(entity.Tags)
	if err != nil {
		return nil, err
	}

	msg := &edge_cmd_pb.Mfa{
		Id:            entity.Id,
		Tags:          tags,
		IsVerified:    entity.IsVerified,
		IdentityId:    entity.IdentityId,
		Secret:        entity.Secret,
		RecoveryCodes: entity.RecoveryCodes,
	}

	return proto.Marshal(msg)
}

func (self *MfaManager) Unmarshall(bytes []byte) (*Mfa, error) {
	msg := &edge_cmd_pb.Mfa{}
	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}

	identity, err := self.env.GetManagers().Identity.Read(msg.IdentityId)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to lookup identity for mfa with id=[%v]", msg.Id)
	}

	return &Mfa{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: edge_cmd_pb.DecodeTags(msg.Tags),
		},
		IsVerified:    msg.IsVerified,
		IdentityId:    msg.IdentityId,
		Identity:      identity,
		Secret:        msg.Secret,
		RecoveryCodes: msg.RecoveryCodes,
	}, nil
}

// DeleteAllForIdentity is meant for administrators to remove all MFAs (enrolled or not) from an identity
func (self *MfaManager) DeleteAllForIdentity(id string, ctx *change.Context) error {
	return self.GetDb().Update(ctx.NewMutateContext(), func(ctx boltz.MutateContext) error {
		return self.Store.DeleteWhere(ctx, fmt.Sprintf("identity = \"%s\"", id))
	})
}

type MfaListResult struct {
	manager *MfaManager
	Mfas    []*Mfa
	models.QueryMetaData
}

func (result *MfaListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *models.QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		Mfa, err := result.manager.readInTx(tx, key)
		if err != nil {
			return err
		}
		result.Mfas = append(result.Mfas, Mfa)
	}
	return nil
}
