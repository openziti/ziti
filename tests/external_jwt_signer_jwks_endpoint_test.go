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
	"testing"

	"github.com/go-openapi/strfmt"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/common/eid"
)

// Test_ExternalJwtSigner_JwksEndpoint covers the create-time check on an external JWT signer's
// jwksEndpoint. The controller fetches that URL itself, so an endpoint that the configured
// fetch policy would refuse is rejected up front rather than failing later during a fetch.
//
// A model-layer field error surfaces as COULD_NOT_VALIDATE rather than INVALID_FIELD: the
// responder rewrites the code when the cause is a field error, and the offending field and
// reason are carried in the cause.
func Test_ExternalJwtSigner_JwksEndpoint(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	adminManClient := ctx.NewEdgeManagementApi(nil)
	adminManApiSession, err := adminManClient.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(adminManApiSession)

	t.Run("an endpoint pointing at the instance metadata service is rejected", func(t *testing.T) {
		ctx.testContextChanged(t)

		detail, err := adminManClient.CreateExtJwtSigner(newJwksSignerCreate("http://169.254.169.254/latest/meta-data/"))

		ctx.Req.Error(err, "the metadata service must never be fetchable, regardless of configuration")
		ctx.Req.ApiErrorWithCode(err, errorz.CouldNotValidateCode)
		ctx.Req.Nil(detail)
	})

	t.Run("an endpoint with a non-http scheme is rejected", func(t *testing.T) {
		ctx.testContextChanged(t)

		detail, err := adminManClient.CreateExtJwtSigner(newJwksSignerCreate("file:///etc/passwd"))

		ctx.Req.Error(err)
		ctx.Req.ApiErrorWithCode(err, errorz.CouldNotValidateCode)
		ctx.Req.Nil(detail)
	})

	t.Run("an endpoint with a hostname is accepted", func(t *testing.T) {
		ctx.testContextChanged(t)

		// no name resolution happens at create time, the dial-time check covers a hostname
		// that resolves to a blocked address
		detail, err := adminManClient.CreateExtJwtSigner(newJwksSignerCreate("https://idp.example.com/.well-known/jwks.json"))

		ctx.Req.NoError(err)
		ctx.Req.NotNil(detail)
	})
}

// newJwksSignerCreate returns a uniquely named external JWT signer create payload that uses
// the given jwksEndpoint.
func newJwksSignerCreate(jwksEndpoint string) *rest_model.ExternalJWTSignerCreate {
	name := eid.New()
	endpoint := strfmt.URI(jwksEndpoint)

	return &rest_model.ExternalJWTSignerCreate{
		Name:         ToPtr(name),
		Enabled:      ToPtr(true),
		Issuer:       ToPtr(name + "-issuer"),
		Audience:     ToPtr(name + "-audience"),
		JwksEndpoint: &endpoint,
	}
}
