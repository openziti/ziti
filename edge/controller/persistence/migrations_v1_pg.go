/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/edge/controller/predicate"
	"github.com/netfoundry/ziti-edge/edge/migration"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-foundation/util/stringz"
	"time"
)

var queryOptionsListAll = &migration.QueryOptions{
	Paging: &predicate.Paging{
		Offset:    0,
		Limit:     0,
		ReturnAll: true,
	},
}

func upgradeToV1FromPG(mtx *MigrationContext) error {
	pfxlog.Logger().Info("postgres configured, migrating from postgres")

	migrationFuncs := []func(mtx *MigrationContext) error{
		migrateIdentitiesFromPG,
		migrateCasFromPG,
		migrateAuthenticatorsFromPG,
		migrateEnrollmentsFromPG,
		migrateEventLogsFromPG,
		migrateClusterFromPG,
		migrateEdgeRoutersFromPG,
		migrateServicesFromPG,
		migrateAppWansFromPG,
	}

	for _, migrationFunc := range migrationFuncs {
		if err := migrationFunc(mtx); err != nil {
			return err
		}
	}

	return nil
}

func migrateCasFromPG(mtx *MigrationContext) error {
	cas, err := mtx.DbStores.Ca.LoadList(queryOptionsListAll)
	if err != nil {
		return err
	}

	for _, pgCa := range cas {
		if err != nil {
			return err
		}
		ca := &Ca{
			BaseEdgeEntityImpl:        *toBaseBoltEntity(&pgCa.BaseDbEntity),
			Name:                      *pgCa.Name,
			Fingerprint:               *pgCa.Fingerprint,
			CertPem:                   *pgCa.CertPem,
			IsVerified:                *pgCa.IsVerified,
			VerificationToken:         stringz.OrEmpty(pgCa.VerificationToken),
			IsAutoCaEnrollmentEnabled: *pgCa.IsAutoCaEnrollmentEnabled,
			IsOttCaEnrollmentEnabled:  *pgCa.IsOttCaEnrollmentEnabled,
			IsAuthEnabled:             *pgCa.IsAuthEnabled,
		}
		err = mtx.Stores.Ca.Create(mtx.Ctx, ca)
	}
	pfxlog.Logger().Infof("migrated %v cas from pg to bolt", len(cas))

	return nil
}

func migrateAuthenticatorsFromPG(mtx *MigrationContext) error {
	authenticators, err := mtx.DbStores.Authenticator.LoadList(queryOptionsListAll)
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
			pgUpdb, err := mtx.DbStores.AuthenticatorUpdb.LoadOneByAuthenticatorId(pgAuthenticator.ID, nil)

			if err != nil {
				return fmt.Errorf("error migrating authenticator updb for authenticator with id %s: %s", pgAuthenticator.ID, err)
			}
			subtype = &AuthenticatorUpdb{
				Username: *pgUpdb.Username,
				Password: *pgUpdb.Password,
				Salt:     *pgUpdb.Salt,
			}
		case "cert":
			pgCert, err := mtx.DbStores.AuthenticatorCert.LoadOneByAuthenticatorId(pgAuthenticator.ID, nil)

			if err != nil {
				return fmt.Errorf("error migrating authenticator cert for authenticator with id %s: %s", pgAuthenticator.ID, err)
			}
			subtype = &AuthenticatorCert{
				Fingerprint: *pgCert.Fingerprint,
			}
		}

		authenticator := &Authenticator{
			BaseEdgeEntityImpl: *toBaseBoltEntity(&pgAuthenticator.BaseDbEntity),
			Type:               *pgAuthenticator.Method,
			IdentityId:         *pgAuthenticator.IdentityID,
			SubType:            subtype,
		}
		err = mtx.Stores.Authenticator.Create(mtx.Ctx, authenticator)
	}
	pfxlog.Logger().Infof("migrated %v authenticators from pg to bolt", len(authenticators))

	return nil
}

func migrateEnrollmentsFromPG(mtx *MigrationContext) error {
	enrollments, err := mtx.DbStores.Enrollment.LoadList(queryOptionsListAll)
	if err != nil {
		return err
	}

	for _, pgEnrollment := range enrollments {
		if err != nil {
			return err
		}

		enrollment := &Enrollment{
			BaseEdgeEntityImpl: *toBaseBoltEntity(&pgEnrollment.BaseDbEntity),
			Token:              *pgEnrollment.Token,
			Method:             *pgEnrollment.Method,
			IdentityId:         *pgEnrollment.IdentityID,
			ExpiresAt:          pgEnrollment.ExpiresAt,
			IssuedAt:           pgEnrollment.CreatedAt,
			CaId:               nil,
			Username:           nil,
			Jwt:                "",
		}
		method := *pgEnrollment.Method
		switch {
		case method == "updb":
			pgUpdb, err := mtx.DbStores.EnrollmentUpdb.LoadOneByEnrollmentId(pgEnrollment.ID, nil)

			if err != nil {
				return fmt.Errorf("error migrating enrollment updb for enrollment with id %s: %s", pgEnrollment.ID, err)
			}
			enrollment.Username = pgUpdb.Username
		case method == "ott" || method == "ottca":
			pgCert, err := mtx.DbStores.EnrollmentCert.LoadOneByEnrollmentId(pgEnrollment.ID, nil)

			if err != nil {
				return fmt.Errorf("error migrating enrollment cert for enrollment with id %s: %s", pgEnrollment.ID, err)
			}
			enrollment.CaId = pgCert.CaID
			enrollment.Jwt = *pgCert.Jwt
		}

		err = mtx.Stores.Enrollment.Create(mtx.Ctx, enrollment)
	}
	pfxlog.Logger().Infof("migrated %v enrollments from pg to bolt", len(enrollments))

	return nil
}

func migrateServicesFromPG(mtx *MigrationContext) error {
	services, err := mtx.DbStores.Service.LoadList(queryOptionsListAll)
	if err != nil {
		return err
	}

	for _, pgService := range services {
		if err != nil {
			return err
		}
		var clusterIds []string
		for _, cluster := range pgService.Clusters {
			clusterIds = append(clusterIds, cluster.ID)
		}

		var hostedIds []string
		for _, hostId := range pgService.HostIds {
			hostedIds = append(hostedIds, hostId.ID)
		}

		edgeService := &EdgeService{
			Service: network.Service{
				Id:              pgService.ID,
				Binding:         "edge", //todo confirm this
				EndpointAddress: stringz.OrEmpty(pgService.EndpointAddress),
				Egress:          stringz.OrEmpty(pgService.EgressRouter),
			},

			EdgeEntityFields: toBaseBoltEntity(&pgService.BaseDbEntity).EdgeEntityFields,
			Name:             *pgService.Name,
			DnsHostname:      stringz.OrEmpty(pgService.DnsHostname),
			DnsPort:          *pgService.DnsPort,
			Clusters:         clusterIds,
			HostIds:          hostedIds,
		}
		err = mtx.Stores.EdgeService.Create(mtx.Ctx, edgeService)
	}
	pfxlog.Logger().Infof("migrated %v services from pg to bolt", len(services))

	return nil
}

func migrateAppWansFromPG(mtx *MigrationContext) error {
	appwans, err := mtx.DbStores.AppWan.LoadList(queryOptionsListAll)
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
			BaseEdgeEntityImpl: *toBaseBoltEntity(&pgAppwan.BaseDbEntity),
			Name:               *pgAppwan.Name,
			Identities:         identityIds,
			Services:           serviceIds,
		}
		err = mtx.Stores.Appwan.Create(mtx.Ctx, appwan)
	}
	pfxlog.Logger().Infof("migrated %v appwans from pg to bolt", len(appwans))

	return nil
}

func migrateIdentitiesFromPG(mtx *MigrationContext) error {
	identities, err := mtx.DbStores.Identity.LoadList(queryOptionsListAll)
	if err != nil {
		return err
	}

	for _, pgIdentity := range identities {
		if err != nil {
			return err
		}
		identity := &Identity{
			BaseEdgeEntityImpl: *toBaseBoltEntity(&pgIdentity.BaseDbEntity),
			Name:               *pgIdentity.Name,
			IdentityTypeId:     pgIdentity.Type.ID,
			IsDefaultAdmin:     *pgIdentity.IsDefaultAdmin,
			IsAdmin:            *pgIdentity.IsAdmin,
			//enrollments & auths done in their own section
		}
		err = mtx.Stores.Identity.Create(mtx.Ctx, identity)
	}
	pfxlog.Logger().Infof("migrated %v identities from pg to bolt", len(identities))

	return nil
}

func migrateEventLogsFromPG(mtx *MigrationContext) error {

	eventLogs, err := mtx.DbStores.EventLog.LoadList(queryOptionsListAll)
	if err != nil {
		return err
	}

	for _, pgEventLog := range eventLogs {
		if err != nil {
			return err
		}
		eventLog := &EventLog{
			BaseEdgeEntityImpl: *toBaseBoltEntity(&pgEventLog.BaseDbEntity),
			Data:               pgEventLog.Data,
			Type:               pgEventLog.Type,
			FormattedMessage:   pgEventLog.FormattedMessage,
			FormatString:       pgEventLog.FormatString,
			FormatData:         pgEventLog.FormatData,
			EntityType:         pgEventLog.EntityType,
			EntityId:           pgEventLog.EntityId,
			ActorType:          pgEventLog.ActorType,
			ActorId:            pgEventLog.ActorId,
		}
		err = mtx.Stores.EventLog.Create(mtx.Ctx, eventLog)
	}
	pfxlog.Logger().Infof("migrated %v geo regions from pg to bolt", len(eventLogs))

	return nil
}

func migrateClusterFromPG(mtx *MigrationContext) error {
	clusters, err := mtx.DbStores.Cluster.LoadList(queryOptionsListAll)

	if err != nil {
		return err
	}

	for _, pgCluster := range clusters {
		if err != nil {
			return err
		}
		cluster := &Cluster{
			BaseEdgeEntityImpl: *toBaseBoltEntity(&pgCluster.BaseDbEntity),
			Name:               *pgCluster.Name,
		}
		err = mtx.Stores.Cluster.Create(mtx.Ctx, cluster)
	}
	pfxlog.Logger().Infof("migrated %v clusters from pg to bolt", len(clusters))

	return nil
}

func migrateEdgeRoutersFromPG(mtx *MigrationContext) error {
	edgeRouters, err := mtx.DbStores.Gateway.LoadList(queryOptionsListAll)

	if err != nil {
		return err
	}

	for _, pgEdgeRouter := range edgeRouters {
		if err != nil {
			return err
		}

		edgeRouter := &EdgeRouter{
			BaseEdgeEntityImpl:  *toBaseBoltEntity(&pgEdgeRouter.BaseDbEntity),
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
		err = mtx.Stores.EdgeRouter.Create(mtx.Ctx, edgeRouter)
	}
	pfxlog.Logger().Infof("migrated %v edge-routers from pg to bolt", len(edgeRouters))

	return nil
}

func toBaseBoltEntity(entity *migration.BaseDbEntity) *BaseEdgeEntityImpl {
	result := BaseEdgeEntityImpl{Id: entity.ID}
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
