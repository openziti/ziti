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

package apiproxy

import (
	"crypto/tls"
	"encoding/base64"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/router/internal/edgerouter"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

const (
	clientCertHeader       = "X-Client-CertPem"
	edgeRouterProxyRequest = "X-Edge-Router-Proxy-Request"
)

type Config struct {
	UpstreamUrl         *url.URL
	UpstreamTlsConfig   *tls.Config
	BindAddr            string
	DownstreamTlsConfig *tls.Config
}

func Start(config *edgerouter.Config) {

	if !config.ApiProxy.Enabled {
		pfxlog.Logger().Debug("API Proxy disabled")
		return
	}

	pfxlog.Logger().Info("API Proxy enabled")
	log := pfxlog.Logger()

	clientTlsConfig := config.RouterConfig.Id.ClientTLSConfig()

	serverTlsConfig := config.RouterConfig.Id.ServerTLSConfig()

	serverTlsConfig.ClientAuth = tls.RequestClientCert

	upstreamUrl := &url.URL{
		Scheme: "https",
		Host:   config.ApiProxy.Upstream,
	}

	apiProxyConfig := Config{
		DownstreamTlsConfig: serverTlsConfig,
		UpstreamTlsConfig:   clientTlsConfig,
		UpstreamUrl:         upstreamUrl,
		BindAddr:            config.ApiProxy.Listener,
	}

	apiClose := make(chan interface{})

	go Listen(apiProxyConfig, apiClose)

	for range apiClose {
		log.Warn("API server disrupted, attempting to restart")
		apiClose = make(chan interface{})
		go Listen(apiProxyConfig, apiClose)
	}
}

func Listen(c Config, cc chan interface{}) {
	defer close(cc)
	log := pfxlog.Logger()
	proxy := httputil.NewSingleHostReverseProxy(c.UpstreamUrl)
	proxy.Transport = &http.Transport{
		TLSClientConfig: c.UpstreamTlsConfig,
	}

	director := proxy.Director

	proxy.Director = func(req *http.Request) {
		log.WithField("method", req.Method).
			WithField("uri", req.RequestURI).
			Tracef("proxying API request method [%s], URL [%s]", req.Method, req.RequestURI)
		director(req)
		req.Header.Add(edgeRouterProxyRequest, time.Now().String())
		if req.TLS.PeerCertificates != nil {
			for i := range req.TLS.PeerCertificates {
				cert := base64.StdEncoding.EncodeToString(req.TLS.PeerCertificates[i].Raw)
				req.Header.Add(clientCertHeader, cert)
			}
		} else {
			req.Header.Del(clientCertHeader)
		}
	}

	server := &http.Server{
		Addr:      c.BindAddr,
		TLSConfig: c.DownstreamTlsConfig,
		Handler:   proxy,
	}

	log.WithField("address", c.BindAddr).
		Info("starting API proxy, binding to: ", c.BindAddr)
	log.WithField("targetAPI", c.UpstreamUrl.String()).
		Info("starting API proxy, targeting to: ", c.UpstreamUrl.String())

	err := server.ListenAndServeTLS("", "")
	if err != nil {
		log.WithField("cause", err).Fatal("API proxy server error: ", err)
	}
}
