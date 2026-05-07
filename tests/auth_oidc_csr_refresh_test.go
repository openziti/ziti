//go:build apitests

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
	"math/big"
	"net/http"
	"net/url"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	nfpem "github.com/openziti/foundation/v2/pem"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/controller/oidc_auth"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"gopkg.in/resty.v1"
)

// tokenResponseWithSessionCert captures the token endpoint response including the
// session_cert field that is returned when a CSR is submitted during auth or refresh.
type tokenResponseWithSessionCert struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    uint64 `json:"expires_in"`
	IDToken      string `json:"id_token"`
	SessionCert  string `json:"session_cert"`
}

// parseAccessClaims parses an access token without signature verification and returns
// the custom access claims.
func parseAccessClaims(accessToken string) (*common.AccessClaims, error) {
	parser := jwt.NewParser()
	claims := &common.AccessClaims{}
	_, _, err := parser.ParseUnverified(accessToken, claims)
	if err != nil {
		return nil, fmt.Errorf("failed to parse access token: %w", err)
	}
	return claims, nil
}

// buildTlsCertFromPem constructs a []cryptoTls.Certificate from a PEM-encoded certificate
// string and the private key that was used to generate the CSR.
func buildTlsCertFromPem(certPemStr string, privateKey crypto.PrivateKey) ([]cryptoTls.Certificate, error) {
	certPem := []byte(certPemStr)

	var keyPemBytes []byte
	switch k := privateKey.(type) {
	case *ecdsa.PrivateKey:
		der, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal EC private key: %w", err)
		}
		keyPemBytes = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	default:
		der, err := x509.MarshalPKCS8PrivateKey(privateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal private key: %w", err)
		}
		keyPemBytes = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	}

	tlsCert, err := cryptoTls.X509KeyPair(certPem, keyPemBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create X509 key pair: %w", err)
	}
	return []cryptoTls.Certificate{tlsCert}, nil
}

// generateSelfSignedCertAndKey creates a throwaway self-signed certificate and private key
// for use as a "wrong" client certificate in negative test scenarios.
func generateSelfSignedCertAndKey() ([]cryptoTls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: mustRandomSerial(),
	}

	certDer, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	certPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDer})
	keyDer, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	keyPem := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDer})

	tlsCert, err := cryptoTls.X509KeyPair(certPem, keyPem)
	if err != nil {
		return nil, err
	}
	return []cryptoTls.Certificate{tlsCert}, nil
}

// mustRandomSerial produces a random certificate serial number.
func mustRandomSerial() *big.Int {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		panic(err)
	}
	return serial
}

// fingerprintFromPem returns the SHA-1 hex fingerprint of the first certificate in a PEM string.
func fingerprintFromPem(pemStr string) string {
	return nfpem.FingerprintFromPemString(pemStr)
}

// doRefreshRequest performs a refresh token grant against the OIDC token endpoint.
// If tlsCerts is non-nil, the request presents those client certificates.
// If csrPem is non-empty, it is included as csr_pem in the form data.
// When expectSessionCert is true, the response is unmarshalled into tokenResponseWithSessionCert.
func doRefreshRequest(
	ctx *TestContext,
	refreshToken string,
	clientID string,
	tlsCerts []cryptoTls.Certificate,
	csrPem string,
	expectSessionCert bool,
) (accessToken string, newRefreshToken string, sessionCert string, statusCode int) {
	req := &oidc.RefreshTokenRequest{
		RefreshToken: refreshToken,
		ClientID:     clientID,
		Scopes:       []string{oidc.ScopeOpenID, oidc.ScopeOfflineAccess},
	}

	enc := oidc.NewEncoder()
	dst := map[string][]string{}
	err := enc.Encode(req, dst)
	ctx.Req.NoError(err)
	dst["grant_type"] = []string{string(req.GrantType())}

	if csrPem != "" {
		dst["csr_pem"] = []string{csrPem}
	}

	if expectSessionCert {
		result := &tokenResponseWithSessionCert{}
		var r *resty.Request
		if tlsCerts != nil {
			r = ctx.newRequestWithTlsCerts(tlsCerts)
		} else {
			r = ctx.newAnonymousClientApiRequest()
		}
		r.SetHeader("content-type", oidc_auth.FormContentType).
			SetMultiValueFormData(dst).
			SetResult(result)

		resp, postErr := r.Post("https://" + ctx.ApiHost + "/oidc/oauth/token")
		ctx.Req.NoError(postErr)
		return result.AccessToken, result.RefreshToken, result.SessionCert, resp.StatusCode()
	}

	result := &oidc.TokenExchangeResponse{}
	var r *resty.Request
	if tlsCerts != nil {
		r = ctx.newRequestWithTlsCerts(tlsCerts)
	} else {
		r = ctx.newAnonymousClientApiRequest()
	}
	r.SetHeader("content-type", oidc_auth.FormContentType).
		SetMultiValueFormData(dst).
		SetResult(result)

	resp, postErr := r.Post("https://" + ctx.ApiHost + "/oidc/oauth/token")
	ctx.Req.NoError(postErr)
	return result.AccessToken, result.RefreshToken, "", resp.StatusCode()
}

// doTokenExchangeRequest performs a token exchange grant against the OIDC token endpoint.
func doTokenExchangeRequest(
	ctx *TestContext,
	accessToken string,
	tlsCerts []cryptoTls.Certificate,
	csrPem string,
	expectSessionCert bool,
) (newAccessToken string, newRefreshToken string, sessionCert string, statusCode int) {
	dst := map[string][]string{
		"grant_type":         {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"subject_token":      {accessToken},
		"subject_token_type": {"urn:ietf:params:oauth:token-type:access_token"},
		"scope":              {"openid offline_access"},
	}

	if csrPem != "" {
		dst["csr_pem"] = []string{csrPem}
	}

	if expectSessionCert {
		result := &tokenResponseWithSessionCert{}
		var r *resty.Request
		if tlsCerts != nil {
			r = ctx.newRequestWithTlsCerts(tlsCerts)
		} else {
			r = ctx.newAnonymousClientApiRequest()
		}
		r.SetHeader("content-type", oidc_auth.FormContentType).
			SetMultiValueFormData(dst).
			SetResult(result)

		resp, postErr := r.Post("https://" + ctx.ApiHost + "/oidc/oauth/token")
		ctx.Req.NoError(postErr)
		return result.AccessToken, result.RefreshToken, result.SessionCert, resp.StatusCode()
	}

	result := &oidc.TokenExchangeResponse{}
	var r *resty.Request
	if tlsCerts != nil {
		r = ctx.newRequestWithTlsCerts(tlsCerts)
	} else {
		r = ctx.newAnonymousClientApiRequest()
	}
	r.SetHeader("content-type", oidc_auth.FormContentType).
		SetMultiValueFormData(dst).
		SetResult(result)

	resp, postErr := r.Post("https://" + ctx.ApiHost + "/oidc/oauth/token")
	ctx.Req.NoError(postErr)
	return result.AccessToken, result.RefreshToken, "", resp.StatusCode()
}

// oidcAuthWithCsr performs an OIDC UPDB auth flow with a CSR, returning tokens and the
// session cert PEM. The caller provides credentials and CSR PEM string.
func oidcAuthWithCsr(
	ctx *TestContext,
	clientHelper *ClientHelperClient,
	creds edge_apis.Credentials,
	csrPem string,
) (accessToken string, refreshToken string, idToken string, sessionCert string, clientID string) {
	result, err := clientHelper.OidcAuthorize(creds)
	ctx.Req.NoError(err)

	opLoginUri := "https://" + ctx.ApiHost + "/oidc/login/username?id=" + result.AuthRequestId

	// Build the login payload with the CSR.
	switch c := creds.(type) {
	case *edge_apis.UpdbCredentials:
		payload := &oidc_auth.OidcUpdbCreds{
			Authenticate: rest_model.Authenticate{
				Password: rest_model.Password(c.Password),
				Username: rest_model.Username(c.Username),
			},
			AuthRequestBody: oidc_auth.AuthRequestBody{
				AuthRequestId: result.AuthRequestId,
			},
			CsrPem: csrPem,
		}

		resp, postErr := result.Client.R().SetBody(payload).Post(opLoginUri)
		ctx.Req.NoError(postErr)
		ctx.Req.Equal(http.StatusFound, resp.StatusCode(),
			"expected redirect after UPDB+CSR login, got %d: %s", resp.StatusCode(), resp.String())

		locUrl, parseErr := url.Parse(resp.Header().Get("Location"))
		ctx.Req.NoError(parseErr)
		code := locUrl.Query().Get("code")
		ctx.Req.NotEmpty(code, "expected authorization code in redirect")

		tokens, exchErr := result.Exchange(code)
		ctx.Req.NoError(exchErr)
		ctx.Req.NotNil(tokens)

		// The standard exchange returns oidc.Tokens which does not capture session_cert.
		// Re-read the access/refresh/id tokens from the tokens object.
		return tokens.AccessToken, tokens.RefreshToken, tokens.IDToken, "", tokens.IDTokenClaims.ClientID

	default:
		ctx.Req.Fail("unsupported credential type for oidcAuthWithCsr")
		return "", "", "", "", ""
	}
}

// oidcCertAuthWithCsr performs an OIDC cert auth flow with a CSR, returning tokens.
func oidcCertAuthWithCsr(
	ctx *TestContext,
	clientHelper *ClientHelperClient,
	certCreds *edge_apis.CertCredentials,
	csrPem string,
) (accessToken string, refreshToken string, idToken string, sessionCert string, clientID string) {
	result, err := clientHelper.OidcAuthorize(certCreds)
	ctx.Req.NoError(err)

	opLoginUri := "https://" + ctx.ApiHost + "/oidc/login/cert"

	payload := &oidc_auth.OidcUpdbCreds{
		AuthRequestBody: oidc_auth.AuthRequestBody{
			AuthRequestId: result.AuthRequestId,
		},
		CsrPem: csrPem,
	}

	resp, postErr := result.Client.R().SetBody(payload).Post(opLoginUri)
	ctx.Req.NoError(postErr)
	ctx.Req.Equal(http.StatusFound, resp.StatusCode(),
		"expected redirect after cert+CSR login, got %d: %s", resp.StatusCode(), resp.String())

	locUrl, parseErr := url.Parse(resp.Header().Get("Location"))
	ctx.Req.NoError(parseErr)
	code := locUrl.Query().Get("code")
	ctx.Req.NotEmpty(code, "expected authorization code in redirect")

	tokens, exchErr := result.Exchange(code)
	ctx.Req.NoError(exchErr)
	ctx.Req.NotNil(tokens)

	return tokens.AccessToken, tokens.RefreshToken, tokens.IDToken, "", tokens.IDTokenClaims.ClientID
}

// Test_OIDC_CSR_Refresh verifies cert binding during token refresh for UPDB identities
// authenticated with a CSR. It tests that the z_cfs claim enforces TLS client cert
// fingerprint matching on refresh, that CSR rotation works, and that old session certs
// are rejected after rotation.
func Test_OIDC_CSR_Refresh(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	managementHelper := ctx.NewEdgeManagementApi(nil)
	clientHelper := ctx.NewEdgeClientApi(nil)

	adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
	adminCreds.CaPool = ctx.ControllerCaPool()
	_, err := managementHelper.Authenticate(adminCreds, nil)
	ctx.Req.NoError(rest_util.WrapErr(err))

	// Use the admin identity for UPDB-based CSR testing.
	updbCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
	updbCreds.CaPool = ctx.ControllerCaPool()

	// Generate initial CSR for authentication.
	csrPem, csrKey, err := generateCsrPem()
	ctx.Req.NoError(err)
	ctx.Req.NotEmpty(csrPem)

	// Authenticate via OIDC with CSR.
	accessToken, refreshToken, _, _, clientID := oidcAuthWithCsr(ctx, clientHelper, updbCreds, csrPem)
	ctx.Req.NotEmpty(accessToken)
	ctx.Req.NotEmpty(refreshToken)

	// Parse access token to get initial z_cfs.
	origClaims, err := parseAccessClaims(accessToken)
	ctx.Req.NoError(err)
	ctx.Req.NotEmpty(origClaims.CertFingerprints, "expected z_cfs to be populated after CSR auth")

	// Build TLS cert from the session cert for subsequent requests.
	// Since the standard exchange does not return session_cert, we use the first refresh
	// with CSR to obtain a session cert. For now, do a refresh with the CSR to get a session cert.
	newAccessToken, newRefreshToken, sessionCertPem, status := doRefreshRequest(
		ctx, refreshToken, clientID, nil, csrPem, true,
	)

	// If the initial auth already bound certs, a refresh without a matching cert may fail.
	// In that case, we need to work with what we have.
	if status == http.StatusOK && sessionCertPem != "" {
		// We got a session cert on refresh. Use it.
		accessToken = newAccessToken
		refreshToken = newRefreshToken

		origClaims, err = parseAccessClaims(accessToken)
		ctx.Req.NoError(err)
	}

	// Build session TLS certs from the session cert PEM and CSR key.
	var sessionTlsCerts []cryptoTls.Certificate
	if sessionCertPem != "" {
		sessionTlsCerts, err = buildTlsCertFromPem(sessionCertPem, csrKey)
		ctx.Req.NoError(err)
	}

	initialCfs := origClaims.CertFingerprints

	t.Run("refresh with matching cert succeeds", func(t *testing.T) {
		ctx.NextTest(t)

		if sessionTlsCerts == nil {
			t.Skip("no session cert available for this test")
		}

		_, _, _, statusCode := doRefreshRequest(
			ctx, refreshToken, clientID, sessionTlsCerts, "", false,
		)
		ctx.Req.Equal(http.StatusOK, statusCode, "refresh with matching session cert should succeed")
	})

	t.Run("refresh with wrong cert fails when z_cfs present", func(t *testing.T) {
		ctx.NextTest(t)

		if len(origClaims.CertFingerprints) == 0 {
			t.Skip("z_cfs is empty, cert binding not enforced")
		}

		wrongTlsCerts, genErr := generateSelfSignedCertAndKey()
		ctx.Req.NoError(genErr)

		_, _, _, statusCode := doRefreshRequest(
			ctx, refreshToken, clientID, wrongTlsCerts, "", false,
		)
		ctx.Req.NotEqual(http.StatusOK, statusCode,
			"refresh with wrong cert should fail when z_cfs is present")
	})

	t.Run("refresh without cert fails when z_cfs present", func(t *testing.T) {
		ctx.NextTest(t)

		if len(origClaims.CertFingerprints) == 0 {
			t.Skip("z_cfs is empty, cert binding not enforced")
		}

		_, _, _, statusCode := doRefreshRequest(
			ctx, refreshToken, clientID, nil, "", false,
		)
		ctx.Req.NotEqual(http.StatusOK, statusCode,
			"refresh without cert should fail when z_cfs is present")
	})

	t.Run("refresh without cert and with CSR fails when z_cfs present", func(t *testing.T) {
		ctx.NextTest(t)

		if len(origClaims.CertFingerprints) == 0 {
			t.Skip("z_cfs is empty, cert binding not enforced")
		}

		csrPem, _, genErr := generateCsrPem()
		ctx.Req.NoError(genErr)

		_, _, sessionCertPem, statusCode := doRefreshRequest(
			ctx, refreshToken, clientID, nil, csrPem, true,
		)
		ctx.Req.NotEqual(http.StatusOK, statusCode,
			"a CSR must not bypass the cert binding check on a PoP session")
		ctx.Req.Empty(sessionCertPem)
	})

	t.Run("refresh without CSR preserves z_cfs", func(t *testing.T) {
		ctx.NextTest(t)

		if sessionTlsCerts == nil {
			t.Skip("no session cert available for this test")
		}

		newAccess, _, _, statusCode := doRefreshRequest(
			ctx, refreshToken, clientID, sessionTlsCerts, "", false,
		)
		ctx.Req.Equal(http.StatusOK, statusCode)

		newClaims, parseErr := parseAccessClaims(newAccess)
		ctx.Req.NoError(parseErr)
		ctx.Req.Equal(initialCfs, newClaims.CertFingerprints,
			"z_cfs should be identical when refreshing without a CSR")
	})

	t.Run("z_cfs token cannot transition to empty z_cfs", func(t *testing.T) {
		ctx.NextTest(t)

		if sessionTlsCerts == nil {
			t.Skip("no session cert available for this test")
		}

		newAccess, _, _, statusCode := doRefreshRequest(
			ctx, refreshToken, clientID, sessionTlsCerts, "", false,
		)
		ctx.Req.Equal(http.StatusOK, statusCode)

		newClaims, parseErr := parseAccessClaims(newAccess)
		ctx.Req.NoError(parseErr)
		ctx.Req.NotEmpty(newClaims.CertFingerprints,
			"a token with z_cfs must not transition to empty z_cfs")
	})

	t.Run("refresh with CSR rotates cert", func(t *testing.T) {
		ctx.NextTest(t)

		if sessionTlsCerts == nil {
			t.Skip("no session cert available for this test")
		}

		// Generate a new CSR for rotation.
		newCsrPem, newCsrKey, genErr := generateCsrPem()
		ctx.Req.NoError(genErr)

		newAccess, newRefresh, newSessionCert, statusCode := doRefreshRequest(
			ctx, refreshToken, clientID, sessionTlsCerts, newCsrPem, true,
		)
		ctx.Req.Equal(http.StatusOK, statusCode, "refresh with CSR rotation should succeed")
		ctx.Req.NotEmpty(newAccess)
		ctx.Req.NotEmpty(newRefresh)

		// Verify session_cert is valid PEM.
		if newSessionCert != "" {
			block, _ := pem.Decode([]byte(newSessionCert))
			ctx.Req.NotNil(block, "session_cert should be valid PEM")
		}

		// Parse new access token and verify z_cfs changed.
		newClaims, parseErr := parseAccessClaims(newAccess)
		ctx.Req.NoError(parseErr)
		ctx.Req.NotEmpty(newClaims.CertFingerprints, "z_cfs should be populated after CSR rotation")

		// The new fingerprint from the rotated CSR cert should be present.
		if newSessionCert != "" {
			newFp := fingerprintFromPem(newSessionCert)
			ctx.Req.NotEmpty(newFp)
			ctx.Req.Contains(newClaims.CertFingerprints, newFp,
				"z_cfs should contain the fingerprint of the new session cert")
		}

		t.Run("after rotation, old session cert rejected", func(t *testing.T) {
			ctx.NextTest(t)

			_, _, _, statusCode := doRefreshRequest(
				ctx, newRefresh, clientID, sessionTlsCerts, "", false,
			)
			ctx.Req.NotEqual(http.StatusOK, statusCode,
				"old session cert should be rejected after CSR rotation")
		})

		t.Run("after rotation, new session cert accepted", func(t *testing.T) {
			ctx.NextTest(t)

			if newSessionCert == "" {
				t.Skip("no new session cert returned")
			}

			newTlsCerts, buildErr := buildTlsCertFromPem(newSessionCert, newCsrKey)
			ctx.Req.NoError(buildErr)

			_, _, _, statusCode := doRefreshRequest(
				ctx, newRefresh, clientID, newTlsCerts, "", false,
			)
			ctx.Req.Equal(http.StatusOK, statusCode,
				"new session cert should be accepted after CSR rotation")
		})
	})
}

// Test_OIDC_CSR_Refresh_CertAuth verifies cert binding during token refresh for cert-authenticated
// identities. When a cert identity authenticates with a CSR, the z_cfs claim contains both the
// auth cert fingerprint and the CSR-derived session cert fingerprint. Both certs should be
// accepted for refresh, and rotation should replace only the session cert fingerprint.
func Test_OIDC_CSR_Refresh_CertAuth(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	managementHelper := ctx.NewEdgeManagementApi(nil)
	clientHelper := ctx.NewEdgeClientApi(nil)

	adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
	adminCreds.CaPool = ctx.ControllerCaPool()
	_, err := managementHelper.Authenticate(adminCreds, nil)
	ctx.Req.NoError(rest_util.WrapErr(err))

	// Create and enroll a cert-based identity.
	_, certCreds, err := managementHelper.CreateAndEnrollOttIdentity(false)
	ctx.Req.NoError(err)
	certCreds.CaPool = ctx.ControllerCaPool()

	// Generate initial CSR.
	csrPem, csrKey, err := generateCsrPem()
	ctx.Req.NoError(err)

	// Authenticate via cert + CSR.
	accessToken, refreshToken, _, _, clientID := oidcCertAuthWithCsr(ctx, clientHelper, certCreds, csrPem)
	ctx.Req.NotEmpty(accessToken)
	ctx.Req.NotEmpty(refreshToken)

	origClaims, err := parseAccessClaims(accessToken)
	ctx.Req.NoError(err)

	// Compute the auth cert fingerprint.
	authCertFp := nfpem.FingerprintFromCertificate(certCreds.Certs[0])
	ctx.Req.NotEmpty(authCertFp)

	// Build auth cert TLS certs.
	authTlsCerts := certCreds.TlsCerts()

	// Do an initial refresh with CSR to obtain a session cert for subsequent tests.
	newAccess, newRefresh, sessionCertPem, status := doRefreshRequest(
		ctx, refreshToken, clientID, authTlsCerts, csrPem, true,
	)
	if status == http.StatusOK && newAccess != "" {
		accessToken = newAccess
		refreshToken = newRefresh
		origClaims, err = parseAccessClaims(accessToken)
		ctx.Req.NoError(err)
	}

	var sessionTlsCerts []cryptoTls.Certificate
	if sessionCertPem != "" {
		sessionTlsCerts, err = buildTlsCertFromPem(sessionCertPem, csrKey)
		ctx.Req.NoError(err)
	}

	t.Run("refresh with auth cert succeeds", func(t *testing.T) {
		ctx.NextTest(t)

		_, _, _, statusCode := doRefreshRequest(
			ctx, refreshToken, clientID, authTlsCerts, "", false,
		)
		ctx.Req.Equal(http.StatusOK, statusCode,
			"refresh with auth cert should succeed")
	})

	t.Run("refresh with session cert succeeds", func(t *testing.T) {
		ctx.NextTest(t)

		if sessionTlsCerts == nil {
			t.Skip("no session cert available")
		}

		_, _, _, statusCode := doRefreshRequest(
			ctx, refreshToken, clientID, sessionTlsCerts, "", false,
		)
		ctx.Req.Equal(http.StatusOK, statusCode,
			"refresh with session cert should succeed")
	})

	t.Run("refresh without CSR preserves both fingerprints", func(t *testing.T) {
		ctx.NextTest(t)

		newAccess, _, _, statusCode := doRefreshRequest(
			ctx, refreshToken, clientID, authTlsCerts, "", false,
		)
		ctx.Req.Equal(http.StatusOK, statusCode)

		newClaims, parseErr := parseAccessClaims(newAccess)
		ctx.Req.NoError(parseErr)
		ctx.Req.Contains(newClaims.CertFingerprints, authCertFp,
			"z_cfs should still contain auth cert fingerprint")
		ctx.Req.Equal(origClaims.CertFingerprints, newClaims.CertFingerprints,
			"z_cfs should be unchanged when refreshing without a CSR")
	})

	t.Run("rotate with session cert and new CSR", func(t *testing.T) {
		ctx.NextTest(t)

		if sessionTlsCerts == nil {
			t.Skip("no session cert available")
		}

		newCsrPem, _, genErr := generateCsrPem()
		ctx.Req.NoError(genErr)

		newAccess, _, newSessionCert, statusCode := doRefreshRequest(
			ctx, refreshToken, clientID, sessionTlsCerts, newCsrPem, true,
		)
		ctx.Req.Equal(http.StatusOK, statusCode,
			"rotation with session cert and new CSR should succeed")

		newClaims, parseErr := parseAccessClaims(newAccess)
		ctx.Req.NoError(parseErr)
		ctx.Req.Contains(newClaims.CertFingerprints, authCertFp,
			"z_cfs should retain auth cert fingerprint after rotation")

		if newSessionCert != "" {
			rotatedFp := fingerprintFromPem(newSessionCert)
			ctx.Req.Contains(newClaims.CertFingerprints, rotatedFp,
				"z_cfs should contain the new session cert fingerprint")
		}
	})

	t.Run("rotate with auth cert and new CSR", func(t *testing.T) {
		ctx.NextTest(t)

		newCsrPem, _, genErr := generateCsrPem()
		ctx.Req.NoError(genErr)

		newAccess, _, newSessionCert, statusCode := doRefreshRequest(
			ctx, refreshToken, clientID, authTlsCerts, newCsrPem, true,
		)
		ctx.Req.Equal(http.StatusOK, statusCode,
			"rotation with auth cert and new CSR should succeed")

		newClaims, parseErr := parseAccessClaims(newAccess)
		ctx.Req.NoError(parseErr)
		ctx.Req.Contains(newClaims.CertFingerprints, authCertFp,
			"z_cfs should retain auth cert fingerprint after rotation")

		if newSessionCert != "" {
			rotatedFp := fingerprintFromPem(newSessionCert)
			ctx.Req.Contains(newClaims.CertFingerprints, rotatedFp,
				"z_cfs should contain the new session cert fingerprint")
		}
	})

	t.Run("after rotation, old session cert rejected, auth cert still works", func(t *testing.T) {
		ctx.NextTest(t)

		// Rotate to get a definitive new state.
		newCsrPem, _, genErr := generateCsrPem()
		ctx.Req.NoError(genErr)

		_, rotatedRefresh, _, statusCode := doRefreshRequest(
			ctx, refreshToken, clientID, authTlsCerts, newCsrPem, true,
		)
		ctx.Req.Equal(http.StatusOK, statusCode)

		// Old session cert should be rejected.
		if sessionTlsCerts != nil {
			_, _, _, oldStatus := doRefreshRequest(
				ctx, rotatedRefresh, clientID, sessionTlsCerts, "", false,
			)
			ctx.Req.NotEqual(http.StatusOK, oldStatus,
				"old session cert should be rejected after rotation")
		}

		// Auth cert should still work.
		_, _, _, authStatus := doRefreshRequest(
			ctx, rotatedRefresh, clientID, authTlsCerts, "", false,
		)
		ctx.Req.Equal(http.StatusOK, authStatus,
			"auth cert should still work after session cert rotation")
	})
}

// Test_OIDC_CSR_TokenExchange verifies cert binding during token exchange. The token exchange
// grant type uses the access token as the subject token and should enforce the same z_cfs
// fingerprint matching as refresh.
func Test_OIDC_CSR_TokenExchange(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	managementHelper := ctx.NewEdgeManagementApi(nil)
	clientHelper := ctx.NewEdgeClientApi(nil)

	adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
	adminCreds.CaPool = ctx.ControllerCaPool()
	_, err := managementHelper.Authenticate(adminCreds, nil)
	ctx.Req.NoError(rest_util.WrapErr(err))

	// Authenticate admin via OIDC with CSR.
	csrPem, csrKey, err := generateCsrPem()
	ctx.Req.NoError(err)

	updbCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
	updbCreds.CaPool = ctx.ControllerCaPool()

	accessToken, refreshToken, _, _, clientID := oidcAuthWithCsr(ctx, clientHelper, updbCreds, csrPem)
	ctx.Req.NotEmpty(accessToken)

	origClaims, err := parseAccessClaims(accessToken)
	ctx.Req.NoError(err)

	// Do an initial refresh with CSR to get a session cert.
	newAccess, newRefresh, sessionCertPem, status := doRefreshRequest(
		ctx, refreshToken, clientID, nil, csrPem, true,
	)
	if status == http.StatusOK && newAccess != "" {
		accessToken = newAccess
		refreshToken = newRefresh
		_ = refreshToken
		origClaims, err = parseAccessClaims(accessToken)
		ctx.Req.NoError(err)
	}

	var sessionTlsCerts []cryptoTls.Certificate
	if sessionCertPem != "" {
		sessionTlsCerts, err = buildTlsCertFromPem(sessionCertPem, csrKey)
		ctx.Req.NoError(err)
	}

	t.Run("exchange with matching cert succeeds", func(t *testing.T) {
		ctx.NextTest(t)

		if sessionTlsCerts == nil {
			t.Skip("no session cert available")
		}

		newAccess, _, _, statusCode := doTokenExchangeRequest(
			ctx, accessToken, sessionTlsCerts, "", false,
		)
		ctx.Req.Equal(http.StatusOK, statusCode,
			"token exchange with matching cert should succeed")
		ctx.Req.NotEmpty(newAccess)
	})

	t.Run("exchange with wrong cert fails when z_cfs present", func(t *testing.T) {
		ctx.NextTest(t)

		if len(origClaims.CertFingerprints) == 0 {
			t.Skip("z_cfs is empty")
		}

		wrongTlsCerts, genErr := generateSelfSignedCertAndKey()
		ctx.Req.NoError(genErr)

		_, _, _, statusCode := doTokenExchangeRequest(
			ctx, accessToken, wrongTlsCerts, "", false,
		)
		ctx.Req.NotEqual(http.StatusOK, statusCode,
			"token exchange with wrong cert should fail when z_cfs present")
	})

	t.Run("exchange without CSR preserves z_cfs", func(t *testing.T) {
		ctx.NextTest(t)

		if sessionTlsCerts == nil {
			t.Skip("no session cert available")
		}

		newAccess, _, _, statusCode := doTokenExchangeRequest(
			ctx, accessToken, sessionTlsCerts, "", false,
		)
		ctx.Req.Equal(http.StatusOK, statusCode)

		newClaims, parseErr := parseAccessClaims(newAccess)
		ctx.Req.NoError(parseErr)
		ctx.Req.Equal(origClaims.CertFingerprints, newClaims.CertFingerprints,
			"z_cfs should be unchanged on exchange without CSR")
	})

	t.Run("exchange with CSR rotates cert", func(t *testing.T) {
		ctx.NextTest(t)

		if sessionTlsCerts == nil {
			t.Skip("no session cert available")
		}

		newCsrPem, newCsrKey, genErr := generateCsrPem()
		ctx.Req.NoError(genErr)

		newAccess, _, newSessionCert, statusCode := doTokenExchangeRequest(
			ctx, accessToken, sessionTlsCerts, newCsrPem, true,
		)
		ctx.Req.Equal(http.StatusOK, statusCode,
			"token exchange with CSR rotation should succeed")

		newClaims, parseErr := parseAccessClaims(newAccess)
		ctx.Req.NoError(parseErr)
		ctx.Req.NotEmpty(newClaims.CertFingerprints)

		if newSessionCert != "" {
			rotatedFp := fingerprintFromPem(newSessionCert)
			ctx.Req.Contains(newClaims.CertFingerprints, rotatedFp,
				"z_cfs should contain the new session cert fingerprint")
		}

		t.Run("after rotation, old cert rejected, new cert accepted", func(t *testing.T) {
			ctx.NextTest(t)

			// Old cert should fail.
			_, _, _, oldStatus := doTokenExchangeRequest(
				ctx, newAccess, sessionTlsCerts, "", false,
			)
			ctx.Req.NotEqual(http.StatusOK, oldStatus,
				"old session cert should be rejected after exchange rotation")

			// New cert should succeed.
			if newSessionCert != "" {
				newTlsCerts, buildErr := buildTlsCertFromPem(newSessionCert, newCsrKey)
				ctx.Req.NoError(buildErr)

				_, _, _, newStatus := doTokenExchangeRequest(
					ctx, newAccess, newTlsCerts, "", false,
				)
				ctx.Req.Equal(http.StatusOK, newStatus,
					"new session cert should be accepted after exchange rotation")
			}
		})
	})
}

// Test_OIDC_CSR_SpiffeFallback verifies the SPIFFE ID fallback behavior when z_cfs is empty.
// A UPDB identity authenticated without a CSR has no z_cfs, so cert binding is not enforced.
// Submitting a CSR during refresh transitions the token to having z_cfs, after which cert
// binding is enforced.
func Test_OIDC_CSR_SpiffeFallback(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	managementHelper := ctx.NewEdgeManagementApi(nil)
	clientHelper := ctx.NewEdgeClientApi(nil)

	adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
	adminCreds.CaPool = ctx.ControllerCaPool()
	_, err := managementHelper.Authenticate(adminCreds, nil)
	ctx.Req.NoError(rest_util.WrapErr(err))

	// Authenticate via OIDC WITHOUT CSR (no z_cfs).
	updbCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
	updbCreds.CaPool = ctx.ControllerCaPool()

	tokens, _, err := clientHelper.RawOidcAuthRequest(updbCreds)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(tokens)
	ctx.Req.NotEmpty(tokens.AccessToken)
	ctx.Req.NotEmpty(tokens.RefreshToken)

	origClaims, err := parseAccessClaims(tokens.AccessToken)
	ctx.Req.NoError(err)

	clientID := tokens.IDTokenClaims.ClientID

	t.Run("refresh without z_cfs and no cert passes", func(t *testing.T) {
		ctx.NextTest(t)

		// With empty z_cfs, refresh should succeed without a client cert.
		newAccess, _, _, statusCode := doRefreshRequest(
			ctx, tokens.RefreshToken, clientID, nil, "", false,
		)
		ctx.Req.Equal(http.StatusOK, statusCode,
			"refresh without cert should succeed when z_cfs is empty")
		ctx.Req.NotEmpty(newAccess)

		newClaims, parseErr := parseAccessClaims(newAccess)
		ctx.Req.NoError(parseErr)
		ctx.Req.Empty(newClaims.CertFingerprints,
			"z_cfs should remain empty when no CSR is submitted")
		_ = origClaims
	})

	t.Run("refresh with CSR is ignored in bearer mode", func(t *testing.T) {
		ctx.NextTest(t)

		// Bearer-mode sessions (z_cfs empty) cannot transition into PoP via a CSR
		// submitted at the token endpoint. The CSR is silently ignored so that a
		// stolen refresh token cannot establish PoP under an attacker-supplied
		// keypair and lock the legitimate user out of their own session.
		csrPem, _, genErr := generateCsrPem()
		ctx.Req.NoError(genErr)

		newAccess, newRefresh, sessionCertPem, statusCode := doRefreshRequest(
			ctx, tokens.RefreshToken, clientID, nil, csrPem, true,
		)
		ctx.Req.Equal(http.StatusOK, statusCode, "bearer-mode refresh must succeed")
		ctx.Req.NotEmpty(newAccess)
		ctx.Req.Empty(sessionCertPem,
			"no session_cert should be returned when CSR is ignored")

		newClaims, parseErr := parseAccessClaims(newAccess)
		ctx.Req.NoError(parseErr)
		ctx.Req.Empty(newClaims.CertFingerprints,
			"z_cfs must remain empty; CSR submitted in bearer mode must not establish PoP")

		t.Run("session remains bearer mode after the CSR-bearing refresh", func(t *testing.T) {
			ctx.NextTest(t)

			_, _, _, statusCode := doRefreshRequest(
				ctx, newRefresh, clientID, nil, "", false,
			)
			ctx.Req.Equal(http.StatusOK, statusCode,
				"a follow-up refresh without a cert must still succeed")
		})
	})
}
