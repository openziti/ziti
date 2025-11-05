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

package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-openapi/jsonpointer"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/apierror"
)

const (
	FieldExternalJwtSignerFingerprint                   = "fingerprint"
	FieldExternalJwtSignerCertPem                       = "certPem"
	FieldExternalJwtSignerJwksEndpoint                  = "jwksEndpoint"
	FieldExternalJwtSignerCommonName                    = "commonName"
	FieldExternalJwtSignerNotAfter                      = "notAfter"
	FieldExternalJwtSignerNotBefore                     = "notBefore"
	FieldExternalJwtSignerEnabled                       = "enabled"
	FieldExternalJwtSignerExternalAuthUrl               = "externalAuthUrl"
	FieldExternalJwtSignerAuthPolicies                  = "authPolicies"
	FieldExternalJwtSignerIdentityIdClaimSelector       = "claimsProperty"
	FieldExternalJwtSignerUseExternalId                 = "useExternalId"
	FieldExternalJwtSignerKid                           = "kid"
	FieldExternalJwtSignerIssuer                        = "issuer"
	FieldExternalJwtSignerAudience                      = "audience"
	FieldExternalJwtSignerClientId                      = "clientId"
	FieldExternalJwtSignerScopes                        = "scopes"
	FieldExternalJwtSignerTargetToken                   = "targetToken"
	FieldExternalJwtSignerEnrollmentToCertEnabled       = "enrollToCertEnabled"
	FieldExternalJwtSignerEnrollToTokenEnabled          = "enrollToTokenEnabled"
	FieldExternalJwtSignerEnrollAttributeClaimsSelector = "enrollAttributeClaimsSelector"
	FieldExternalJwtSignerEnrollNameClaimsSelector      = "enrollNameClaimsSelector"
	FieldExternalJwtSignerEnrollAuthPolicyId            = "enrollAuthPolicyId"

	DefaultIdentityIdClaimsSelector        = "/sub"
	DefaultEnrollIdentityNameClaimSelector = "/sub"

	TargetTokenAccess = "ACCESS"
)

type ExternalJwtSigner struct {
	boltz.BaseExtEntity
	Name                          string     `json:"name"`
	Fingerprint                   *string    `json:"fingerprint"`
	Kid                           *string    `json:"kid"`
	CertPem                       *string    `json:"certPem"`
	JwksEndpoint                  *string    `json:"jwksEndpoint"`
	CommonName                    string     `json:"commonName"`
	NotAfter                      *time.Time `json:"notAfter"`
	NotBefore                     *time.Time `json:"notBefore"`
	Enabled                       bool       `json:"enabled"`
	ExternalAuthUrl               *string    `json:"externalAuthUrl"`
	IdentityIdClaimsSelector      *string    `json:"identityIdClaimsSelector"`
	UseExternalId                 bool       `json:"useExternalId"`
	Issuer                        *string    `json:"issuer"`
	Audience                      *string    `json:"audience"`
	ClientId                      *string    `json:"clientId"`
	Scopes                        []string   `json:"scopes"`
	TargetToken                   string     `json:"targetToken"`
	EnrollToCertEnabled           bool       `json:"enrollToCertEnabled"`
	EnrollToTokenEnabled          bool       `json:"enrollToTokenEnabled"`
	EnrollAttributeClaimsSelector string     `json:"enrollAttributeClaimsSelector"`
	EnrollAuthPolicyId            string     `json:"enrollAuthPolicyId"`
	EnrollNameClaimSelector       string     `json:"enrollNameClaimsSelector"`
}

func (entity *ExternalJwtSigner) GetName() string {
	return entity.Name
}

func (entity *ExternalJwtSigner) GetEntityType() string {
	return EntityTypeExternalJwtSigners
}

var _ ExternalJwtSignerStore = (*externalJwtSignerStoreImpl)(nil)

type ExternalJwtSignerStore interface {
	NameIndexed
	Store[*ExternalJwtSigner]
}

func newExternalJwtSignerStore(stores *stores) *externalJwtSignerStoreImpl {
	store := &externalJwtSignerStoreImpl{}
	store.baseStore = newBaseStore[*ExternalJwtSigner](stores, store)
	store.InitImpl(store)
	return store
}

type externalJwtSignerStoreImpl struct {
	*baseStore[*ExternalJwtSigner]
	indexName          boltz.ReadIndex
	symbolFingerprint  boltz.EntitySymbol
	symbolAuthPolicies boltz.EntitySetSymbol
	fingerprintIndex   boltz.ReadIndex
	symbolKid          boltz.EntitySymbol
	kidIndex           boltz.ReadIndex
	symbolIssuer       boltz.EntitySymbol
	issuerIndex        boltz.ReadIndex
	enrollAuthPolicyId boltz.EntitySymbol
}

func (store *externalJwtSignerStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *externalJwtSignerStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.indexName = store.addUniqueNameField()

	store.symbolFingerprint = store.AddSymbol(FieldExternalJwtSignerFingerprint, ast.NodeTypeString)
	store.fingerprintIndex = store.AddNullableUniqueIndex(store.symbolFingerprint)

	store.symbolKid = store.AddSymbol(FieldExternalJwtSignerKid, ast.NodeTypeString)
	store.kidIndex = store.AddNullableUniqueIndex(store.symbolKid)

	store.symbolIssuer = store.AddSymbol(FieldExternalJwtSignerIssuer, ast.NodeTypeString)
	store.issuerIndex = store.AddUniqueIndex(store.symbolIssuer)

	store.AddSymbol(FieldExternalJwtSignerCertPem, ast.NodeTypeString)
	store.AddSymbol(FieldExternalJwtSignerCommonName, ast.NodeTypeString)
	store.AddSymbol(FieldExternalJwtSignerNotAfter, ast.NodeTypeDatetime)
	store.AddSymbol(FieldExternalJwtSignerNotBefore, ast.NodeTypeDatetime)
	store.AddSymbol(FieldExternalJwtSignerEnabled, ast.NodeTypeBool)
	store.AddSymbol(FieldExternalJwtSignerIdentityIdClaimSelector, ast.NodeTypeString)
	store.AddSymbol(FieldExternalJwtSignerUseExternalId, ast.NodeTypeBool)
	store.AddSymbol(FieldExternalJwtSignerAudience, ast.NodeTypeString)
	store.AddSymbol(FieldExternalJwtSignerClientId, ast.NodeTypeString)
	store.AddSymbol(FieldExternalJwtSignerScopes, ast.NodeTypeString)
	store.AddSymbol(FieldExternalJwtSignerTargetToken, ast.NodeTypeString)
	store.AddSymbol(FieldExternalJwtSignerEnrollmentToCertEnabled, ast.NodeTypeBool)
	store.AddSymbol(FieldExternalJwtSignerEnrollToTokenEnabled, ast.NodeTypeBool)
	store.AddSymbol(FieldExternalJwtSignerEnrollAttributeClaimsSelector, ast.NodeTypeString)
	store.AddSymbol(FieldExternalJwtSignerEnrollNameClaimsSelector, ast.NodeTypeString)

	store.enrollAuthPolicyId = store.AddFkSymbol(FieldExternalJwtSignerEnrollAuthPolicyId, store.stores.authPolicy)
	store.AddFkConstraint(store.enrollAuthPolicyId, true, boltz.CascadeNone)

	store.symbolAuthPolicies = store.AddFkSetSymbol(FieldExternalJwtSignerAuthPolicies, store.stores.authPolicy)
}

func (store *externalJwtSignerStoreImpl) initializeLinked() {
}

func (store *externalJwtSignerStoreImpl) NewEntity() *ExternalJwtSigner {
	return &ExternalJwtSigner{}
}

func (store *externalJwtSignerStoreImpl) FillEntity(entity *ExternalJwtSigner, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringWithDefault(FieldName, "")
	entity.CertPem = bucket.GetString(FieldExternalJwtSignerCertPem)
	entity.JwksEndpoint = bucket.GetString(FieldExternalJwtSignerJwksEndpoint)
	entity.Fingerprint = bucket.GetString(FieldExternalJwtSignerFingerprint)
	entity.Kid = bucket.GetString(FieldExternalJwtSignerKid)
	entity.CommonName = bucket.GetStringWithDefault(FieldExternalJwtSignerCommonName, "")
	entity.NotAfter = bucket.GetTime(FieldExternalJwtSignerNotAfter)
	entity.NotBefore = bucket.GetTime(FieldExternalJwtSignerNotBefore)
	entity.Enabled = bucket.GetBoolWithDefault(FieldExternalJwtSignerEnabled, false)
	entity.ExternalAuthUrl = bucket.GetString(FieldExternalJwtSignerExternalAuthUrl)
	entity.IdentityIdClaimsSelector = bucket.GetString(FieldExternalJwtSignerIdentityIdClaimSelector)
	entity.UseExternalId = bucket.GetBoolWithDefault(FieldExternalJwtSignerUseExternalId, false)
	entity.Issuer = bucket.GetString(FieldExternalJwtSignerIssuer)
	entity.Audience = bucket.GetString(FieldExternalJwtSignerAudience)
	entity.ClientId = bucket.GetString(FieldExternalJwtSignerClientId)
	entity.Scopes = bucket.GetStringList(FieldExternalJwtSignerScopes)
	entity.TargetToken = bucket.GetStringWithDefault(FieldExternalJwtSignerTargetToken, TargetTokenAccess)
	entity.EnrollToCertEnabled = bucket.GetBoolWithDefault(FieldExternalJwtSignerEnrollmentToCertEnabled, false)
	entity.EnrollToTokenEnabled = bucket.GetBoolWithDefault(FieldExternalJwtSignerEnrollToTokenEnabled, false)
	entity.EnrollAttributeClaimsSelector = bucket.GetStringWithDefault(FieldExternalJwtSignerEnrollAttributeClaimsSelector, "")
	entity.EnrollNameClaimSelector = bucket.GetStringWithDefault(FieldExternalJwtSignerEnrollNameClaimsSelector, DefaultEnrollIdentityNameClaimSelector)
	entity.EnrollAuthPolicyId = bucket.GetStringWithDefault(FieldExternalJwtSignerEnrollAuthPolicyId, "")

	if entity.TargetToken == "" {
		entity.TargetToken = TargetTokenAccess
	}
}

func (store *externalJwtSignerStoreImpl) PersistEntity(entity *ExternalJwtSigner, ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)

	ctx.SetString(FieldName, entity.Name)
	ctx.SetStringP(FieldExternalJwtSignerCertPem, entity.CertPem)
	ctx.SetStringP(FieldExternalJwtSignerJwksEndpoint, entity.JwksEndpoint)
	ctx.SetStringP(FieldExternalJwtSignerFingerprint, entity.Fingerprint)
	ctx.SetStringP(FieldExternalJwtSignerKid, entity.Kid)
	ctx.SetString(FieldExternalJwtSignerCommonName, entity.CommonName)
	ctx.SetTimeP(FieldExternalJwtSignerNotAfter, entity.NotAfter)
	ctx.SetTimeP(FieldExternalJwtSignerNotBefore, entity.NotBefore)
	ctx.SetBool(FieldExternalJwtSignerEnabled, entity.Enabled)
	ctx.SetBool(FieldExternalJwtSignerUseExternalId, entity.UseExternalId)
	ctx.SetStringP(FieldExternalJwtSignerClientId, entity.ClientId)
	ctx.SetStringList(FieldExternalJwtSignerScopes, entity.Scopes)
	ctx.SetString(FieldExternalJwtSignerTargetToken, entity.TargetToken)
	ctx.SetBool(FieldExternalJwtSignerEnrollmentToCertEnabled, entity.EnrollToCertEnabled)
	ctx.SetBool(FieldExternalJwtSignerEnrollToTokenEnabled, entity.EnrollToTokenEnabled)
	ctx.SetString(FieldExternalJwtSignerEnrollAttributeClaimsSelector, entity.EnrollAttributeClaimsSelector)

	if entity.EnrollNameClaimSelector == "" {
		entity.EnrollNameClaimSelector = DefaultEnrollIdentityNameClaimSelector
	}
	ctx.SetString(FieldExternalJwtSignerEnrollNameClaimsSelector, entity.EnrollNameClaimSelector)

	if entity.EnrollAuthPolicyId == "" {
		entity.EnrollAuthPolicyId = DefaultAuthPolicyId
	}
	ctx.SetString(FieldExternalJwtSignerEnrollAuthPolicyId, entity.EnrollAuthPolicyId)

	if entity.ExternalAuthUrl != nil && strings.TrimSpace(*entity.ExternalAuthUrl) == "" {
		entity.ExternalAuthUrl = nil
	}
	ctx.SetStringP(FieldExternalJwtSignerExternalAuthUrl, entity.ExternalAuthUrl)

	if entity.Issuer != nil && strings.TrimSpace(*entity.Issuer) == "" {
		entity.Issuer = nil
	}
	ctx.SetStringP(FieldExternalJwtSignerIssuer, entity.Issuer)

	if entity.Audience != nil && strings.TrimSpace(*entity.Audience) == "" {
		entity.Audience = nil
	}
	ctx.SetStringP(FieldExternalJwtSignerAudience, entity.Audience)

	if entity.IdentityIdClaimsSelector == nil || strings.TrimSpace(*entity.IdentityIdClaimsSelector) == "" {
		ctx.SetString(FieldExternalJwtSignerIdentityIdClaimSelector, DefaultIdentityIdClaimsSelector)
	} else {
		ctx.SetStringP(FieldExternalJwtSignerIdentityIdClaimSelector, entity.IdentityIdClaimsSelector)
	}

	jwksEndpoint := ctx.Bucket.GetString(FieldExternalJwtSignerJwksEndpoint)
	certPem := ctx.Bucket.GetString(FieldExternalJwtSignerCertPem)

	if (jwksEndpoint == nil || *jwksEndpoint == "") && (certPem == nil || *certPem == "") {
		ctx.Bucket.SetError(apierror.NewBadRequestFieldError(*errorz.NewFieldError("jwksEndpoint or certPem is required", "certPem", certPem)))
	}

	if jwksEndpoint != nil && certPem != nil {
		ctx.Bucket.SetError(apierror.NewBadRequestFieldError(
			*errorz.NewFieldError("only one of jwksEndpoint or certPem may be defined", FieldExternalJwtSignerJwksEndpoint, jwksEndpoint)))
	}

	var authPolicy *AuthPolicy
	enrollAuthPolicyId := ctx.Bucket.GetStringWithDefault(FieldExternalJwtSignerEnrollAuthPolicyId, "")

	if enrollAuthPolicyId != "" {
		var err error
		authPolicy, _, err = store.stores.authPolicy.FindById(ctx.Tx(), enrollAuthPolicyId)
		if err != nil {
			ctx.Bucket.SetError(errorz.NewFieldApiError(errorz.NewFieldError("the auth policy could not be found", "enrollAuthPolicyId", entity.EnrollAuthPolicyId)))
			return
		}
	}

	enrollToTokenEnabled := ctx.Bucket.GetBoolWithDefault(FieldExternalJwtSignerEnrollToTokenEnabled, false)

	if enrollToTokenEnabled {
		if authPolicy == nil {
			ctx.Bucket.SetError(errorz.NewFieldApiError(errorz.NewFieldError("the auth policy must be specified if enroll to token is enabled", "enrollAuthPolicyId", entity.EnrollAuthPolicyId)))
			return
		}

		if !authPolicy.Primary.ExtJwt.Allowed {
			ctx.Bucket.SetError(errorz.NewFieldApiError(errorz.NewFieldError("primary external jwt authentication on auth policy is disabled", "enrollAuthPolicyId", entity.EnrollAuthPolicyId)))
			return
		}
	}

	enrollToCertEnabled := ctx.Bucket.GetBoolWithDefault(FieldExternalJwtSignerEnrollmentToCertEnabled, false)

	if enrollToCertEnabled {
		if authPolicy == nil {
			ctx.Bucket.SetError(errorz.NewFieldApiError(errorz.NewFieldError("the auth policy must be specified if enroll to cert is enabled", "enrollAuthPolicyId", entity.EnrollAuthPolicyId)))
			return
		}

		if !authPolicy.Primary.Cert.Allowed {
			ctx.Bucket.SetError(errorz.NewFieldApiError(errorz.NewFieldError("primary certificate authentication on auth policy is disabled", "enrollAuthPolicyId", entity.EnrollAuthPolicyId)))
			return
		}
	}

	enrollAttributeClaimsSelector := ctx.Bucket.GetStringWithDefault(FieldExternalJwtSignerEnrollAttributeClaimsSelector, "")
	err := store.verifyJsonPointer(enrollAttributeClaimsSelector)

	if err != nil {
		ctx.Bucket.SetError(errorz.NewFieldApiError(errorz.NewFieldError("the attribute claims selector is invalid: "+err.Error(), FieldExternalJwtSignerEnrollAttributeClaimsSelector, enrollAttributeClaimsSelector)))
		return
	}

	enrollNameClaimSelector := ctx.Bucket.GetStringWithDefault(FieldExternalJwtSignerEnrollNameClaimsSelector, "")
	err = store.verifyJsonPointer(enrollNameClaimSelector)

	if err != nil {
		ctx.Bucket.SetError(errorz.NewFieldApiError(errorz.NewFieldError("the name attribute claims selector is invalid: "+err.Error(), FieldExternalJwtSignerEnrollNameClaimsSelector, enrollNameClaimSelector)))
		return
	}
}

func (store *externalJwtSignerStoreImpl) verifyJsonPointer(selector string) error {
	if selector == "" {
		return nil
	}

	if !strings.HasPrefix(selector, "/") {
		selector = "/" + selector
	}

	_, err := jsonpointer.New(selector)

	return err
}

func (store *externalJwtSignerStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	ids, _, err := store.stores.authPolicy.QueryIds(ctx.Tx(), fmt.Sprintf(`anyOf(%s) = "%s"`, FieldAuthPolicyPrimaryExtJwtAllowedSigners, id))

	if err != nil {
		return err
	}

	if len(ids) > 0 {
		return boltz.NewReferenceByIdsError(EntityTypeExternalJwtSigners, id, EntityTypeAuthPolicies, ids, FieldAuthPolicyPrimaryExtJwtAllowedSigners)
	}

	err = store.BaseStore.DeleteById(ctx, id)

	return err
}
