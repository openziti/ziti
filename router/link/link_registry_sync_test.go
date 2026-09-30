package link

import (
	"sync"
	"testing"

	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/xlink"
	"github.com/stretchr/testify/require"
)

// GetCtrlChannel resolves a controller's channel the way production does: nil when the id is not registered.
func (self *testCtrls) GetCtrlChannel(ctrlId string) channel.Channel {
	if ctrl, ok := self.all[ctrlId]; ok {
		return ctrl.Channel()
	}
	return nil
}

// inlinePool runs queued work on the caller, so a test sees the work's outcome, including a panic, directly.
type inlinePool struct {
	goroutines.Pool
}

func (self *inlinePool) QueueOrError(f func()) error {
	f()
	return nil
}

// syncTestLink is an xlink.MultiConnXLink double owing one state to a fixed controller set.
type syncTestLink struct {
	xlink.MultiConnXLink
	id      string
	stateId string
	ctrlIds []string

	lock   sync.Mutex
	synced []string
}

func (self *syncTestLink) Id() string                               { return self.id }
func (self *syncTestLink) IsClosed() bool                           { return false }
func (self *syncTestLink) Iteration() uint32                        { return 1 }
func (self *syncTestLink) GetLinkConnState() *ctrl_pb.LinkConnState { return nil }
func (self *syncTestLink) GetCtrlRequiringSync() (string, []string) {
	return self.stateId, self.ctrlIds
}

func (self *syncTestLink) MarkLinkStateSyncedForState(ctrlId string, stateId string) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.synced = append(self.synced, ctrlId+"/"+stateId)
}

func (self *syncTestLink) syncedStates() []string {
	self.lock.Lock()
	defer self.lock.Unlock()
	return append([]string(nil), self.synced...)
}

// Test_syncRequiredLinkStates_UnregisteredControllerIsDeferred: a controller the close handler has
// unregistered has no channel to send to. The state stays owed instead of being sent to a nil channel,
// and goes out once the controller is registered again.
func Test_syncRequiredLinkStates_UnregisteredControllerIsDeferred(t *testing.T) {
	req := require.New(t)
	reg, routerEnv := newReconnectTestRegistry(t)
	routerEnv.rateLimiterPool = &inlinePool{}

	link := &syncTestLink{id: "l1", stateId: "s1", ctrlIds: []string{"ctrl1"}}
	reg.linkMap[link.Id()] = link
	reg.ctrls = &testCtrls{all: map[string]env.NetworkController{}}

	req.NotPanics(reg.syncRequiredLinkStates)
	req.Empty(link.syncedStates())

	withCtrl(reg, "ctrl1", newFlakySendChannel(0))
	reg.syncRequiredLinkStates()
	req.Equal([]string{"ctrl1/s1"}, link.syncedStates())
}
