/*
	Copyright NetFoundry, Inc.

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
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/util/errorz"
	"github.com/skip2/go-qrcode"
	"go.etcd.io/bbolt"
	"strings"
)

const (
	WindowSizeTOTP int = 5
)

func NewMfaHandler(env Env) *MfaHandler {
	handler := &MfaHandler{
		baseEntityManager: newBaseEntityManager(env, env.GetStores().Mfa),
	}
	handler.impl = handler

	return handler
}

type MfaHandler struct {
	baseEntityManager
}

func (handler *MfaHandler) newModelEntity() boltEntitySink {
	return &Mfa{}
}

func (handler *MfaHandler) CreateForIdentity(identity *Identity) (string, error) {
	secretBytes := make([]byte, 10)
	_, _ = rand.Read(secretBytes)
	secret := base32.StdEncoding.EncodeToString(secretBytes)

	recoveryCodes := handler.generateRecoveryCodes()

	mfa := &Mfa{
		BaseEntity:    models.BaseEntity{},
		IsVerified:    false,
		IdentityId:    identity.Id,
		Identity:      identity,
		Secret:        secret,
		RecoveryCodes: recoveryCodes,
	}

	return handler.Create(mfa)
}

func (handler *MfaHandler) Create(entity *Mfa) (string, error) {
	return handler.createEntity(entity)
}

func (handler *MfaHandler) Read(id string) (*Mfa, error) {
	modelMfa := &Mfa{}
	if err := handler.readEntity(id, modelMfa); err != nil {
		return nil, err
	}
	return modelMfa, nil
}

func (handler *MfaHandler) readInTx(tx *bbolt.Tx, id string) (*Mfa, error) {
	modelMfa := &Mfa{}
	if err := handler.readEntityInTx(tx, id, modelMfa); err != nil {
		return nil, err
	}
	return modelMfa, nil
}

func (handler *MfaHandler) IsUpdated(field string) bool {
	return field == persistence.FieldMfaIsVerified || field == persistence.FieldMfaRecoveryCodes
}

func (handler *MfaHandler) Update(Mfa *Mfa) error {
	return handler.updateEntity(Mfa, handler)
}

func (handler *MfaHandler) Delete(id string) error {
	return handler.deleteEntity(id)
}

func (handler *MfaHandler) Query(query string) (*MfaListResult, error) {
	result := &MfaListResult{handler: handler}
	err := handler.list(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *MfaHandler) ReadByIdentityId(identityId string) (*Mfa, error) {
	query := fmt.Sprintf(`identity = "%s"`, identityId)

	resultList, err := handler.Query(query)

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

func (handler *MfaHandler) Verify(mfa *Mfa, code string) (bool, error) {
	//check recovery codes
	for i, recoveryCode := range mfa.RecoveryCodes {
		if recoveryCode == code {
			mfa.RecoveryCodes = append(mfa.RecoveryCodes[:i], mfa.RecoveryCodes[i+1:]...)
			if err := handler.Update(mfa); err != nil {
				return false, err
			}
			return true, nil
		}
	}

	return handler.VerifyTOTP(mfa, code)
}

// VerifyTOTP verifies TOTP values only, not recovery codes
func (handler *MfaHandler) VerifyTOTP(mfa *Mfa, code string) (bool, error) {
	otp := dgoogauth.OTPConfig{
		Secret:     mfa.Secret,
		WindowSize: WindowSizeTOTP,
		UTC:        true,
	}

	return otp.Authenticate(code)
}

func (handler *MfaHandler) DeleteForIdentity(identity *Identity, code string) error {
	mfa, err := handler.ReadByIdentityId(identity.Id)

	if err != nil {
		return err
	}

	if mfa == nil {
		return errorz.NewNotFound()
	}

	if mfa.IsVerified {
		//if MFA is enabled require a valid code
		valid, err := handler.Verify(mfa, code)

		if err != nil || !valid {
			return apierror.NewInvalidMfaTokenError()
		}
	}

	if err = handler.Delete(mfa.Id); err != nil {
		return err
	}

	return nil
}

func (handler *MfaHandler) QrCodePng(mfa *Mfa) ([]byte, error) {
	if mfa.IsVerified {
		return nil, fmt.Errorf("MFA is already verified")
	}

	url := handler.GetProvisioningUrl(mfa)

	return qrcode.Encode(url, qrcode.Medium, 256)
}

func (handler *MfaHandler) GetProvisioningUrl(mfa *Mfa) string {
	otcConfig := &dgoogauth.OTPConfig{
		Secret:     mfa.Secret,
		WindowSize: WindowSizeTOTP,
		UTC:        true,
	}
	return otcConfig.ProvisionURIWithIssuer(mfa.Identity.Name, "ziti.dev")
}

func (handler *MfaHandler) RecreateRecoveryCodes(mfa *Mfa) error {
	newCodes := handler.generateRecoveryCodes()

	mfa.RecoveryCodes = newCodes

	return handler.Update(mfa)
}

func (handler *MfaHandler) generateRecoveryCodes() []string {
	recoveryCodes := []string{}

	for i := 0; i < 20; i++ {
		backupBytes := make([]byte, 8)
		rand.Read(backupBytes)
		backupStr := base32.StdEncoding.EncodeToString(backupBytes)
		backupCode := strings.Replace(backupStr, "=", "", -1)[:6]
		recoveryCodes = append(recoveryCodes, backupCode)
	}

	return recoveryCodes
}

type MfaListResult struct {
	handler *MfaHandler
	Mfas    []*Mfa
	models.QueryMetaData
}

func (result *MfaListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *models.QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		Mfa, err := result.handler.readInTx(tx, key)
		if err != nil {
			return err
		}
		result.Mfas = append(result.Mfas, Mfa)
	}
	return nil
}
