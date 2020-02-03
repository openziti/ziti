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
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"fmt"
	"github.com/netfoundry/ziti-foundation/util/mirror"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"

	jwt2 "github.com/dgrijalva/jwt-go"
	"github.com/gobuffalo/packr"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/apierror"
	edgeconfig "github.com/netfoundry/ziti-edge/controller/config"
	"github.com/netfoundry/ziti-edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/controller/middleware"
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-edge/controller/util"
	"github.com/netfoundry/ziti-edge/internal/cert"
	"github.com/netfoundry/ziti-edge/internal/jwt"
	"github.com/netfoundry/ziti-edge/migration"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-fabric/xctrl"
	"github.com/netfoundry/ziti-fabric/xmgmt"
	"github.com/netfoundry/ziti-foundation/common/constants"
	"github.com/netfoundry/ziti-sdk-golang/ziti/config"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/oleiade/reflections.v1"
)

type AppEnv struct {
	BoltStores              *persistence.Stores
	Handlers                *model.Handlers
	Embedded                *packr.Box
	Schemes                 *Schemes
	Config                  *edgeconfig.Config
	EnrollmentJwtGenerator  jwt.EnrollmentGenerator
	Versions                *config.Versions
	AuthHeaderName          string
	AuthCookieName          string
	ApiServerCsrSigner      cert.Signer
	ApiClientCsrSigner      cert.Signer
	ControlClientCsrSigner  cert.Signer
	FingerprintGenerator    cert.FingerprintGenerator
	RootRouter              *mux.Router
	RequestResponderFactory response.RequestResponderFactory
	ModelHandlers           *migration.ModelHandlers
	AuthRegistry            model.AuthRegistry
	EnrollRegistry          model.EnrollmentRegistry
	Broker                  *Broker
	HostController          HostController
}

func (ae *AppEnv) GetApiServerCsrSigner() cert.Signer {
	return ae.ApiServerCsrSigner
}

func (ae *AppEnv) GetControlClientCsrSigner() cert.Signer {
	return ae.ControlClientCsrSigner
}

func (ae *AppEnv) GetHostController() model.HostController {
	return ae.HostController
}

func (ae *AppEnv) GetSchemas() model.Schemas {
	return ae.Schemes
}

func (ae *AppEnv) GetHandlers() *model.Handlers {
	return ae.Handlers
}

func (ae *AppEnv) GetConfig() *edgeconfig.Config {
	return ae.Config
}

func (ae *AppEnv) GetEnrollmentJwtGenerator() jwt.EnrollmentGenerator {
	return ae.EnrollmentJwtGenerator
}

func (ae *AppEnv) GetDbProvider() persistence.DbProvider {
	return ae.HostController.GetNetwork()
}

func (ae *AppEnv) GetStores() *persistence.Stores {
	return ae.BoltStores
}

func (ae *AppEnv) GetAuthRegistry() model.AuthRegistry {
	return ae.AuthRegistry
}

func (ae *AppEnv) GetEnrollRegistry() model.EnrollmentRegistry {
	return ae.EnrollRegistry
}

func (ae *AppEnv) IsEdgeRouterOnline(id string) bool {
	return ae.Broker.GetOnlineEdgeRouter(id) != nil
}

func (ae *AppEnv) GetApiClientCsrSigner() cert.Signer {
	return ae.ApiClientCsrSigner
}

type HostController interface {
	RegisterXctrl(x xctrl.Xctrl) error
	RegisterXmgmt(x xmgmt.Xmgmt) error
	GetNetwork() *network.Network
}

type Schemes struct {
	Association      *BasicEntitySchema
	Ca               *BasicEntitySchema
	Config           *BasicEntitySchema
	ConfigType       *BasicEntitySchema
	Enroller         *BasicEntitySchema
	EnrollEr         *BasicEntitySchema
	EnrollUpdb       *BasicEntitySchema
	EdgeRouter       *BasicEntitySchema
	EdgeRouterPolicy *BasicEntitySchema
	Identity         *IdentityEntitySchema
	Service          *BasicEntitySchema
	ServicePolicy    *BasicEntitySchema
	Session          *BasicEntitySchema
}

func (s Schemes) GetEnrollErPost() *gojsonschema.Schema {
	return s.EnrollEr.Post
}

func (s Schemes) GetEnrollUpdbPost() *gojsonschema.Schema {
	return s.EnrollUpdb.Post
}

type IdentityEntitySchema struct {
	Post           *gojsonschema.Schema
	Patch          *gojsonschema.Schema
	Put            *gojsonschema.Schema
	ServiceConfigs *gojsonschema.Schema
}

type BasicEntitySchema struct {
	Post  *gojsonschema.Schema
	Patch *gojsonschema.Schema
	Put   *gojsonschema.Schema
}

type AppHandler func(ae *AppEnv, rc *response.RequestContext)

type AppMiddleware func(*AppEnv, http.Handler) http.Handler

func NewAppEnv(c *edgeconfig.Config) *AppEnv {

	// See README.md in /ziti/edge/embedded for details
	// This path is relative to source location of this file
	embedded := packr.NewBox("../embedded")

	c.Persistence.AddMigrationBox(&embedded)

	ae := &AppEnv{
		Config:   c,
		Embedded: &embedded,
		Versions: &config.Versions{
			Api:           "1.0.0",
			EnrollmentApi: "1.0.0",
		},
		AuthCookieName:          constants.ZitiSession,
		AuthHeaderName:          constants.ZitiSession,
		RootRouter:              mux.NewRouter(),
		RequestResponderFactory: response.NewRequestResponder,
		ModelHandlers:           migration.GetModelHandlers(),
		AuthRegistry:            &model.AuthProcessorRegistryImpl{},
		EnrollRegistry:          &model.EnrollmentRegistryImpl{},
	}

	sm := getJwtSigningMethod(c.Api.Identity.ServerCert())
	key := c.Api.Identity.ServerCert().PrivateKey

	ae.EnrollmentJwtGenerator = jwt.NewJwtIdentityEnrollmentGenerator(ae.Config.Api.Advertise, sm, key)

	ae.ApiClientCsrSigner = cert.NewClientSigner(ae.Config.Enrollment.SigningCert.Cert().Leaf, ae.Config.Enrollment.SigningCert.Cert().PrivateKey)
	ae.ApiServerCsrSigner = cert.NewServerSigner(ae.Config.Enrollment.SigningCert.Cert().Leaf, ae.Config.Enrollment.SigningCert.Cert().PrivateKey)
	ae.ControlClientCsrSigner = cert.NewClientSigner(ae.Config.Enrollment.SigningCert.Cert().Leaf, ae.Config.Enrollment.SigningCert.Cert().PrivateKey)

	ae.Schemes = &Schemes{}

	err := ae.LoadSchemas()

	ae.FingerprintGenerator = cert.NewFingerprintGenerator()

	if err != nil {
		log := pfxlog.Logger()
		log.WithField("cause", err).Fatal("could not load schemas")
	}

	return ae
}

func (ae *AppEnv) InitPersistence() error {
	var err error

	db := InitPersistence(&ae.Config.Persistence)
	var dbStores *migration.Stores
	if db != nil {
		dbWithPreload := db.New().Set("gorm:auto_preload", true)
		dbStores = migration.NewGormStores(db, dbWithPreload)
	}

	ae.BoltStores, err = persistence.NewBoltStores(ae.HostController.GetNetwork())
	if err == nil {
		err = persistence.RunMigrations(ae.HostController.GetNetwork(), ae.BoltStores, dbStores)
	}

	if err == nil {
		ae.Handlers = model.InitHandlers(ae)
	}

	return err
}

func getJwtSigningMethod(cert *tls.Certificate) jwt2.SigningMethod {

	var sm jwt2.SigningMethod = jwt2.SigningMethodNone

	switch cert.Leaf.PublicKey.(type) {
	case *ecdsa.PublicKey:
		key := cert.Leaf.PublicKey.(*ecdsa.PublicKey)
		switch key.Params().BitSize {
		case jwt2.SigningMethodES256.CurveBits:
			sm = jwt2.SigningMethodES256
		case jwt2.SigningMethodES384.CurveBits:
			sm = jwt2.SigningMethodES384
		case jwt2.SigningMethodES512.CurveBits:
			sm = jwt2.SigningMethodES512
		default:
			pfxlog.Logger().Panic("unsupported EC key size: ", key.Params().BitSize)
		}
	case *rsa.PublicKey:
		sm = jwt2.SigningMethodRS256
	default:
		pfxlog.Logger().Panic("unknown certificate type, unable to determine signing method")
	}

	return sm
}

func (ae *AppEnv) getSessionTokenFromRequest(r *http.Request) string {
	token := r.Header.Get(ae.AuthHeaderName)

	if token == "" {
		sessionCookie, _ := r.Cookie(ae.AuthCookieName)
		if sessionCookie != nil {
			token = sessionCookie.Value
		}
	}
	return token
}

func (ae *AppEnv) WrapHandler(f AppHandler, prs ...permissions.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := uuid.New()

		sw, ok := w.(*middleware.StatusWriter)

		if ok {
			rid = sw.RequestId
		}

		rc := &response.RequestContext{
			Id:                rid,
			Identity:          nil,
			ApiSession:        nil,
			ActivePermissions: []string{},
			ResponseWriter:    w,
			Request:           r,
			EventLogger:       &DefaultEventLogger{Ae: ae},
		}

		rc.RequestResponder = ae.RequestResponderFactory(rc)

		sessionToken := ae.getSessionTokenFromRequest(r)
		log := pfxlog.Logger()
		_, err := uuid.Parse(sessionToken)

		if err != nil {
			log.WithError(err).Debug("failed to parse session id")
			sessionToken = ""
		} else {
			log.Tracef("authorizing request using session id '%v'", sessionToken)
		}

		if sessionToken != "" {
			rc.ApiSession, err = ae.GetHandlers().ApiSession.ReadByToken(sessionToken)

			if err != nil {
				//don't error on "not found", just an un-authed session, rely on permissions below
				//error on anything else as we  failed to work with the store
				if !util.IsErrNotFoundErr(err) {
					log.WithError(err).Debug("error requesting session")
					rc.RequestResponder.RespondWithError(err)
					return
				}
			}
		}

		//updates updatedAt for session timeouts
		if rc.ApiSession != nil {
			err := ae.GetHandlers().ApiSession.Update(rc.ApiSession)
			if err != nil && !util.IsErrNotFoundErr(err) {
				log.WithError(err).Debug("failed to update session activity")
				rc.RequestResponder.RespondWithError(err)
				return
			}
			//re-read session to get new updatedAt
			rc.ApiSession, _ = ae.GetHandlers().ApiSession.Read(rc.ApiSession.Id)
		}

		if rc.ApiSession != nil {
			rc.Identity, err = ae.GetHandlers().Identity.Read(rc.ApiSession.IdentityId)
			if err != nil {
				if util.IsErrNotFoundErr(err) {
					apiErr := apierror.NewUnauthorized()
					apiErr.Cause = fmt.Errorf("associated identity %s not found", rc.ApiSession.IdentityId)
					apiErr.AppendCause = true
					rc.RequestResponder.RespondWithApiError(apiErr)
				} else {
					rc.RequestResponder.RespondWithError(err)
				}
				return
			}
		}

		if rc.Identity != nil {
			rc.ActivePermissions = append(rc.ActivePermissions, permissions.AuthenticatedPermission)

			if rc.Identity.IsAdmin {
				rc.ActivePermissions = append(rc.ActivePermissions, permissions.AdminPermission)
			}
		}

		for _, pr := range prs {
			if !pr.IsAllowed(rc.ActivePermissions...) {
				rc.RequestResponder.RespondWithUnauthorizedError(rc)
				return
			}
		}

		f(ae, rc)
	}
}

func (ae *AppEnv) WrapMiddleware(f AppMiddleware) mux.MiddlewareFunc {
	return func(h http.Handler) http.Handler {
		return f(ae, h)
	}
}

func (ae *AppEnv) GetEmbeddedFileContent(filePath string) (string, error) {
	file, err := ae.Embedded.Open(filePath)

	if err != nil {
		return "", err
	}

	fileContents, err := ioutil.ReadAll(file)

	if err != nil {
		return "", err
	}

	return string(fileContents), err
}

func (ae *AppEnv) LoadSchemas() error {
	// add custom format checkers
	gojsonschema.FormatCheckers.Add("uuid", gojsonschema.UUIDFormatChecker{})

	defContent, err := ae.GetEmbeddedFileContent("api-schema/definitions.all.json")

	if err != nil {
		panic(err.Error())
	}

	defLoader := gojsonschema.NewStringLoader(defContent)

	return ae.Embedded.WalkPrefix("api-schema/entities/", func(s string, f packr.File) error {
		if err != nil {
			panic(err.Error())
		}
		fileName := filepath.Base(s)

		fileNameParts := strings.Split(fileName, ".")

		if len(fileNameParts) < 3 {
			return fmt.Errorf("schema file name '%s' is invalid, ensture <entity.method>.<ext> format and packing", fileName)
		}

		entityName := fileNameParts[0]
		entityPropertyName := kebabToCamelCase(entityName)
		method := kebabToCamelCase(fileNameParts[1])

		schemaContent, err := ioutil.ReadAll(f)

		if err != nil {
			panic(err)
		}

		entitySchemaLoader := gojsonschema.NewStringLoader(string(schemaContent))

		schemaLoader := gojsonschema.NewSchemaLoader()

		if err := schemaLoader.AddSchemas(defLoader); err != nil {
			pfxlog.Logger().WithField("cause", err).Panic("error adding definitions schema")
		}

		schema, err := schemaLoader.Compile(entitySchemaLoader)

		if err != nil {
			pfxlog.Logger().WithField("cause", err).Panic("error compiling schema")
		}

		if hasProp, _ := reflections.HasField(ae.Schemes, entityPropertyName); !hasProp {
			return fmt.Errorf("found schema with property name '%s' from file '%s', no matching schema property", entityPropertyName, fileName)
		}
		entitySchema, err := reflections.GetField(ae.Schemes, entityPropertyName)

		if entitySchema == nil || reflect.ValueOf(entitySchema).IsNil() {
			if err := mirror.InitializeStructField(ae.Schemes, entityPropertyName); err != nil {
				return errors.Errorf("found schema with property name '%s', unable to initialized: %w", entityPropertyName, err)
			}
			entitySchema, err = reflections.GetField(ae.Schemes, entityPropertyName)
		}

		if err != nil {
			return err
		}

		err = reflections.SetField(entitySchema, method, schema)

		if err != nil {
			return fmt.Errorf("found schema with property name '%s', could not set methods '%s'", entityPropertyName, method)
		}

		return nil
	})
}

func kebabToCamelCase(kebab string) (camelCase string) {
	isToUpper := true
	for _, runeValue := range kebab {
		if isToUpper {
			camelCase += strings.ToUpper(string(runeValue))
			isToUpper = false
		} else {
			if runeValue == '-' {
				isToUpper = true
			} else {
				camelCase += string(runeValue)
			}
		}
	}
	return
}
