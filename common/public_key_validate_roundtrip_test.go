package common

import (
	"testing"

	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// Test_PublicKeyIntermediatesSurviveValidateRoundTrip mirrors the router-data-model validate
// flow: a router's live model and a bare model rebuilt from a proto-round-tripped controller
// snapshot must agree on a public key that carries intermediates.
func Test_PublicKeyIntermediatesSurviveValidateRoundTrip(t *testing.T) {
	req := require.New(t)

	key := &edge_ctrl_pb.DataState_PublicKey{
		Kid:    "kid1",
		Data:   []byte("anchor-der"),
		Format: edge_ctrl_pb.DataState_PublicKey_X509CertDer,
		Usages: []edge_ctrl_pb.DataState_PublicKey_Usage{
			edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation,
			edge_ctrl_pb.DataState_PublicKey_FirstPartyX509CertValidation,
		},
		Intermediates: [][]byte{[]byte("intermediate-der")},
	}
	evt := &edge_ctrl_pb.DataState_Event{
		Action:      edge_ctrl_pb.DataState_Create,
		Model:       &edge_ctrl_pb.DataState_Event_PublicKey{PublicKey: key},
		IsSynthetic: true,
	}

	current := NewBareRouterDataModel("r1")
	current.WhileLocked(func(u uint64) {
		current.Handle(1, evt)
		current.SetCurrentIndex(1)
	})

	state := &edge_ctrl_pb.DataState{Events: []*edge_ctrl_pb.DataState_Event{evt}, EndIndex: 1}
	request := &edge_ctrl_pb.RouterDataModelValidateRequest{State: state}
	wire, err := proto.Marshal(request)
	req.NoError(err)
	parsed := &edge_ctrl_pb.RouterDataModelValidateRequest{}
	req.NoError(proto.Unmarshal(wire, parsed))

	model := NewBareRouterDataModel("")
	model.WhileLocked(func(u uint64) {
		for _, e := range parsed.State.Events {
			model.Handle(parsed.State.EndIndex, e)
		}
		model.SetCurrentIndex(parsed.State.EndIndex)
	})

	var diffs []string
	current.Validate(model, func(entityType string, id string, diffType DiffType, detail string) {
		diffs = append(diffs, entityType+" "+id+" "+detail)
	})
	req.Empty(diffs)
}
