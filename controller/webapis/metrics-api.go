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

package webapis

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/identity"
	"github.com/openziti/xweb/v2"
	"github.com/openziti/ziti/controller/api_impl"
	"github.com/openziti/ziti/controller/network"
	"net/http"
	"os"
	"strings"
)

var _ xweb.ApiHandlerFactory = &MetricsApiFactory{}

type MetricsApiFactory struct {
	network *network.Network
	nodeId  identity.Identity
}

func (factory *MetricsApiFactory) Validate(_ *xweb.InstanceConfig) error {
	return nil
}

func NewMetricsApiFactory(nodeId identity.Identity, network *network.Network) *MetricsApiFactory {
	return &MetricsApiFactory{
		network: network,
		nodeId:  nodeId,
	}
}

func (factory *MetricsApiFactory) Binding() string {
	return api_impl.MetricApiBinding
}

func (factory *MetricsApiFactory) New(_ *xweb.ServerConfig, options map[interface{}]interface{}) (xweb.ApiHandler, error) {

	metricsApiHandler, err := NewMetricsApiHandler(factory.network, options)

	if err != nil {
		return nil, err
	}

	return metricsApiHandler, nil
}

func NewMetricsApiHandler(n *network.Network, options map[interface{}]interface{}) (*MetricsApiHandler, error) {
	metricsApi := &MetricsApiHandler{
		options:    options,
		network:    n,
		inspectMgr: network.NewInspectionsManager(n),
	}

	if value, found := options["scrapeCert"]; found {
		if f, ok := value.(string); ok {
			p, err := os.ReadFile(f)
			if nil != err {
				return nil, err
			}

			block, _ := pem.Decode(p)
			if block == nil {
				err := errors.New("failed to decode metrics api scrapeCert")
				return nil, err
			}
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				err := errors.New("failed to parse certificate: " + err.Error())
				return nil, err
			}
			metricsApi.scrapeCert = cert
		} else {
			return nil, errors.New("invalid configuration found for metrics pem.  The scrapeCert must be a string")
		}
	} else {
		pfxlog.Logger().Info("Metrics are enabled on /metrics, but no scrapeCert is provided in the controller configuration. Metrics are exposed without any authorization.")
	}

	includeTimestamps := false
	if value, found := options["includeTimestamps"]; found {
		if t, ok := value.(bool); ok {
			includeTimestamps = t
			pfxlog.Logger().Debugf("includeTimestamps set to %v in Prometheus metrics exporter", t)
		}
	}

	metricsApi.modelMapper = api_impl.NewMetricsModelMapper(n, "prometheus", includeTimestamps)
	metricsApi.handler = metricsApi.newHandler()

	return metricsApi, nil
}

type MetricsApiHandler struct {
	inspectMgr  *network.InspectionsManager
	handler     http.Handler
	network     *network.Network
	scrapeCert  *x509.Certificate
	modelMapper api_impl.MetricsModelMapper
	options     map[interface{}]interface{}
}

func (metricsApi *MetricsApiHandler) Binding() string {
	return api_impl.MetricApiBinding
}

func (metricsApi *MetricsApiHandler) Options() map[interface{}]interface{} {
	return metricsApi.options
}

func (metricsApi *MetricsApiHandler) RootPath() string {
	return "/metrics"
}

func (metricsApi *MetricsApiHandler) IsHandler(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, metricsApi.RootPath())
}

func (metricsApi *MetricsApiHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	metricsApi.handler.ServeHTTP(writer, request)
}

func (metricsApi *MetricsApiHandler) newHandler() http.Handler {
	handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Set Content-Type, see https://github.com/openziti/ziti/issues/2608
		rw.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		if nil != metricsApi.scrapeCert {
			certOk := false
			for _, r := range r.TLS.PeerCertificates {
				if bytes.Equal(metricsApi.scrapeCert.Signature, r.Signature) {
					certOk = true
				}
			}

			if !certOk {
				rw.WriteHeader(http.StatusUnauthorized)
				return
			}
		}

		inspection := metricsApi.inspectMgr.Inspect(".*", []string{"metrics:prometheus"})

		metricsResult, err := metricsApi.modelMapper.MapInspectResultToMetricsResult(inspection)

		if err != nil {
			_, _ = rw.Write([]byte(fmt.Sprintf("Failed to convert metrics to prometheus format %s:%s", metricsApi.network.GetAppId(), err.Error())))
			rw.WriteHeader(http.StatusInternalServerError)
		} else {
			if _, err = rw.Write([]byte(*metricsResult)); err != nil {
				rw.WriteHeader(http.StatusInternalServerError)
			}
		}
	})

	return handler
}
