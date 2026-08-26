package model

import (
	"crypto/x509"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/stretchr/testify/require"
)

// Test_TokenIssuerExtJwt_VerifyToken_EnforcesIssuerAndAudience verifies that a validly-signed
// token minted for a different issuer or audience is rejected. The by-inspection enrollment path
// already enforces this; VerifyToken (used by the by-issuer-id enrollment path) must not be weaker.
func Test_TokenIssuerExtJwt_VerifyToken_EnforcesIssuerAndAudience(t *testing.T) {
	req := require.New(t)

	testRootCa := newRootCa()
	leafKeyPair := testRootCa.NewLeafWithAKID()

	jwksEndpoint := "https://example.com/.well-known/jwks"

	jwksResolver, err := newTestJwksResolver()
	req.NoError(err)

	leafKey, err := newKey(leafKeyPair.cert, []*x509.Certificate{leafKeyPair.cert, testRootCa.cert})
	req.NoError(err)

	jwksResolver.AddKey(leafKey, leafKeyPair.key)

	expectedIssuer := "https://idp.example.com"
	expectedAudience := "ziti-controller"

	signerRec := &TokenIssuerExtJwt{
		kidToPubKey: map[string]common.IssuerPublicKey{},
		externalJwtSigner: &db.ExternalJwtSigner{
			BaseExtEntity: boltz.BaseExtEntity{
				Id:        "fake-id",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			Name:         "test-signer",
			JwksEndpoint: &jwksEndpoint,
			Issuer:       &expectedIssuer,
			Audience:     &expectedAudience,
			Enabled:      true,
		},
		jwksResolver: jwksResolver,
	}

	req.NoError(signerRec.Resolve(false))

	sign := func(claims jwt.MapClaims) string {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header["kid"] = leafKey.KeyId
		signed, err := token.SignedString(leafKeyPair.key)
		req.NoError(err)
		return signed
	}

	t.Run("accepts a token with matching issuer and audience", func(t *testing.T) {
		req := require.New(t)
		signed := sign(jwt.MapClaims{
			"iss": expectedIssuer,
			"aud": expectedAudience,
			"sub": "user-123",
			"exp": time.Now().Add(time.Hour).Unix(),
		})

		result := signerRec.VerifyToken(signed)
		req.Truef(result.IsValid(), "token with matching issuer/audience must be accepted: %v", result.Error)
	})

	t.Run("rejects a validly-signed token with a foreign audience", func(t *testing.T) {
		req := require.New(t)
		signed := sign(jwt.MapClaims{
			"iss": expectedIssuer,
			"aud": "some-other-audience",
			"sub": "user-123",
			"exp": time.Now().Add(time.Hour).Unix(),
		})

		result := signerRec.VerifyToken(signed)
		req.False(result.IsValid(), "token minted for a foreign audience must be rejected")
	})

	t.Run("rejects a validly-signed token with a foreign issuer", func(t *testing.T) {
		req := require.New(t)
		signed := sign(jwt.MapClaims{
			"iss": "https://attacker.example.com",
			"aud": expectedAudience,
			"sub": "user-123",
			"exp": time.Now().Add(time.Hour).Unix(),
		})

		result := signerRec.VerifyToken(signed)
		req.False(result.IsValid(), "token minted by a foreign issuer must be rejected")
	})

	t.Run("rejects a token missing the audience claim", func(t *testing.T) {
		req := require.New(t)
		signed := sign(jwt.MapClaims{
			"iss": expectedIssuer,
			"sub": "user-123",
			"exp": time.Now().Add(time.Hour).Unix(),
		})

		result := signerRec.VerifyToken(signed)
		req.False(result.IsValid(), "token without an audience claim must be rejected")
	})
}

func Test_resolveStringSliceClaimProperty(t *testing.T) {
	t.Run("returns empty when the selector is unset", func(t *testing.T) {
		req := require.New(t)

		vals, err := resolveStringSliceClaimProperty(jwt.MapClaims{"roles": "admin"}, "")

		req.NoError(err)
		req.Empty(vals)
	})

	t.Run("returns empty when the claim is absent at the selected path", func(t *testing.T) {
		req := require.New(t)

		vals, err := resolveStringSliceClaimProperty(jwt.MapClaims{"name": "bob"}, "/roles")

		req.NoError(err)
		req.Empty(vals)
	})

	t.Run("returns empty when a nested claim is absent at the selected path", func(t *testing.T) {
		req := require.New(t)

		claims := jwt.MapClaims{"resource_access": map[string]any{"other": "x"}}

		vals, err := resolveStringSliceClaimProperty(claims, "/resource_access/ziti/roles")

		req.NoError(err)
		req.Empty(vals)
	})

	t.Run("returns empty when the claim is present but null", func(t *testing.T) {
		req := require.New(t)

		vals, err := resolveStringSliceClaimProperty(jwt.MapClaims{"roles": nil}, "/roles")

		req.NoError(err)
		req.Empty(vals)
	})

	t.Run("returns empty when a nested claim is present but null", func(t *testing.T) {
		req := require.New(t)

		claims := jwt.MapClaims{"resource_access": map[string]any{"ziti": map[string]any{"roles": nil}}}

		vals, err := resolveStringSliceClaimProperty(claims, "/resource_access/ziti/roles")

		req.NoError(err)
		req.Empty(vals)
	})

	t.Run("returns empty when the claim is present but an empty string", func(t *testing.T) {
		req := require.New(t)

		vals, err := resolveStringSliceClaimProperty(jwt.MapClaims{"roles": ""}, "/roles")

		req.NoError(err)
		req.Empty(vals)
	})

	t.Run("returns a single value when the claim is a string", func(t *testing.T) {
		req := require.New(t)

		vals, err := resolveStringSliceClaimProperty(jwt.MapClaims{"roles": "admin"}, "/roles")

		req.NoError(err)
		req.Equal([]string{"admin"}, vals)
	})

	t.Run("returns all values when the claim is a string array", func(t *testing.T) {
		req := require.New(t)

		claims := jwt.MapClaims{"roles": []any{"admin", "support"}}

		vals, err := resolveStringSliceClaimProperty(claims, "/roles")

		req.NoError(err)
		req.Equal([]string{"admin", "support"}, vals)
	})

	t.Run("resolves a nested claim that is present", func(t *testing.T) {
		req := require.New(t)

		claims := jwt.MapClaims{"resource_access": map[string]any{"ziti": map[string]any{"roles": []any{"admin"}}}}

		vals, err := resolveStringSliceClaimProperty(claims, "/resource_access/ziti/roles")

		req.NoError(err)
		req.Equal([]string{"admin"}, vals)
	})

	t.Run("errors when the claim is present but not a string or array of strings", func(t *testing.T) {
		req := require.New(t)

		claims := jwt.MapClaims{"roles": map[string]any{"unexpected": "object"}}

		_, err := resolveStringSliceClaimProperty(claims, "/roles")

		req.Error(err)
		req.ErrorContains(err, "map[unexpected:object]")
	})
}
