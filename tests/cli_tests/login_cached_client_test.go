//go:build cli_tests

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
package cli_tests

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_management_api_client/auth_policy"
	"github.com/openziti/edge-api/rest_management_api_client/external_jwt_signer"
	"github.com/openziti/edge-api/rest_management_api_client/identity"
	"github.com/openziti/edge-api/rest_model"
	nfpem "github.com/openziti/foundation/v2/pem"
	edge_apis "github.com/openziti/sdk-golang/v2/edge-apis"
	"github.com/openziti/ziti/v2/ziti/cmd/edge"
	"github.com/openziti/ziti/v2/ziti/util"
	"github.com/stretchr/testify/require"
)

// ziti ops export, ops import and ops verify traffic do not reach the controller through the cached
// RestClientEdgeIdentity. They call LoginOptions.NewManagementClient(true), which rebuilds a client from
// the cached login and refreshes the session before using it. Whatever a login mode needs in order to be
// replayed has to survive into that client, so every mode that persists is exercised here.
//
// Each case logs in, then throws away everything but the cache and asks for a management client the way
// those commands do. Running under the overlay phases covers the zitified transport as well.
func (s *cliTestState) cachedMgmtClientTests(t *testing.T) {
	cases := []struct {
		name string
		opts func(t *testing.T) *edge.LoginOptions
	}{
		{
			name: "updb",
			opts: func(*testing.T) *edge.LoginOptions {
				return &edge.LoginOptions{
					Options:       s.commonOpts,
					Username:      s.controllerUnderTest.Username,
					Password:      s.controllerUnderTest.Password,
					ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
					Yes:           true,
					NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
				}
			},
		},
		{
			name: "identity file",
			opts: func(*testing.T) *edge.LoginOptions {
				return &edge.LoginOptions{
					Options:       s.commonOpts,
					ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
					Yes:           true,
					File:          s.controllerUnderTest.AdminIdFile,
					NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
				}
			},
		},
		{
			name: "client cert",
			opts: func(*testing.T) *edge.LoginOptions {
				return &edge.LoginOptions{
					Options:       s.commonOpts,
					ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
					Yes:           true,
					ClientCert:    s.controllerUnderTest.AdminCertFile,
					ClientKey:     s.controllerUnderTest.AdminKeyFile,
					NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
				}
			},
		},
		{
			name: "external jwt",
			opts: func(t *testing.T) *edge.LoginOptions {
				return &edge.LoginOptions{
					Options:       s.commonOpts,
					ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
					Yes:           true,
					ExtJwtToken:   s.newExternalJwtToken(t),
					NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s.removeZitiDir(t)

			opts := tc.opts(t)
			require.NoError(t, opts.Run(), "%s login should succeed", tc.name)
			util.ReloadConfig()

			// nothing but the cache from here on, exactly as an ops command starts
			cachedOpts := &edge.LoginOptions{Options: s.commonOpts}
			client, err := cachedOpts.NewManagementClient(true)
			require.NoError(t, err, "'ziti ops export' and friends build their client this way")
			require.NotNil(t, client)

			params := identity.NewListIdentitiesParams()
			params.SetTimeout(30 * time.Second)
			list, err := client.API.Identity.ListIdentities(params, nil)
			require.NoError(t, err, "a management call over the cached %s login must be authorized", tc.name)
			require.NotEmpty(t, list.GetPayload().Data)
		})
	}
}

// newExternalJwtToken registers a signer with the controller, allows it on the default auth policy, and
// returns a JWT for the admin identity that the signer vouches for.
func (s *cliTestState) newExternalJwtToken(t *testing.T) string {
	t.Helper()

	admin := &edge.LoginOptions{
		Options:       s.commonOpts,
		Username:      s.controllerUnderTest.Username,
		Password:      s.controllerUnderTest.Password,
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}
	require.NoError(t, admin.Run(), "admin login should succeed")
	util.ReloadConfig()

	// NewManagementClient(false) hands back a client that has not authenticated, so go through the cache
	client, err := (&edge.LoginOptions{Options: s.commonOpts}).NewManagementClient(true)
	require.NoError(t, err)

	signerCert, signerKey := newExtJwtSigningCert(t, "cli-test-ext-jwt-signer")
	issuer := uuid.NewString()
	audience := uuid.NewString()
	kid := uuid.NewString()

	createSigner := external_jwt_signer.NewCreateExternalJWTSignerParams()
	createSigner.ExternalJWTSigner = &rest_model.ExternalJWTSignerCreate{
		CertPem:  toPtr(nfpem.EncodeToString(signerCert)),
		Enabled:  toPtr(true),
		Name:     toPtr("cli-test-ext-jwt-signer-" + uuid.NewString()),
		Kid:      toPtr(kid),
		Issuer:   toPtr(issuer),
		Audience: toPtr(audience),
	}
	created, err := client.API.ExternalJWTSigner.CreateExternalJWTSigner(createSigner, nil)
	require.NoError(t, err, "creating the signer should succeed")
	signerId := created.Payload.Data.ID

	patchPolicy := auth_policy.NewPatchAuthPolicyParams()
	patchPolicy.ID = "default"
	patchPolicy.AuthPolicy = &rest_model.AuthPolicyPatch{
		Primary: &rest_model.AuthPolicyPrimaryPatch{
			ExtJWT: &rest_model.AuthPolicyPrimaryExtJWTPatch{
				Allowed:        toPtr(true),
				AllowedSigners: []string{signerId},
			},
		},
	}
	_, err = client.API.AuthPolicy.PatchAuthPolicy(patchPolicy, nil)
	require.NoError(t, err, "allowing the signer on the default policy should succeed")

	token := jwt.New(jwt.SigningMethodES256)
	token.Claims = jwt.RegisteredClaims{
		Audience:  []string{audience},
		Issuer:    issuer,
		Subject:   s.adminIdentityId(t, client),
		ID:        uuid.NewString(),
		IssuedAt:  &jwt.NumericDate{Time: time.Now()},
		NotBefore: &jwt.NumericDate{Time: time.Now()},
		ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
	}
	token.Header["kid"] = kid

	signed, err := token.SignedString(signerKey)
	require.NoError(t, err)

	return signed
}

// adminIdentityId looks up the identity the admin username belongs to, which the JWT names as its subject.
func (s *cliTestState) adminIdentityId(t *testing.T, client *edge_apis.ManagementApiClient) string {
	t.Helper()

	params := identity.NewListIdentitiesParams()
	params.Filter = toPtr(`name="` + s.controllerUnderTest.Username + `"`)
	params.SetTimeout(30 * time.Second)

	list, err := client.API.Identity.ListIdentities(params, nil)
	require.NoError(t, err)
	require.NotEmpty(t, list.GetPayload().Data, "the admin identity should exist")

	return *list.GetPayload().Data[0].ID
}

func newExtJwtSigningCert(t *testing.T, commonName string) (*x509.Certificate, crypto.PrivateKey) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return cert, key
}

func toPtr[T any](v T) *T {
	return &v
}
