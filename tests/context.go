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
	cryptoTls "crypto/tls"
	"crypto/x509"
	"github.com/go-resty/resty/v2"
	"github.com/openziti/fabric/router"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jeffail/gabs/v2"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller"
	"github.com/openziti/foundation/transport"
	"github.com/openziti/foundation/transport/quic"
	"github.com/openziti/foundation/transport/tcp"
	"github.com/openziti/foundation/transport/tls"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

const (
	ControllerConfFile    = "ctrl.yml"
	TransitRouterConfFile = "router.yml"
)

func init() {
	logOptions := pfxlog.DefaultOptions().
		SetTrimPrefix("github.com/openziti/").
		StartingToday()

	pfxlog.GlobalInit(logrus.DebugLevel, logOptions)
	pfxlog.SetFormatter(pfxlog.NewFormatter(logOptions))

	_ = os.Setenv("ZITI_TRACE_ENABLED", "false")

	transport.AddAddressParser(quic.AddressParser{})
	transport.AddAddressParser(tls.AddressParser{})
	transport.AddAddressParser(tcp.AddressParser{})
}

type TestContext struct {
	fabricController    *controller.Controller
	Req                 *require.Assertions
	managementApiClient *resty.Client
	enabledJsonLogging  bool

	router           *router.Router
	testing          *testing.T
	LogLevel         string
	ControllerConfig *controller.Config
}

func NewTestContext(t *testing.T) *TestContext {
	ret := &TestContext{
		LogLevel: os.Getenv("ZITI_TEST_LOG_LEVEL"),
		Req:      require.New(t),
	}
	ret.testContextChanged(t)

	return ret
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

	go func() {
		err = ctx.fabricController.Run()
		ctx.Req.NoError(err)
	}()
	err = ctx.waitForCtrlPort(time.Second * 5)
	ctx.Req.NoError(err)
}

func (ctx *TestContext) startTransitRouter() {
	config, err := router.LoadConfig(TransitRouterConfFile)
	ctx.Req.NoError(err)
	ctx.router = router.Create(config, NewVersionProviderTest())

	ctx.Req.NoError(ctx.router.Start())
}

func (ctx *TestContext) shutdownRouter() {
	if ctx.router != nil {
		ctx.Req.NoError(ctx.router.Shutdown())
		ctx.router = nil
	}
}

func (ctx *TestContext) waitForCtrlPort(duration time.Duration) error {
	return ctx.waitForPort("127.0.0.1:6363", duration)
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

func (ctx *TestContext) Teardown() {
	pfxlog.Logger().Info("tearing down test context")
	ctx.shutdownRouter()
	if ctx.fabricController != nil {
		ctx.fabricController.Shutdown()
		ctx.fabricController = nil
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
