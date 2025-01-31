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

package oidc_auth

import (
	"context"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/model"
	"github.com/pkg/errors"
	"github.com/zitadel/oidc/v2/pkg/op"
	"golang.org/x/text/language"
	"net/http"
	"strings"
)

const (
	pathLoggedOut              = "/oidc/logged-out"
	WellKnownOidcConfiguration = "/.well-known/openid-configuration"

	SourceTypeOidc = "oidc_auth"

	AuthMethodPassword = model.AuthMethodPassword
	AuthMethodExtJwt   = model.AuthMethodExtJwt
	AuthMethodCert     = db.MethodAuthenticatorCert

	AuthMethodSecondaryTotp   = "totp"
	AuthMethodSecondaryExtJwt = "ejs"
)

func createIssuerSpecificOidcProvider(ctx context.Context, issuer string, config Config) (http.Handler, error) {
	issuerUrl := "https://" + issuer
	provider, err := newOidcProvider(ctx, issuerUrl, config)
	if err != nil {
		return nil, fmt.Errorf("could not create OpenIdProvider: %w", err)
	}

	oidcHandler, err := newHttpRouter(provider, config)

	if err != nil {
		return nil, err
	}

	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		r := request.WithContext(context.WithValue(request.Context(), contextKeyHttpRequest, request))
		r = request.WithContext(context.WithValue(r.Context(), contextKeyTokenState, &TokenState{}))
		r = request.WithContext(op.ContextWithIssuer(r.Context(), issuerUrl))

		oidcHandler.ServeHTTP(writer, r)
	})

	return handler, nil
}

// NewNativeOnlyOP creates an OIDC Provider that allows native clients and only the AuthCode PKCE flow.
func NewNativeOnlyOP(ctx context.Context, env model.Env, config Config) (http.Handler, error) {
	cert, kid, method := env.GetServerCert()
	config.Storage = NewStorage(kid, cert.Leaf.PublicKey, cert.PrivateKey, method, &config, env)

	openzitiClient := NativeClient(common.ClaimClientIdOpenZiti, config.RedirectURIs, config.PostLogoutURIs)
	openzitiClient.idTokenDuration = config.IdTokenDuration
	openzitiClient.loginURL = newLoginResolver(config.Storage)
	config.Storage.AddClient(openzitiClient)

	//backwards compatibility client w/ early HA SDKs. Should be removed by the time HA is GA'ed.
	nativeClient := NativeClient(common.ClaimLegacyNative, config.RedirectURIs, config.PostLogoutURIs)
	nativeClient.idTokenDuration = config.IdTokenDuration
	nativeClient.loginURL = newLoginResolver(config.Storage)
	config.Storage.AddClient(nativeClient)

	handlers := map[string]http.Handler{}

	for _, issuer := range config.Issuers {
		oidcIssuer := issuer + "/oidc"

		handler, err := createIssuerSpecificOidcProvider(ctx, oidcIssuer, config)
		if err != nil {
			return nil, err
		}

		handlers[oidcIssuer] = handler
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issuer := r.Host
		if strings.HasPrefix(r.URL.Path, "/oidc") {
			issuer = r.Host + "/oidc"
		}

		handler, ok := handlers[issuer]

		if !ok {
			http.NotFound(w, r)
			return
		}

		handler.ServeHTTP(w, r)
	}), nil

}

// newHttpRouter creates an OIDC HTTP router
func newHttpRouter(provider op.OpenIDProvider, config Config) (*mux.Router, error) {
	if config.TokenSecret == "" {
		return nil, errors.New("token secret must not be empty")
	}

	router := mux.NewRouter()

	router.HandleFunc(pathLoggedOut, func(w http.ResponseWriter, req *http.Request) {
		_, err := w.Write([]byte("signed out successfully"))
		if err != nil {
			pfxlog.Logger().Errorf("error serving logged out page: %v", err)
		}
	})

	loginRouter := newLogin(config.Storage, op.AuthCallbackURL(provider), op.NewIssuerInterceptor(provider.IssuerFromRequest))

	router.Handle("/oidc/"+WellKnownOidcConfiguration, http.StripPrefix("/oidc", provider.HttpHandler()))
	router.Handle(WellKnownOidcConfiguration, provider.HttpHandler())

	router.PathPrefix("/oidc/login").Handler(http.StripPrefix("/oidc/login", loginRouter.router))

	router.PathPrefix("/oidc").Handler(http.StripPrefix("/oidc", provider.HttpHandler()))

	return router, nil
}

// newOidcProvider will create an OpenID Provider that allows refresh tokens, authentication via form post and basic auth, and support request object params
func newOidcProvider(_ context.Context, issuer string, oidcConfig Config) (op.OpenIDProvider, error) {
	config := &op.Config{
		CryptoKey:                oidcConfig.Secret(),
		DefaultLogoutRedirectURI: pathLoggedOut,
		CodeMethodS256:           true,
		AuthMethodPost:           true,
		AuthMethodPrivateKeyJWT:  true,
		GrantTypeRefreshToken:    true,
		RequestObjectSupported:   true,
		SupportedUILocales:       []language.Tag{language.English},
	}

	handler, err := op.NewOpenIDProvider(issuer, config, oidcConfig.Storage)

	if err != nil {
		return nil, err
	}
	return handler, nil
}

// newLoginResolver returns func capable of determining default login URLs based on authId
func newLoginResolver(storage Storage) func(string) string {
	return func(authId string) string {
		authRequest, err := storage.GetAuthRequest(authId)

		if err != nil || authRequest == nil {
			return passwordLoginUrl + authId
		}

		switch authRequest.RequestedMethod {
		case AuthMethodPassword:
			return passwordLoginUrl + authId
		case AuthMethodExtJwt:
			return extJwtLoginUrl + authId
		case AuthMethodCert:
			return certLoginUrl + authId
		}

		if len(authRequest.PeerCerts) > 0 {
			return certLoginUrl + authId
		}

		return passwordLoginUrl + authId
	}
}
