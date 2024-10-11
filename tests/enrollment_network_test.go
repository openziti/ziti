//go:build apitests
// +build apitests

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
	"github.com/golang-jwt/jwt/v5"
	enrollment_client "github.com/openziti/edge-api/rest_client_api_client/enrollment"
	enrollment_management "github.com/openziti/edge-api/rest_management_api_client/enrollment"
	"github.com/openziti/sdk-golang/ziti"
	"testing"
)

func Test_EnrollmentNetwork(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	t.Run("can query network JWT on client API", func(t *testing.T) {
		ctx.testContextChanged(t)

		edgeClient := ctx.NewEdgeClientApi(nil)
		params := enrollment_client.NewListNetworkJWTsParams()
		resp, err := edgeClient.API.Enrollment.ListNetworkJWTs(params)

		ctx.NoError(err)
		ctx.NotNil(resp)
		ctx.NotNil(resp.Payload)
		ctx.Len(resp.Payload.Data, 1)
		ctx.NotNil(resp.Payload.Data[0].Name)
		ctx.Equal("default", *resp.Payload.Data[0].Name)
		ctx.NotNil(resp.Payload.Data[0].Token)
		ctx.NotEmpty(*resp.Payload.Data[0].Token)

		t.Run("the token has the correct values", func(t *testing.T) {
			ctx.testContextChanged(t)

			parser := jwt.NewParser()

			claims := ziti.EnrollmentClaims{}
			token, err := parser.ParseWithClaims(*resp.Payload.Data[0].Token, &claims, ctx.EdgeController.AppEnv.JwtSignerKeyFunc)

			ctx.NoError(err)
			ctx.True(token.Valid)
			ctx.Equal("network", claims.EnrollmentMethod)
			ctx.Equal("https://127.0.0.1:1281/", claims.Issuer)
			ctx.Equal(claims.Issuer, claims.Subject)
		})
	})

	t.Run("can query network JWT on management API", func(t *testing.T) {
		ctx.testContextChanged(t)

		edgeManagement := ctx.NewEdgeManagementApi(nil)
		params := enrollment_management.NewListNetworkJWTsParams()
		resp, err := edgeManagement.API.Enrollment.ListNetworkJWTs(params)

		ctx.NoError(err)
		ctx.NotNil(resp)
		ctx.NotNil(resp.Payload)
		ctx.Len(resp.Payload.Data, 1)
		ctx.NotNil(resp.Payload.Data[0].Name)
		ctx.Equal("default", *resp.Payload.Data[0].Name)
		ctx.NotNil(resp.Payload.Data[0].Token)
		ctx.NotEmpty(*resp.Payload.Data[0].Token)

		t.Run("the token has the correct values", func(t *testing.T) {
			ctx.testContextChanged(t)

			parser := jwt.NewParser()

			claims := ziti.EnrollmentClaims{}
			token, err := parser.ParseWithClaims(*resp.Payload.Data[0].Token, &claims, ctx.EdgeController.AppEnv.JwtSignerKeyFunc)

			ctx.NoError(err)
			ctx.True(token.Valid)
			ctx.Equal("network", claims.EnrollmentMethod)
			ctx.Equal("https://127.0.0.1:1281/", claims.Issuer)
			ctx.Equal(claims.Issuer, claims.Subject)
		})
	})
}
