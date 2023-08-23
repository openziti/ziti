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

package tests

import (
	"context"
	"crypto/sha1"
	tls2 "crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/openziti/fabric/controller/api_impl"
	"github.com/openziti/fabric/controller/rest_client"
	restClientRouter "github.com/openziti/fabric/controller/rest_client/router"
	"github.com/openziti/fabric/controller/rest_model"
	"github.com/openziti/fabric/controller/rest_util"
	"github.com/openziti/fabric/router"
	"github.com/openziti/foundation/v2/util"
	"github.com/openziti/identity"
	"github.com/openziti/identity/certtools"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller"
	"github.com/openziti/transport/v2"
	"github.com/openziti/transport/v2/tcp"
	"github.com/openziti/transport/v2/tls"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

const (
	FabricControllerConfFile = "./testdata/config/ctrl.yml"
	FabricRouterConfFile     = "./testdata/config/router-%v.yml"
)

func init() {
	logOptions := pfxlog.DefaultOptions().
		SetTrimPrefix("github.com/openziti/").
		StartingToday()

	pfxlog.GlobalInit(logrus.InfoLevel, logOptions)
	pfxlog.SetFormatter(pfxlog.NewFormatter(logOptions))

	_ = os.Setenv("ZITI_TRACE_ENABLED", "false")

	transport.AddAddressParser(tls.AddressParser{})
	transport.AddAddressParser(tcp.AddressParser{})
}

type FabricTestContext struct {
	fabricController *controller.Controller
	Req              *require.Assertions

	routers          []*router.Router
	testing          *testing.T
	LogLevel         string
	ControllerConfig *controller.Config
}

func NewFabricTestContext(t *testing.T) *FabricTestContext {
	ret := &FabricTestContext{
		LogLevel: os.Getenv("ZITI_TEST_LOG_LEVEL"),
		Req:      require.New(t),
	}
	ret.testContextChanged(t)

	return ret
}

// testContextChanged is used to update the *testing.T reference used by library
// level tests. Necessary because using the wrong *testing.T will cause go test library
// errors.
func (ctx *FabricTestContext) testContextChanged(t *testing.T) {
	ctx.testing = t
	ctx.Req = require.New(t)
}

func (ctx *FabricTestContext) T() *testing.T {
	return ctx.testing
}

func (ctx *FabricTestContext) NewTransport(i identity.Identity) *http.Transport {
	var tlsClientConfig *tls2.Config

	if i != nil {
		tlsClientConfig = i.ClientTLSConfig()
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

func (ctx *FabricTestContext) NewHttpClient(transport *http.Transport) *http.Client {
	jar, err := cookiejar.New(&cookiejar.Options{})
	ctx.Req.NoError(err)

	return &http.Client{
		Transport:     transport,
		CheckRedirect: nil,
		Jar:           jar,
		Timeout:       2000 * time.Second,
	}
}

func (ctx *FabricTestContext) NewRestClient(i identity.Identity) *resty.Client {
	return resty.NewWithClient(ctx.NewHttpClient(ctx.NewTransport(i)))
}

func (ctx *FabricTestContext) NewRestClientWithDefaults() *resty.Client {
	id, err := identity.LoadClientIdentity(
		"./testdata/valid_client_cert/client.cert",
		"./testdata/valid_client_cert/client.key",
		"./testdata/ca/intermediate/certs/ca-chain.cert.pem")
	ctx.Req.NoError(err)
	return resty.NewWithClient(ctx.NewHttpClient(ctx.NewTransport(id)))
}

func (ctx *FabricTestContext) StartServer() {
	ctx.StartServerFor("default", true)
}

func (ctx *FabricTestContext) StartServerFor(test string, clean bool) {
	api_impl.OverrideRequestWrapper(nil) // clear possible wrapper from another test
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
	config, err := controller.LoadConfig(FabricControllerConfFile)
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

	ctx.requireRestPort(time.Second * 5)
}

func (ctx *FabricTestContext) startRouter(index uint8) *router.Router {
	config, err := router.LoadConfig(fmt.Sprintf(FabricRouterConfFile, index))
	ctx.Req.NoError(err)
	r := router.Create(config, NewVersionProviderTest())
	ctx.Req.NoError(r.Start())

	ctx.routers = append(ctx.routers, r)
	return r
}

func (ctx *FabricTestContext) shutdownRouters() {
	for _, r := range ctx.routers {
		ctx.Req.NoError(r.Shutdown())
	}
	ctx.routers = nil
}

func (ctx *FabricTestContext) waitForCtrlPort(duration time.Duration) error {
	return ctx.waitForPort("127.0.0.1:6363", duration)
}

func (ctx *FabricTestContext) requireRestPort(duration time.Duration) {
	err := ctx.waitForPort("127.0.0.1:1281", duration)
	ctx.Req.NoError(err)
}

func (ctx *FabricTestContext) waitForPort(address string, duration time.Duration) error {
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

func (ctx *FabricTestContext) waitForPortClose(address string, duration time.Duration) error {
	now := time.Now()
	endTime := now.Add(duration)
	maxWait := duration
	for {
		conn, err := net.DialTimeout("tcp", address, maxWait)
		if err != nil {
			return nil
		}
		_ = conn.Close()
		now = time.Now()
		if !now.Before(endTime) {
			return err
		}
		maxWait = endTime.Sub(now)
		time.Sleep(10 * time.Millisecond)
	}
}

func (ctx *FabricTestContext) Teardown() {
	pfxlog.Logger().Info("tearing down test context")
	ctx.shutdownRouters()
	if ctx.fabricController != nil {
		ctx.fabricController.Shutdown()
		ctx.fabricController = nil
	}
}

func (ctx *FabricTestContext) createFabricRestClient() *rest_client.ZitiFabric {
	id, err := identity.LoadClientIdentity(
		"./testdata/valid_client_cert/client.cert",
		"./testdata/valid_client_cert/client.key",
		"./testdata/ca/intermediate/certs/ca-chain.cert.pem")
	ctx.Req.NoError(err)

	client, err := rest_util.NewFabricClientWithIdentity(id, "https://localhost:1281/")
	ctx.Req.NoError(err)
	return client
}

func (ctx *FabricTestContext) createTestFabricRestClient() *RestClient {
	client := ctx.createFabricRestClient()
	return &RestClient{
		FabricTestContext: ctx,
		client:            client,
	}
}

type RestClient struct {
	*FabricTestContext
	client *rest_client.ZitiFabric
}

func (self *RestClient) EnrollRouter(id string, name string, certFile string) {
	cert, err := certtools.LoadCertFromFile(certFile)
	self.Req.NoError(err)

	fingerprint := fmt.Sprintf("%x", sha1.Sum(cert[0].Raw))

	timeoutContext, cancelF := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelF()

	createRouterParams := &restClientRouter.CreateRouterParams{
		Router: &rest_model.RouterCreate{
			Cost:        util.Ptr(int64(0)),
			Fingerprint: &fingerprint,
			ID:          &id,
			Name:        &name,
			NoTraversal: util.Ptr(false),
		},
		Context: timeoutContext,
	}
	_, err = self.client.Router.CreateRouter(createRouterParams)
	if err != nil {
		js, _ := json.MarshalIndent(err, "", "    ")
		fmt.Println(string(js))
	}
	self.Req.NoError(err)
}
