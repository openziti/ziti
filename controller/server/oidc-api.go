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

package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/xweb/v2"
	"github.com/openziti/ziti/controller"
	"github.com/openziti/ziti/controller/api"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/oidc_auth"
)

var _ xweb.ApiHandlerFactory = &OidcApiFactory{}

type OidcApiFactory struct {
	InitFunc func(*OidcApiHandler) error
	appEnv   *env.AppEnv
}

func (factory OidcApiFactory) Validate(config *xweb.InstanceConfig) error {
	return nil
}

func NewOidcApiFactory(appEnv *env.AppEnv) *OidcApiFactory {
	return &OidcApiFactory{
		appEnv: appEnv,
	}
}

func (factory OidcApiFactory) Binding() string {
	return controller.OidcApiBinding
}

func (factory OidcApiFactory) New(serverConfig *xweb.ServerConfig, options map[interface{}]interface{}) (xweb.ApiHandler, error) {
	oidcApi, err := NewOidcApiHandler(serverConfig, factory.appEnv, options)

	if err != nil {
		return nil, err
	}

	if factory.InitFunc != nil {
		if err := factory.InitFunc(oidcApi); err != nil {
			return nil, fmt.Errorf("error running on init func: %v", err)
		}
	}

	return oidcApi, nil
}

type OidcApiHandler struct {
	handler http.Handler
	appEnv  *env.AppEnv
	options map[interface{}]interface{}
}

func (h OidcApiHandler) Binding() string {
	return controller.OidcApiBinding
}

func (h OidcApiHandler) Options() map[interface{}]interface{} {
	return h.options
}

func (h OidcApiHandler) RootPath() string {
	return "/oidc"
}

func (h OidcApiHandler) IsHandler(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, h.RootPath()) || strings.HasPrefix(r.URL.Path, oidc_auth.WellKnownOidcConfiguration)
}

func (h OidcApiHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	h.handler.ServeHTTP(writer, request)
}

func (h OidcApiHandler) IsDefault() bool {
	return true
}

func NewOidcApiHandler(serverConfig *xweb.ServerConfig, ae *env.AppEnv, options map[interface{}]interface{}) (*OidcApiHandler, error) {
	oidcApi := &OidcApiHandler{
		options: options,
		appEnv:  ae,
	}

	serverCert := serverConfig.Identity.ServerCert()

	cert := serverCert[0].Leaf
	key := serverCert[0].PrivateKey

	issuer := "https://" + ae.Config.Api.Address + "/oidc"
	oidcConfig := oidc_auth.NewConfig(issuer, cert, key)

	if secretVal, ok := options["secret"]; ok {
		if secret, ok := secretVal.(string); ok {
			secret = strings.TrimSpace(secret)
			if secret == "" {
				return nil, fmt.Errorf("[edge-oidc.options.secret] must not be empty")
			}

			oidcConfig.TokenSecret = secret
		} else {
			return nil, fmt.Errorf("[edge-oidc.options.secret] must be a string")
		}
	} else {
		return nil, fmt.Errorf("[edge-oidc.options.secret] must be defined")
	}

	if redirectVal, ok := options["redirectURIs"]; ok {
		if redirects, ok := redirectVal.([]interface{}); ok {
			for _, redirectVal := range redirects {
				if redirect, ok := redirectVal.(string); ok {
					oidcConfig.RedirectURIs = append(oidcConfig.RedirectURIs, redirect)
				}
			}
		}
	}

	if postLogoutVal, ok := options["postLogoutURIs"]; ok {
		if postLogs, ok := postLogoutVal.([]interface{}); ok {
			for _, postLogVal := range postLogs {
				if postLog, ok := postLogVal.(string); ok {
					oidcConfig.PostLogoutURIs = append(oidcConfig.PostLogoutURIs, postLog)
				}
			}
		}
	}

	if !stringz.Contains(oidcConfig.RedirectURIs, "openziti://auth/callback") {
		oidcConfig.RedirectURIs = append(oidcConfig.RedirectURIs, "openziti://auth/callback")
	}

	if !stringz.Contains(oidcConfig.PostLogoutURIs, "openziti://auth/logout") {
		oidcConfig.PostLogoutURIs = append(oidcConfig.PostLogoutURIs, "openziti://auth/logout")
	}

	var err error
	oidcApi.handler, err = oidc_auth.NewNativeOnlyOP(context.Background(), ae, oidcConfig)

	if err != nil {
		return nil, err
	}
	oidcApi.handler = api.WrapCorsHandler(oidcApi.handler)

	return oidcApi, nil
}
