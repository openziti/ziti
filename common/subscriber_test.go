package common

import (
	"testing"

	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
)

type subscriberTracker struct {
}

func TestSubscriberScale(t *testing.T) {
	closeNotify := make(chan struct{})
	rdm := NewReceiverRouterDataModel(0, closeNotify)

	identityCount := 100
	serviceCount := 10000

	var index uint64
	for i := 0; i < identityCount; i++ {
		event := &edge_ctrl_pb.DataState_Event{
			Action:      edge_ctrl_pb.DataState_Create,
			IsSynthetic: false,
			Model: &edge_ctrl_pb.DataState_Event_Identity{
				Identity: &edge_ctrl_pb.DataState_Identity{
					Id:   eid.New(),
					Name: eid.New(),
				},
			},
		}
		index++
		rdm.Handle(index, event)
	}

	for i := 0; i < serviceCount; i++ {
		event := &edge_ctrl_pb.DataState_Event{
			Action:      edge_ctrl_pb.DataState_Create,
			IsSynthetic: false,
			Model: &edge_ctrl_pb.DataState_Event_Service{
				Service: &edge_ctrl_pb.DataState_Service{
					Id:   eid.New(),
					Name: eid.New(),
				},
			},
		}
		index++
		rdm.Handle(index, event)
	}

	for _, identityId := range rdm.Identities.Keys() {
		rdm.SubscribeToIdentityChanges(identityId)
	}
}
