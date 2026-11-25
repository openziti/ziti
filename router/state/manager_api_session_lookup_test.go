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

package state

import (
	"testing"
	"time"

	"github.com/openziti/ziti/v2/common"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
)

// newLookupTestManager builds a ManagerImpl with just enough state for api session token
// lookups: an empty legacy token cache and an empty router data model, so JWT signer
// lookups find no key and fail verification.
func newLookupTestManager() *ManagerImpl {
	mgr := &ManagerImpl{
		legacyApiSessionsByToken: cmap.New[*ApiSessionToken](),
	}
	mgr.routerDataModel.Store(common.NewBareRouterDataModel(""))
	return mgr
}

func Test_GetApiSessionTokenWithTimeout_JwtFailsWithoutRetrying(t *testing.T) {
	req := require.New(t)
	mgr := newLookupTestManager()

	// Shaped like a JWT so it takes the JWT branch, and unverifiable because the data model
	// holds no signing keys.
	token := "eyJhbGciOiJSUzI1NiIsImtpZCI6Im5vcGUifQ.e30.c2ln"

	start := time.Now()
	result := mgr.GetApiSessionTokenWithTimeout(token, 5*time.Second)
	elapsed := time.Since(start)

	req.Nil(result)
	req.Less(elapsed, time.Second, "a JWT that fails verification must be rejected on the first attempt, took %v", elapsed)
}

func Test_GetApiSessionTokenWithTimeout_LegacyTokenStillWaits(t *testing.T) {
	req := require.New(t)
	mgr := newLookupTestManager()

	timeout := 100 * time.Millisecond

	start := time.Now()
	result := mgr.GetApiSessionTokenWithTimeout("legacy-token-not-in-cache", timeout)
	elapsed := time.Since(start)

	req.Nil(result)
	req.GreaterOrEqual(elapsed, timeout, "legacy tokens must keep the retry window for controller sync")
}

func Test_GetApiSessionTokenWithTimeout_LegacyTokenFoundInCache(t *testing.T) {
	req := require.New(t)
	mgr := newLookupTestManager()

	expected := &ApiSessionToken{Type: ApiSessionTokenLegacyTokenOnly}
	mgr.legacyApiSessionsByToken.Set("legacy-token", expected)

	result := mgr.GetApiSessionTokenWithTimeout("legacy-token", 5*time.Second)

	req.Same(expected, result)
}
