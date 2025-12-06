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

package permissions

const (
	AdminPermission                 = "admin"
	AdminReadOnlyPermission         = "admin_readonly"
	AuthenticatedPermission         = "authenticated"
	PartiallyAuthenticatePermission = "partial_auth"
)

const Ops = "ops"

type Action string

const (
	Read   Action = "read"
	Create Action = "create"
	Update Action = "update"
	Delete Action = "delete"
)

type Api string

const (
	Management Api = "management"
)

// AllPermissions contains all entity type permissions and entity-action permissions
var AllPermissions = map[string]struct{}{
	// Base permissions
	AdminReadOnlyPermission: {},

	// auth-policy permissions
	"auth-policy":        {},
	"auth-policy.read":   {},
	"auth-policy.create": {},
	"auth-policy.update": {},
	"auth-policy.delete": {},

	// authenticator permissions - for now, lock down authenticator to admin and admin readonly
	//"authenticator":      {},
	//"authenticator.read": {},
	//"authenticator.create": {},
	//"authenticator.update": {},
	//"authenticator.delete": {},

	// ca permissions
	"ca":        {},
	"ca.read":   {},
	"ca.create": {},
	"ca.update": {},
	"ca.delete": {},

	// config permissions
	"config":        {},
	"config.read":   {},
	"config.create": {},
	"config.update": {},
	"config.delete": {},

	// config-type permissions
	"config-type":        {},
	"config-type.read":   {},
	"config-type.create": {},
	"config-type.update": {},
	"config-type.delete": {},

	// edge-router-policy permissions
	"edge-router-policy":        {},
	"edge-router-policy.read":   {},
	"edge-router-policy.create": {},
	"edge-router-policy.update": {},
	"edge-router-policy.delete": {},

	// enrollment permissions
	"enrollment":        {},
	"enrollment.read":   {},
	"enrollment.create": {},
	"enrollment.update": {},
	"enrollment.delete": {},

	// external-jwt-signer permissions
	"external-jwt-signer":        {},
	"external-jwt-signer.read":   {},
	"external-jwt-signer.create": {},
	"external-jwt-signer.update": {},
	"external-jwt-signer.delete": {},

	// identity permissions
	"identity":        {},
	"identity.read":   {},
	"identity.create": {},
	"identity.update": {},
	"identity.delete": {},

	// ops permissions
	// covers api-sessions, sessions, circuits, links, inspect and validate operations
	"ops":        {},
	"ops.read":   {},
	"ops.update": {},
	"ops.delete": {},

	// posture-check permissions
	"posture-check":        {},
	"posture-check.read":   {},
	"posture-check.create": {},
	"posture-check.update": {},
	"posture-check.delete": {},

	// router permissions
	"router":        {},
	"router.read":   {},
	"router.create": {},
	"router.update": {},
	"router.delete": {},

	// service permissions
	"service":        {},
	"service.read":   {},
	"service.create": {},
	"service.update": {},
	"service.delete": {},

	// service-edge-router-policy permissions
	"service-edge-router-policy":        {},
	"service-edge-router-policy.read":   {},
	"service-edge-router-policy.create": {},
	"service-edge-router-policy.update": {},
	"service-edge-router-policy.delete": {},

	// service-policy permissions
	"service-policy":        {},
	"service-policy.read":   {},
	"service-policy.create": {},
	"service-policy.update": {},
	"service-policy.delete": {},

	// terminator permissions
	"terminator":        {},
	"terminator.read":   {},
	"terminator.create": {},
	"terminator.update": {},
	"terminator.delete": {},
}

type Context interface {
	GetApi() Api // client, management, etc
	HasPermission(string) bool
	GetEntityType() string
	GetEntityAction() string
	GetAction() Action
}

type Resolver interface {
	IsAllowed(ctx Context) bool
}

type ResolverF func(ctx Context) bool

func (r ResolverF) IsAllowed(ctx Context) bool {
	return r(ctx)
}
