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

package handler_edge_ctrl

import (
	"fmt"
	"testing"

	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/apierror"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func Test_ErrorsIs(t *testing.T) {
	err := error(InvalidSessionError{})
	req := require.New(t)
	req.True(errors.Is(err, InvalidSessionError{}))
}

// These three tests come from a real failure. On a cold HA start, a router tried to create its
// tunnel terminator before the controller had elected a raft leader. The controller returned
// CLUSTER_NO_LEADER, internalError classified it as FailedOther (not retryable), and the router
// gave up. The terminator was never created, so every dial to that service failed with "service
// has no terminators" - permanently, even after a leader was elected seconds later. The fix is to
// treat CLUSTER_NO_LEADER as retryable (FailedBusy) so the router backs off and tries again. These
// tests lock in that classification: the no-leader case retries, a wrapped no-leader case still
// retries, and an unrelated error does not.

// A "cluster has no leader" condition is transient and should be retryable.
// internalError must map it to FailedBusy so routers retry instead of giving up.
func Test_internalError_clusterNoLeaderIsRetryable(t *testing.T) {
	req := require.New(t)

	ctrlErr := internalError(apierror.NewClusterHasNoLeaderError())

	req.Equal(edge.RetryTooBusy, ctrlErr.GetRetryHint())
	req.Equal(edge_ctrl_pb.CreateTerminatorResult_FailedBusy, retryHintToResult(ctrlErr.GetRetryHint()))
}

// internalError uses errors.As to find the no-leader error, which means it keeps working even
// when that error is wrapped (e.g. fmt.Errorf("...: %w", err)). Today's terminator path passes
// the error unwrapped, but a caller could wrap it, so this test pins the wrapped case to
// FailedBusy too. If someone changes internalError to a direct type check, this test fails.
func Test_internalError_wrappedClusterNoLeaderIsRetryable(t *testing.T) {
	req := require.New(t)

	wrapped := fmt.Errorf("could not create terminator: %w", apierror.NewClusterHasNoLeaderError())
	ctrlErr := internalError(wrapped)

	req.Equal(edge_ctrl_pb.CreateTerminatorResult_FailedBusy, retryHintToResult(ctrlErr.GetRetryHint()))
}

// Only the no-leader error should be retryable. A generic error must map to FailedOther so the
// router gives up instead of retrying forever. If someone makes internalError treat all errors
// as retryable, this test catches it.
func Test_internalError_genericErrorIsNotBusy(t *testing.T) {
	req := require.New(t)

	ctrlErr := internalError(errors.New("some unexpected failure"))

	req.Equal(edge_ctrl_pb.CreateTerminatorResult_FailedOther, retryHintToResult(ctrlErr.GetRetryHint()))
}
