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

package common

import (
	"testing"
	"time"

	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestRouterDataModelDiffNestedProtoMessages guards against go-cmp panicking when Diff
// recurses into proto-message fields whose unexported fields are not ignored. Both
// DataState_Revocation.ExpiresAt (a timestamppb.Timestamp) and
// Identity.ServiceConfigs (DataState_ServiceConfigs values) are nested proto messages
// that must be covered by the diff's ignore list.
func TestRouterDataModelDiffNestedProtoMessages(t *testing.T) {
	req := require.New(t)

	newModel := func() *RouterDataModel {
		rdm := NewBareRouterDataModel("router-1")
		rdm.Revocations.Set("revocation-1", &edge_ctrl_pb.DataState_Revocation{
			Id:        "revocation-1",
			ExpiresAt: timestamppb.New(time.Unix(1700000000, 0)),
		})
		rdm.Identities.Set("identity-1", &Identity{
			Id:   "identity-1",
			Name: "identity-1",
			ServiceConfigs: map[string]*edge_ctrl_pb.DataState_ServiceConfigs{
				"service-1": {
					Configs: map[string]string{"config-1": "value-1"},
				},
			},
		})
		return rdm
	}

	model := newModel()
	correct := newModel()

	var diffs []string
	sink := func(entityType string, id string, diffType DiffType, detail string) {
		diffs = append(diffs, entityType+"/"+id+": "+detail)
	}

	req.NotPanics(func() {
		model.Validate(correct, sink)
	})
	req.Empty(diffs, "expected no diffs between identical models, got: %v", diffs)
}

// fixedTimelineSource is a TimelineIdSource that returns a constant id, used to construct
// a RouterDataModelSender in tests without standing up a real timeline.
type fixedTimelineSource struct{}

func (fixedTimelineSource) TimelineId() string { return "timeline-1" }

// TestRouterDataModelSenderDiffNestedProtoMessages is the sender-side counterpart to
// TestRouterDataModelDiffNestedProtoMessages. RouterDataModelSender.Diff walks the same
// nested proto messages (DataState_ServiceConfigs reached through SenderIdentity's embedded
// DataState_Identity, and timestamppb.Timestamp on revocations), so its ignore list must
// cover them too or go-cmp panics on their unexported fields.
func TestRouterDataModelSenderDiffNestedProtoMessages(t *testing.T) {
	req := require.New(t)

	newModel := func() *RouterDataModelSender {
		rdm := NewRouterDataModelSender(fixedTimelineSource{}, 10, 0)
		rdm.Revocations.Set("revocation-1", &edge_ctrl_pb.DataState_Revocation{
			Id:        "revocation-1",
			ExpiresAt: timestamppb.New(time.Unix(1700000000, 0)),
		})
		rdm.Identities.Set("identity-1", &SenderIdentity{
			DataStateIdentity: &edge_ctrl_pb.DataState_Identity{
				Id:   "identity-1",
				Name: "identity-1",
				ServiceConfigs: map[string]*edge_ctrl_pb.DataState_ServiceConfigs{
					"service-1": {
						Configs: map[string]string{"config-1": "value-1"},
					},
				},
			},
		})
		return rdm
	}

	model := newModel()
	correct := newModel()

	var diffs []string
	sink := func(entityType string, id string, diffType DiffType, detail string) {
		diffs = append(diffs, entityType+"/"+id+": "+detail)
	}

	req.NotPanics(func() {
		model.Validate(correct, sink)
	})
	req.Empty(diffs, "expected no diffs between identical models, got: %v", diffs)
}
