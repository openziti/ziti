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

package oidc_auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

// testTokenExchangeRequest provides the parts of op.TokenExchangeRequest that
// ValidateTokenExchangeRequest inspects.
type testTokenExchangeRequest struct {
	op.TokenExchangeRequest
	subjectTokenType   oidc.TokenType
	requestedTokenType oidc.TokenType
}

func (r *testTokenExchangeRequest) GetExchangeSubjectTokenType() oidc.TokenType {
	return r.subjectTokenType
}

func (r *testTokenExchangeRequest) GetRequestedTokenType() oidc.TokenType {
	return r.requestedTokenType
}

func (r *testTokenExchangeRequest) SetRequestedTokenType(tt oidc.TokenType) {
	r.requestedTokenType = tt
}

func (r *testTokenExchangeRequest) GetAudience() []string {
	return nil
}

func (r *testTokenExchangeRequest) GetScopes() []string {
	return []string{oidc.ScopeOpenID, oidc.ScopeOfflineAccess}
}

func Test_ValidateTokenExchangeRequest_TokenTypes(t *testing.T) {
	tests := []struct {
		name          string
		subjectType   oidc.TokenType
		requestedType oidc.TokenType
		expectedType  oidc.TokenType
		expectErr     bool
	}{
		{name: "access for default", subjectType: oidc.AccessTokenType, expectedType: oidc.AccessTokenType},
		{name: "access for access", subjectType: oidc.AccessTokenType, requestedType: oidc.AccessTokenType, expectedType: oidc.AccessTokenType},
		{name: "access for id", subjectType: oidc.AccessTokenType, requestedType: oidc.IDTokenType, expectedType: oidc.IDTokenType},
		{name: "access for refresh", subjectType: oidc.AccessTokenType, requestedType: oidc.RefreshTokenType, expectErr: true},
		{name: "access for jwt", subjectType: oidc.AccessTokenType, requestedType: oidc.JWTTokenType, expectErr: true},
		{name: "refresh for default", subjectType: oidc.RefreshTokenType, expectedType: oidc.RefreshTokenType},
		{name: "refresh for refresh", subjectType: oidc.RefreshTokenType, requestedType: oidc.RefreshTokenType, expectedType: oidc.RefreshTokenType},
		{name: "refresh for access", subjectType: oidc.RefreshTokenType, requestedType: oidc.AccessTokenType, expectedType: oidc.AccessTokenType},
		{name: "refresh for id", subjectType: oidc.RefreshTokenType, requestedType: oidc.IDTokenType, expectedType: oidc.IDTokenType},
		{name: "refresh for jwt", subjectType: oidc.RefreshTokenType, requestedType: oidc.JWTTokenType, expectErr: true},
		{name: "id for default", subjectType: oidc.IDTokenType, expectErr: true},
		{name: "id for access", subjectType: oidc.IDTokenType, requestedType: oidc.AccessTokenType, expectErr: true},
		{name: "id for refresh", subjectType: oidc.IDTokenType, requestedType: oidc.RefreshTokenType, expectErr: true},
		{name: "id for id", subjectType: oidc.IDTokenType, requestedType: oidc.IDTokenType, expectErr: true},
		{name: "jwt for access", subjectType: oidc.JWTTokenType, requestedType: oidc.AccessTokenType, expectErr: true},
	}

	storage := &HybridStorage{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &testTokenExchangeRequest{
				subjectTokenType:   tt.subjectType,
				requestedTokenType: tt.requestedType,
			}

			err := storage.ValidateTokenExchangeRequest(context.Background(), request)

			if tt.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectedType, request.GetRequestedTokenType())
		})
	}
}
