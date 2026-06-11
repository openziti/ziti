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
	"fmt"
	"net/url"

	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/x509-claims/x509claims"
	"github.com/openziti/ziti/v2/common/cert"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/apierror"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

type Ca struct {
	models.BaseEntity
	Name                      string
	Fingerprint               string
	CertPem                   string
	IsVerified                bool
	VerificationToken         string
	IsAutoCaEnrollmentEnabled bool
	IsOttCaEnrollmentEnabled  bool
	IsAuthEnabled             bool
	IdentityRoles             []string
	IdentityNameFormat        string
	ExternalIdClaim           *ExternalIdClaim
}

type ExternalIdClaim struct {
	Location        string
	Matcher         string
	MatcherCriteria string
	Parser          string
	ParserCriteria  string
	Index           int64
}

type ExternalIdFieldType string

func (entity *Ca) fillFrom(_ Env, _ *bbolt.Tx, boltCa *db.Ca) error {
	entity.FillCommon(boltCa)
	entity.Name = boltCa.Name
	entity.Fingerprint = boltCa.Fingerprint
	entity.CertPem = boltCa.CertPem
	entity.IsVerified = boltCa.IsVerified
	entity.VerificationToken = boltCa.VerificationToken
	entity.IsAutoCaEnrollmentEnabled = boltCa.IsAutoCaEnrollmentEnabled
	entity.IsOttCaEnrollmentEnabled = boltCa.IsOttCaEnrollmentEnabled
	entity.IsAuthEnabled = boltCa.IsAuthEnabled
	entity.IdentityRoles = boltCa.IdentityRoles
	entity.IdentityNameFormat = boltCa.IdentityNameFormat

	if boltCa.ExternalIdClaim != nil {
		entity.ExternalIdClaim = &ExternalIdClaim{}
		entity.ExternalIdClaim.Location = boltCa.ExternalIdClaim.Location
		entity.ExternalIdClaim.Index = boltCa.ExternalIdClaim.Index
		entity.ExternalIdClaim.Matcher = boltCa.ExternalIdClaim.Matcher
		entity.ExternalIdClaim.MatcherCriteria = boltCa.ExternalIdClaim.MatcherCriteria
		entity.ExternalIdClaim.Parser = boltCa.ExternalIdClaim.Parser
		entity.ExternalIdClaim.ParserCriteria = boltCa.ExternalIdClaim.ParserCriteria
	}

	return nil
}

func (entity *Ca) toBoltEntityForCreate(tx *bbolt.Tx, env Env) (*db.Ca, error) {
	if entity.IdentityNameFormat == "" {
		entity.IdentityNameFormat = DefaultCaIdentityNameFormat
	}

	if err := validateExternalIdClaim(entity.ExternalIdClaim); err != nil {
		return nil, err
	}

	var fp string

	if entity.CertPem != "" {
		blocks, err := cert.PemChain2Blocks(entity.CertPem)

		if err != nil {
			return nil, errorz.NewFieldError(err.Error(), "certPem", entity.CertPem)
		}

		if len(blocks) == 0 {
			return nil, errorz.NewFieldError("at least one leaf certificate must be supplied", "certPem", entity.CertPem)
		}

		certs, err := cert.Blocks2Certs(blocks)

		if err != nil {
			return nil, errorz.NewFieldError(err.Error(), "certPem", entity.CertPem)
		}

		leaf := certs[0]

		if !leaf.IsCA {
			//return nil, &response.ApiError{
			//	Code:           response.CertificateIsNotCaCode,
			//	Message:        response.CertificateIsNotCaMessage,
			//	HttpStatusCode: http.StatusBadRequest,
			//}
			return nil, errors.New("certificate is not a CA")
		}
		fp = cert.NewFingerprintGenerator().FromCert(certs[0])
	}

	if fp == "" {
		return nil, fmt.Errorf("invalid certificate, could not parse PEM body")
	}

	query := fmt.Sprintf(`fingerprint = "%v"`, fp)
	queryResults, _, err := env.GetStores().Ca.QueryIds(tx, query)

	if err != nil {
		return nil, err
	}
	if len(queryResults) > 0 {
		return nil, errorz.NewFieldError(fmt.Sprintf("certificate already used as CA %s", queryResults[0]), "certPem", entity.CertPem)
	}

	boltEntity := &db.Ca{
		BaseExtEntity:             *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:                      entity.Name,
		CertPem:                   entity.CertPem,
		Fingerprint:               fp,
		IsVerified:                false,
		VerificationToken:         eid.New(),
		IsAuthEnabled:             entity.IsAuthEnabled,
		IsAutoCaEnrollmentEnabled: entity.IsAutoCaEnrollmentEnabled,
		IsOttCaEnrollmentEnabled:  entity.IsOttCaEnrollmentEnabled,
		IdentityRoles:             entity.IdentityRoles,
		IdentityNameFormat:        entity.IdentityNameFormat,
	}

	if entity.ExternalIdClaim != nil {
		boltEntity.ExternalIdClaim = &db.ExternalIdClaim{
			Location:        entity.ExternalIdClaim.Location,
			Matcher:         entity.ExternalIdClaim.Matcher,
			MatcherCriteria: entity.ExternalIdClaim.MatcherCriteria,
			Parser:          entity.ExternalIdClaim.Parser,
			ParserCriteria:  entity.ExternalIdClaim.ParserCriteria,
			Index:           entity.ExternalIdClaim.Index,
		}
	}

	return boltEntity, nil
}

func (entity *Ca) toBoltEntityForUpdate(tx *bbolt.Tx, env Env, checker boltz.FieldChecker) (*db.Ca, error) {
	if entity.IdentityNameFormat == "" {
		entity.IdentityNameFormat = DefaultCaIdentityNameFormat
	}

	claim, err := entity.effectiveExternalIdClaim(tx, env, checker)
	if err != nil {
		return nil, err
	}

	if err := validateExternalIdClaim(claim); err != nil {
		return nil, err
	}

	boltEntity := &db.Ca{
		BaseExtEntity:             *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:                      entity.Name,
		IsAuthEnabled:             entity.IsAuthEnabled,
		IsAutoCaEnrollmentEnabled: entity.IsAutoCaEnrollmentEnabled,
		IsOttCaEnrollmentEnabled:  entity.IsOttCaEnrollmentEnabled,
		IsVerified:                entity.IsVerified,
		IdentityRoles:             entity.IdentityRoles,
		IdentityNameFormat:        entity.IdentityNameFormat,
	}

	if entity.ExternalIdClaim != nil {
		boltEntity.ExternalIdClaim = &db.ExternalIdClaim{
			Location:        entity.ExternalIdClaim.Location,
			Matcher:         entity.ExternalIdClaim.Matcher,
			MatcherCriteria: entity.ExternalIdClaim.MatcherCriteria,
			Parser:          entity.ExternalIdClaim.Parser,
			ParserCriteria:  entity.ExternalIdClaim.ParserCriteria,
			Index:           entity.ExternalIdClaim.Index,
		}
	}

	return boltEntity, nil
}

// caExternalIdClaimFields are the persisted externalIdClaim subfields, used to detect which
// subfields a partial update touches.
var caExternalIdClaimFields = []string{
	db.FieldCaExternalIdClaimLocation,
	db.FieldCaExternalIdClaimMatcher,
	db.FieldCaExternalIdClaimMatcherCriteria,
	db.FieldCaExternalIdClaimParser,
	db.FieldCaExternalIdClaimParserCriteria,
	db.FieldCaExternalIdClaimIndex,
}

// effectiveExternalIdClaim returns the externalIdClaim to validate for an update so validation
// matches what will be stored. A nil checker is a full replace, so the request claim stands as-is.
// For a patch only the subfields named in the checker change: if none touch the claim it is left
// unchanged (nil, validation skipped), otherwise the supplied subfields are overlaid onto the stored
// claim and the merged result is validated. Subfield gating mirrors PersistEntity exactly.
func (entity *Ca) effectiveExternalIdClaim(tx *bbolt.Tx, env Env, checker boltz.FieldChecker) (*ExternalIdClaim, error) {
	// No claim supplied: the update either clears the claim or leaves it untouched. Nothing to validate.
	if entity.ExternalIdClaim == nil {
		return nil, nil
	}

	// Full replace (nil checker): validate the supplied claim as-is.
	if checker == nil {
		return entity.ExternalIdClaim, nil
	}

	// Partial update: if no claim subfield is being set, the stored claim is unchanged (the empty {}
	// object older CLIs send on every update). Skip validation rather than treating it as incomplete.
	touched := false
	for _, field := range caExternalIdClaimFields {
		if checker.IsUpdated(field) {
			touched = true
			break
		}
	}
	if !touched {
		return nil, nil
	}

	// Overlay the supplied subfields onto the stored claim so the merged result is what gets validated.
	merged := &ExternalIdClaim{}
	if existing, err := env.GetStores().Ca.LoadById(tx, entity.Id); err != nil {
		return nil, err
	} else if existing != nil && existing.ExternalIdClaim != nil {
		merged.Location = existing.ExternalIdClaim.Location
		merged.Matcher = existing.ExternalIdClaim.Matcher
		merged.MatcherCriteria = existing.ExternalIdClaim.MatcherCriteria
		merged.Parser = existing.ExternalIdClaim.Parser
		merged.ParserCriteria = existing.ExternalIdClaim.ParserCriteria
		merged.Index = existing.ExternalIdClaim.Index
	}

	if checker.IsUpdated(db.FieldCaExternalIdClaimLocation) {
		merged.Location = entity.ExternalIdClaim.Location
	}
	if checker.IsUpdated(db.FieldCaExternalIdClaimMatcher) {
		merged.Matcher = entity.ExternalIdClaim.Matcher
	}
	if checker.IsUpdated(db.FieldCaExternalIdClaimMatcherCriteria) {
		merged.MatcherCriteria = entity.ExternalIdClaim.MatcherCriteria
	}
	if checker.IsUpdated(db.FieldCaExternalIdClaimParser) {
		merged.Parser = entity.ExternalIdClaim.Parser
	}
	if checker.IsUpdated(db.FieldCaExternalIdClaimParserCriteria) {
		merged.ParserCriteria = entity.ExternalIdClaim.ParserCriteria
	}
	if checker.IsUpdated(db.FieldCaExternalIdClaimIndex) {
		merged.Index = entity.ExternalIdClaim.Index
	}

	return merged, nil
}

// validateExternalIdClaim rejects unsupported or incomplete externalIdClaim configurations
// with a bad request error so they cannot be stored and later fail enrollment/authentication.
// It exercises the same location/matcher/parser rules used to extract the claim, so any
// configuration that would error at extraction time is rejected up front at CA create/update.
func validateExternalIdClaim(claim *ExternalIdClaim) error {
	if claim == nil {
		return nil
	}

	if claim.Index < 0 {
		return apierror.NewBadRequestFieldError(*errorz.NewFieldError("index must be greater than or equal to zero", "externalIdClaim.index", claim.Index))
	}

	var err error
	switch claim.Location {
	case db.ExternalIdClaimLocCommonName, db.ExternalIdClaimLocSanEmail:
		_, err = getStringDefinition(claim)
	case db.ExternalIdClaimLocSanUri:
		_, err = getUriDefinition(claim)
	default:
		return apierror.NewBadRequestFieldError(*errorz.NewFieldError(fmt.Sprintf("unsupported location [%s]", claim.Location), "externalIdClaim.location", claim.Location))
	}

	if err != nil {
		value := fmt.Sprintf("location=%s matcher=%s parser=%s", claim.Location, claim.Matcher, claim.Parser)
		return apierror.NewBadRequestFieldError(*errorz.NewFieldError(err.Error(), "externalIdClaim", value))
	}

	return nil
}

// GetExternalId will attempt to retrieve a string claim from a x509 Certificate based on
// location, matching, and parsing of various x509 Certificate fields.
func (entity *Ca) GetExternalId(cert *x509.Certificate) (string, error) {
	// A claim with no location is unconfigured (e.g. the empty bucket an older CLI's empty {} patch
	// leaves behind). Treat it as no claim so enrollment falls back to a certificate fingerprint.
	if entity.ExternalIdClaim == nil || entity.ExternalIdClaim.Location == "" {
		return "", nil
	}

	provider := &x509claims.ProviderBasic{
		Definitions: []x509claims.Definition{},
	}

	switch entity.ExternalIdClaim.Location {
	case db.ExternalIdClaimLocCommonName:
		definition, err := getStringDefinition(entity.ExternalIdClaim)
		if err != nil {
			return "", err
		}
		definition.Locator = &x509claims.LocatorCommonName{}

		provider.Definitions = append(provider.Definitions, definition)

	case db.ExternalIdClaimLocSanUri:
		definition, err := getUriDefinition(entity.ExternalIdClaim)
		if err != nil {
			return "", err
		}
		definition.Locator = &x509claims.LocatorSanUri{}

		provider.Definitions = append(provider.Definitions, definition)
	case db.ExternalIdClaimLocSanEmail:
		definition, err := getStringDefinition(entity.ExternalIdClaim)
		if err != nil {
			return "", err
		}
		definition.Locator = &x509claims.LocatorSanEmail{}

		provider.Definitions = append(provider.Definitions, definition)
	default:
		return "", fmt.Errorf("unsupported location [%s]", entity.ExternalIdClaim.Location)
	}

	claims := provider.Claims(cert)

	if entity.ExternalIdClaim.Index < 0 || entity.ExternalIdClaim.Index >= int64(len(claims)) {
		return "", errors.New("no claim found")
	}

	externalId := claims[entity.ExternalIdClaim.Index]

	if externalId == "" {
		return "", errors.New("claim resolved to an empty value")
	}

	return externalId, nil
}

// getUriDefinition returns an x509Claims.DefinitionLMP that will locate, match, and parse url.URL properties.
func getUriDefinition(externalIdClaim *ExternalIdClaim) (*x509claims.DefinitionLMP[*url.URL], error) {
	definition := &x509claims.DefinitionLMP[*url.URL]{}

	switch externalIdClaim.Matcher {
	case db.ExternalIdClaimMatcherAll:
		definition.Matcher = &x509claims.MatcherAll[*url.URL]{}
	case db.ExternalIdClaimMatcherScheme:
		if externalIdClaim.MatcherCriteria == "" {
			return nil, fmt.Errorf("invalid criteria [%s] for matcher [%s]", externalIdClaim.MatcherCriteria, externalIdClaim.Matcher)
		}

		definition.Matcher = &x509claims.MatcherScheme{Scheme: externalIdClaim.MatcherCriteria}
	default:
		return nil, fmt.Errorf("unsupported matcher [%s] for location [%s]", externalIdClaim.Matcher, externalIdClaim.Location)
	}

	var err error
	definition.Parser, err = getStringParser(externalIdClaim)

	if err != nil {
		return nil, err
	}

	return definition, nil
}

// getStringDefinition returns an x509Claims.DefinitionLMP that will locate, match, and parse string properties.
func getStringDefinition(externalIdClaim *ExternalIdClaim) (*x509claims.DefinitionLMP[string], error) {
	definition := &x509claims.DefinitionLMP[string]{}

	switch externalIdClaim.Matcher {
	case db.ExternalIdClaimMatcherAll:
		definition.Matcher = &x509claims.MatcherAll[string]{}
	case db.ExternalIdClaimMatcherPrefix:
		if externalIdClaim.MatcherCriteria == "" {
			return nil, fmt.Errorf("invalid criteria [%s] for matcher [%s]", externalIdClaim.MatcherCriteria, externalIdClaim.Matcher)
		}

		definition.Matcher = &x509claims.MatcherPrefix{Prefix: externalIdClaim.MatcherCriteria}
	case db.ExternalIdClaimMatcherSuffix:
		if externalIdClaim.MatcherCriteria == "" {
			return nil, fmt.Errorf("invalid criteria [%s] for matcher [%s]", externalIdClaim.MatcherCriteria, externalIdClaim.Matcher)
		}

		definition.Matcher = &x509claims.MatcherSuffix{Suffix: externalIdClaim.MatcherCriteria}
	default:
		return nil, fmt.Errorf("unsupported matcher [%s] for location [%s]", externalIdClaim.Matcher, externalIdClaim.Location)
	}

	var err error
	definition.Parser, err = getStringParser(externalIdClaim)

	if err != nil {
		return nil, err
	}

	return definition, nil
}

// getStringParser returns a x509claims.Parser that parses string values into claims
func getStringParser(externalIdClaim *ExternalIdClaim) (x509claims.Parser, error) {
	switch externalIdClaim.Parser {
	case db.ExternalIdClaimParserNone:
		return &x509claims.ParserNoOp{}, nil
	case db.ExternalIdClaimParserSplit:
		if externalIdClaim.ParserCriteria == "" {
			return nil, fmt.Errorf("invalid criteria [%s] for parser [%s]", externalIdClaim.ParserCriteria, externalIdClaim.Parser)
		}

		return &x509claims.ParserSplit{Separator: externalIdClaim.ParserCriteria}, nil
	}

	return nil, fmt.Errorf("unsupported parser [%s] for location [%s]", externalIdClaim.Parser, externalIdClaim.Location)
}
