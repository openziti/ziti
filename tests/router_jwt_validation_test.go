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
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/openziti/ziti/common"
)

// Test_Router_ParseJwt_TokenValidation boots a controller and edge router, then drives the
// router's ParseJwt through its real public-key lookup (keys distributed via the RDM). It
// confirms a correctly signed, unexpired access token is accepted and that tokens signed by an
// unknown key or carrying an expired claim are rejected.
func Test_Router_ParseJwt_TokenValidation(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	routerHelper := ctx.CreateEnrollAndStartEdgeRouter()
	ctx.Req.True(routerHelper.WaitForRouterSync(30*time.Second), "router did not sync its data model")

	stateManager := routerHelper.GetStateManager()

	// the controller signs access tokens with this signer; the matching public key is distributed
	// to the router via the RDM keyed by the same kid, so a token minted here is recognizable.
	signer := ctx.EdgeController.AppEnv.GetRootTlsJwtSigner()

	now := time.Now()

	// claims are built as maps so the z_t (token type) claim is serialized directly; AccessClaims
	// itself has no MarshalJSON and would drop its embedded custom claims.

	// a properly signed, unexpired access token is accepted
	validToken, err := signer.Generate(jwt.MapClaims{
		"sub": uuid.NewString(),
		"aud": []string{common.ClaimAudienceOpenZiti},
		"exp": now.Add(30 * time.Minute).Unix(),
		"iat": now.Unix(),
		"nbf": now.Unix(),
		"z_t": common.TokenTypeAccess,
	})
	ctx.Req.NoError(err)

	jwtToken, accessClaims, err := stateManager.ParseJwt(validToken)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(jwtToken)
	ctx.Req.True(jwtToken.Valid)
	ctx.Req.Equal(common.TokenTypeAccess, accessClaims.Type)

	// a token signed by an unknown key is rejected; the controller's kid is reused so pubKeyLookup
	// resolves the real public key, against which the forged signature fails to verify.
	wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
	ctx.Req.NoError(err)

	forged := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": uuid.NewString(),
		"aud": []string{common.ClaimAudienceOpenZiti},
		"exp": now.Add(30 * time.Minute).Unix(),
		"iat": now.Unix(),
		"nbf": now.Unix(),
		"z_t": common.TokenTypeAccess,
	})
	forged.Header["kid"] = signer.KeyId()
	wrongKeyToken, err := forged.SignedString(wrongKey)
	ctx.Req.NoError(err)

	_, _, err = stateManager.ParseJwt(wrongKeyToken)
	ctx.Req.Error(err)

	// an expired but correctly signed access token is rejected
	expiredToken, err := signer.Generate(jwt.MapClaims{
		"sub": uuid.NewString(),
		"aud": []string{common.ClaimAudienceOpenZiti},
		"exp": now.Add(-1 * time.Minute).Unix(),
		"iat": now.Add(-30 * time.Minute).Unix(),
		"nbf": now.Add(-30 * time.Minute).Unix(),
		"z_t": common.TokenTypeAccess,
	})
	ctx.Req.NoError(err)

	_, _, err = stateManager.ParseJwt(expiredToken)
	ctx.Req.Error(err)
}
