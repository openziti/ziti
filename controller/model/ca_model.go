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
	"crypto/x509"
	"fmt"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/eid"
	"github.com/openziti/edge/internal/cert"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/x509-claims/x509claims"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"net/url"
	"reflect"
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

func (entity *Ca) fillFrom(_ EntityManager, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltCa, ok := boltEntity.(*persistence.Ca)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model ca", reflect.TypeOf(boltEntity))
	}
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

func (entity *Ca) toBoltEntityForCreate(tx *bbolt.Tx, handler EntityManager) (boltz.Entity, error) {
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
	queryResults, _, err := handler.GetEnv().GetStores().Ca.QueryIds(tx, query)

	if err != nil {
		return nil, err
	}
	if len(queryResults) > 0 {
		return nil, errorz.NewFieldError(fmt.Sprintf("certificate already used as CA %s", queryResults[0]), "certPem", entity.CertPem)
	}

	boltEntity := &persistence.Ca{
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
		boltEntity.ExternalIdClaim = &persistence.ExternalIdClaim{
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

func (entity *Ca) toBoltEntityForUpdate(_ *bbolt.Tx, _ EntityManager) (boltz.Entity, error) {
	boltEntity := &persistence.Ca{
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
		boltEntity.ExternalIdClaim = &persistence.ExternalIdClaim{
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

func (entity *Ca) toBoltEntityForPatch(tx *bbolt.Tx, handler EntityManager, _ boltz.FieldChecker) (boltz.Entity, error) {
	return entity.toBoltEntityForUpdate(tx, handler)
}

// GetExternalId will attempt to retrieve a string claim from a x509 Certificate based on
// location, matching, and parsing of various x509 Certificate fields.
func (entity *Ca) GetExternalId(cert *x509.Certificate) (string, error) {
	if entity.ExternalIdClaim == nil {
		return "", nil
	}

	provider := &x509claims.ProviderBasic{
		Definitions: make([]x509claims.Definition, 1),
	}

	switch entity.ExternalIdClaim.Location {
	case persistence.ExternalIdClaimLocCommonName:
		definition, err := getStringDefinition(entity.ExternalIdClaim)
		definition.Locator = &x509claims.LocatorCommonName{}
		if err != nil {
			return "", err
		}

		provider.Definitions[0] = definition

	case persistence.ExternalIdClaimLocSanUri:
		definition, err := getUriDefinition(entity.ExternalIdClaim)
		definition.Locator = &x509claims.LocatorSanUri{}
		if err != nil {
			return "", err
		}

		provider.Definitions[0] = definition
	case persistence.ExternalIdClaimLocSanEmail:
		definition, err := getStringDefinition(entity.ExternalIdClaim)
		definition.Locator = &x509claims.LocatorSanEmail{}
		if err != nil {
			return "", err
		}

		provider.Definitions[0] = definition
	}

	claims := provider.Claims(cert)

	if int64(len(claims)) > entity.ExternalIdClaim.Index {
		return claims[entity.ExternalIdClaim.Index], nil
	}

	return "", errors.New("no claim found")
}

// getUriDefinition returns an x509Claims.DefinitionLMP that will locate, match, and parse url.URL properties.
func getUriDefinition(externalIdClaim *ExternalIdClaim) (*x509claims.DefinitionLMP[*url.URL], error) {
	definition := &x509claims.DefinitionLMP[*url.URL]{}

	switch externalIdClaim.Matcher {
	case persistence.ExternalIdClaimMatcherAll:
		definition.Matcher = &x509claims.MatcherAll[*url.URL]{}
	case persistence.ExternalIdClaimMatcherScheme:
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
	case persistence.ExternalIdClaimMatcherAll:
		definition.Matcher = &x509claims.MatcherAll[string]{}
	case persistence.ExternalIdClaimMatcherPrefix:
		if externalIdClaim.MatcherCriteria == "" {
			return nil, fmt.Errorf("invalid criteria [%s] for matcher [%s]", externalIdClaim.MatcherCriteria, externalIdClaim.Matcher)
		}

		definition.Matcher = &x509claims.MatcherPrefix{Prefix: externalIdClaim.MatcherCriteria}
	case persistence.ExternalIdClaimMatcherSuffix:
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
	case persistence.ExternalIdClaimParserNone:
		return &x509claims.ParserNoOp{}, nil
	case persistence.ExternalIdClaimParserSplit:
		if externalIdClaim.ParserCriteria == "" {
			return nil, fmt.Errorf("invalid criteria [%s] for parser [%s]", externalIdClaim.ParserCriteria, externalIdClaim.Parser)
		}

		return &x509claims.ParserSplit{Separator: externalIdClaim.ParserCriteria}, nil
	}

	return nil, fmt.Errorf("unsupported parser [%s] for location [%s]", externalIdClaim.Parser, externalIdClaim.Location)
}
