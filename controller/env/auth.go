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

package env

import (
	"github.com/gobuffalo/packr"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"github.com/netfoundry/ziti-edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-edge/migration"
	"net/http"
)

const (
	ClientCertHeader       = "X-Client-CertPem"
	EdgeRouterProxyRequest = "X-Edge-Router-Proxy-Request"
)

type AuthStore interface {
	BaseLoadOneByAuthenticatorId(authId string, pl *migration.Preloads) (migration.BaseDbModel, error)
	migration.Store
}

type AuthModuler interface {
	Registerer
	Migrater
	Authenticator
	Enroller
}

type AuthHandlerFunc func(ae *AppEnv, rc *response.RequestContext, hc *HandlerContext)

type HandlerContext struct {
	TargetIdentity *model.Identity
	Data           map[string]interface{}
}

type Migrater interface {
	GetMigrations() []*packr.Box
}

type EnrollmentContextOld struct {
	Enrollment    *migration.Enrollment
	Authenticator *migration.Authenticator
	Identity      *migration.Identity
	Method        string //dupes Enrollment.Method, but Enrollment is not always defined
}

type EnrollmentContext struct {
	Enrollment    *model.Enrollment
	Authenticator *model.Authenticator
	Identity      *model.Identity
	Method        string //dupes Enrollment.Method, but Enrollment is not always defined
}

type Enroller interface {
	ProcessEnrollmentConfig(ae *AppEnv, method string, config interface{}) (interface{}, error)
	Enroll(ae *AppEnv, rc *response.RequestContext, ec *EnrollmentContextOld)
	CreateEnrollment(ae *AppEnv, ec *EnrollmentContextOld, tx *gorm.DB) error
	IsIdentityEnrollment(m string) bool
	IsEnrollerForMethod(m string) bool
	RenderEnrollmentDetails(e *migration.Enrollment) (interface{}, error)
}

type Authenticator interface {
	IsAuthenticatorForMethod(string) bool
	IsAuthorized(*AppEnv, *http.Request) (string, error)
	RenderAuthenticatorDetails(e *migration.Authenticator) (interface{}, error)
	AuthStore() AuthStore
	Fingerprints(auth *migration.Authenticator) ([]string, error)
}

type Registerer interface {
	Register(ae *AppEnv, identityRouter *mux.Router, currentUserRouter *mux.Router, idType response.IdType)
}

type Registry struct {
	enrollers      []Enroller
	authenticators []Authenticator
	migraters      []Migrater
	registerers    []Registerer
}

func (r *Registry) AddModule(a AuthModuler) {
	r.AddEnroller(a)
	r.AddAuthenticator(a)
	r.AddMigrater(a)
	r.AddRegisterer(a)
}

func (r *Registry) AddMigrater(m Migrater) {
	r.migraters = append(r.migraters, m)
}

func (r *Registry) AddRegisterer(rer Registerer) {
	r.registerers = append(r.registerers, rer)
}

func (r *Registry) AddAuthenticator(a Authenticator) {
	r.authenticators = append(r.authenticators, a)
}

func (r *Registry) AddEnroller(e Enroller) {
	r.enrollers = append(r.enrollers, e)
}

func (r *Registry) GetAuthenticatorByMethod(m string) Authenticator {
	for _, a := range r.authenticators {
		if a.IsAuthenticatorForMethod(m) {
			return a
		}
	}

	return nil
}

func (r *Registry) GetEnrollerByMethod(m string) Enroller {
	for _, a := range r.enrollers {
		if a.IsEnrollerForMethod(m) {
			return a
		}
	}

	return nil
}

func (r *Registry) GetAuthenticators() []Authenticator {
	return r.authenticators
}

func (r *Registry) GetEnrollers() []Enroller {
	return r.enrollers
}

func (r *Registry) GetMigraters() []Migrater {
	return r.migraters
}

func (r *Registry) GetRegisterers() []Registerer {
	return r.registerers
}

func WrapByIdHandler(ae *AppEnv, f AuthHandlerFunc, idType response.IdType, prs ...permissions.Resolver) http.HandlerFunc {
	return ae.WrapHandler(func(ae *AppEnv, rc *response.RequestContext) {

		id, err := rc.GetIdFromRequest(idType)

		if err != nil {
			rc.RequestResponder.RespondWithNotFound()
			return
		}

		i, err := ae.GetHandlers().Identity.HandleRead(id)

		if err != nil {
			rc.RequestResponder.RespondWithNotFound()
			return
		}

		hc := &HandlerContext{
			TargetIdentity: i,
			Data:           map[string]interface{}{},
		}
		f(ae, rc, hc)
	}, prs...)
}
