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

package migration

import (
	"github.com/jinzhu/gorm"
)

type Stores struct {
	AppWan             AppWanStore
	Authenticator      AuthenticatorStore
	Ca                 CaStore
	Cluster            ClusterStore
	Enrollment         EnrollmentStore
	EventLog           EventLogStore
	Gateway            GatewayStore
	GeoRegion          GeoRegionStore
	Identity           IdentityStore
	IdentityType       IdentityTypeStore
	NetworkSession     NetworkSessionStore
	Service            ServiceStore
	Session            SessionStore
	NetworkSessionCert *NetworkSessionCertGormStore
	AuthenticatorUpdb  *AuthenticatorUpdbGormStore
	AuthenticatorCert  *AuthenticatorCertGormStore
	EnrollmentCert     *EnrollmentCertGormStore
	EnrollmentUpdb     *EnrollmentUpdbGormStore
}

func NewGormStores(db *gorm.DB, dbWithPreloads *gorm.DB) *Stores {
	return &Stores{
		AppWan:             NewAppWanGormStore(db, dbWithPreloads),
		Authenticator:      NewAuthenticatorGormStore(db, dbWithPreloads),
		Ca:                 NewCaGormStore(db, dbWithPreloads),
		Cluster:            NewClusterGormStore(db, dbWithPreloads),
		Enrollment:         NewEnrollmentGormStore(db, dbWithPreloads),
		EventLog:           NewEventLogGormStore(db, dbWithPreloads),
		Gateway:            NewGatewayGormStore(db, dbWithPreloads),
		GeoRegion:          NewGeoRegionGormStore(db, dbWithPreloads),
		Identity:           NewIdentityGormStore(db, dbWithPreloads),
		IdentityType:       NewIdentityTypeGormStore(db, dbWithPreloads),
		NetworkSession:     NewNetworkSessionGormStore(db, dbWithPreloads),
		Service:            NewServiceGormStore(db, dbWithPreloads),
		Session:            NewSessionGormStore(db, dbWithPreloads),
		NetworkSessionCert: NewNetworkSessionCertGormStore(db, dbWithPreloads),
		AuthenticatorUpdb:  NewAuthenticatorUpdbGormStore(db, dbWithPreloads),
		AuthenticatorCert:  NewAuthenticatorCertGormStore(db, dbWithPreloads),
		EnrollmentCert:     NewEnrollmentCertGormStore(db, dbWithPreloads),
		EnrollmentUpdb:     NewEnrollmentUpdbGormStore(db, dbWithPreloads),
	}
}
