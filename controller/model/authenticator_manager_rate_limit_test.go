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
	"errors"
	"net/http"
	"testing"

	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/rate"
	"github.com/openziti/ziti/v2/controller/apierror"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/stretchr/testify/require"
)

// authEnvStub provides the two Env methods Authorize touches. Everything else inherits the
// TestContext behaviour of panicking, which keeps accidental dependencies visible.
type authEnvStub struct {
	*TestContext
	limiter  rate.AdaptiveRateLimiter
	registry AuthRegistry
}

func (self *authEnvStub) GetAuthRateLimiter() rate.AdaptiveRateLimiter {
	return self.limiter
}

func (self *authEnvStub) GetAuthRegistry() AuthRegistry {
	return self.registry
}

// countingAuthProcessor records how often the credential check ran.
type countingAuthProcessor struct {
	calls  int
	result AuthResult
	err    error
}

func (self *countingAuthProcessor) CanHandle(string) bool {
	return true
}

func (self *countingAuthProcessor) Process(AuthContext) (AuthResult, error) {
	self.calls++
	return self.result, self.err
}

type stubAuthRegistry struct {
	processor AuthProcessor
}

func (self *stubAuthRegistry) Add(AuthProcessor) {}

func (self *stubAuthRegistry) GetByMethod(string) AuthProcessor {
	return self.processor
}

// countingRateLimiter runs the work and records the call, standing in for a limiter with room.
type countingRateLimiter struct {
	calls int
}

func (self *countingRateLimiter) RunRateLimited(f func() error) (rate.RateLimitControl, error) {
	self.calls++
	return rate.NoOpRateLimitControl(), f()
}

// rejectingRateLimiter stands in for a limiter whose window is full.
type rejectingRateLimiter struct {
	calls int
}

func (self *rejectingRateLimiter) RunRateLimited(func() error) (rate.RateLimitControl, error) {
	self.calls++
	return rate.NoOpRateLimitControl(), apierror.NewTooManyUpdatesError()
}

func newAuthManagerForTest(env Env) *AuthenticatorManager {
	return &AuthenticatorManager{
		baseEntityManager: baseEntityManager[*Authenticator, *db.Authenticator]{env: env},
	}
}

func Test_Authorize_RunsCredentialCheckThroughLimiter(t *testing.T) {
	req := require.New(t)

	processor := &countingAuthProcessor{result: &AuthResultBase{}}
	limiter := &countingRateLimiter{}

	mgr := newAuthManagerForTest(&authEnvStub{
		limiter:  limiter,
		registry: &stubAuthRegistry{processor: processor},
	})

	result, err := mgr.Authorize(&AuthContextHttp{Method: AuthMethodPassword})

	req.NoError(err)
	req.NotNil(result)
	req.Equal(1, processor.calls)
	req.Equal(1, limiter.calls, "the credential check must go through the rate limiter")
}

func Test_Authorize_SkipsCredentialCheckWhenRateLimited(t *testing.T) {
	req := require.New(t)

	processor := &countingAuthProcessor{result: &AuthResultBase{}}
	limiter := &rejectingRateLimiter{}

	mgr := newAuthManagerForTest(&authEnvStub{
		limiter:  limiter,
		registry: &stubAuthRegistry{processor: processor},
	})

	result, err := mgr.Authorize(&AuthContextHttp{Method: AuthMethodPassword})

	req.Error(err)
	req.Nil(result)
	req.Equal(0, processor.calls, "no argon2 work may run once the limiter is full")

	apiErr := &errorz.ApiError{}
	req.True(errors.As(err, &apiErr))
	req.Equal(http.StatusTooManyRequests, apiErr.Status)
}

func Test_Authorize_PropagatesModuleError(t *testing.T) {
	req := require.New(t)

	expected := errors.New("bad credentials")
	processor := &countingAuthProcessor{err: expected}

	mgr := newAuthManagerForTest(&authEnvStub{
		limiter:  &countingRateLimiter{},
		registry: &stubAuthRegistry{processor: processor},
	})

	result, err := mgr.Authorize(&AuthContextHttp{Method: AuthMethodPassword})

	req.ErrorIs(err, expected)
	req.Nil(result)
}

func Test_Authorize_UnknownMethodTakesNoSlot(t *testing.T) {
	req := require.New(t)

	limiter := &countingRateLimiter{}

	mgr := newAuthManagerForTest(&authEnvStub{
		limiter:  limiter,
		registry: &stubAuthRegistry{},
	})

	result, err := mgr.Authorize(&AuthContextHttp{Method: "nope"})

	req.Error(err)
	req.Nil(result)
	req.Equal(0, limiter.calls, "an unknown method is rejected before taking a limiter slot")
}
