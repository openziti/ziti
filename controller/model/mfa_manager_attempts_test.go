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
	"encoding/base32"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/dgryski/dgoogauth"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/controller/models"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
)

const testMfaId = "test-mfa"

// newMfaManagerForAttemptTest builds a manager with only the failed attempt state. Code
// checking and the attempt limit never touch the store, so nothing else is needed.
func newMfaManagerForAttemptTest() *MfaManager {
	return &MfaManager{
		failedAttempts: cmap.New[*totpFailedAttempts](),
	}
}

func newTestMfa() *Mfa {
	return &Mfa{
		BaseEntity:    models.BaseEntity{Id: testMfaId},
		Secret:        base32.StdEncoding.EncodeToString([]byte("0123456789")),
		RecoveryCodes: []string{"11111111"},
	}
}

// currentTotpCode returns a code the manager accepts right now.
func currentTotpCode(mfa *Mfa) string {
	return fmt.Sprintf("%06d", dgoogauth.ComputeCode(mfa.Secret, time.Now().UTC().Unix()/30))
}

func requireTooManyAttempts(req *require.Assertions, err error) {
	req.Error(err)

	apiErr := &errorz.ApiError{}
	req.True(errors.As(err, &apiErr))
	req.Equal(http.StatusTooManyRequests, apiErr.Status)
}

func Test_VerifyTOTP_RefusesAfterTooManyFailures(t *testing.T) {
	req := require.New(t)

	mgr := newMfaManagerForAttemptTest()
	mfa := newTestMfa()

	for i := 0; i < TotpMaxFailedAttempts; i++ {
		ok, err := mgr.VerifyTOTP(mfa, "000000")
		req.NoError(err, "attempt %d must be answered as a plain failure", i+1)
		req.False(ok)
	}

	ok, err := mgr.VerifyTOTP(mfa, "000000")
	req.False(ok)
	requireTooManyAttempts(req, err)
}

func Test_VerifyTOTP_MalformedCodesCountAsFailures(t *testing.T) {
	req := require.New(t)

	mgr := newMfaManagerForAttemptTest()
	mfa := newTestMfa()

	for i := 0; i < TotpMaxFailedAttempts; i++ {
		ok, err := mgr.VerifyTOTP(mfa, "nope")
		req.NoError(err)
		req.False(ok)
	}

	ok, err := mgr.VerifyTOTP(mfa, "000000")
	req.False(ok)
	requireTooManyAttempts(req, err)
}

func Test_Verify_RefusesRecoveryCodesOnceLimitIsReached(t *testing.T) {
	req := require.New(t)

	mgr := newMfaManagerForAttemptTest()
	mfa := newTestMfa()

	for i := 0; i < TotpMaxFailedAttempts; i++ {
		_, err := mgr.VerifyTOTP(mfa, "000000")
		req.NoError(err)
	}

	// The valid recovery code must not get through either, otherwise the limit only covers
	// one of the two ways to answer the challenge.
	ok, err := mgr.Verify(mfa, "11111111", nil)
	req.False(ok)
	requireTooManyAttempts(req, err)
}

func Test_VerifyTOTP_CorrectCodeClearsTheCount(t *testing.T) {
	req := require.New(t)

	mgr := newMfaManagerForAttemptTest()
	mfa := newTestMfa()

	for i := 0; i < TotpMaxFailedAttempts-1; i++ {
		_, err := mgr.VerifyTOTP(mfa, "000000")
		req.NoError(err)
	}

	ok, err := mgr.VerifyTOTP(mfa, currentTotpCode(mfa))
	req.NoError(err)
	req.True(ok)

	req.False(mgr.failedAttempts.Has(testMfaId))

	// A user who mistyped and then got it right starts from a clean count.
	ok, err = mgr.VerifyTOTP(mfa, "000000")
	req.NoError(err)
	req.False(ok)
}

func Test_VerifyTOTP_ReleasedAfterWindowPasses(t *testing.T) {
	req := require.New(t)

	mgr := newMfaManagerForAttemptTest()
	mfa := newTestMfa()

	for i := 0; i < TotpMaxFailedAttempts; i++ {
		_, err := mgr.VerifyTOTP(mfa, "000000")
		req.NoError(err)
	}

	_, err := mgr.VerifyTOTP(mfa, "000000")
	requireTooManyAttempts(req, err)

	attempts, found := mgr.failedAttempts.Get(testMfaId)
	req.True(found)
	attempts.windowEnd = time.Now().Add(-time.Second)

	ok, err := mgr.VerifyTOTP(mfa, currentTotpCode(mfa))
	req.NoError(err)
	req.True(ok)
}

func Test_VerifyTOTP_LimitIsPerMfaRecord(t *testing.T) {
	req := require.New(t)

	mgr := newMfaManagerForAttemptTest()
	mfa := newTestMfa()

	other := newTestMfa()
	other.Id = "other-mfa"

	for i := 0; i < TotpMaxFailedAttempts; i++ {
		_, err := mgr.VerifyTOTP(mfa, "000000")
		req.NoError(err)
	}

	_, err := mgr.VerifyTOTP(other, "000000")
	req.NoError(err, "one identity's failures must not hold another identity")
}
