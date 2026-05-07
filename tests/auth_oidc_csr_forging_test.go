//go:build apitests

package tests

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	nfpem "github.com/openziti/foundation/v2/pem"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/controller/oidc_auth"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

// Test_OIDC_CSR_Forging verifies that the controller ignores attacker-controlled fields in a
// CSR submitted during OIDC token refresh, using only the public key from the CSR when signing
// the session certificate. The issued certificate must not contain any Subject, DNS, IP, email,
// or URI SAN values from the CSR; the only URI SAN must be the controller-generated SPIFFE ID.
func Test_OIDC_CSR_Forging(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	managementHelper := ctx.NewEdgeManagementApi(nil)
	clientHelper := ctx.NewEdgeClientApi(nil)

	adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
	adminCreds.CaPool = ctx.ControllerCaPool()
	_, err := managementHelper.Authenticate(adminCreds, nil)
	ctx.Req.NoError(err)

	// Forging the CSR signing path requires a session that is already in PoP mode
	_, certCreds, err := managementHelper.CreateAndEnrollOttIdentity(false)
	ctx.Req.NoError(err)
	certCreds.CaPool = ctx.ControllerCaPool()
	authTlsCerts := certCreds.TlsCerts()

	type forgingTokenResponse struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    uint64 `json:"expires_in"`
		IDToken      string `json:"id_token"`
		SessionCert  string `json:"session_cert"`
	}

	refreshWithCsr := func(t *testing.T, csrPem string) (*x509.Certificate, *forgingTokenResponse, int) {
		t.Helper()

		tokens, _, authErr := clientHelper.RawOidcAuthRequest(certCreds)
		ctx.Req.NoError(authErr)
		ctx.Req.NotNil(tokens)
		ctx.Req.NotEmpty(tokens.RefreshToken)

		req := &oidc.RefreshTokenRequest{
			RefreshToken: tokens.RefreshToken,
			ClientID:     tokens.IDTokenClaims.ClientID,
			Scopes:       []string{oidc.ScopeOpenID, oidc.ScopeOfflineAccess},
		}

		enc := oidc.NewEncoder()
		dst := map[string][]string{}
		encErr := enc.Encode(req, dst)
		ctx.Req.NoError(encErr)

		dst["grant_type"] = []string{string(req.GrantType())}
		dst["csr_pem"] = []string{csrPem}

		result := &forgingTokenResponse{}
		resp, httpErr := ctx.newRequestWithTlsCerts(authTlsCerts).
			SetHeader("content-type", oidc_auth.FormContentType).
			SetMultiValueFormData(dst).
			SetResult(result).
			Post("https://" + ctx.ApiHost + "/oidc/oauth/token")
		ctx.Req.NoError(httpErr)

		if resp.StatusCode() != http.StatusOK || result.SessionCert == "" {
			return nil, result, resp.StatusCode()
		}

		certs := nfpem.PemStringToCertificates(result.SessionCert)
		ctx.Req.NotEmpty(certs, "expected at least one certificate in session_cert")

		return certs[0], result, resp.StatusCode()
	}

	t.Run("CSR Subject fields are not accepted", func(t *testing.T) {
		ctx.NextTest(t)

		csrPem := generateForgedCsrPem(t)
		cert, result, statusCode := refreshWithCsr(t, csrPem)
		ctx.Req.Equal(http.StatusOK, statusCode)
		ctx.Req.NotNil(cert)
		ctx.Req.NotNil(result)

		parser := jwt.NewParser()
		accessClaims := &common.AccessClaims{}
		_, _, parseErr := parser.ParseUnverified(result.AccessToken, accessClaims)
		ctx.Req.NoError(parseErr)
		ctx.Req.NotEmpty(accessClaims.Subject)

		// Subject CN must be the controller-set identity id, not anything from the CSR.
		ctx.Req.Equal(accessClaims.Subject, cert.Subject.CommonName,
			"issued cert Subject CN must be the identity id")

		// All other Subject fields from the CSR (Organization=Evil Corp, Country=RU, etc.)
		// must be stripped.
		ctx.Req.Empty(cert.Subject.Organization,
			"CSR Subject Organization must not be copied to issued cert")
		ctx.Req.Empty(cert.Subject.Country,
			"CSR Subject Country must not be copied to issued cert")
		ctx.Req.Empty(cert.Subject.OrganizationalUnit,
			"CSR Subject OrganizationalUnit must not be copied to issued cert")
	})

	t.Run("CSR SAN URIs are not in issued cert", func(t *testing.T) {
		ctx.NextTest(t)

		csrPem := generateForgedCsrPem(t)
		cert, _, statusCode := refreshWithCsr(t, csrPem)
		ctx.Req.Equal(http.StatusOK, statusCode)
		ctx.Req.NotNil(cert)

		// The only URI SAN should be the controller-generated SPIFFE ID.
		ctx.Req.NotEmpty(cert.URIs, "expected at least one URI SAN (the SPIFFE ID)")

		for _, u := range cert.URIs {
			ctx.Req.NotContains(u.String(), "evil.trust.domain",
				"issued cert must not contain forged SPIFFE URI")
		}

		// Verify the SPIFFE ID has the expected structure.
		foundSpiffe := false
		for _, u := range cert.URIs {
			if u.Scheme == "spiffe" {
				foundSpiffe = true
				ctx.Req.True(strings.Contains(u.Path, "identity/"),
					"SPIFFE URI path must contain 'identity/' segment, got: %s", u.String())
				ctx.Req.True(strings.Contains(u.Path, "apiSession/"),
					"SPIFFE URI path must contain 'apiSession/' segment, got: %s", u.String())
			}
		}
		ctx.Req.True(foundSpiffe, "expected a spiffe:// URI SAN in the issued cert")
	})

	t.Run("CSR SAN DNS names are not in issued cert", func(t *testing.T) {
		ctx.NextTest(t)

		csrPem := generateForgedCsrPem(t)
		cert, _, statusCode := refreshWithCsr(t, csrPem)
		ctx.Req.Equal(http.StatusOK, statusCode)
		ctx.Req.NotNil(cert)

		ctx.Req.Empty(cert.DNSNames, "issued cert must not contain any DNS SANs from the CSR")
	})

	t.Run("CSR SAN IP addresses are not in issued cert", func(t *testing.T) {
		ctx.NextTest(t)

		csrPem := generateForgedCsrPem(t)
		cert, _, statusCode := refreshWithCsr(t, csrPem)
		ctx.Req.Equal(http.StatusOK, statusCode)
		ctx.Req.NotNil(cert)

		ctx.Req.Empty(cert.IPAddresses, "issued cert must not contain any IP SANs from the CSR")
	})

	t.Run("CSR SAN email addresses are not in issued cert", func(t *testing.T) {
		ctx.NextTest(t)

		csrPem := generateForgedCsrPem(t)
		cert, _, statusCode := refreshWithCsr(t, csrPem)
		ctx.Req.Equal(http.StatusOK, statusCode)
		ctx.Req.NotNil(cert)

		ctx.Req.Empty(cert.EmailAddresses, "issued cert must not contain any email SANs from the CSR")
	})

	t.Run("issued cert SPIFFE ID matches token apiSessionId", func(t *testing.T) {
		ctx.NextTest(t)

		csrPem := generateForgedCsrPem(t)
		cert, result, statusCode := refreshWithCsr(t, csrPem)
		ctx.Req.Equal(http.StatusOK, statusCode)
		ctx.Req.NotNil(cert)
		ctx.Req.NotNil(result)

		// Parse the access token to extract the identity ID and API session ID.
		parser := jwt.NewParser()
		accessClaims := &common.AccessClaims{}
		_, _, parseErr := parser.ParseUnverified(result.AccessToken, accessClaims)
		ctx.Req.NoError(parseErr)
		ctx.Req.NotEmpty(accessClaims.Subject)
		ctx.Req.NotEmpty(accessClaims.ApiSessionId)

		// Find the SPIFFE URI in the issued cert and verify it references the correct IDs.
		var spiffeUri *url.URL
		for _, u := range cert.URIs {
			if u.Scheme == "spiffe" {
				spiffeUri = u
				break
			}
		}
		ctx.Req.NotNil(spiffeUri, "expected a spiffe:// URI SAN in the issued cert")
		ctx.Req.True(strings.Contains(spiffeUri.Path, "identity/"+accessClaims.Subject),
			"SPIFFE URI must contain identity ID %q, got path: %s", accessClaims.Subject, spiffeUri.Path)
		ctx.Req.True(strings.Contains(spiffeUri.Path, "apiSession/"+accessClaims.ApiSessionId),
			"SPIFFE URI must contain apiSessionId %q, got path: %s", accessClaims.ApiSessionId, spiffeUri.Path)
	})

	t.Run("CSR signature is verified", func(t *testing.T) {
		ctx.NextTest(t)

		// Generate a valid CSR, then corrupt its base64 body to produce an invalid signature.
		validCsrPem := generateForgedCsrPem(t)
		corruptedCsrPem := corruptCsrPem(t, validCsrPem)

		_, _, statusCode := refreshWithCsr(t, corruptedCsrPem)
		ctx.Req.NotEqual(http.StatusOK, statusCode,
			"corrupted CSR must be rejected, but got status %d", statusCode)
	})
}

// generateForgedCsrPem creates a CSR with attacker-controlled Subject, DNS, IP, email, and URI
// SAN fields. The CSR is valid (properly signed) but contains values that the controller must
// ignore when issuing the session certificate.
func generateForgedCsrPem(t *testing.T) string {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	evilSpiffe, err := url.Parse("spiffe://evil.trust.domain/identity/hacked-id/apiSession/hacked-session/apiSessionCertificate/hacked-cert")
	if err != nil {
		t.Fatalf("failed to parse evil SPIFFE URL: %v", err)
	}

	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			Organization: []string{"Evil Corp"},
			CommonName:   "admin",
			Country:      []string{"RU"},
		},
		DNSNames:       []string{"evil.example.com", "*.internal.corp"},
		IPAddresses:    []net.IP{net.ParseIP("10.0.0.1"), net.ParseIP("192.168.1.1")},
		EmailAddresses: []string{"admin@evil.com"},
		URIs:           []*url.URL{evilSpiffe},
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, template, privateKey)
	if err != nil {
		t.Fatalf("failed to create CSR: %v", err)
	}

	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes}))
}

// corruptCsrPem takes a valid CSR PEM string and flips a byte in the base64 body to produce a
// CSR that will fail signature verification.
func corruptCsrPem(t *testing.T, csrPem string) string {
	t.Helper()

	block, rest := pem.Decode([]byte(csrPem))
	if block == nil {
		t.Fatalf("failed to decode PEM block, remaining: %s", string(rest))
	}

	// Flip a byte near the end of the DER data (in the signature area).
	if len(block.Bytes) < 10 {
		t.Fatalf("CSR DER data too short to corrupt")
	}
	block.Bytes[len(block.Bytes)-5] ^= 0xFF

	return string(pem.EncodeToMemory(block))
}
