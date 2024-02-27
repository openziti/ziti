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
	"github.com/openziti/storage/boltz"
	"strings"
)

const (
	EntityTypeApiSessions               = "apiSessions"
	EntityTypeApiSessionCertificates    = "apiSessionCertificates"
	EntityTypeAuthPolicies              = "authPolicies"
	EntityTypeEventualEvents            = "eventualEvents"
	EntityTypeCas                       = "cas"
	EntityTypeConfigs                   = "configs"
	EntityTypeConfigTypes               = "configTypes"
	EntityTypeEdgeRouterPolicies        = "edgeRouterPolicies"
	EntityTypeExternalJwtSigners        = "externalJwtSigners"
	EntityTypeIdentities                = "identities"
	EntityTypeIdentityTypes             = "identityTypes"
	EntityTypeMfas                      = "mfas"
	EntityTypeRevocations               = "revocations"
	EntityTypeServicePolicies           = "servicePolicies"
	EntityTypeServiceEdgeRouterPolicies = "serviceEdgeRouterPolicies"
	EntityTypeSessions                  = "sessions"
	EntityTypeSessionCerts              = "sessionCerts"
	EntityTypeEnrollments               = "enrollments"
	EntityTypeAuthenticators            = "authenticators"
	EntityTypePostureChecks             = "postureChecks"
	EntityTypePostureCheckTypes         = "postureCheckTypes"
	EdgeBucket                          = "edge"

	FieldName           = "name"
	FieldSemantic       = "semantic"
	FieldRoleAttributes = "roleAttributes"

	FieldEdgeRouterRoles   = "edgeRouterRoles"
	FieldIdentityRoles     = "identityRoles"
	FieldServiceRoles      = "serviceRoles"
	FieldPostureCheckRoles = "postureCheckRoles"

	SemanticAllOf = "AllOf"
	SemanticAnyOf = "AnyOf"
)

var validSemantics = []string{SemanticAllOf, SemanticAnyOf}

func isSemanticValid(semantic string) bool {
	for _, validSemantic := range validSemantics {
		if strings.EqualFold(validSemantic, semantic) {
			return true
		}
	}
	return false
}

type Policy interface {
	boltz.NamedExtEntity
}
