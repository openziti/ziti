/*
	Copyright 2020 NetFoundry, Inc.

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

package persistence

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-fabric/controller/db"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/predicate"
	"github.com/netfoundry/ziti-edge/migration"
	"github.com/netfoundry/ziti-foundation/util/stringz"
)

var queryOptionsListAll = &migration.QueryOptions{
	Paging: &predicate.Paging{
		Offset:    0,
		Limit:     0,
		ReturnAll: true,
	},
}

func (m *Migrations) upgradeToV1FromPG(step *boltz.MigrationStep) {
	pfxlog.Logger().Info("postgres configured, migrating from postgres")

	migrationFuncs := []func(step *boltz.MigrationStep) error{
		m.migrateIdentitiesFromPG,
		m.migrateCasFromPG,
		m.migrateAuthenticatorsFromPG,
		m.migrateEnrollmentsFromPG,
		m.migrateEventLogsFromPG,
		m.migrateClusterFromPG,
		m.migrateEdgeRoutersFromPG,
		m.migrateServicesFromPG,
		m.migrateAppWansFromPG,
	}

	for _, migrationFunc := range migrationFuncs {
		if step.SetError(migrationFunc(step)) {
			return
		}
	}
}

func (m *Migrations) migrateCasFromPG(step *boltz.MigrationStep) error {
	cas, err := m.dbStores.Ca.LoadList(queryOptionsListAll)
	if err != nil {
		return err
	}

	for _, pgCa := range cas {
		if err != nil {
			return err
		}
		ca := &Ca{
			BaseExtEntity:             *toBaseBoltEntity(&pgCa.BaseDbEntity),
			Name:                      *pgCa.Name,
			Fingerprint:               *pgCa.Fingerprint,
			CertPem:                   *pgCa.CertPem,
			IsVerified:                *pgCa.IsVerified,
			VerificationToken:         stringz.OrEmpty(pgCa.VerificationToken),
			IsAutoCaEnrollmentEnabled: *pgCa.IsAutoCaEnrollmentEnabled,
			IsOttCaEnrollmentEnabled:  *pgCa.IsOttCaEnrollmentEnabled,
			IsAuthEnabled:             *pgCa.IsAuthEnabled,
		}
		err = m.stores.Ca.Create(step.Ctx, ca)
	}
	pfxlog.Logger().Infof("migrated %v cas from pg to bolt", len(cas))

	return nil
}

func (m *Migrations) migrateAuthenticatorsFromPG(step *boltz.MigrationStep) error {
	authenticators, err := m.dbStores.Authenticator.LoadList(queryOptionsListAll)
	if err != nil {
		return err
	}

	for _, pgAuthenticator := range authenticators {
		if err != nil {
			return err
		}
		var subtype AuthenticatorSubType
		switch *pgAuthenticator.Method {
		case "updb":
			pgUpdb, err := m.dbStores.AuthenticatorUpdb.LoadOneByAuthenticatorId(pgAuthenticator.ID, nil)

			if err != nil {
				return fmt.Errorf("error migrating authenticator updb for authenticator with id %s: %s", pgAuthenticator.ID, err)
			}
			subtype = &AuthenticatorUpdb{
				Username: *pgUpdb.Username,
				Password: *pgUpdb.Password,
				Salt:     *pgUpdb.Salt,
			}
		case "cert":
			pgCert, err := m.dbStores.AuthenticatorCert.LoadOneByAuthenticatorId(pgAuthenticator.ID, nil)

			if err != nil {
				return fmt.Errorf("error migrating authenticator cert for authenticator with id %s: %s", pgAuthenticator.ID, err)
			}
			subtype = &AuthenticatorCert{
				Fingerprint: *pgCert.Fingerprint,
			}
		}

		authenticator := &Authenticator{
			BaseExtEntity: *toBaseBoltEntity(&pgAuthenticator.BaseDbEntity),
			Type:          *pgAuthenticator.Method,
			IdentityId:    *pgAuthenticator.IdentityID,
			SubType:       subtype,
		}
		err = m.stores.Authenticator.Create(step.Ctx, authenticator)
	}
	pfxlog.Logger().Infof("migrated %v authenticators from pg to bolt", len(authenticators))

	return nil
}

func (m *Migrations) migrateEnrollmentsFromPG(step *boltz.MigrationStep) error {
	enrollments, err := m.dbStores.Enrollment.LoadList(queryOptionsListAll)
	if err != nil {
		return err
	}

	for _, pgEnrollment := range enrollments {
		if err != nil {
			return err
		}

		enrollment := &Enrollment{
			BaseExtEntity: *toBaseBoltEntity(&pgEnrollment.BaseDbEntity),
			Token:         *pgEnrollment.Token,
			Method:        *pgEnrollment.Method,
			IdentityId:    *pgEnrollment.IdentityID,
			ExpiresAt:     pgEnrollment.ExpiresAt,
			IssuedAt:      pgEnrollment.CreatedAt,
			CaId:          nil,
			Username:      nil,
			Jwt:           "",
		}
		method := *pgEnrollment.Method
		switch {
		case method == "updb":
			pgUpdb, err := m.dbStores.EnrollmentUpdb.LoadOneByEnrollmentId(pgEnrollment.ID, nil)

			if err != nil {
				return fmt.Errorf("error migrating enrollment updb for enrollment with id %s: %s", pgEnrollment.ID, err)
			}
			enrollment.Username = pgUpdb.Username
		case method == "ott" || method == "ottca":
			pgCert, err := m.dbStores.EnrollmentCert.LoadOneByEnrollmentId(pgEnrollment.ID, nil)

			if err != nil {
				return fmt.Errorf("error migrating enrollment cert for enrollment with id %s: %s", pgEnrollment.ID, err)
			}
			enrollment.CaId = pgCert.CaID
			enrollment.Jwt = *pgCert.Jwt
		}

		err = m.stores.Enrollment.Create(step.Ctx, enrollment)
	}
	pfxlog.Logger().Infof("migrated %v enrollments from pg to bolt", len(enrollments))

	return nil
}

func (m *Migrations) migrateServicesFromPG(step *boltz.MigrationStep) error {
	services, err := m.dbStores.Service.LoadList(queryOptionsListAll)
	if err != nil {
		return err
	}

	for _, pgService := range services {
		var clusterIds []string
		for _, cluster := range pgService.Clusters {
			clusterIds = append(clusterIds, cluster.ID)
		}

		edgeService := &EdgeService{
			Service: db.Service{
				BaseExtEntity:      *toBaseBoltEntity(&pgService.BaseDbEntity),
				TerminatorStrategy: "",
			},
			Name: *pgService.Name,
		}

		if err = m.stores.EdgeService.Create(step.Ctx, edgeService); err != nil {
			return err
		}

		terminator := &db.Terminator{
			BaseExtEntity: *boltz.NewExtEntity(uuid.New().String(), nil),
			Service:       edgeService.Id,
			Router:        stringz.OrEmpty(pgService.EgressRouter),
			Binding:       "transport",
			Address:       stringz.OrEmpty(pgService.EndpointAddress),
			PeerData:      nil,
		}

		if err = m.stores.Terminator.Create(step.Ctx, terminator); err != nil {
			return err
		}

		linkCollection := m.stores.EdgeService.GetLinkCollection(EntityTypeClusters)
		if err = linkCollection.SetLinks(step.Ctx.Tx(), pgService.ID, clusterIds); err != nil {
			return err
		}

		finalPort := 0
		if pgService.DnsPort != nil {
			finalPort = int(*pgService.DnsPort)
		}
		if err = m.createServiceConfigs(step, edgeService, pgService.DnsHostname, finalPort); err != nil {
			return err
		}
	}
	pfxlog.Logger().Infof("migrated %v services from pg to bolt", len(services))

	return nil
}

func (m *Migrations) migrateAppWansFromPG(step *boltz.MigrationStep) error {
	appwans, err := m.dbStores.AppWan.LoadList(queryOptionsListAll)
	if err != nil {
		return err
	}

	for _, pgAppwan := range appwans {
		if err != nil {
			return err
		}

		var serviceIds []string
		for _, service := range pgAppwan.Services {
			serviceIds = append(serviceIds, service.ID)
		}

		var identityIds []string
		for _, identity := range pgAppwan.Identities {
			identityIds = append(identityIds, identity.ID)
		}

		appwan := &Appwan{
			BaseExtEntity: *toBaseBoltEntity(&pgAppwan.BaseDbEntity),
			Name:          *pgAppwan.Name,
			Identities:    identityIds,
			Services:      serviceIds,
		}
		err = m.stores.Appwan.Create(step.Ctx, appwan)
	}
	pfxlog.Logger().Infof("migrated %v appwans from pg to bolt", len(appwans))

	return nil
}

func (m *Migrations) migrateIdentitiesFromPG(step *boltz.MigrationStep) error {
	identities, err := m.dbStores.Identity.LoadList(queryOptionsListAll)

	for _, pgIdentity := range identities {
		if err != nil {
			return err
		}
		identity := &Identity{
			BaseExtEntity:  *toBaseBoltEntity(&pgIdentity.BaseDbEntity),
			Name:           *pgIdentity.Name,
			IdentityTypeId: pgIdentity.Type.ID,
			IsDefaultAdmin: *pgIdentity.IsDefaultAdmin,
			IsAdmin:        *pgIdentity.IsAdmin,
			//enrollments & auths done in their own section
		}
		err = m.stores.Identity.Create(step.Ctx, identity)
	}
	pfxlog.Logger().Infof("migrated %v identities from pg to bolt", len(identities))

	return nil
}

func (m *Migrations) migrateEventLogsFromPG(step *boltz.MigrationStep) error {
	eventLogs, err := m.dbStores.EventLog.LoadList(queryOptionsListAll)
	if err != nil {
		return err
	}

	for _, pgEventLog := range eventLogs {
		if err != nil {
			return err
		}
		eventLog := &EventLog{
			BaseExtEntity:    *toBaseBoltEntity(&pgEventLog.BaseDbEntity),
			Data:             pgEventLog.Data,
			Type:             pgEventLog.Type,
			FormattedMessage: pgEventLog.FormattedMessage,
			FormatString:     pgEventLog.FormatString,
			FormatData:       pgEventLog.FormatData,
			EntityType:       pgEventLog.EntityType,
			EntityId:         pgEventLog.EntityId,
			ActorType:        pgEventLog.ActorType,
			ActorId:          pgEventLog.ActorId,
		}
		err = m.stores.EventLog.Create(step.Ctx, eventLog)
	}
	pfxlog.Logger().Infof("migrated %v geo regions from pg to bolt", len(eventLogs))

	return nil
}

func (m *Migrations) migrateClusterFromPG(step *boltz.MigrationStep) error {
	clusters, err := m.dbStores.Cluster.LoadList(queryOptionsListAll)

	if err != nil {
		return err
	}

	for _, pgCluster := range clusters {
		if err != nil {
			return err
		}
		cluster := &Cluster{
			BaseExtEntity: *toBaseBoltEntity(&pgCluster.BaseDbEntity),
			Name:          *pgCluster.Name,
		}
		err = m.stores.Cluster.Create(step.Ctx, cluster)
	}
	pfxlog.Logger().Infof("migrated %v clusters from pg to bolt", len(clusters))

	return nil
}

func (m *Migrations) migrateEdgeRoutersFromPG(step *boltz.MigrationStep) error {
	edgeRouters, err := m.dbStores.Gateway.LoadList(queryOptionsListAll)

	if err != nil {
		return err
	}

	for _, pgEdgeRouter := range edgeRouters {
		if err != nil {
			return err
		}

		edgeRouter := &EdgeRouter{
			BaseExtEntity:       *toBaseBoltEntity(&pgEdgeRouter.BaseDbEntity),
			Name:                *pgEdgeRouter.Name,
			ClusterId:           pgEdgeRouter.ClusterID,
			Fingerprint:         pgEdgeRouter.Fingerprint,
			CertPem:             pgEdgeRouter.CertPem,
			IsVerified:          *pgEdgeRouter.IsVerified,
			EnrollmentToken:     pgEdgeRouter.EnrollmentToken,
			Hostname:            pgEdgeRouter.Hostname,
			EnrollmentJwt:       pgEdgeRouter.EnrollmentToken,
			EnrollmentCreatedAt: pgEdgeRouter.EnrollmentCreatedAt,
			EnrollmentExpiresAt: pgEdgeRouter.EnrollmentExpiresAt,
			EdgeRouterProtocols: pgEdgeRouter.GatewayProtocols,
		}
		err = m.stores.EdgeRouter.Create(step.Ctx, edgeRouter)
	}
	pfxlog.Logger().Infof("migrated %v edge-routers from pg to bolt", len(edgeRouters))

	return nil
}

func toBaseBoltEntity(entity *migration.BaseDbEntity) *boltz.BaseExtEntity {
	result := boltz.BaseExtEntity{Id: entity.ID}
	if entity.Tags != nil {
		result.Tags = *entity.Tags
	}
	if entity.CreatedAt != nil {
		result.CreatedAt = *entity.CreatedAt
		result.Migrate = true
	}
	if entity.UpdatedAt != nil {
		result.UpdatedAt = *entity.UpdatedAt
	} else if result.Migrate {
		result.UpdatedAt = time.Now()
	}
	return &result
}
