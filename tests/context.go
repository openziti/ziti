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

package tests

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	cryptoTls "crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/google/uuid"
	"github.com/openziti/edge/edge_common"
	"github.com/openziti/edge/eid"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/edge/router/enroll"
	"github.com/openziti/edge/router/fabric"
	"github.com/openziti/edge/router/xgress_edge"
	"github.com/openziti/edge/router/xgress_edge_tunnel"
	"github.com/openziti/fabric/controller/xt_smartrouting"
	"github.com/openziti/fabric/router"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/common"
	"github.com/openziti/foundation/identity/certtools"
	nfPem "github.com/openziti/foundation/util/pem"
	sdkConfig "github.com/openziti/sdk-golang/ziti/config"
	"github.com/openziti/sdk-golang/ziti/edge"
	sdkEnroll "github.com/openziti/sdk-golang/ziti/enroll"
	"github.com/pkg/errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/resty.v1"

	"github.com/Jeffail/gabs"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/server"
	"github.com/openziti/fabric/controller"
	idlib "github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/transport"
	"github.com/openziti/foundation/transport/quic"
	"github.com/openziti/foundation/transport/tcp"
	"github.com/openziti/foundation/transport/tls"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

const (
	ControllerConfFile         = "ats-ctrl.yml"
	EdgeRouterConfFile         = "ats-edge.router.yml"
	TunnelerEdgeRouterConfFile = "ats-edge-tunneler.router.yml"
	TransitRouterConfFile      = "ats-transit.router.yml"
)

func init() {
	pfxlog.GlobalInit(logrus.DebugLevel, pfxlog.DefaultOptions().SetTrimPrefix("github.com/openziti/").StartingToday())

	_ = os.Setenv("ZITI_TRACE_ENABLED", "false")

	transport.AddAddressParser(quic.AddressParser{})
	transport.AddAddressParser(tls.AddressParser{})
	transport.AddAddressParser(tcp.AddressParser{})
}

type TestContext struct {
	ApiHost                string
	AdminAuthenticator     *updbAuthenticator
	AdminManagementSession *session
	AdminClientSession     *session
	fabricController       *controller.Controller
	EdgeController         *server.Controller
	Req                    *require.Assertions
	clientApiClient        *resty.Client
	managementApiClient    *resty.Client
	enabledJsonLogging     bool

	edgeRouterEntity    *edgeRouter
	transitRouterEntity *transitRouter
	router              *router.Router
	testing             *testing.T
	LogLevel            string
	ControllerConfig    *controller.Config
}

var defaultTestContext = &TestContext{
	AdminAuthenticator: &updbAuthenticator{
		Username: eid.New(),
		Password: eid.New(),
	},
}

func NewTestContext(t *testing.T) *TestContext {
	ret := &TestContext{
		ApiHost: "127.0.0.1:1281",
		AdminAuthenticator: &updbAuthenticator{
			Username: eid.New(),
			Password: eid.New(),
		},
		LogLevel: os.Getenv("ZITI_TEST_LOG_LEVEL"),
		Req:      require.New(t),
	}
	ret.testContextChanged(t)

	return ret
}

func GetTestContext() *TestContext {
	return defaultTestContext
}

// testContextChanged is used to update the *testing.T reference used by library
// level tests. Necessary because using the wrong *testing.T will cause go test library
// errors.
func (ctx *TestContext) testContextChanged(t *testing.T) {
	ctx.testing = t
	ctx.Req = require.New(t)
}

func (ctx *TestContext) T() *testing.T {
	return ctx.testing
}

func (ctx *TestContext) NewTransport() *http.Transport {
	return ctx.NewTransportWithClientCert(nil, nil)
}

func (ctx *TestContext) NewTransportWithIdentity(i idlib.Identity) *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       i.ClientTLSConfig(),
	}
}

func (ctx *TestContext) NewTransportWithClientCert(cert *x509.Certificate, privateKey crypto.PrivateKey) *http.Transport {
	// #nosec
	tlsClientConfig := &cryptoTls.Config{
		InsecureSkipVerify: true,
	}

	if cert != nil && privateKey != nil {
		tlsClientConfig.Certificates = []cryptoTls.Certificate{
			{Certificate: [][]byte{cert.Raw}, PrivateKey: privateKey, Leaf: cert},
		}
	}

	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       tlsClientConfig,
	}
}

func (ctx *TestContext) NewHttpClient(transport *http.Transport) *http.Client {
	jar, err := cookiejar.New(&cookiejar.Options{})
	ctx.Req.NoError(err)

	return &http.Client{
		Transport:     transport,
		CheckRedirect: nil,
		Jar:           jar,
		Timeout:       2000 * time.Second,
	}
}

func (ctx *TestContext) NewRestClientWithDefaults() *resty.Client {
	return resty.NewWithClient(ctx.NewHttpClient(ctx.NewTransport()))
}

func (ctx *TestContext) NewRestClient(i idlib.Identity) *resty.Client {
	return resty.NewWithClient(ctx.NewHttpClient(ctx.NewTransportWithIdentity(i)))
}

func (ctx *TestContext) DefaultClientApiClient() *resty.Client {
	if ctx.clientApiClient == nil {
		ctx.clientApiClient, _, _ = ctx.NewClientComponents(EdgeClientApiPath)
		ctx.clientApiClient.AllowGetMethodPayload = true
	}
	return ctx.clientApiClient
}

func (ctx *TestContext) DefaultManagementApiClient() *resty.Client {
	if ctx.managementApiClient == nil {
		ctx.managementApiClient, _, _ = ctx.NewClientComponents(EdgeManagementApiPath)
		ctx.managementApiClient.AllowGetMethodPayload = true
	}
	return ctx.clientApiClient
}

func (ctx *TestContext) NewClientComponents(apiPath string) (*resty.Client, *http.Client, *http.Transport) {
	clientTransport := ctx.NewTransport()
	httpClient := ctx.NewHttpClient(clientTransport)
	client := resty.NewWithClient(httpClient)

	apiUrl, err := url.Parse("https://" + ctx.ApiHost)

	if err != nil {
		panic(err)
	}

	apiPathUrl, err := url.Parse(apiPath)

	if err != nil {
		panic(err)
	}

	baseUrl := apiUrl.ResolveReference(apiPathUrl)

	client.SetHostURL(baseUrl.String())

	return client, httpClient, clientTransport
}

func (ctx *TestContext) NewClientComponentsWithClientCert(cert *x509.Certificate, privateKey crypto.PrivateKey) (*resty.Client, *http.Client, *http.Transport) {
	clientTransport := ctx.NewTransportWithClientCert(cert, privateKey)
	httpClient := ctx.NewHttpClient(clientTransport)
	client := resty.NewWithClient(httpClient)

	client.SetHostURL("https://" + ctx.ApiHost)

	return client, httpClient, clientTransport

}

func (ctx *TestContext) StartServer() {
	ctx.StartServerFor("default", true)
}

func (ctx *TestContext) StartServerFor(test string, clean bool) {
	if ctx.LogLevel != "" {
		if level, err := logrus.ParseLevel(ctx.LogLevel); err == nil {
			logrus.StandardLogger().SetLevel(level)
		}
	}

	log := pfxlog.Logger()
	_ = os.Mkdir("testdata", os.FileMode(0755))
	if clean {
		err := filepath.Walk("testdata", func(path string, info os.FileInfo, err error) error {
			if err == nil {
				if !info.IsDir() && strings.HasPrefix(info.Name(), test+".db") {
					pfxlog.Logger().Infof("removing test bolt file or backup: %v", path)
					err = os.Remove(path)
				}
			}
			return err
		})
		ctx.Req.NoError(err)
	}

	err := os.Setenv("ZITI_TEST_DB", test)
	ctx.Req.NoError(err)

	log.Info("loading config")
	config, err := controller.LoadConfig(ControllerConfFile)
	ctx.Req.NoError(err)

	ctx.ControllerConfig = config

	log.Info("creating fabric controller")
	ctx.fabricController, err = controller.NewController(config, NewVersionProviderTest())
	ctx.Req.NoError(err)

	log.Info("creating edge controller")
	ctx.EdgeController, err = server.NewController(config, ctx.fabricController)
	ctx.Req.NoError(err)

	ctx.EdgeController.Initialize()

	err = ctx.EdgeController.AppEnv.Handlers.Identity.InitializeDefaultAdmin(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password, eid.New())
	if err != nil {
		log.WithError(err).Warn("error during initialize admin")
	}

	logrus.Infof("username: %v", ctx.AdminAuthenticator.Username)
	logrus.Infof("password: %v", ctx.AdminAuthenticator.Password)

	ctx.EdgeController.Run()
	go func() {
		err = ctx.fabricController.Run()
		ctx.Req.NoError(err)
	}()
	err = ctx.waitForCtrlPort(time.Minute * 5)
	ctx.Req.NoError(err)
}

func (ctx *TestContext) createAndEnrollEdgeRouter(tunneler bool, roleAttributes ...string) *edgeRouter {
	// If an edge router has already been created, delete it and create a new one
	if ctx.edgeRouterEntity != nil {
		ctx.AdminManagementSession.requireDeleteEntity(ctx.edgeRouterEntity)
		ctx.edgeRouterEntity = nil
	}

	_ = os.MkdirAll("testdata/edge-router", os.FileMode(0755))

	if tunneler {
		ctx.edgeRouterEntity = ctx.AdminManagementSession.requireNewTunnelerEnabledEdgeRouter(roleAttributes...)
	} else {
		ctx.edgeRouterEntity = ctx.AdminManagementSession.requireNewEdgeRouter(roleAttributes...)
	}
	jwt := ctx.AdminManagementSession.getEdgeRouterJwt(ctx.edgeRouterEntity.id)

	configFile := EdgeRouterConfFile
	if tunneler {
		configFile = TunnelerEdgeRouterConfFile
	}
	configMap, err := router.LoadConfigMap(configFile)
	ctx.Req.NoError(err)

	enroller := enroll.NewRestEnroller()
	ctx.Req.NoError(enroller.LoadConfig(configMap))
	var keyAlg sdkConfig.KeyAlgVar
	_ = keyAlg.Set("RSA")
	ctx.Req.NoError(enroller.Enroll([]byte(jwt), true, "", keyAlg))

	return ctx.edgeRouterEntity
}

func (ctx *TestContext) createAndEnrollTransitRouter() *transitRouter {
	// If a tx router has already been created, delete it and create a new one
	if ctx.transitRouterEntity != nil {
		ctx.AdminManagementSession.requireDeleteEntity(ctx.transitRouterEntity)
		ctx.transitRouterEntity = nil
	}

	_ = os.MkdirAll("testdata/transit-router", os.FileMode(0755))

	ctx.transitRouterEntity = ctx.AdminManagementSession.requireNewTransitRouter()
	jwt := ctx.AdminManagementSession.getTransitRouterJwt(ctx.transitRouterEntity.id)

	configMap, err := router.LoadConfigMap(TransitRouterConfFile)
	ctx.Req.NoError(err)

	enroller := enroll.NewRestEnroller()
	ctx.Req.NoError(enroller.LoadConfig(configMap))
	var keyAlg sdkConfig.KeyAlgVar
	_ = keyAlg.Set("RSA")
	ctx.Req.NoError(enroller.Enroll([]byte(jwt), true, "", keyAlg))

	return ctx.transitRouterEntity
}

func (ctx *TestContext) createEnrollAndStartTransitRouter() {
	ctx.createAndEnrollTransitRouter()
	ctx.startTransitRouter()
}

func (ctx *TestContext) startTransitRouter() {
	config, err := router.LoadConfig(TransitRouterConfFile)
	ctx.Req.NoError(err)
	ctx.router = router.Create(config, NewVersionProviderTest())

	ctx.Req.NoError(ctx.router.Start())
}

func (ctx *TestContext) CreateEnrollAndStartTunnelerEdgeRouter(roleAttributes ...string) {
	ctx.shutdownRouter()
	ctx.createAndEnrollEdgeRouter(true, roleAttributes...)
	ctx.startEdgeRouter()
}

func (ctx *TestContext) CreateEnrollAndStartEdgeRouter(roleAttributes ...string) {
	ctx.shutdownRouter()
	ctx.createAndEnrollEdgeRouter(false, roleAttributes...)
	ctx.startEdgeRouter()
}

func (ctx *TestContext) shutdownRouter() {
	if ctx.router != nil {
		ctx.Req.NoError(ctx.router.Shutdown())
		ctx.router = nil
	}
}

func (ctx *TestContext) startEdgeRouter() {
	configFile := EdgeRouterConfFile
	if ctx.edgeRouterEntity.isTunnelerEnabled {
		configFile = TunnelerEdgeRouterConfFile
	}
	config, err := router.LoadConfig(configFile)
	ctx.Req.NoError(err)
	ctx.router = router.Create(config, NewVersionProviderTest())

	stateManager := fabric.NewStateManager()
	xgressEdgeFactory := xgress_edge.NewFactory(config, NewVersionProviderTest(), stateManager, ctx.router.MetricsRegistry())
	xgress.GlobalRegistry().Register(edge_common.EdgeBinding, xgressEdgeFactory)

	xgressEdgeTunnelFactory := xgress_edge_tunnel.NewFactory(config, stateManager)
	xgress.GlobalRegistry().Register(edge_common.TunnelBinding, xgressEdgeTunnelFactory)

	ctx.Req.NoError(ctx.router.RegisterXctrl(xgressEdgeFactory))
	ctx.Req.NoError(ctx.router.RegisterXctrl(xgressEdgeTunnelFactory))
	ctx.Req.NoError(ctx.router.Start())
}

func (ctx *TestContext) EnrollIdentity(identityId string) *sdkConfig.Config {
	jwt := ctx.AdminManagementSession.getIdentityJwt(identityId)
	tkn, _, err := sdkEnroll.ParseToken(jwt)
	ctx.Req.NoError(err)

	flags := sdkEnroll.EnrollmentFlags{
		Token:  tkn,
		KeyAlg: "RSA",
	}
	conf, err := sdkEnroll.Enroll(flags)
	ctx.Req.NoError(err)
	return conf
}

func (ctx *TestContext) waitForCtrlPort(duration time.Duration) error {
	return ctx.waitForPort(ctx.ApiHost, duration)
}

func (ctx *TestContext) waitForPort(address string, duration time.Duration) error {
	now := time.Now()
	endTime := now.Add(duration)
	maxWait := duration
	for {
		conn, err := net.DialTimeout("tcp", address, maxWait)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		now = time.Now()
		if !now.Before(endTime) {
			return err
		}
		maxWait = endTime.Sub(now)
		time.Sleep(10 * time.Millisecond)
	}
}

func (ctx *TestContext) RequireAdminManagementApiLogin() {
	var err error
	ctx.AdminManagementSession, err = ctx.AdminAuthenticator.AuthenticateManagementApi(ctx)
	ctx.Req.NoError(err)
}

func (ctx *TestContext) RequireAdminClientApiLogin() {
	var err error
	ctx.AdminClientSession, err = ctx.AdminAuthenticator.AuthenticateClientApi(ctx)
	ctx.Req.NoError(err)
}

func (ctx *TestContext) Teardown() {
	pfxlog.Logger().Info("tearing down test context")
	ctx.shutdownRouter()
	if ctx.EdgeController != nil {
		ctx.EdgeController.Shutdown()
		ctx.EdgeController = nil
	}
	if ctx.fabricController != nil {
		ctx.fabricController.Shutdown()
		ctx.fabricController = nil
	}
}

func (ctx *TestContext) newAnonymousClientApiRequest() *resty.Request {
	return ctx.DefaultClientApiClient().R().
		SetHeader("content-type", "application/json")
}

func (ctx *TestContext) newAnonymousManagementApiRequest() *resty.Request {
	return ctx.DefaultClientApiClient().R().
		SetHeader("content-type", "application/json")
}

func (ctx *TestContext) newRequestWithClientCert(cert *x509.Certificate, privateKey crypto.PrivateKey) *resty.Request {
	client, _, _ := ctx.NewClientComponentsWithClientCert(cert, privateKey)

	return client.R().
		SetHeader("content-type", "application/json")
}

func (ctx *TestContext) completeUpdbEnrollment(identityId string, password string) {
	result := ctx.AdminManagementSession.requireQuery(fmt.Sprintf("identities/%v", identityId))
	path := result.Search(path("data.enrollment.updb.token")...)
	ctx.Req.NotNil(path)
	str, ok := path.Data().(string)
	ctx.Req.True(ok)

	enrollBody := gabs.New()
	ctx.setJsonValue(enrollBody, password, "password")

	resp, err := ctx.newAnonymousClientApiRequest().
		SetBody(enrollBody.String()).
		Post("enroll?token=" + str)
	ctx.Req.NoError(err)
	ctx.logJson(resp.Body())
	ctx.Req.Equal(http.StatusOK, resp.StatusCode())
}

func (ctx *TestContext) completeCaAutoEnrollment(certAuth *certAuthenticator) {
	trans := ctx.NewTransport()
	trans.TLSClientConfig.Certificates = []cryptoTls.Certificate{
		{
			Certificate: [][]byte{certAuth.cert.Raw},
			PrivateKey:  certAuth.key,
		},
	}
	client := resty.NewWithClient(ctx.NewHttpClient(trans))
	client.SetHostURL("https://" + ctx.ApiHost)

	resp, err := client.NewRequest().
		SetBody("{}").
		SetHeader("content-type", "application/x-pem-file").
		Post("enroll?method=ca")
	ctx.Req.NoError(err)
	ctx.logJson(resp.Body())
	ctx.Req.Equal(http.StatusOK, resp.StatusCode())
}

func (ctx *TestContext) completeCaAutoEnrollmentWithName(certAuth *certAuthenticator, name string) {
	trans := ctx.NewTransport()
	trans.TLSClientConfig.Certificates = []cryptoTls.Certificate{
		{
			Certificate: [][]byte{certAuth.cert.Raw},
			PrivateKey:  certAuth.key,
		},
	}
	client := resty.NewWithClient(ctx.NewHttpClient(trans))
	client.SetHostURL("https://" + ctx.ApiHost)

	body := gabs.New()
	_, _ = body.SetP(name, "name")

	resp, err := client.NewRequest().
		SetHeader("content-type", "application/json").
		SetBody(body.String()).
		Post("enroll?method=ca")
	ctx.Req.NoError(err)
	ctx.logJson(resp.Body())
	ctx.Req.Equal(http.StatusOK, resp.StatusCode())
}

func (ctx *TestContext) completeOttEnrollment(identityId string) *certAuthenticator {
	result := ctx.AdminManagementSession.requireQuery(fmt.Sprintf("identities/%v", identityId))

	tokenValue := result.Path("data.enrollment.ott.token")

	ctx.Req.NotNil(tokenValue)
	token, ok := tokenValue.Data().(string)
	ctx.Req.True(ok)

	privateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	ctx.Req.NoError(err)

	request, err := certtools.NewCertRequest(map[string]string{
		"C": "US", "O": "NetFoundry-API-Test", "CN": identityId,
	}, nil)

	csr, err := x509.CreateCertificateRequest(rand.Reader, request, privateKey)
	ctx.Req.NoError(err)

	csrPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})

	resp, err := ctx.newAnonymousClientApiRequest().
		SetBody(csrPem).
		SetHeader("content-type", "application/x-pem-file").
		SetHeader("accept", "application/json").
		Post("enroll?token=" + token)
	ctx.Req.NoError(err)
	ctx.logJson(resp.Body())
	ctx.Req.Equal(http.StatusOK, resp.StatusCode())

	envelope := &rest_model.EnrollmentCertsEnvelope{}

	err = json.Unmarshal(resp.Body(), envelope)
	ctx.Req.NoError(err)

	certs := nfPem.PemToX509(envelope.Data.Cert)

	ctx.Req.NotEmpty(certs)

	return &certAuthenticator{
		cert:    certs[0],
		key:     privateKey,
		certPem: envelope.Data.Cert,
	}
}

func (ctx *TestContext) validateDateFieldsForCreate(start time.Time, jsonEntity *gabs.Container) time.Time {
	// we lose a little time resolution, so if it's in the same millisecond, it's ok
	start = start.Add(-time.Millisecond)
	now := time.Now().Add(time.Millisecond)
	createdAt, updatedAt := ctx.getEntityDates(jsonEntity)
	ctx.Req.Equal(createdAt, updatedAt)

	ctx.Req.True(start.Before(createdAt) || start.Equal(createdAt), "%v should be before or equal to %v", start, createdAt)
	ctx.Req.True(now.After(createdAt) || now.Equal(createdAt), "%v should be after or equal to %v", now, createdAt)

	return createdAt
}

func (ctx *TestContext) newPostureCheckMFA(roleAttributes []string) *postureCheck {
	return &postureCheck{
		name:           eid.New(),
		typeId:         "MFA",
		roleAttributes: roleAttributes,
		tags:           nil,
	}
}

func (ctx *TestContext) newPostureCheckProcessMulti(semantic rest_model.Semantic, processes []*rest_model.ProcessMulti, roleAttributes []string) *rest_model.PostureCheckProcessMultiCreate {
	check := &rest_model.PostureCheckProcessMultiCreate{
		Processes: processes,
		Semantic:  &semantic,
	}

	attributes := rest_model.Attributes(roleAttributes)

	check.SetRoleAttributes(&attributes)

	name := uuid.New().String()
	check.SetName(&name)

	check.SetTypeID(rest_model.PostureCheckTypePROCESSMULTI)

	return check
}

func (ctx *TestContext) newPostureCheckDomain(domains []string, roleAttributes []string) *postureCheckDomain {
	return &postureCheckDomain{
		postureCheck: postureCheck{
			name:           eid.New(),
			typeId:         "DOMAIN",
			roleAttributes: roleAttributes,
			tags:           nil,
		},
		domains: domains,
	}
}

func (ctx *TestContext) newService(roleAttributes, configs []string) *service {
	return &service{
		Name:               eid.New(),
		terminatorStrategy: xt_smartrouting.Name,
		roleAttributes:     roleAttributes,
		configs:            configs,
		encryptionRequired: false,
		tags:               nil,
	}
}

func (ctx *TestContext) newTerminator(serviceId, routerId, binding, address string) *terminator {
	return &terminator{
		serviceId:  serviceId,
		routerId:   routerId,
		binding:    binding,
		address:    address,
		cost:       0,
		precedence: "default",
		tags:       nil,
	}
}

func (ctx *TestContext) newConfig(configType string, data map[string]interface{}) *Config {
	return &Config{
		Name:         eid.New(),
		ConfigTypeId: configType,
		Data:         data,
		Tags:         nil,
	}
}

func (ctx *TestContext) newConfigType() *configType {
	return &configType{
		Name: eid.New(),
		Tags: nil,
	}
}

func (ctx *TestContext) getEntityDates(jsonEntity *gabs.Container) (time.Time, time.Time) {
	createdAtStr := jsonEntity.S("createdAt").Data().(string)
	updatedAtStr := jsonEntity.S("updatedAt").Data().(string)

	ctx.Req.NotNil(createdAtStr)
	ctx.Req.NotNil(updatedAtStr)

	createdAt, err := time.Parse(time.RFC3339, createdAtStr)
	ctx.Req.NoError(err)
	updatedAt, err := time.Parse(time.RFC3339, updatedAtStr)
	ctx.Req.NoError(err)
	return createdAt, updatedAt
}

func (ctx *TestContext) validateDateFieldsForUpdate(start time.Time, origCreatedAt time.Time, jsonEntity *gabs.Container) time.Time {
	// we lose a little time resolution, so if it's in the same millisecond, it's ok
	start = start.Add(-time.Millisecond)
	now := time.Now().Add(time.Millisecond)
	createdAt, updatedAt := ctx.getEntityDates(jsonEntity)
	ctx.Req.Equal(origCreatedAt, createdAt)

	ctx.Req.True(createdAt.Before(updatedAt))
	ctx.Req.True(start.Before(updatedAt) || start.Equal(updatedAt))
	ctx.Req.True(now.After(updatedAt) || now.Equal(updatedAt))

	return createdAt
}

func (ctx *TestContext) validateEntity(entity entity, jsonEntity *gabs.Container) *gabs.Container {
	entity.validate(ctx, jsonEntity)
	return jsonEntity
}

func (ctx *TestContext) idsJson(ids ...string) *gabs.Container {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, ids, "ids")
	return entityData
}

func (ctx *TestContext) requireEntityNotEnrolled(name string, entity *gabs.Container) {
	fingerprint := entity.Path("fingerprint").Data()
	ctx.Req.Nil(fingerprint, "expected "+name+" with isVerified=false to have an empty fingerprint")

	token, ok := entity.Path("enrollmentToken").Data().(string)
	ctx.Req.True(ok, "expected "+name+" with isVerified=false to have an enrollment token, could not cast")
	ctx.Req.NotEmpty(token, "expected "+name+" with isVerified=false to have an enrollment token, was empty")

	jwt, ok := entity.Path("enrollmentJwt").Data().(string)
	ctx.Req.True(ok, "expected "+name+" with isVerified=false to have an enrollment jwt, could not cast")
	ctx.Req.NotEmpty(jwt, "expected "+name+" with isVerified=false to have an enrollment jwt, was empty")

	createdAtStr, ok := entity.Path("enrollmentCreatedAt").Data().(string)
	ctx.Req.True(ok, "expected "+name+" with isVerified=false to have an enrollment created at date, could not cast")
	ctx.Req.NotEmpty(createdAtStr, "expected "+name+" with isVerified=false to have an enrollment created at date string, was empty")

	createdAt, err := time.Parse(time.RFC3339, createdAtStr)
	ctx.Req.NoError(err, "expected "+name+" with isVerified=false to have a parsable created at date time string")
	ctx.Req.NotEmpty(createdAt, "expected "+name+" with isVerified=false to have an enrollment created at date, was empty")

	expiresAtStr, ok := entity.Path("enrollmentExpiresAt").Data().(string)
	ctx.Req.True(ok, "expected "+name+" with isVerified=false to have an enrollment expires at date, could not cast")
	ctx.Req.NotEmpty(expiresAtStr, "expected "+name+" with isVerified=false to have an enrollment expires at date string, was empty")

	expiresAt, err := time.Parse(time.RFC3339, expiresAtStr)
	ctx.Req.NoError(err, "expected "+name+" with isVerified=false to have a parsable expires at date time string")

	ctx.Req.True(ok, "expected "+name+" with isVerified=false to have an enrollment expires at date, could not cast")
	ctx.Req.NotEmpty(expiresAt, "expected "+name+" with isVerified=false to have an enrollment expires at date, was empty")

	ctx.Req.True(expiresAt.After(createdAt), "expected "+name+" with isVerified=false to have an enrollment expires at date after the created at date")
}

func (ctx *TestContext) requireEntityEnrolled(name string, entity *gabs.Container) {
	fingerprint, ok := entity.Path("fingerprint").Data().(string)
	ctx.Req.True(ok, "expected "+name+" with isVerified=true to have a fingerprint, could not cast")
	ctx.Req.NotEmpty(fingerprint, "expected "+name+" with isVerified=true to have a fingerprint, was empty")
	ctx.Req.False(strings.Contains(fingerprint, ":"), "fingerprint should not contain colons")
	ctx.Req.False(strings.ToLower(fingerprint) != fingerprint, "fingerprint should not contain uppercase characters")

	token := entity.Path("enrollmentToken").Data()
	ctx.Req.Nil(token, "expected "+name+" with isVerified=true to have an nil enrollment token")

	jwt := entity.Path("enrollmentJwt").Data()
	ctx.Req.Nil(jwt, "expected "+name+" with isVerified=true to have an nil enrollment jwt")

	createdAt := entity.Path("enrollmentCreatedAt").Data()
	ctx.Req.Nil(createdAt, "expected "+name+" with isVerified=true to have an nil enrollment created at date")

	expiresAt := entity.Path("enrollmentExpiresAt").Data()
	ctx.Req.Nil(expiresAt, "expected "+name+" with isVerified=true to have an nil enrollment expires at date")
}

func (ctx *TestContext) requireNListener(count int, l edge.Listener, timeout time.Duration) {
	sl, ok := l.(edge.SessionListener)
	ctx.Req.True(ok, "must be session listener")

	c := make(chan []edge.Listener, 5)
	sl.SetConnectionChangeHandler(func(conn []edge.Listener) {
		select {
		case c <- conn:
		default:
		}
	})

	t := time.After(timeout)

	for {
		select {
		case state := <-c:
			if len(state) >= count {
				return
			}
		case <-t:
			ctx.Req.Failf("timeout", "listener did not have %v connections within %v", count, timeout)
		}
	}
}

func (ctx *TestContext) WrapNetConn(conn edge.Conn, err error) *testConn {
	ctx.Req.NoError(err)
	return &testConn{
		Conn: conn,
		ctx:  ctx,
	}
}

func (ctx *TestContext) WrapConn(conn edge.Conn, err error) *testConn {
	ctx.Req.NoError(err)
	return &testConn{
		Conn: conn,
		ctx:  ctx,
	}
}

type testConn struct {
	edge.Conn
	ctx *TestContext
}

func (conn *testConn) WriteString(val string, timeout time.Duration) {
	conn.ctx.Req.NoError(conn.SetWriteDeadline(time.Now().Add(timeout)))
	defer func() { _ = conn.SetWriteDeadline(time.Time{}) }()

	buf := []byte(val)
	n, err := conn.Write(buf)
	conn.ctx.Req.NoError(err)
	conn.ctx.Req.Equal(n, len(buf))
}

func (conn *testConn) ReadString(maxSize int, timeout time.Duration) string {
	conn.ctx.Req.NoError(conn.SetReadDeadline(time.Now().Add(timeout)))
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	buf := make([]byte, maxSize)
	n, err := conn.Read(buf)
	conn.ctx.Req.NoError(err, "read timeout on connId=%v", conn.Id())
	return string(buf[:n])
}

func (conn *testConn) ReadExpected(expected string, timeout time.Duration) {
	val := conn.ReadString(len(expected)+1, timeout)
	conn.ctx.Req.Equal(expected, val, "read failure on connId=%v", conn.Id())
}

func (conn *testConn) RequireClose() {
	conn.ctx.Req.NoError(conn.Close())
}

var testServerCounter uint64

func newTestServer(listener edge.Listener, dispatcher func(conn *testServerConn) error) *testServer {
	idx := atomic.AddUint64(&testServerCounter, 1)
	return &testServer{
		idx:        idx,
		listener:   listener,
		errorC:     make(chan error, 10),
		msgCount:   0,
		dispatcher: dispatcher,
		waiter:     &sync.WaitGroup{},
	}
}

type testServer struct {
	idx        uint64
	listener   edge.Listener
	errorC     chan error
	msgCount   uint32
	dispatcher func(conn *testServerConn) error
	waiter     *sync.WaitGroup
	connIdGen  uint32
}

func (server *testServer) waitForDone(ctx *TestContext, timeout time.Duration) {
	select {
	case err, ok := <-server.errorC:
		if ok {
			ctx.Req.NoError(err)
		}
	case <-time.After(timeout):
		ctx.Req.Fail("wait for done on test server timed out")
	}
}

func (server *testServer) start() {
	go server.acceptLoop()
}

func (server *testServer) close() error {
	return server.listener.Close()
}

func (server *testServer) acceptLoop() {
	var err error
	for !server.listener.IsClosed() {
		var conn net.Conn
		conn, err = server.listener.Accept()
		if conn != nil {
			server.waiter.Add(1)
			connId := atomic.AddUint32(&server.connIdGen, 1)
			go server.dispatch(&testServerConn{id: connId, Conn: conn, server: server})
		} else {
			break
		}
	}

	// If listener is closed, assume this error is just letting us know the listener was closed
	if !server.listener.IsClosed() {
		if err != nil {
			server.errorC <- err
		}
	}

	waitDone := make(chan struct{})
	go func() {
		server.waiter.Wait()
		close(waitDone)
	}()

	select {
	case _, ok := <-waitDone:
		if !ok {
			pfxlog.Logger().Debugf("all connections closed")
		}
	case <-time.After(10 * time.Second):
		pfxlog.Logger().Warn("timed out waiting for all connections to close")
	}

	close(server.errorC)
	pfxlog.Logger().Debugf("%v: service exiting", server.idx)
}

func (server *testServer) dispatch(conn *testServerConn) {
	defer func() {
		server.waiter.Done()
	}()

	log := pfxlog.Logger()

	defer func() {
		val := recover()
		if val != nil {
			if err, ok := val.(error); ok {
				log.WithError(err).Error("panic from server.dispatch")
				server.errorC <- err
			}
		}
	}()

	defer func() {
		conn.RequireClose()
	}()

	log.Debugf("beginnging dispatch to conn %v-%v", conn.server.idx, conn.id)
	err := server.dispatcher(conn)
	log.Debugf("finished dispatch to conn %v-%v", conn.server.idx, conn.id)
	if err != nil {
		log.WithError(err).Error("failure from server.dispatch")
		server.errorC <- err
	}
}

type testServerConn struct {
	id uint32
	net.Conn
	server *testServer
}

func (conn *testServerConn) WriteString(val string, timeout time.Duration) {
	err := conn.SetWriteDeadline(time.Now().Add(timeout))
	if err != nil {
		panic(err)
	}
	defer func() { _ = conn.SetWriteDeadline(time.Time{}) }()

	buf := []byte(val)
	n, err := conn.Write(buf)
	if err != nil {
		panic(fmt.Errorf("conn %v-%v timed out trying to write string %v (%w)", conn.server.idx, conn.id, val, err))
	}
	if n != len(buf) {
		panic(errors.Errorf("conn %v-%v expected to write %v bytes, but only wrote %v", conn.server.idx, conn.id, len(buf), n))
	}
}

func (conn *testServerConn) ReadString(maxSize int, timeout time.Duration) (string, bool) {
	err := conn.SetReadDeadline(time.Now().Add(timeout))
	if err != nil {
		panic(err)
	}
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	buf := make([]byte, maxSize)
	n, err := conn.Read(buf)
	if err != nil {
		if err == io.EOF {
			return "", true
		}
		panic(fmt.Errorf("conn %v-%v timed out trying to read (%w)", conn.server.idx, conn.id, err))
	}
	return string(buf[:n]), false
}

func (conn *testServerConn) ReadExpected(expected string, timeout time.Duration) {
	val, eof := conn.ReadString(len(expected)+1, timeout)
	if eof {
		panic(errors.Errorf("expected to read string '%v', but got EOF", expected))
	}
	if val != expected {
		panic(errors.Errorf("expected to read string '%v', but got '%v'", expected, val))
	}
}

func (conn *testServerConn) RequireClose() {
	err := conn.Close()
	if err != nil {
		panic(err)
	}
}

type VersionProviderTest struct {
}

func (v VersionProviderTest) Branch() string {
	return "local"
}

func (v VersionProviderTest) EncoderDecoder() common.VersionEncDec {
	return &common.StdVersionEncDec
}

func (v VersionProviderTest) Version() string {
	return "v0.0.0"
}

func (v VersionProviderTest) BuildDate() string {
	return time.Now().String()
}

func (v VersionProviderTest) Revision() string {
	return ""
}

func (v VersionProviderTest) AsVersionInfo() *common.VersionInfo {
	return &common.VersionInfo{
		Version:   v.Version(),
		Revision:  v.Revision(),
		BuildDate: v.BuildDate(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

func NewVersionProviderTest() common.VersionProvider {
	return &VersionProviderTest{}
}
