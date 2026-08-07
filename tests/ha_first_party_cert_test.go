//go:build apitests

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

package tests

import (
	"crypto"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_management_api_client/auth_policy"
	"github.com/openziti/edge-api/rest_management_api_client/external_jwt_signer"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	nfpem "github.com/openziti/foundation/v2/pem"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/oidc_auth"
)

// extJwtTestSigner holds the signing material and claims values of a registered test
// ext-jwt-signer, so tests can mint bearer tokens the controller will accept.
type extJwtTestSigner struct {
	audience string
	issuer   string
	kid      string
	key      crypto.PrivateKey
}

// newJwtForIdentity mints a bearer token for the given identity, signed by the test signer.
func (s *extJwtTestSigner) newJwtForIdentity(identityId string) (string, error) {
	jwtToken := jwt.New(jwt.SigningMethodES256)
	jwtToken.Claims = jwt.RegisteredClaims{
		Audience:  []string{s.audience},
		ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
		ID:        uuid.NewString(),
		IssuedAt:  &jwt.NumericDate{Time: time.Now()},
		Issuer:    s.issuer,
		NotBefore: &jwt.NumericDate{Time: time.Now()},
		Subject:   identityId,
	}
	jwtToken.Header["kid"] = s.kid
	return jwtToken.SignedString(s.key)
}

// registerExtJwtSignerAllowedByDefaultPolicy creates an enabled ext-jwt-signer and patches the
// default auth policy to allow it as a primary auth method.
func registerExtJwtSignerAllowedByDefaultPolicy(managementHelper *ManagementHelperClient) (*extJwtTestSigner, error) {
	signerCert, signerKey := newSelfSignedCert("Test Ext-Jwt Signer")
	testSigner := &extJwtTestSigner{
		audience: uuid.NewString(),
		issuer:   uuid.NewString(),
		kid:      uuid.NewString(),
		key:      signerKey,
	}

	createSigner := external_jwt_signer.NewCreateExternalJWTSignerParams()
	createSigner.ExternalJWTSigner = &rest_model.ExternalJWTSignerCreate{
		CertPem:  ToPtr(nfpem.EncodeToString(signerCert)),
		Enabled:  ToPtr(true),
		Name:     ToPtr("Test Ext-Jwt Signer"),
		Kid:      ToPtr(testSigner.kid),
		Issuer:   ToPtr(testSigner.issuer),
		Audience: ToPtr(testSigner.audience),
	}
	signerCreateResp, err := managementHelper.API.ExternalJWTSigner.CreateExternalJWTSigner(createSigner, nil)
	if err != nil {
		return nil, rest_util.WrapErr(err)
	}

	patchPolicy := auth_policy.NewPatchAuthPolicyParams()
	patchPolicy.ID = "default"
	patchPolicy.AuthPolicy = &rest_model.AuthPolicyPatch{
		Primary: &rest_model.AuthPolicyPrimaryPatch{
			ExtJWT: &rest_model.AuthPolicyPrimaryExtJWTPatch{
				Allowed:        ToPtr(true),
				AllowedSigners: []string{signerCreateResp.Payload.Data.ID},
			},
		},
	}
	if _, err = managementHelper.API.AuthPolicy.PatchAuthPolicy(patchPolicy, nil); err != nil {
		return nil, rest_util.WrapErr(err)
	}

	return testSigner, nil
}

// dialServiceWithFirstPartyCert authenticates via ext-jwt while presenting the given
// first-party cert at the TLS layer, verifies the session carries no cert fingerprints (so
// router-side acceptance must come from the first-party origin check, not a fingerprint
// match), and dials the service, requiring an edge router connection.
func dialServiceWithFirstPartyCert(ctx *TestContext, testSigner *extJwtTestSigner, identityId string, certAuth *certAuthenticator, serviceName string) error {
	jwtStr, err := testSigner.newJwtForIdentity(identityId)
	if err != nil {
		return err
	}

	// An enrolled client trusts the controller's full CA bundle, which includes the edge
	// signing CA chain that issues router server certs, not just the ctrl-channel root.
	caPool := ctx.ControllerCaPool().Clone()
	caPool.AppendCertsFromPEM(ctx.ControllerConfig.Edge.CaPems())

	baseCreds := edge_apis.NewJwtCredentials(jwtStr)
	baseCreds.CaPool = caPool
	creds := &extJwtCredsWithFirstPartyCert{
		JwtCredentials: baseCreds,
		certs:          certAuth.certs,
		key:            certAuth.key,
	}

	clientContext, err := ziti.NewContext(&ziti.Config{
		ZtAPI:       "https://" + ctx.ApiHost + EdgeClientApiPath,
		Credentials: creds,
	})
	if err != nil {
		return err
	}
	defer clientContext.Close()

	if err = clientContext.Authenticate(); err != nil {
		return err
	}

	ctxImpl, ok := clientContext.(*ziti.ContextImpl)
	if !ok {
		return errors.New("expected *ziti.ContextImpl from ziti.NewContext")
	}
	oidcSession, ok := ctxImpl.CtrlClt.GetCurrentApiSession().(*edge_apis.ApiSessionOidc)
	if !ok {
		return errors.New("expected OIDC api session; SDK fell back to legacy, this test requires OIDC mode")
	}
	claims := &common.AccessClaims{}
	if _, _, err = jwt.NewParser().ParseUnverified(oidcSession.OidcTokens.AccessToken, claims); err != nil {
		return err
	}
	if !slices.Contains(claims.AuthenticationMethodsReferences, oidc_auth.AuthMethodExtJwt) {
		return errors.New("expected ext-jwt authentication method reference on the session")
	}
	if len(claims.CertFingerprints) > 0 {
		return errors.New("ext-jwt auth must not bind the first-party cert into the session fingerprints")
	}

	connectChan := make(chan struct{}, 1)
	clientContext.Events().AddRouterConnectedListener(func(ziti.Context, string, string) {
		select {
		case connectChan <- struct{}{}:
		default:
		}
	})
	conn, _ := clientContext.Dial(serviceName)
	defer func() {
		if conn != nil {
			_ = conn.Close()
		}
	}()

	select {
	case <-connectChan:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("router connection did not occur within 5 seconds")
	}
}

// Test_FirstPartyCert_SeparateSigningCa runs a three-controller cluster whose edge signing CA
// root is distinct from the ctrl-channel root CA. An identity authenticates via ext-jwt (so no
// cert fingerprints bind to the session) and attaches to an edge router presenting its
// first-party enrollment cert. The router must recognize the cert as first-party via the
// signing CA published in the router data model, for certs issued by the controller the router
// is subscribed to as well as by peer controllers.
func Test_FirstPartyCert_SeparateSigningCa(t *testing.T) {
	ctx := NewTestContextWithConfigSet(t, Ha3)
	defer ctx.Teardown()
	ctx.StartHaCluster(Ha3DataDir)

	managementHelper := ctx.NewEdgeManagementApi(nil)
	adminCreds := ctx.NewAdminCredentials()
	_, err := managementHelper.Authenticate(adminCreds, nil)
	ctx.Req.NoError(rest_util.WrapErr(err))
	ctx.RequireAdminManagementApiLogin()

	testService := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll("smartrouting")
	ctx.CreateEnrollAndStartEdgeRouter()

	testSigner, err := registerExtJwtSignerAllowedByDefaultPolicy(managementHelper)
	ctx.Req.NoError(err)

	t.Run("cert enrolled via the primary controller is accepted by the edge router", func(t *testing.T) {
		ctx.NextTest(t)

		identityId, certAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(eid.New(), false)

		// The enrollment cert must chain to the signing root, not the ctrl-channel root.
		ctx.Req.Equal("Controller One Edge Signing Cert", certAuth.certs[0].Issuer.CommonName)

		ctx.Req.NoError(dialServiceWithFirstPartyCert(ctx, testSigner, identityId, certAuth, testService.Name))
	})

	t.Run("cert enrolled via a peer controller is accepted by the edge router", func(t *testing.T) {
		ctx.NextTest(t)

		identityId := ctx.AdminManagementSession.requireCreateIdentityOttEnrollmentUnfinished(eid.New(), false)
		ctx.Req.NoError(ctx.waitForIdentityOnPeer(0, identityId, 5*time.Second))
		certAuth := ctx.completeOttEnrollmentAtApiHost(identityId, ctx.PeerControllerApiHosts()[0])

		// The peer controller signs with its own intermediate under the shared signing root.
		ctx.Req.Equal("Controller Two Edge Signing Cert", certAuth.certs[0].Issuer.CommonName)

		ctx.Req.NoError(dialServiceWithFirstPartyCert(ctx, testSigner, identityId, certAuth, testService.Name))
	})

	t.Run("controller store records hold full cert chains", func(t *testing.T) {
		ctx.NextTest(t)

		result, err := ctx.EdgeController.AppEnv.Managers.Controller.BaseList("true limit none")
		ctx.Req.NoError(err)
		ctx.Req.Len(result.Entities, 3)

		for _, controllerEntity := range result.Entities {
			certs := nfpem.PemStringToCertificates(controllerEntity.CertPem)
			ctx.Req.GreaterOrEqual(len(certs), 2, "controller %s certPem should hold the full chain", controllerEntity.Id)
		}
	})
}
