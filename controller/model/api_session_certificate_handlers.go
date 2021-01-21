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
	"encoding/pem"
	"fmt"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/internal/cert"
	"github.com/openziti/fabric/controller/models"
	"go.etcd.io/bbolt"
	"time"
)

func NewApiSessionCertificateHandler(env Env) *ApiSessionCertificateHandler {
	handler := &ApiSessionCertificateHandler{
		baseHandler: newBaseHandler(env, env.GetStores().ApiSessionCertificate),
	}
	handler.impl = handler

	return handler
}

type ApiSessionCertificateHandler struct {
	baseHandler
}

func (handler *ApiSessionCertificateHandler) newModelEntity() boltEntitySink {
	return &ApiSessionCertificate{}
}

func (handler *ApiSessionCertificateHandler) Create(entity *ApiSessionCertificate) (string, error) {
	return handler.createEntity(entity)
}

func (handler *ApiSessionCertificateHandler) CreateFromCSR(apiSessionId string, lifespan time.Duration, csr []byte) (string, error) {
	notBefore := time.Now()
	notAfter := time.Now().Add(lifespan)

	certRaw, err := handler.env.GetApiClientCsrSigner().Sign(csr, &cert.SigningOpts{
		NotAfter:  &notAfter,
		NotBefore: &notBefore,
	})

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return "", apiErr
	}

	fp := handler.env.GetFingerprintGenerator().FromRaw(certRaw)

	certPem := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certRaw,
	})

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

	return handler.Create(entity)
}

func (handler *ApiSessionCertificateHandler) Read(id string) (*ApiSessionCertificate, error) {
	modelApiSessionCertificate := &ApiSessionCertificate{}
	if err := handler.readEntity(id, modelApiSessionCertificate); err != nil {
		return nil, err
	}
	return modelApiSessionCertificate, nil
}

func (handler *ApiSessionCertificateHandler) ReadByFingerprint(fingerprint string) (*ApiSessionCertificate, error) {
	modelApiSessionCertificate := &ApiSessionCertificate{}
	tokenIndex := handler.env.GetStores().ApiSessionCertificate.GetFingerprintIndex()
	if err := handler.readEntityWithIndex("fingerprint", []byte(fingerprint), tokenIndex, modelApiSessionCertificate); err != nil {
		return nil, err
	}
	return modelApiSessionCertificate, nil
}

func (handler *ApiSessionCertificateHandler) readInTx(tx *bbolt.Tx, id string) (*ApiSessionCertificate, error) {
	modelApiSessionCertificate := &ApiSessionCertificate{}
	if err := handler.readEntityInTx(tx, id, modelApiSessionCertificate); err != nil {
		return nil, err
	}
	return modelApiSessionCertificate, nil
}

func (handler *ApiSessionCertificateHandler) IsUpdated(_ string) bool {
	return false
}

func (handler *ApiSessionCertificateHandler) Delete(id string) error {
	return handler.deleteEntity(id)
}

func (handler *ApiSessionCertificateHandler) Query(query string) (*ApiSessionCertificateListResult, error) {
	result := &ApiSessionCertificateListResult{handler: handler}
	err := handler.list(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *ApiSessionCertificateHandler) ReadByApiSessionId(apiSessionId string) ([]*ApiSessionCertificate, error) {
	result, err := handler.Query(fmt.Sprintf(`apiSession = "%s"`, apiSessionId))

	if err != nil {
		return nil, err
	}

	return result.ApiSessionCertificates, nil
}

type ApiSessionCertificateListResult struct {
	handler                *ApiSessionCertificateHandler
	ApiSessionCertificates []*ApiSessionCertificate
	models.QueryMetaData
}

func (result *ApiSessionCertificateListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *models.QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		ApiSessionCertificate, err := result.handler.readInTx(tx, key)
		if err != nil {
			return err
		}
		result.ApiSessionCertificates = append(result.ApiSessionCertificates, ApiSessionCertificate)
	}
	return nil
}
