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
	"strings"
	"time"

	"github.com/dgryski/dgoogauth"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/common/pb/cmd_pb"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/common/pb/edge_cmd_pb"
	"github.com/openziti/ziti/v2/controller/apierror"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/fields"
	"github.com/openziti/ziti/v2/controller/models"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"github.com/skip2/go-qrcode"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
)

const (
	WindowSizeTOTP int = 5

	// TotpMaxFailedAttempts is how many codes an MFA record may get wrong inside
	// TotpFailedAttemptWindow before further attempts are refused. A TOTP code is six digits
	// and WindowSizeTOTP of them are valid at any moment, so without a cap an attacker holding
	// the first factor can reach a working code by guessing.
	TotpMaxFailedAttempts int = 10

	// TotpFailedAttemptWindow is how long failed attempts count against the limit, and how
	// long a record stays refused once the limit is reached. A correct code clears the count,
	// so a user who mistypes and then gets it right is never held.
	TotpFailedAttemptWindow = 5 * time.Minute
)

func NewMfaManager(env Env) *MfaManager {
	manager := &MfaManager{
		baseEntityManager: newBaseEntityManager[*Mfa, *db.Mfa](env, env.GetStores().Mfa),
		failedAttempts:    cmap.New[*totpFailedAttempts](),
	}
	manager.impl = manager

	RegisterManagerDecoder[*Mfa](env, manager)
	RegisterCommand(env, &RemoveMfaForIdentityCmd{}, &edge_cmd_pb.RemoveMfaForIdentityCmd{})

	return manager
}

type MfaManager struct {
	baseEntityManager[*Mfa, *db.Mfa]

	// failedAttempts holds one entry per MFA record that has recently failed a code check, so
	// it is bounded by the number of enrolled identities. Entries are dropped on a correct
	// code and when the record is deleted.
	failedAttempts cmap.ConcurrentMap[string, *totpFailedAttempts]
}

// totpFailedAttempts counts failures for a single MFA record inside one window.
type totpFailedAttempts struct {
	count     int
	windowEnd time.Time
}

func (self *MfaManager) NewModelEntity() *Mfa {
	return &Mfa{}
}

func (self *MfaManager) CreateForIdentityId(identityId string, ctx *change.Context) (string, error) {
	identity, err := self.env.GetManagers().Identity.Read(identityId)

	if err != nil {
		return "", err
	}

	return self.CreateForIdentity(identity, ctx)
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
	return DispatchCreate[*Mfa](self, entity, ctx)
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
	return DispatchUpdate[*Mfa](self, entity, checker, ctx)
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

// Verify will attempt to check a code (recovery or totp) against the current secret. Once an
// MFA record has failed TotpMaxFailedAttempts checks inside TotpFailedAttemptWindow it is
// refused with MFA_TOO_MANY_ATTEMPTS until the window passes.
func (self *MfaManager) Verify(mfa *Mfa, code string, ctx *change.Context) (bool, error) {
	if self.isRefusingAttempts(mfa.Id) {
		return false, apierror.NewMfaTooManyAttemptsError()
	}

	//check recovery codes
	for i, recoveryCode := range mfa.RecoveryCodes {
		if recoveryCode == code {
			mfa.RecoveryCodes = append(mfa.RecoveryCodes[:i], mfa.RecoveryCodes[i+1:]...)
			if err := self.Update(mfa, nil, ctx); err != nil {
				return false, err
			}
			self.clearFailedAttempts(mfa.Id)
			return true, nil
		}
	}

	return self.VerifyTOTP(mfa, code)
}

// VerifyTOTP verifies TOTP values only, not recovery codes. It shares the failed attempt
// limit with Verify.
func (self *MfaManager) VerifyTOTP(mfa *Mfa, code string) (bool, error) {
	if self.isRefusingAttempts(mfa.Id) {
		return false, apierror.NewMfaTooManyAttemptsError()
	}

	otp := dgoogauth.OTPConfig{
		Secret:     mfa.Secret,
		WindowSize: WindowSizeTOTP,
		UTC:        true,
	}

	ok, err := otp.Authenticate(code)

	if errors.Is(err, dgoogauth.ErrInvalidCode) {
		// A code that is not six digits is reported as an error rather than a false result.
		// Callers treat both the same way, so it counts as a failed attempt and returns false.
		err = nil
	}

	if err != nil {
		return false, err
	}

	if !ok {
		self.recordFailedAttempt(mfa.Id)
		return false, nil
	}

	self.clearFailedAttempts(mfa.Id)

	return true, nil
}

// isRefusingAttempts reports whether the record has used up its attempts for the current
// window. The check and the later count are separate, so a burst of concurrent requests can
// slip a few attempts past the limit. That costs an attacker a handful of guesses per window
// and keeps the check off the lock held during verification.
func (self *MfaManager) isRefusingAttempts(mfaId string) bool {
	attempts, found := self.failedAttempts.Get(mfaId)

	if !found {
		return false
	}

	return attempts.count >= TotpMaxFailedAttempts && time.Now().Before(attempts.windowEnd)
}

// recordFailedAttempt counts one wrong code. The window is not extended by later failures,
// so a record held at the limit is released TotpFailedAttemptWindow after the failure that
// opened the window.
func (self *MfaManager) recordFailedAttempt(mfaId string) {
	self.failedAttempts.Upsert(mfaId, nil, func(exists bool, current *totpFailedAttempts, _ *totpFailedAttempts) *totpFailedAttempts {
		now := time.Now()

		if !exists || current == nil || now.After(current.windowEnd) {
			return &totpFailedAttempts{count: 1, windowEnd: now.Add(TotpFailedAttemptWindow)}
		}

		return &totpFailedAttempts{count: current.count + 1, windowEnd: current.windowEnd}
	})
}

func (self *MfaManager) clearFailedAttempts(mfaId string) {
	self.failedAttempts.Remove(mfaId)
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

	self.clearFailedAttempts(mfa.Id)

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
	return otcConfig.ProvisionURIWithIssuer(mfa.Identity.Name, self.env.GetConfig().Edge.Totp.Hostname)
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
		backupCode := strings.ReplaceAll(backupStr, "=", "")[:6]
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
	return self.Dispatch(&RemoveMfaForIdentityCmd{
		manager:    self,
		identityId: id,
		ctx:        ctx,
	})
}

func (self *MfaManager) ApplyRemoveMfaForIdentityCommand(cmd *RemoveMfaForIdentityCmd, ctx boltz.MutateContext) error {
	return self.GetDb().Update(ctx, func(ctx boltz.MutateContext) error {
		return self.Store.DeleteWhere(ctx, fmt.Sprintf(`identity = "%s"`, cmd.identityId))
	})
}

func (self *MfaManager) CompleteTotpEnrollment(identityId string, code string, changeCtx *change.Context) error {
	mfa, err := self.ReadOneByIdentityId(identityId)

	if err != nil {
		return err
	}

	if mfa == nil || mfa.IsVerified {
		return errorz.NewNotFound()
	}

	ok, err := self.VerifyTOTP(mfa, code)

	if err != nil {
		// A refused attempt is reported as such. Anything else is a verification failure and
		// stays indistinguishable from a wrong code.
		apiErr := &errorz.ApiError{}
		if errors.As(err, &apiErr) {
			return apiErr
		}

		pfxlog.Logger().WithError(err).Error("could not verify TOTP code")
		return apierror.NewInvalidMfaTokenError()
	}

	if !ok {
		return apierror.NewInvalidMfaTokenError()
	}

	mfa.IsVerified = true
	if err := self.Update(mfa, nil, changeCtx); err != nil {
		pfxlog.Logger().Errorf("could not update MFA with new MFA status: %v", err)
		return errors.New("could not update MFA status")
	}

	return nil
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

type RemoveMfaForIdentityCmd struct {
	ctx        *change.Context
	manager    *MfaManager
	identityId string
}

func (self *RemoveMfaForIdentityCmd) Apply(ctx boltz.MutateContext) error {
	return self.manager.ApplyRemoveMfaForIdentityCommand(self, ctx)
}

func (self *RemoveMfaForIdentityCmd) Encode() ([]byte, error) {
	cmd := &edge_cmd_pb.RemoveMfaForIdentityCmd{
		Ctx:        ContextToProtobuf(self.ctx),
		IdentityId: self.identityId,
	}
	return cmd_pb.EncodeProtobuf(cmd)
}

func (self *RemoveMfaForIdentityCmd) Decode(env Env, msg *edge_cmd_pb.RemoveMfaForIdentityCmd) error {
	self.ctx = ProtobufToContext(msg.Ctx)
	self.manager = env.GetManagers().Mfa
	self.identityId = msg.IdentityId
	return nil
}

func (self *RemoveMfaForIdentityCmd) GetChangeContext() *change.Context {
	return self.ctx
}
