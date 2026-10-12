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
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/openziti/foundation/v2/errorz"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
)

func Test_HybridStorage_GetAuthRequest(t *testing.T) {
	t.Run("returns an auth request before it expires", func(t *testing.T) {
		req := require.New(t)
		store := &HybridStorage{authRequests: cmap.New[*AuthRequest](), codes: cmap.New[string]()}
		store.authRequests.Set("live", &AuthRequest{Id: "live", ExpiresAt: time.Now().Add(time.Minute)})

		authRequest, err := store.GetAuthRequest("live")

		req.NoError(err)
		req.Equal("live", authRequest.Id)
	})

	t.Run("returns 401 UNAUTHORIZED for an expired auth request", func(t *testing.T) {
		req := require.New(t)
		store := &HybridStorage{authRequests: cmap.New[*AuthRequest](), codes: cmap.New[string]()}
		store.authRequests.Set("expired", &AuthRequest{Id: "expired", ExpiresAt: time.Now().Add(-time.Second)})

		authRequest, err := store.GetAuthRequest("expired")

		req.Nil(authRequest)
		apiErr := &errorz.ApiError{}
		req.True(errors.As(err, &apiErr), "expected an ApiError, got %v", err)
		req.Equal(errorz.UnauthorizedCode, apiErr.AppCode)
		req.Equal(http.StatusUnauthorized, apiErr.Status)
	})

	t.Run("returns 401 UNAUTHORIZED for an unknown auth request id", func(t *testing.T) {
		req := require.New(t)
		store := &HybridStorage{authRequests: cmap.New[*AuthRequest](), codes: cmap.New[string]()}

		authRequest, err := store.GetAuthRequest("unknown")

		req.Nil(authRequest)
		apiErr := &errorz.ApiError{}
		req.True(errors.As(err, &apiErr), "expected an ApiError, got %v", err)
		req.Equal(errorz.UnauthorizedCode, apiErr.AppCode)
		req.Equal(http.StatusUnauthorized, apiErr.Status)
	})
}

func Test_HybridStorage_Clean(t *testing.T) {
	t.Run("removes expired auth requests and their codes and keeps the rest", func(t *testing.T) {
		req := require.New(t)
		store := &HybridStorage{authRequests: cmap.New[*AuthRequest](), codes: cmap.New[string]()}
		store.authRequests.Set("expired", &AuthRequest{
			Id:           "expired",
			CreationDate: time.Now(),
			ExpiresAt:    time.Now().Add(-time.Second),
		})
		store.authRequests.Set("live", &AuthRequest{
			Id:           "live",
			CreationDate: time.Now().Add(-time.Hour),
			ExpiresAt:    time.Now().Add(time.Minute),
		})
		store.codes.Set("expired-code", "expired")
		store.codes.Set("live-code", "live")

		store.Clean()

		req.False(store.authRequests.Has("expired"), "expired auth request should be removed")
		req.False(store.codes.Has("expired-code"), "code for an expired auth request should be removed")
		req.True(store.authRequests.Has("live"), "auth request is kept until ExpiresAt regardless of CreationDate")
		req.True(store.codes.Has("live-code"))
	})
}
