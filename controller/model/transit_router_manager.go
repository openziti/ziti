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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/eid"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

func NewTransitRouterManager(env Env) *TransitRouterManager {
	manager := &TransitRouterManager{
		baseEntityManager: newBaseEntityManager(env, env.GetStores().TransitRouter),
		allowedFields: boltz.MapFieldChecker{
			persistence.FieldName: struct{}{},
			boltz.FieldTags:       struct{}{},
		},
	}
	manager.impl = manager
	return manager
}

type TransitRouterManager struct {
	baseEntityManager
	allowedFields boltz.FieldChecker
}

func (self *TransitRouterManager) Delete(id string) error {
	return self.deleteEntity(id)
}

func (self *TransitRouterManager) newModelEntity() edgeEntity {
	return &TransitRouter{}
}

func (self *TransitRouterManager) Create(entity *TransitRouter) (string, error) {
	enrollment := &Enrollment{
		BaseEntity: models.BaseEntity{},
		Method:     MethodEnrollTransitRouterOtt,
	}

	id, _, err := self.CreateWithEnrollment(entity, enrollment)
	return id, err
}

func (self *TransitRouterManager) CreateWithEnrollment(txRouter *TransitRouter, enrollment *Enrollment) (string, string, error) {

	if txRouter.Id == "" {
		txRouter.Id = eid.New()
	}
	var enrollmentId string

	err := self.GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		boltEntity, err := txRouter.toBoltEntityForCreate(tx, self.impl)
		if err != nil {
			return err
		}
		if err := self.GetStore().Create(ctx, boltEntity); err != nil {
			pfxlog.Logger().WithError(err).Errorf("could not create %v in bolt storage", self.GetStore().GetSingularEntityType())
			return err
		}

		enrollment.TransitRouterId = &txRouter.Id

		err = enrollment.FillJwtInfo(self.env, txRouter.Id)

		if err != nil {
			return err
		}

		enrollmentId, err = self.env.GetManagers().Enrollment.createEntityInTx(ctx, enrollment)

		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return "", "", err
	}

	return txRouter.Id, enrollmentId, nil
}

func (self *TransitRouterManager) Update(entity *TransitRouter, allowAllFields bool) error {
	curEntity, err := self.Read(entity.Id)

	if err != nil {
		return err
	}

	if curEntity.IsBase {
		return apierror.NewFabricRouterCannotBeUpdate()
	}

	if allowAllFields {
		return self.updateEntity(entity, nil)
	}

	return self.updateEntity(entity, self.allowedFields)

}

func (self *TransitRouterManager) Patch(entity *TransitRouter, checker boltz.FieldChecker, allowAllFields bool) error {
	curEntity, err := self.Read(entity.Id)

	if err != nil {
		return err
	}

	if curEntity.IsBase {
		return apierror.NewFabricRouterCannotBeUpdate()
	}

	if allowAllFields {
		return self.patchEntity(entity, checker)
	}
	combinedChecker := &AndFieldChecker{first: self.allowedFields, second: checker}
	return self.patchEntity(entity, combinedChecker)
}

func (self *TransitRouterManager) ReadOneByFingerprint(fingerprint string) (*TransitRouter, error) {
	return self.ReadOneByQuery(fmt.Sprintf(`%s = "%v"`, db.FieldRouterFingerprint, fingerprint))
}

func (self *TransitRouterManager) ReadOneByQuery(query string) (*TransitRouter, error) {
	result, err := self.readEntityByQuery(query)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*TransitRouter), nil
}

func (self *TransitRouterManager) Read(id string) (*TransitRouter, error) {
	result := &TransitRouter{}

	if err := self.readEntity(id, result); err != nil {
		return nil, err
	}

	return result, nil
}

func (self *TransitRouterManager) readInTx(tx *bbolt.Tx, id string) (*TransitRouter, error) {
	modelEntity := &TransitRouter{}
	if err := self.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (self *TransitRouterManager) CollectEnrollments(id string, collector func(entity *Enrollment) error) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.collectEnrollmentsInTx(tx, id, collector)
	})
}

func (self *TransitRouterManager) collectEnrollmentsInTx(tx *bbolt.Tx, id string, collector func(entity *Enrollment) error) error {
	_, err := self.readInTx(tx, id)
	if err != nil {
		return err
	}

	associationIds := self.GetStore().GetRelatedEntitiesIdList(tx, id, persistence.FieldTransitRouterEnrollments)
	for _, enrollmentId := range associationIds {
		enrollment, err := self.env.GetManagers().Enrollment.readInTx(tx, enrollmentId)
		if err != nil {
			return err
		}
		err = collector(enrollment)

		if err != nil {
			return err
		}
	}
	return nil
}

func (self *TransitRouterManager) ExtendEnrollment(router *TransitRouter, clientCsrPem []byte, serverCertCsrPem []byte) (*ExtendedCerts, error) {
	enrollmentModule := self.env.GetEnrollRegistry().GetByMethod("erott").(*EnrollModuleEr)

	clientCertRaw, err := enrollmentModule.ProcessClientCsrPem(clientCsrPem, router.Id)

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr
	}

	serverCertRaw, err := enrollmentModule.ProcessServerCsrPem(serverCertCsrPem)

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr
	}

	fingerprint := self.env.GetFingerprintGenerator().FromRaw(clientCertRaw)

	pfxlog.Logger().Debugf("extending enrollment for router %s, old fingerprint: %s new fingerprint: %s", router.Id, *router.Fingerprint, fingerprint)

	router.Fingerprint = &fingerprint

	err = self.Patch(router, &boltz.MapFieldChecker{
		persistence.FieldEdgeRouterCertPEM: struct{}{},
		db.FieldRouterFingerprint:          struct{}{},
	}, true)

	if err != nil {
		return nil, err
	}

	return &ExtendedCerts{
		RawClientCert: clientCertRaw,
		RawServerCert: serverCertRaw,
	}, nil
}

func (self *TransitRouterManager) ExtendEnrollmentWithVerify(router *TransitRouter, clientCsrPem []byte, serverCertCsrPem []byte) (*ExtendedCerts, error) {
	enrollmentModule := self.env.GetEnrollRegistry().GetByMethod("erott").(*EnrollModuleEr)

	clientCertRaw, err := enrollmentModule.ProcessClientCsrPem(clientCsrPem, router.Id)

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr
	}

	serverCertRaw, err := enrollmentModule.ProcessServerCsrPem(serverCertCsrPem)

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr
	}

	fingerprint := self.env.GetFingerprintGenerator().FromRaw(clientCertRaw)

	pfxlog.Logger().Debugf("extending enrollment for router %s, old fingerprint: %s new fingerprint: %s", router.Id, *router.Fingerprint, fingerprint)

	router.UnverifiedFingerprint = &fingerprint

	err = self.Patch(router, &boltz.MapFieldChecker{
		persistence.FieldEdgeRouterUnverifiedCertPEM:     struct{}{},
		persistence.FieldEdgeRouterUnverifiedFingerprint: struct{}{},
	}, true)

	if err != nil {
		return nil, err
	}

	return &ExtendedCerts{
		RawClientCert: clientCertRaw,
		RawServerCert: serverCertRaw,
	}, nil
}

func (self *TransitRouterManager) ReadOneByUnverifiedFingerprint(fingerprint string) (*TransitRouter, error) {
	return self.ReadOneByQuery(fmt.Sprintf(`%s = "%v"`, persistence.FieldEdgeRouterUnverifiedFingerprint, fingerprint))
}

func (self *TransitRouterManager) ExtendEnrollmentVerify(router *TransitRouter) error {
	if router.UnverifiedFingerprint != nil && router.UnverifiedCertPem != nil {
		router.Fingerprint = router.UnverifiedFingerprint

		router.UnverifiedFingerprint = nil
		router.UnverifiedCertPem = nil

		return self.Patch(router, boltz.MapFieldChecker{
			db.FieldRouterFingerprint:                        struct{}{},
			persistence.FieldEdgeRouterUnverifiedCertPEM:     struct{}{},
			persistence.FieldEdgeRouterUnverifiedFingerprint: struct{}{},
		}, true)
	}

	return errors.New("no outstanding verification necessary")
}
