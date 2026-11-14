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

package model

import (
	"crypto/x509"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_EnrollModuleCa_getIdentityName(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	module := NewEnrollModuleCa(ctx)
	ca := &Ca{IdentityNameFormat: FormatSentinelStart + FormatSymbolRequestedName + FormatSentinelEnd}
	existing := ctx.requireNewIdentity(false)

	// getIdentityName loops until it finds a free name, so a name that is never free must not hang the test
	getIdentityName := func(t *testing.T, requestedName string) string {
		type result struct {
			name string
			err  error
		}
		resultC := make(chan result, 1)

		go func() {
			name, err := module.getIdentityName(ca, &x509.Certificate{}, requestedName, "identity-id")
			resultC <- result{name: name, err: err}
		}()

		select {
		case r := <-resultC:
			require.NoError(t, r.err)
			return r.name
		case <-time.After(10 * time.Second):
			require.FailNow(t, "getIdentityName did not return")
			return ""
		}
	}

	t.Run("a free name is used as is", func(t *testing.T) {
		require.Equal(t, "free-name", getIdentityName(t, "free-name"))
	})

	t.Run("a taken name gets a suffix", func(t *testing.T) {
		require.Equal(t, existing.Name+"000001", getIdentityName(t, existing.Name))
	})

	t.Run("a name with filter syntax is only compared as a name", func(t *testing.T) {
		requestedName := `x" or name != "zz`
		require.Equal(t, requestedName, getIdentityName(t, requestedName))
	})
}
