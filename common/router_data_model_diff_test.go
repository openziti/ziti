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
		rdm := NewBareRouterDataModel()
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

// TestRouterDataModelDiffIgnoresRevocationTimestamps verifies that a revocation's
// ExpiresAt/IssuedBefore are ignored by the diff (they're derived per-node and
// legitimately differ across HA controllers), while the revocation's presence and
// type are still validated.
func TestRouterDataModelDiffIgnoresRevocationTimestamps(t *testing.T) {
	newModel := func() *RouterDataModel {
		rdm := NewBareRouterDataModel()
		rdm.Revocations.Set("revocation-1", &edge_ctrl_pb.DataState_Revocation{
			Id:           "revocation-1",
			Type:         "IDENTITY",
			ExpiresAt:    timestamppb.New(time.Unix(1700000000, 0)),
			IssuedBefore: timestamppb.New(time.Unix(1700000000, 0)),
		})
		return rdm
	}

	collectDiffs := func(model, correct *RouterDataModel) []string {
		var diffs []string
		model.Validate(correct, func(entityType string, id string, diffType DiffType, detail string) {
			diffs = append(diffs, entityType+"/"+id+": "+detail)
		})
		return diffs
	}

	t.Run("differing timestamps produce no diff", func(t *testing.T) {
		req := require.New(t)
		correct := newModel()
		correct.Revocations.Set("revocation-1", &edge_ctrl_pb.DataState_Revocation{
			Id:           "revocation-1",
			Type:         "IDENTITY",
			ExpiresAt:    timestamppb.New(time.Unix(1700000042, 123)),
			IssuedBefore: timestamppb.New(time.Unix(1700000042, 123)),
		})
		diffs := collectDiffs(newModel(), correct)
		req.Empty(diffs, "expected no diffs when only timestamps differ, got: %v", diffs)
	})

	t.Run("a missing revocation is still reported", func(t *testing.T) {
		req := require.New(t)
		diffs := collectDiffs(newModel(), NewBareRouterDataModel())
		req.NotEmpty(diffs, "expected a diff when a revocation is missing")
	})

	t.Run("a changed type is still reported", func(t *testing.T) {
		req := require.New(t)
		correct := newModel()
		correct.Revocations.Set("revocation-1", &edge_ctrl_pb.DataState_Revocation{
			Id:           "revocation-1",
			Type:         "API_SESSION",
			ExpiresAt:    timestamppb.New(time.Unix(1700000000, 0)),
			IssuedBefore: timestamppb.New(time.Unix(1700000000, 0)),
		})
		diffs := collectDiffs(newModel(), correct)
		req.NotEmpty(diffs, "expected a diff when the revocation type changes")
	})
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
