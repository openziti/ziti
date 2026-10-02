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

package command

import (
	"context"
	"testing"
	"time"

	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/controller/apierror"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/stretchr/testify/require"
)

type recordingCommand struct {
	ctx     *change.Context
	applied bool
}

func (self *recordingCommand) Apply(boltz.MutateContext) error {
	self.applied = true
	return nil
}

func (self *recordingCommand) GetChangeContext() *change.Context {
	return self.ctx
}

func (self *recordingCommand) Encode() ([]byte, error) {
	return nil, nil
}

func TestLocalDispatcherRequestDeadline(t *testing.T) {
	dispatcher := &LocalDispatcher{Limiter: NoOpRateLimiter{}}

	t.Run("refuses a command whose request deadline has passed", func(t *testing.T) {
		req := require.New(t)
		requestCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()

		cmd := &recordingCommand{ctx: change.New().SetRequestContext(requestCtx)}
		err := dispatcher.Dispatch(cmd)

		var apiErr *errorz.ApiError
		req.ErrorAs(err, &apiErr)
		req.Equal(apierror.TimeoutCode, apiErr.AppCode)
		req.False(cmd.applied)
	})

	t.Run("applies a command whose request completed before its deadline", func(t *testing.T) {
		req := require.New(t)
		requestCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		cancel()

		cmd := &recordingCommand{ctx: change.New().SetRequestContext(requestCtx)}
		req.NoError(dispatcher.Dispatch(cmd))
		req.True(cmd.applied)
	})

	t.Run("applies a command with no request context", func(t *testing.T) {
		req := require.New(t)
		cmd := &recordingCommand{ctx: change.New()}
		req.NoError(dispatcher.Dispatch(cmd))
		req.True(cmd.applied)
	})
}
