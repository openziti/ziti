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

package util

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/stretchr/testify/require"
)

// makeJwtWithExp builds a signed JWT carrying only an exp claim. The token helpers parse it
// unverified, so the signing key is irrelevant.
func makeJwtWithExp(t *testing.T, exp time.Time) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(exp),
	})
	s, err := tok.SignedString([]byte("test-signing-key"))
	require.NoError(t, err)
	return s
}

func TestOidcAccessTokenExpired(t *testing.T) {
	now := time.Now()

	t.Run("non-oidc session is never treated as expired", func(t *testing.T) {
		legacy := edge_apis.NewApiSessionLegacy("zt-session-token")
		require.False(t, OidcAccessTokenExpired(legacy))
	})

	t.Run("token expiring well in the future is not expired", func(t *testing.T) {
		sess := edge_apis.NewApiSessionOidc(makeJwtWithExp(t, now.Add(time.Hour)), "refresh")
		require.False(t, OidcAccessTokenExpired(sess))
	})

	t.Run("already expired token is expired", func(t *testing.T) {
		sess := edge_apis.NewApiSessionOidc(makeJwtWithExp(t, now.Add(-time.Hour)), "refresh")
		require.True(t, OidcAccessTokenExpired(sess))
	})

	t.Run("token expiring inside the leeway window is treated as expired", func(t *testing.T) {
		sess := edge_apis.NewApiSessionOidc(makeJwtWithExp(t, now.Add(5*time.Second)), "refresh")
		require.True(t, OidcAccessTokenExpired(sess))
	})

	t.Run("token expiring just past the leeway window is not expired", func(t *testing.T) {
		sess := edge_apis.NewApiSessionOidc(makeJwtWithExp(t, now.Add(oidcTokenLeeway+time.Minute)), "refresh")
		require.False(t, OidcAccessTokenExpired(sess))
	})

	t.Run("unreadable token is treated as expired so the caller refreshes", func(t *testing.T) {
		sess := edge_apis.NewApiSessionOidc("not-a-jwt", "refresh")
		require.True(t, OidcAccessTokenExpired(sess))
	})
}

func TestOidcRefreshTokenValid(t *testing.T) {
	now := time.Now()

	t.Run("non-oidc session has no refresh token", func(t *testing.T) {
		legacy := edge_apis.NewApiSessionLegacy("zt-session-token")
		require.False(t, OidcRefreshTokenValid(legacy))
	})

	t.Run("missing refresh token is invalid", func(t *testing.T) {
		sess := edge_apis.NewApiSessionOidc(makeJwtWithExp(t, now.Add(time.Hour)), "")
		require.False(t, OidcRefreshTokenValid(sess))
	})

	t.Run("unreadable refresh token is invalid", func(t *testing.T) {
		sess := edge_apis.NewApiSessionOidc(makeJwtWithExp(t, now.Add(time.Hour)), "not-a-jwt")
		require.False(t, OidcRefreshTokenValid(sess))
	})

	t.Run("expired refresh token is invalid", func(t *testing.T) {
		sess := edge_apis.NewApiSessionOidc(makeJwtWithExp(t, now.Add(-time.Hour)), makeJwtWithExp(t, now.Add(-time.Minute)))
		require.False(t, OidcRefreshTokenValid(sess))
	})

	t.Run("refresh token expiring inside the leeway window is invalid", func(t *testing.T) {
		sess := edge_apis.NewApiSessionOidc(makeJwtWithExp(t, now.Add(-time.Hour)), makeJwtWithExp(t, now.Add(5*time.Second)))
		require.False(t, OidcRefreshTokenValid(sess))
	})

	t.Run("unexpired refresh token is valid", func(t *testing.T) {
		sess := edge_apis.NewApiSessionOidc(makeJwtWithExp(t, now.Add(-time.Hour)), makeJwtWithExp(t, now.Add(time.Hour)))
		require.True(t, OidcRefreshTokenValid(sess))
	})
}
