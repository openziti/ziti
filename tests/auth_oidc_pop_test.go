//go:build apitests

package tests

import (
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_management_api_client/auth_policy"
	authenticator2 "github.com/openziti/edge-api/rest_management_api_client/authenticator"
	"github.com/openziti/edge-api/rest_management_api_client/external_jwt_signer"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	nfpem "github.com/openziti/foundation/v2/pem"
	idlib "github.com/openziti/identity"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/oidc_auth"
)

// updbCredsWithTlsCert is a test-only Credentials wrapper that performs UPDB
// authentication while presenting a separate client certificate at the TLS layer. It
// models a misconfiguration where an SDK has an OTT enrollment cert installed on its
// TLS context but authenticates via UPDB, so the cert is incidental to the auth method.
type updbCredsWithTlsCert struct {
	*edge_apis.UpdbCredentials
	tlsCerts []tls.Certificate
}

func (c *updbCredsWithTlsCert) TlsCerts() []tls.Certificate {
	return c.tlsCerts
}

// extJwtCredsWithFirstPartyCert is a test-only Credentials wrapper that authenticates
// with an external JWT bearer token while presenting a first-party (OTT-enrolled) client
// cert at the TLS layer. It models the C SDK's ext-jwt flow: the SDK has an enrollment
// cert installed on its TLS context and authenticates with a JWT issued by an external
// IdP. The wrapper implements `edge_apis.IdentityProvider` so the SDK uses the OTT cert
// for both controller mTLS and the edge router channel.
type extJwtCredsWithFirstPartyCert struct {
	*edge_apis.JwtCredentials
	certs []*x509.Certificate
	key   crypto.PrivateKey
}

func (c *extJwtCredsWithFirstPartyCert) TlsCerts() []tls.Certificate {
	rawCerts := make([][]byte, len(c.certs))
	for i, cert := range c.certs {
		rawCerts[i] = cert.Raw
	}
	return []tls.Certificate{{
		Certificate: rawCerts,
		PrivateKey:  c.key,
		Leaf:        c.certs[0],
	}}
}

func (c *extJwtCredsWithFirstPartyCert) GetIdentity() idlib.Identity {
	return idlib.NewClientTokenIdentityWithPool(c.certs, c.key, c.GetCaPool())
}

// Test_Authenticate_OIDC_PoP exercises OIDC authentication and proof-of-possession
// across the supported authentication methods. For each method, the test asserts the
// issued access token's `z_cfs` claim matches expectations, that a protected REST
// endpoint is reachable with that token, and that the SDK can attach to an edge router.
// UPDB and ext-jwt are exercised both with and without an incidental TLS client cert
// to verify that non-cert auth never binds the issued token to a cert that happened to
// be on the connection.
//
// Expected `z_cfs` matrix:
//
//	cert auth                  -> populated (cert is the auth method)
//	UPDB without TLS cert      -> empty
//	UPDB with TLS cert         -> empty (no incidental binding)
//	ext-jwt without TLS cert   -> empty
//	ext-jwt with TLS cert      -> empty (no incidental binding)
func Test_Authenticate_OIDC_PoP(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	managementHelper := ctx.NewEdgeManagementApi(nil)
	adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
	adminCreds.CaPool = ctx.ControllerCaPool()
	_, err := managementHelper.Authenticate(adminCreds, nil)
	ctx.Req.NoError(rest_util.WrapErr(err))
	ctx.RequireAdminManagementApiLogin()

	testService := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll("smartrouting")
	ctx.CreateEnrollAndStartEdgeRouter()

	signerCert, signerKey := newSelfSignedCert("Test Ext-Jwt Signer")
	audience := uuid.NewString()
	issuer := uuid.NewString()
	kid := uuid.NewString()

	createSigner := external_jwt_signer.NewCreateExternalJWTSignerParams()
	createSigner.ExternalJWTSigner = &rest_model.ExternalJWTSignerCreate{
		CertPem:  ToPtr(nfpem.EncodeToString(signerCert)),
		Enabled:  ToPtr(true),
		Name:     ToPtr("Test Ext-Jwt Signer"),
		Kid:      ToPtr(kid),
		Issuer:   ToPtr(issuer),
		Audience: ToPtr(audience),
	}
	signerCreateResp, err := managementHelper.API.ExternalJWTSigner.CreateExternalJWTSigner(createSigner, nil)
	ctx.Req.NoError(rest_util.WrapErr(err))
	signerId := signerCreateResp.Payload.Data.ID

	patchPolicy := auth_policy.NewPatchAuthPolicyParams()
	patchPolicy.ID = "default"
	patchPolicy.AuthPolicy = &rest_model.AuthPolicyPatch{
		Primary: &rest_model.AuthPolicyPrimaryPatch{
			ExtJWT: &rest_model.AuthPolicyPrimaryExtJWTPatch{
				Allowed:        ToPtr(true),
				AllowedSigners: []string{signerId},
			},
		},
	}
	_, err = managementHelper.API.AuthPolicy.PatchAuthPolicy(patchPolicy, nil)
	ctx.Req.NoError(rest_util.WrapErr(err))

	identityName := eid.New()
	identityPassword := eid.New()
	identityId, certAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(identityName, false)

	createUpdb := authenticator2.NewCreateAuthenticatorParams()
	createUpdb.Authenticator = &rest_model.AuthenticatorCreate{
		IdentityID: ToPtr(identityId),
		Method:     ToPtr("updb"),
		Password:   identityPassword,
		Username:   identityName,
	}
	_, err = managementHelper.API.Authenticator.CreateAuthenticator(createUpdb, nil)
	ctx.Req.NoError(rest_util.WrapErr(err))

	rawCerts := make([][]byte, len(certAuth.certs))
	for i, c := range certAuth.certs {
		rawCerts[i] = c.Raw
	}
	tlsCertFromOtt := []tls.Certificate{{
		Certificate: rawCerts,
		PrivateKey:  certAuth.key,
		Leaf:        certAuth.certs[0],
	}}

	t.Run("cert auth populates z_cfs", func(t *testing.T) {
		ctx.NextTest(t)
		creds := edge_apis.NewCertCredentials(certAuth.certs, certAuth.key)
		creds.CaPool = ctx.ControllerCaPool()

		clientContext, err := ziti.NewContext(&ziti.Config{
			ZtAPI:       "https://" + ctx.ApiHost + EdgeClientApiPath,
			Credentials: creds,
		})
		ctx.Req.NoError(err)
		defer clientContext.Close()
		ctx.Req.NoError(clientContext.Authenticate())

		ctxImpl, ok := clientContext.(*ziti.ContextImpl)
		ctx.Req.True(ok, "expected *ziti.ContextImpl from ziti.NewContext")
		oidcSession, ok := ctxImpl.CtrlClt.GetCurrentApiSession().(*edge_apis.ApiSessionOidc)
		ctx.Req.True(ok, "expected OIDC api session; SDK fell back to legacy, this test requires OIDC mode")
		claims := &common.AccessClaims{}
		_, _, err = jwt.NewParser().ParseUnverified(oidcSession.OidcTokens.AccessToken, claims)
		ctx.Req.NoError(err)
		ctx.Req.Contains(claims.AuthenticationMethodsReferences, oidc_auth.AuthMethodCert)
		ctx.Req.Len(claims.CertFingerprints, 1, "cert auth must bind exactly one (leaf) fingerprint into z_cfs")

		t.Run("can query a protected endpoint", func(t *testing.T) {
			ctx.NextTest(t)
			identityDetail, err := clientContext.GetCurrentIdentity()
			ctx.Req.NoError(err)
			ctx.Req.NotNil(identityDetail)
		})

		t.Run("can connect to an edge router", func(t *testing.T) {
			ctx.NextTest(t)
			connectChan := make(chan struct{}, 1)
			clientContext.Events().AddRouterConnectedListener(func(ziti.Context, string, string) {
				select {
				case connectChan <- struct{}{}:
				default:
				}
			})
			conn, _ := clientContext.Dial(testService.Name)
			defer func() {
				if conn != nil {
					_ = conn.Close()
				}
			}()

			select {
			case <-connectChan:
			case <-time.After(5 * time.Second):
				ctx.Fail("router connection did not occur within 5 seconds")
			}
		})
	})

	t.Run("cert auth with junk trailing certs binds only the leaf in z_cfs", func(t *testing.T) {
		ctx.NextTest(t)

		// Pad the cert chain with two unrelated self-signed certs at indices >0.
		// TLS client auth uses only the leaf at index 0, so the handshake still
		// succeeds; the junk certs are ignored as potential intermediates. The
		// server's PeerCertificates list nevertheless contains them, which is the
		// scenario this test exercises: z_cfs must not bind any of the trailing
		// junk certs.
		junk1, _ := newSelfSignedCert("junk-trailing-cert-1")
		junk2, _ := newSelfSignedCert("junk-trailing-cert-2")
		paddedCerts := append([]*x509.Certificate{}, certAuth.certs...)
		paddedCerts = append(paddedCerts, junk1, junk2)

		creds := edge_apis.NewCertCredentials(paddedCerts, certAuth.key)
		creds.CaPool = ctx.ControllerCaPool()

		clientContext, err := ziti.NewContext(&ziti.Config{
			ZtAPI:       "https://" + ctx.ApiHost + EdgeClientApiPath,
			Credentials: creds,
		})
		ctx.Req.NoError(err)
		defer clientContext.Close()
		ctx.Req.NoError(clientContext.Authenticate())

		ctxImpl, ok := clientContext.(*ziti.ContextImpl)
		ctx.Req.True(ok, "expected *ziti.ContextImpl from ziti.NewContext")
		oidcSession, ok := ctxImpl.CtrlClt.GetCurrentApiSession().(*edge_apis.ApiSessionOidc)
		ctx.Req.True(ok, "expected OIDC api session; SDK fell back to legacy, this test requires OIDC mode")
		claims := &common.AccessClaims{}
		_, _, err = jwt.NewParser().ParseUnverified(oidcSession.OidcTokens.AccessToken, claims)
		ctx.Req.NoError(err)
		ctx.Req.Contains(claims.AuthenticationMethodsReferences, oidc_auth.AuthMethodCert)
		ctx.Req.Len(claims.CertFingerprints, 1, "junk trailing certs must not appear in z_cfs; only the leaf should bind")
	})

	t.Run("UPDB auth without TLS cert leaves z_cfs empty", func(t *testing.T) {
		ctx.NextTest(t)
		creds := edge_apis.NewUpdbCredentials(identityName, identityPassword)
		creds.CaPool = ctx.ControllerCaPool()

		clientContext, err := ziti.NewContext(&ziti.Config{
			ZtAPI:       "https://" + ctx.ApiHost + EdgeClientApiPath,
			Credentials: creds,
		})
		ctx.Req.NoError(err)
		defer clientContext.Close()
		ctx.Req.NoError(clientContext.Authenticate())

		ctxImpl, ok := clientContext.(*ziti.ContextImpl)
		ctx.Req.True(ok, "expected *ziti.ContextImpl from ziti.NewContext")
		oidcSession, ok := ctxImpl.CtrlClt.GetCurrentApiSession().(*edge_apis.ApiSessionOidc)
		ctx.Req.True(ok, "expected OIDC api session; SDK fell back to legacy, this test requires OIDC mode")
		claims := &common.AccessClaims{}
		_, _, err = jwt.NewParser().ParseUnverified(oidcSession.OidcTokens.AccessToken, claims)
		ctx.Req.NoError(err)
		ctx.Req.Contains(claims.AuthenticationMethodsReferences, oidc_auth.AuthMethodPassword)
		ctx.Req.Empty(claims.CertFingerprints, "UPDB auth without TLS cert must leave z_cfs empty")

		t.Run("can query a protected endpoint", func(t *testing.T) {
			ctx.NextTest(t)
			identityDetail, err := clientContext.GetCurrentIdentity()
			ctx.Req.NoError(err)
			ctx.Req.NotNil(identityDetail)
		})

		t.Run("can connect to an edge router", func(t *testing.T) {
			ctx.NextTest(t)
			connectChan := make(chan struct{}, 1)
			clientContext.Events().AddRouterConnectedListener(func(ziti.Context, string, string) {
				select {
				case connectChan <- struct{}{}:
				default:
				}
			})
			conn, _ := clientContext.Dial(testService.Name)
			defer func() {
				if conn != nil {
					_ = conn.Close()
				}
			}()

			select {
			case <-connectChan:
			case <-time.After(5 * time.Second):
				ctx.Fail("router connection did not occur within 5 seconds")
			}
		})
	})

	t.Run("UPDB auth with incidental TLS cert leaves z_cfs empty", func(t *testing.T) {
		ctx.NextTest(t)
		baseCreds := edge_apis.NewUpdbCredentials(identityName, identityPassword)
		baseCreds.CaPool = ctx.ControllerCaPool()
		creds := &updbCredsWithTlsCert{
			UpdbCredentials: baseCreds,
			tlsCerts:        tlsCertFromOtt,
		}

		clientContext, err := ziti.NewContext(&ziti.Config{
			ZtAPI:       "https://" + ctx.ApiHost + EdgeClientApiPath,
			Credentials: creds,
		})
		ctx.Req.NoError(err)
		defer clientContext.Close()
		ctx.Req.NoError(clientContext.Authenticate())

		ctxImpl, ok := clientContext.(*ziti.ContextImpl)
		ctx.Req.True(ok, "expected *ziti.ContextImpl from ziti.NewContext")
		oidcSession, ok := ctxImpl.CtrlClt.GetCurrentApiSession().(*edge_apis.ApiSessionOidc)
		ctx.Req.True(ok, "expected OIDC api session; SDK fell back to legacy, this test requires OIDC mode")
		claims := &common.AccessClaims{}
		_, _, err = jwt.NewParser().ParseUnverified(oidcSession.OidcTokens.AccessToken, claims)
		ctx.Req.NoError(err)
		ctx.Req.Contains(claims.AuthenticationMethodsReferences, oidc_auth.AuthMethodPassword)
		ctx.Req.Empty(claims.CertFingerprints, "UPDB auth must not bind incidental TLS cert")

		t.Run("can query a protected endpoint", func(t *testing.T) {
			ctx.NextTest(t)
			identityDetail, err := clientContext.GetCurrentIdentity()
			ctx.Req.NoError(err)
			ctx.Req.NotNil(identityDetail)
		})

		t.Run("can connect to an edge router", func(t *testing.T) {
			ctx.NextTest(t)
			connectChan := make(chan struct{}, 1)
			clientContext.Events().AddRouterConnectedListener(func(ziti.Context, string, string) {
				select {
				case connectChan <- struct{}{}:
				default:
				}
			})
			conn, _ := clientContext.Dial(testService.Name)
			defer func() {
				if conn != nil {
					_ = conn.Close()
				}
			}()

			select {
			case <-connectChan:
			case <-time.After(5 * time.Second):
				ctx.Fail("router connection did not occur within 5 seconds")
			}
		})
	})

	t.Run("ext-jwt auth without TLS cert leaves z_cfs empty", func(t *testing.T) {
		ctx.NextTest(t)

		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = jwt.RegisteredClaims{
			Audience:  []string{audience},
			ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
			ID:        uuid.NewString(),
			IssuedAt:  &jwt.NumericDate{Time: time.Now()},
			Issuer:    issuer,
			NotBefore: &jwt.NumericDate{Time: time.Now()},
			Subject:   identityId,
		}
		jwtToken.Header["kid"] = kid
		jwtStr, err := jwtToken.SignedString(signerKey)
		ctx.Req.NoError(err)

		creds := edge_apis.NewJwtCredentials(jwtStr)
		creds.CaPool = ctx.ControllerCaPool()

		clientContext, err := ziti.NewContext(&ziti.Config{
			ZtAPI:       "https://" + ctx.ApiHost + EdgeClientApiPath,
			Credentials: creds,
		})
		ctx.Req.NoError(err)
		defer clientContext.Close()
		ctx.Req.NoError(clientContext.Authenticate())

		ctxImpl, ok := clientContext.(*ziti.ContextImpl)
		ctx.Req.True(ok, "expected *ziti.ContextImpl from ziti.NewContext")
		oidcSession, ok := ctxImpl.CtrlClt.GetCurrentApiSession().(*edge_apis.ApiSessionOidc)
		ctx.Req.True(ok, "expected OIDC api session; SDK fell back to legacy, this test requires OIDC mode")
		claims := &common.AccessClaims{}
		_, _, err = jwt.NewParser().ParseUnverified(oidcSession.OidcTokens.AccessToken, claims)
		ctx.Req.NoError(err)
		ctx.Req.Contains(claims.AuthenticationMethodsReferences, oidc_auth.AuthMethodExtJwt)
		ctx.Req.Empty(claims.CertFingerprints, "ext-jwt auth without TLS cert must leave z_cfs empty")

		t.Run("can query a protected endpoint", func(t *testing.T) {
			ctx.NextTest(t)
			identityDetail, err := clientContext.GetCurrentIdentity()
			ctx.Req.NoError(err)
			ctx.Req.NotNil(identityDetail)
		})

		t.Run("can connect to an edge router", func(t *testing.T) {
			ctx.NextTest(t)
			connectChan := make(chan struct{}, 1)
			clientContext.Events().AddRouterConnectedListener(func(ziti.Context, string, string) {
				select {
				case connectChan <- struct{}{}:
				default:
				}
			})
			conn, _ := clientContext.Dial(testService.Name)
			defer func() {
				if conn != nil {
					_ = conn.Close()
				}
			}()

			select {
			case <-connectChan:
			case <-time.After(5 * time.Second):
				ctx.Fail("router connection did not occur within 5 seconds")
			}
		})
	})

	t.Run("ext-jwt auth with first-party cert leaves z_cfs empty", func(t *testing.T) {
		ctx.NextTest(t)

		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = jwt.RegisteredClaims{
			Audience:  []string{audience},
			ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
			ID:        uuid.NewString(),
			IssuedAt:  &jwt.NumericDate{Time: time.Now()},
			Issuer:    issuer,
			NotBefore: &jwt.NumericDate{Time: time.Now()},
			Subject:   identityId,
		}
		jwtToken.Header["kid"] = kid
		jwtStr, err := jwtToken.SignedString(signerKey)
		ctx.Req.NoError(err)

		baseCreds := edge_apis.NewJwtCredentials(jwtStr)
		baseCreds.CaPool = ctx.ControllerCaPool()
		creds := &extJwtCredsWithFirstPartyCert{
			JwtCredentials: baseCreds,
			certs:          certAuth.certs,
			key:            certAuth.key,
		}

		clientContext, err := ziti.NewContext(&ziti.Config{
			ZtAPI:       "https://" + ctx.ApiHost + EdgeClientApiPath,
			Credentials: creds,
		})
		ctx.Req.NoError(err)
		defer clientContext.Close()
		ctx.Req.NoError(clientContext.Authenticate())

		ctxImpl, ok := clientContext.(*ziti.ContextImpl)
		ctx.Req.True(ok, "expected *ziti.ContextImpl from ziti.NewContext")
		oidcSession, ok := ctxImpl.CtrlClt.GetCurrentApiSession().(*edge_apis.ApiSessionOidc)
		ctx.Req.True(ok, "expected OIDC api session; SDK fell back to legacy, this test requires OIDC mode")
		claims := &common.AccessClaims{}
		_, _, err = jwt.NewParser().ParseUnverified(oidcSession.OidcTokens.AccessToken, claims)
		ctx.Req.NoError(err)
		ctx.Req.Contains(claims.AuthenticationMethodsReferences, oidc_auth.AuthMethodExtJwt)
		ctx.Req.Empty(claims.CertFingerprints, "ext-jwt auth must not bind incidental first-party cert")

		t.Run("can query a protected endpoint", func(t *testing.T) {
			ctx.NextTest(t)
			identityDetail, err := clientContext.GetCurrentIdentity()
			ctx.Req.NoError(err)
			ctx.Req.NotNil(identityDetail)
		})

		t.Run("can connect to an edge router", func(t *testing.T) {
			ctx.NextTest(t)
			connectChan := make(chan struct{}, 1)
			clientContext.Events().AddRouterConnectedListener(func(ziti.Context, string, string) {
				select {
				case connectChan <- struct{}{}:
				default:
				}
			})
			conn, _ := clientContext.Dial(testService.Name)
			defer func() {
				if conn != nil {
					_ = conn.Close()
				}
			}()

			select {
			case <-connectChan:
			case <-time.After(5 * time.Second):
				ctx.Fail("router connection did not occur within 5 seconds")
			}
		})
	})
}
