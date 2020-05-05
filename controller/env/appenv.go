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

package env

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"fmt"
	jwt2 "github.com/dgrijalva/jwt-go"
	"github.com/go-openapi/loads"
	"github.com/go-openapi/runtime"
	openApiMiddleware "github.com/go-openapi/runtime/middleware"
	"github.com/gobuffalo/packr"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/apierror"
	edgeConfig "github.com/openziti/edge/controller/config"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/middleware"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/internal/cert"
	"github.com/openziti/edge/internal/jwt"
	"github.com/openziti/edge/migration"
	"github.com/openziti/edge/rest_server"
	"github.com/openziti/edge/rest_server/operations"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xctrl"
	"github.com/openziti/fabric/controller/xmgmt"
	"github.com/openziti/foundation/common/constants"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/sdk-golang/ziti/config"
	"github.com/xeipuuv/gojsonschema"
	"io"
	"io/ioutil"
	"net/http"
)

type AppEnv struct {
	BoltStores             *persistence.Stores
	Handlers               *model.Handlers
	Embedded               *packr.Box
	Config                 *edgeConfig.Config
	EnrollmentJwtGenerator jwt.EnrollmentGenerator
	Versions               *config.Versions
	AuthHeaderName         string
	AuthCookieName         string
	ApiServerCsrSigner     cert.Signer
	ApiClientCsrSigner     cert.Signer
	ControlClientCsrSigner cert.Signer
	FingerprintGenerator   cert.FingerprintGenerator
	ModelHandlers          *migration.ModelHandlers
	AuthRegistry           model.AuthRegistry
	EnrollRegistry         model.EnrollmentRegistry
	Broker                 *Broker
	HostController         HostController
	Api                    *operations.ZitiEdgeAPI
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

func (ae *AppEnv) GetHandlers() *model.Handlers {
	return ae.Handlers
}

func (ae *AppEnv) GetConfig() *edgeConfig.Config {
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
	Association             *BasicEntitySchema
	Authenticator           *BasicEntitySchema
	AuthenticatorSelf       *BasicEntitySchema
	Ca                      *BasicEntitySchema
	Config                  *BasicEntitySchema
	ConfigType              *BasicEntitySchema
	Enroller                *BasicEntitySchema
	EnrollEr                *BasicEntitySchema
	EnrollUpdb              *BasicEntitySchema
	EdgeRouter              *BasicEntitySchema
	EdgeRouterPolicy        *BasicEntitySchema
	TransitRouter           *BasicEntitySchema
	Identity                *IdentityEntitySchema
	Service                 *BasicEntitySchema
	ServiceEdgeRouterPolicy *BasicEntitySchema
	ServicePolicy           *BasicEntitySchema
	Session                 *BasicEntitySchema
	Terminator              *BasicEntitySchema
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

type authorizer struct {
}

func (a authorizer) Authorize(request *http.Request, principle interface{}) error {
	//principle is an API Session
	_, ok := principle.(*model.ApiSession)

	if !ok {
		pfxlog.Logger().Error("principle expected to be an ApiSession and was not")
		return apierror.NewUnauthorized()
	}

	rc, err := GetRequestContextFromHttpContext(request)

	if rc == nil || err != nil {
		pfxlog.Logger().WithError(err).Error("attempting to retrieve request context failed")
		return apierror.NewUnauthorized()
	}

	if rc.Identity == nil {
		return apierror.NewUnauthorized()
	}

	return nil
}

func (ae *AppEnv) FillRequestContext(rc *response.RequestContext) error {
	rc.SessionToken = ae.GetSessionTokenFromRequest(rc.Request)
	logger := pfxlog.Logger()
	_, err := uuid.Parse(rc.SessionToken)

	if err != nil {
		logger.WithError(err).Debug("failed to parse session id")
		rc.SessionToken = ""
	} else {
		logger.Tracef("authorizing request using session id '%v'", rc.SessionToken)
	}

	if rc.SessionToken != "" {
		rc.ApiSession, err = ae.GetHandlers().ApiSession.ReadByToken(rc.SessionToken)
		if err != nil {
			logger.WithError(err).Debugf("looking up API session for %s resulted in an error, request will continue unauthenticated", rc.SessionToken)
			rc.ApiSession = nil
			rc.SessionToken = ""
		}
	}

	//updates updatedAt for session timeouts
	if rc.ApiSession != nil {
		err := ae.GetHandlers().ApiSession.Update(rc.ApiSession)
		if err == nil {
			//re-read session to get new updatedAt
			rc.ApiSession, _ = ae.GetHandlers().ApiSession.Read(rc.ApiSession.Id)
		} else {
			logger.WithError(err).Errorf("could not update API session to extend timeout for token %s", rc.SessionToken)
		}
	}

	if rc.ApiSession != nil {
		rc.Identity, err = ae.GetHandlers().Identity.Read(rc.ApiSession.IdentityId)
		if err != nil {
			if boltz.IsErrNotFoundErr(err) {
				apiErr := apierror.NewUnauthorized()
				apiErr.Cause = fmt.Errorf("associated identity %s not found", rc.ApiSession.IdentityId)
				apiErr.AppendCause = true
				return apiErr
			} else {
				return err
			}
		}
	}

	if rc.Identity != nil {
		rc.ActivePermissions = append(rc.ActivePermissions, permissions.AuthenticatedPermission)

		if rc.Identity.IsAdmin {
			rc.ActivePermissions = append(rc.ActivePermissions, permissions.AdminPermission)
		}
	}
	return nil
}

type TextProducer struct{}

func (p TextProducer) Produce(writer io.Writer, i interface{}) error {
	if buffer, ok := i.([]byte); ok {
		_, err := writer.Write(buffer)
		return err
	} else if reader, ok := i.(io.Reader); ok {
		buffer, err := ioutil.ReadAll(reader)
		if err != nil {
			return err
		}
		_, err = writer.Write(buffer)
		return err
	} else if buffer, ok := i.(string); ok {
		_, err := writer.Write([]byte(buffer))
		return err
	} else if apiError, ok := i.(*apierror.ApiError); ok {
		output := fmt.Sprintf("CODE: %s\nMESSAGE: %s\nNote: cannot render additional information for this content-type", apiError.Code, apiError.Message)
		_, err := writer.Write([]byte(output))
		return err
	} else if _, ok := i.(error); ok {
		//don't attempt to render random error information, may leak sensitive information
		_, err := writer.Write([]byte("an error has occurred, more detail cannot be displayed in this content type"))
		return err
	} else {
		return fmt.Errorf("unsupported type for text producer: %T", i)
	}
}

func NewAppEnv(c *edgeConfig.Config) *AppEnv {
	swaggerSpec, err := loads.Embedded(rest_server.SwaggerJSON, rest_server.FlatSwaggerJSON)
	if err != nil {
		pfxlog.Logger().Fatalln(err)
	}
	api := operations.NewZitiEdgeAPI(swaggerSpec)
	api.ServeError = ServeError
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
		AuthCookieName: constants.ZitiSession,
		AuthHeaderName: constants.ZitiSession,
		ModelHandlers:  migration.GetModelHandlers(),
		AuthRegistry:   &model.AuthProcessorRegistryImpl{},
		EnrollRegistry: &model.EnrollmentRegistryImpl{},
		Api:            api,
	}

	api.APIAuthorizer = authorizer{}

	noOpConsumer := runtime.ConsumerFunc(func(reader io.Reader, data interface{}) error {
		return nil //do nothing
	})

	//enrollment consumer, leave content unread, allow modules to read
	api.ApplicationXPemFileConsumer = noOpConsumer
	api.ApplicationPkcs10Consumer = noOpConsumer

	api.ApplicationXPemFileProducer = &TextProducer{}
	api.ZtSessionAuth = func(token string) (principle interface{}, err error) {
		principle, err = ae.GetHandlers().ApiSession.ReadByToken(token)

		if err != nil {
			if !boltz.IsErrNotFoundErr(err) {
				pfxlog.Logger().WithError(err).Errorf("encountered error checking for session that was not expected; returning masking unauthorized response")
			}

			return nil, apierror.NewUnauthorized()
		}

		return principle, nil
	}

	sm := getJwtSigningMethod(c.Api.Identity.ServerCert())
	key := c.Api.Identity.ServerCert().PrivateKey

	ae.EnrollmentJwtGenerator = jwt.NewJwtIdentityEnrollmentGenerator(ae.Config.Api.Advertise, sm, key)

	ae.ApiClientCsrSigner = cert.NewClientSigner(ae.Config.Enrollment.SigningCert.Cert().Leaf, ae.Config.Enrollment.SigningCert.Cert().PrivateKey)
	ae.ApiServerCsrSigner = cert.NewServerSigner(ae.Config.Enrollment.SigningCert.Cert().Leaf, ae.Config.Enrollment.SigningCert.Cert().PrivateKey)
	ae.ControlClientCsrSigner = cert.NewClientSigner(ae.Config.Enrollment.SigningCert.Cert().Leaf, ae.Config.Enrollment.SigningCert.Cert().PrivateKey)

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
		err = persistence.RunMigrations(ae.GetDbProvider().GetDb(), ae.BoltStores, dbStores)
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

func (ae *AppEnv) GetSessionTokenFromRequest(r *http.Request) string {
	token := r.Header.Get(ae.AuthHeaderName)

	if token == "" {
		sessionCookie, _ := r.Cookie(ae.AuthCookieName)
		if sessionCookie != nil {
			token = sessionCookie.Value
		}
	}
	return token
}

func (ae *AppEnv) CreateRequestContext(rw http.ResponseWriter, r *http.Request) *response.RequestContext {
	rid := uuid.New()

	sw, ok := rw.(*middleware.StatusWriter)

	if ok {
		rid = sw.RequestId
	}

	body, _ := ioutil.ReadAll(r.Body)
	r.Body = ioutil.NopCloser(bytes.NewReader(body))

	requestContext := &response.RequestContext{
		Id:                rid,
		Body:              body,
		Identity:          nil,
		ApiSession:        nil,
		ActivePermissions: []string{},
		ResponseWriter:    rw,
		Request:           r,
		EventLogger:       &DefaultEventLogger{Ae: ae},
	}

	requestContext.Responder = response.NewResponder(requestContext)

	return requestContext
}

//use own type to avoid collisions
type ContextKey string

const EdgeContextKey = ContextKey("edgeContext")

func AddRequestContextToHttpContext(r *http.Request, rc *response.RequestContext) {
	ctx := context.WithValue(r.Context(), EdgeContextKey, rc)
	*r = *r.WithContext(ctx)
}

func GetRequestContextFromHttpContext(r *http.Request) (*response.RequestContext, error) {
	val := r.Context().Value(EdgeContextKey)
	if val == nil {
		return nil, fmt.Errorf("value for key %s no found in context", EdgeContextKey)
	}

	requestContext := val.(*response.RequestContext)

	if requestContext == nil {
		return nil, fmt.Errorf("value for key %s is not a request context", EdgeContextKey)
	}

	return requestContext, nil
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

func (ae *AppEnv) IsAllowed(responderFunc func(ae *AppEnv, rc *response.RequestContext), request *http.Request, entityId string, entitySubId string, permissions ...permissions.Resolver) openApiMiddleware.Responder {
	return openApiMiddleware.ResponderFunc(func(writer http.ResponseWriter, producer runtime.Producer) {

		rc, err := GetRequestContextFromHttpContext(request)

		if err != nil {
			pfxlog.Logger().WithError(err).Error("could not retrieve request context")
			response.RespondWithError(writer, uuid.New(), producer, err)
			return
		}

		rc.SetProducer(producer)
		rc.SetEntityId(entityId)
		rc.SetEntitySubId(entitySubId)

		for _, permission := range permissions {
			if !permission.IsAllowed(rc.ActivePermissions...) {
				rc.RespondWithApiError(apierror.NewUnauthorized())
				return
			}
		}

		responderFunc(ae, rc)
	})
}
