// +build apitests

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

package tests

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	cryptoTls "crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/netfoundry/ziti-foundation/identity/certtools"
	nfpem "github.com/netfoundry/ziti-foundation/util/pem"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"testing"
	"time"

	"gopkg.in/resty.v1"

	"github.com/Jeffail/gabs"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/server"
	"github.com/netfoundry/ziti-fabric/controller"
	"github.com/netfoundry/ziti-foundation/transport"
	"github.com/netfoundry/ziti-foundation/transport/quic"
	"github.com/netfoundry/ziti-foundation/transport/tcp"
	"github.com/netfoundry/ziti-foundation/transport/tls"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func init() {
	pfxlog.Global(logrus.DebugLevel)
	pfxlog.SetPrefix("bitbucket.org/netfoundry/")
	logrus.SetFormatter(pfxlog.NewFormatterStartingToday())

	transport.AddAddressParser(quic.AddressParser{})
	transport.AddAddressParser(tls.AddressParser{})
	transport.AddAddressParser(tcp.AddressParser{})
}

type TestContext struct {
	ApiHost            string
	AdminAuthenticator *updbAuthenticator
	AdminSession       *session
	fabricController   *controller.Controller
	EdgeController     *server.Controller
	req                *require.Assertions
	client             *resty.Client
	enabledJsonLogging bool
}

var defaultTestContext = &TestContext{
	AdminAuthenticator: &updbAuthenticator{
		Username: uuid.New().String(),
		Password: uuid.New().String(),
	},
}

func NewTestContext(t *testing.T) *TestContext {
	return &TestContext{
		ApiHost: "127.0.0.1:1281",
		AdminAuthenticator: &updbAuthenticator{
			Username: uuid.New().String(),
			Password: uuid.New().String(),
		},
		req: require.New(t),
	}
}

func GetTestContext() *TestContext {
	return defaultTestContext
}

func (ctx *TestContext) testContextChanged(t *testing.T) {
	ctx.req = require.New(t)
}

func (ctx *TestContext) Transport() *http.Transport {
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
		TLSClientConfig: &cryptoTls.Config{
			InsecureSkipVerify: true,
		},
	}
}

func (ctx *TestContext) HttpClient(transport *http.Transport) *http.Client {
	jar, err := cookiejar.New(&cookiejar.Options{})
	ctx.req.NoError(err)

	return &http.Client{
		Transport:     transport,
		CheckRedirect: nil,
		Jar:           jar,
		Timeout:       2000 * time.Second,
	}
}

func (ctx *TestContext) Client(httpClient *http.Client) *resty.Client {
	client := resty.NewWithClient(httpClient)
	return client
}

func (ctx *TestContext) NewClient() *resty.Client {
	return ctx.Client(ctx.HttpClient(ctx.Transport()))
}

func (ctx *TestContext) DefaultClient() *resty.Client {
	if ctx.client == nil {
		ctx.client, _, _ = ctx.NewClientComponents()
	}
	return ctx.client
}

func (ctx *TestContext) NewClientComponents() (*resty.Client, *http.Client, *http.Transport) {
	transport := ctx.Transport()
	httpClient := ctx.HttpClient(transport)
	client := ctx.Client(httpClient)

	client.SetHostURL("https://" + ctx.ApiHost)

	return client, httpClient, transport

}

func (ctx *TestContext) startServer() {
	log := pfxlog.Logger()
	_ = os.Mkdir("testdata", os.FileMode(0755))
	_ = os.Remove("testdata/ctrl.db")

	log.Info("loading config")
	config, err := controller.LoadConfig("ats-ctrl.yml")
	ctx.req.NoError(err)

	log.Info("creating fabric controller")
	ctx.fabricController, err = controller.NewController(config)
	ctx.req.NoError(err)

	log.Info("creating edge controller")
	ctx.EdgeController, err = server.NewController(config)
	ctx.req.NoError(err)

	ctx.EdgeController.SetHostController(ctx.fabricController)

	ctx.EdgeController.Initialize()

	err = ctx.EdgeController.AppEnv.Handlers.Identity.InitializeDefaultAdmin(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password, uuid.New().String())
	if err != nil {
		log.WithError(err).Warn("error during initialize admin")
	}

	// Note we're not starting the fabric controller. Shouldn't need any of it for testing the edge API
	ctx.EdgeController.Run()
	go func() {
		err = ctx.fabricController.Run()
		ctx.req.NoError(err)
	}()
	err = ctx.waitForPort(time.Minute * 5)
	ctx.req.NoError(err)
}

func (ctx *TestContext) waitForPort(duration time.Duration) error {
	now := time.Now()
	endTime := now.Add(duration)
	maxWait := duration
	for {
		conn, err := net.DialTimeout("tcp", ctx.ApiHost, maxWait)
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

func (ctx *TestContext) unauthenticatedSession() *session {
	return &session{
		testContext:   ctx,
		token:         "",
		authenticator: nil,
		identityId:    "",
	}
}

func (ctx *TestContext) loginWithCert(cert *x509.Certificate, key *crypto.PrivateKey) (*session, error) {
	return (&certAuthenticator{
		cert: cert,
		key:  key,
	}).Authenticate(ctx)
}

func (ctx *TestContext) requireAdminLogin() {
	var err error
	ctx.AdminSession, err = ctx.AdminAuthenticator.Authenticate(ctx)
	ctx.req.NoError(err)
}

func (ctx *TestContext) requireLogin(username, password string) *session {
	session, err := ctx.login(username, password)
	ctx.req.NoError(err)
	return session
}

func (ctx *TestContext) login(username, password string) (*session, error) {
	return (&updbAuthenticator{
		Username: username,
		Password: password,
	}).Authenticate(ctx)
}

func (ctx *TestContext) teardown() {
	pfxlog.Logger().Info("tearing down test context")
	ctx.EdgeController.Shutdown()
	ctx.fabricController.Shutdown()
}

func (ctx *TestContext) newRequest() *resty.Request {
	return ctx.DefaultClient().R().
		SetHeader("Content-Type", "application/json")
}

func (ctx *TestContext) completeUpdbEnrollment(identityId string, password string) {
	result := ctx.AdminSession.requireQuery(fmt.Sprintf("identities/%v", identityId))
	path := result.Search(path("data.enrollment.updb.token")...)
	ctx.req.NotNil(path)
	str, ok := path.Data().(string)
	ctx.req.True(ok)

	enrollBody := gabs.New()
	ctx.setJsonValue(enrollBody, password, "password")

	resp, err := ctx.newRequest().
		SetBody(enrollBody.String()).
		Post("enroll?token=" + str)
	ctx.req.NoError(err)
	ctx.logJson(resp.Body())
	ctx.req.Equal(http.StatusOK, resp.StatusCode())
}

func (ctx *TestContext) completeOttEnrollment(identityId string) *certAuthenticator {
	result := ctx.AdminSession.requireQuery(fmt.Sprintf("identities/%v", identityId))

	tokenValue := result.Path("data.enrollment.ott.token")

	ctx.req.NotNil(tokenValue)
	token, ok := tokenValue.Data().(string)
	ctx.req.True(ok)

	privateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	ctx.req.NoError(err)

	request, err := certtools.NewCertRequest(map[string]string{
		"C": "US", "O": "Netfoundry-APi-Test", "CN": identityId,
	}, nil)

	csr, err := x509.CreateCertificateRequest(rand.Reader, request, privateKey)
	ctx.req.NoError(err)

	csrPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})

	resp, err := ctx.newRequest().
		SetBody(csrPem).
		SetHeader("content-type", "application/x-pem-file").
		Post("enroll?token=" + token)
	ctx.req.NoError(err)
	ctx.logJson(resp.Body())
	ctx.req.Equal(http.StatusOK, resp.StatusCode())

	certs := nfpem.PemToX509(string(resp.Body()))

	ctx.req.NotEmpty(certs)

	return &certAuthenticator{
		cert: certs[0],
		key:  privateKey,
	}
}

func (ctx *TestContext) validateDateFieldsForCreate(start time.Time, jsonEntity *gabs.Container) time.Time {
	now := time.Now()
	createdAt, updatedAt := ctx.getEntityDates(jsonEntity)
	ctx.req.Equal(createdAt, updatedAt)

	ctx.req.True(start.Before(createdAt) || start.Equal(createdAt))
	ctx.req.True(now.After(createdAt) || now.Equal(createdAt))

	return createdAt
}

func (ctx *TestContext) newTestService(roleAttributes, configs []string) *service {
	return &service{
		name:            uuid.New().String(),
		egressRouter:    uuid.New().String(),
		endpointAddress: uuid.New().String(),
		roleAttributes:  roleAttributes,
		configs:         configs,
		tags:            nil,
	}
}

func (ctx *TestContext) newTestConfig(configType string, data map[string]interface{}) *config {
	return &config{
		name:       uuid.New().String(),
		configType: configType,
		data:       data,
		tags:       nil,
	}
}

func (ctx *TestContext) newTestConfigType() *configType {
	return &configType{
		name: uuid.New().String(),
		tags: nil,
	}
}

func (ctx *TestContext) getEntityDates(jsonEntity *gabs.Container) (time.Time, time.Time) {
	createdAtStr := jsonEntity.S("createdAt").Data().(string)
	updatedAtStr := jsonEntity.S("updatedAt").Data().(string)

	ctx.req.NotNil(createdAtStr)
	ctx.req.NotNil(updatedAtStr)

	createdAt, err := time.Parse(time.RFC3339, createdAtStr)
	ctx.req.NoError(err)
	updatedAt, err := time.Parse(time.RFC3339, updatedAtStr)
	ctx.req.NoError(err)
	return createdAt, updatedAt
}

func (ctx *TestContext) validateDateFieldsForUpdate(start time.Time, origCreatedAt time.Time, jsonEntity *gabs.Container) time.Time {
	now := time.Now()
	createdAt, updatedAt := ctx.getEntityDates(jsonEntity)
	ctx.req.Equal(origCreatedAt, createdAt)

	ctx.req.True(createdAt.Before(updatedAt))
	ctx.req.True(start.Before(updatedAt) || start.Equal(updatedAt))
	ctx.req.True(now.After(updatedAt) || now.Equal(updatedAt))

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
