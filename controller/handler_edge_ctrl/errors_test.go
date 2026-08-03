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
	"testing"

	"github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func Test_ErrorsIs(t *testing.T) {
	err := error(InvalidSessionError{})
	req := require.New(t)
	req.True(errors.Is(err, InvalidSessionError{}))
}

// TestNewEdgeRouterAccessDeniedError checks that the denial names the specific policy that is
// missing for each way access can be denied, and that all three are reported as access denied.
func TestNewEdgeRouterAccessDeniedError(t *testing.T) {
	ctx := &baseSessionRequestContext{
		sourceRouter: &model.Router{Name: "er1"},
		service:      &model.EdgeService{Name: "svc1"},
	}

	tests := []struct {
		name           string
		access         model.EdgeRouterAccess
		expectedReason string
	}{
		{
			name:           "neither policy links",
			access:         model.EdgeRouterAccess{IdentityAllowed: false, ServiceAllowed: false},
			expectedReason: "no edge router policy links the identity to the edge router and no service edge router policy links the service to the edge router",
		},
		{
			name:           "only service linked",
			access:         model.EdgeRouterAccess{IdentityAllowed: false, ServiceAllowed: true},
			expectedReason: "no edge router policy links the identity to the edge router",
		},
		{
			name:           "only identity linked",
			access:         model.EdgeRouterAccess{IdentityAllowed: true, ServiceAllowed: false},
			expectedReason: "no service edge router policy links the service to the edge router",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := require.New(t)

			err := ctx.newEdgeRouterAccessDeniedError("identity1", test.access)
			req.Equal(edge.ErrorCodeAccessDenied, err.ErrorCode())
			req.Contains(err.Error(), test.expectedReason)
			req.Contains(err.Error(), "er1")
			req.Contains(err.Error(), "svc1")
			req.Contains(err.Error(), "identity1")
		})
	}
}
