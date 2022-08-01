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
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/internal/cert"
	"github.com/openziti/fabric/controller/models"
	"go.etcd.io/bbolt"
	"time"
)

func NewApiSessionCertificateManager(env Env) *ApiSessionCertificateManager {
	manager := &ApiSessionCertificateManager{
		baseEntityManager: newBaseEntityManager(env, env.GetStores().ApiSessionCertificate),
	}
	manager.impl = manager

	return manager
}

type ApiSessionCertificateManager struct {
	baseEntityManager
}

func (self *ApiSessionCertificateManager) newModelEntity() edgeEntity {
	return &ApiSessionCertificate{}
}

func (self *ApiSessionCertificateManager) Create(entity *ApiSessionCertificate) (string, error) {
	return self.createEntity(entity)
}

func (self *ApiSessionCertificateManager) CreateFromCSR(apiSessionId string, lifespan time.Duration, csrPem []byte) (string, error) {
	notBefore := time.Now()
	notAfter := time.Now().Add(lifespan)

	csr, err := cert.ParseCsrPem(csrPem)

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return "", apiErr
	}

	certRaw, err := self.env.GetApiClientCsrSigner().SignCsr(csr, &cert.SigningOpts{
		NotAfter:  &notAfter,
		NotBefore: &notBefore,
	})

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return "", apiErr
	}

	fp := self.env.GetFingerprintGenerator().FromRaw(certRaw)

	certPem, _ := cert.RawToPem(certRaw)

	cert, _ := x509.ParseCertificate(certRaw)

	entity := &ApiSessionCertificate{
		BaseEntity:   models.BaseEntity{},
		ApiSessionId: apiSessionId,
		Subject:      cert.Subject.String(),
		Fingerprint:  fp,
		ValidAfter:   &notBefore,
		ValidBefore:  &notAfter,
		PEM:          string(certPem),
	}

	return self.Create(entity)
}

func (self *ApiSessionCertificateManager) Read(id string) (*ApiSessionCertificate, error) {
	modelApiSessionCertificate := &ApiSessionCertificate{}
	if err := self.readEntity(id, modelApiSessionCertificate); err != nil {
		return nil, err
	}
	return modelApiSessionCertificate, nil
}

func (self *ApiSessionCertificateManager) readInTx(tx *bbolt.Tx, id string) (*ApiSessionCertificate, error) {
	modelApiSessionCertificate := &ApiSessionCertificate{}
	if err := self.readEntityInTx(tx, id, modelApiSessionCertificate); err != nil {
		return nil, err
	}
	return modelApiSessionCertificate, nil
}

func (self *ApiSessionCertificateManager) IsUpdated(_ string) bool {
	return false
}

func (self *ApiSessionCertificateManager) Delete(id string) error {
	return self.deleteEntity(id)
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
	result, err := self.Query(tx, fmt.Sprintf(`apiSession = "%s"`, apiSessionId))

	if err != nil {
		return nil, err
	}

	return result.ApiSessionCertificates, nil
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
