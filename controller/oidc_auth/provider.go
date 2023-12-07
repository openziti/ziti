package oidc_auth

import (
	"context"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/model"
	"github.com/pkg/errors"
	"github.com/zitadel/oidc/v2/pkg/op"
	"golang.org/x/text/language"
	"net/http"
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

	DefaultNativeClientId = "native"
)

// NewNativeOnlyOP creates an OIDC Provider that allows native clients and only the AutCode PKCE flow.
func NewNativeOnlyOP(ctx context.Context, env model.Env, config Config) (http.Handler, error) {
	cert, kid, method := env.GetServerCert()
	config.Storage = NewStorage(kid, cert.Leaf.PublicKey, cert.PrivateKey, method, &config, env)

	oidcHandler, err := newHttpRouter(ctx, config)

	nativeClient := NativeClient(DefaultNativeClientId, config.RedirectURIs, config.PostLogoutURIs)
	nativeClient.idTokenDuration = config.IdTokenDuration
	nativeClient.loginURL = newLoginResolver(config.Storage)
	config.Storage.AddClient(nativeClient)

	if err != nil {
		return nil, err
	}

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		r := request.WithContext(context.WithValue(request.Context(), contextKeyHttpRequest, request))

		oidcHandler.ServeHTTP(writer, r)
	}), nil

}

// newHttpRouter creates an OIDC HTTP router
func newHttpRouter(ctx context.Context, config Config) (*mux.Router, error) {
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

	provider, err := newOidcProvider(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("could not create OpenIdProvider: %w", err)
	}

	loginRouter := newLogin(config.Storage, op.AuthCallbackURL(provider), op.NewIssuerInterceptor(provider.IssuerFromRequest))

	router.Handle("/oidc/"+WellKnownOidcConfiguration, http.StripPrefix("/oidc", provider.HttpHandler()))
	router.Handle(WellKnownOidcConfiguration, provider.HttpHandler())

	router.PathPrefix("/oidc/login").Handler(http.StripPrefix("/oidc/login", loginRouter.router))

	router.PathPrefix("/oidc").Handler(http.StripPrefix("/oidc", provider.HttpHandler()))

	return router, nil
}

// newOidcProvider will create an OpenID Provider that allows refresh tokens, authentication via form post and basic auth, and support request object params
func newOidcProvider(_ context.Context, oidcConfig Config) (op.OpenIDProvider, error) {
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

	handler, err := op.NewOpenIDProvider(oidcConfig.Issuer, config, oidcConfig.Storage)

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
