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
	"github.com/openziti/ziti/common/cert"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"go.etcd.io/bbolt"
	"net/url"
	"time"
)

func NewApiSessionCertificateManager(env Env) *ApiSessionCertificateManager {
	manager := &ApiSessionCertificateManager{
		baseEntityManager: newBaseEntityManager[*ApiSessionCertificate, *db.ApiSessionCertificate](env, env.GetStores().ApiSessionCertificate),
	}
	manager.impl = manager

	return manager
}

type ApiSessionCertificateManager struct {
	baseEntityManager[*ApiSessionCertificate, *db.ApiSessionCertificate]
}

func (self *ApiSessionCertificateManager) newModelEntity() *ApiSessionCertificate {
	return &ApiSessionCertificate{}
}

func (self *ApiSessionCertificateManager) Create(entity *ApiSessionCertificate, ctx *change.Context) (string, error) {
	return self.createEntity(entity, ctx.NewMutateContext())
}

func (self *ApiSessionCertificateManager) CreateFromCSR(identity *Identity, apiSession *ApiSession, isJwt bool, lifespan time.Duration, csrPem []byte, ctx *change.Context) (*ApiSessionCertificate, error) {
	notBefore := time.Now()
	notAfter := time.Now().Add(lifespan)

	csr, err := cert.ParseCsrPem(csrPem)

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr
	}

	newId := eid.New()

	trustDomain := self.env.GetConfig().SpiffeIdTrustDomain.Hostname()
	spiffeId := &url.URL{
		Scheme: "spiffe",
		Host:   trustDomain,
		Path:   fmt.Sprintf("identity/%s/apiSession/%s/apiSessionCertificate/%s", identity.Id, apiSession.Id, newId),
	}

	certRaw, err := self.env.GetApiClientCsrSigner().SignCsr(csr, &cert.SigningOpts{
		NotAfter:  &notAfter,
		NotBefore: &notBefore,
		URIs: []*url.URL{
			spiffeId,
		},
	})

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr
	}

	newCert, _ := x509.ParseCertificate(certRaw)

	fp := self.env.GetFingerprintGenerator().FromCert(newCert)

	chainPem, err := self.env.GetManagers().Enrollment.GetCertChainPem(certRaw)

	if err != nil {
		return nil, err
	}

	entity := &ApiSessionCertificate{
		BaseEntity: models.BaseEntity{
			Id: newId,
		},
		ApiSessionId: apiSession.Id,
		Subject:      newCert.Subject.String(),
		Fingerprint:  fp,
		ValidAfter:   &notBefore,
		ValidBefore:  &notAfter,
		PEM:          chainPem,
	}

	if isJwt {
		// can't create if using bearer tokens, the API Session will not exist
		return entity, nil
	}

	entity.Id, err = self.Create(entity, ctx)

	return entity, err
}

func (self *ApiSessionCertificateManager) IsUpdated(_ string) bool {
	return false
}

func (self *ApiSessionCertificateManager) Delete(id string, ctx *change.Context) error {
	return self.deleteEntity(id, ctx)
}

func (self *ApiSessionCertificateManager) Query(tx *bbolt.Tx, query string) (*ApiSessionCertificateListResult, error) {
	result := &ApiSessionCertificateListResult{manager: self}
	err := self.ListWithTx(tx, query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *ApiSessionCertificateManager) ReadByApiSessionId(tx *bbolt.Tx, apiSessionId string) ([]*ApiSessionCertificate, error) {
	var result []*ApiSessionCertificate

	certIds := self.env.GetStores().ApiSession.GetRelatedEntitiesIdList(tx, apiSessionId, db.EntityTypeApiSessionCertificates)
	for _, key := range certIds {
		apiSessionCert, err := self.readInTx(tx, key)
		if err != nil {
			return nil, err
		}
		result = append(result, apiSessionCert)
	}

	return result, nil
}

type ApiSessionCertificateListResult struct {
	manager                *ApiSessionCertificateManager
	ApiSessionCertificates []*ApiSessionCertificate
	models.QueryMetaData
}

func (result *ApiSessionCertificateListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *models.QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		ApiSessionCertificate, err := result.manager.readInTx(tx, key)
		if err != nil {
			return err
		}
		result.ApiSessionCertificates = append(result.ApiSessionCertificates, ApiSessionCertificate)
	}
	return nil
}
