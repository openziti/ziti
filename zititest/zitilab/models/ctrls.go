package models

import (
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/v2/zitirest"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
)

// DefaultReAuthInterval is the fallback re-auth cadence used when the caller
// does not configure ReAuthInterval before Init. Session timeout in the test
// configs is 30m, but a chaos-restarted controller drops its in-memory session
// store immediately — well before any timeout — so the binding constraint here
// is restart-recovery, not timeout. One minute keeps the test resilient to
// restarts without spamming the auth endpoint.
const DefaultReAuthInterval = time.Minute

type CtrlClients struct {
	// ReAuthInterval, if non-zero, overrides DefaultReAuthInterval. Set this
	// before calling Init.
	ReAuthInterval time.Duration

	ctrls       []*zitirest.Clients
	ctrlMap     map[string]*zitirest.Clients
	components  []*model.Component
	run         model.Run
	reAuthLock  sync.Mutex
	stop        chan struct{}
	stopOnce    sync.Once
	initialized atomic.Bool
}

func (self *CtrlClients) Init(run model.Run, selector string) error {
	if !self.initialized.CompareAndSwap(false, true) {
		return nil
	}

	self.run = run
	self.stop = make(chan struct{})
	self.ctrlMap = map[string]*zitirest.Clients{}
	ctrls := run.GetModel().SelectComponents(selector)
	self.components = ctrls

	resultC := make(chan struct {
		err     error
		id      string
		clients *zitirest.Clients
	}, len(ctrls))

	for _, ctrl := range ctrls {
		go func() {
			clients, err := chaos.EnsureLoggedIntoCtrl(run, ctrl, time.Minute)
			resultC <- struct {
				err     error
				id      string
				clients *zitirest.Clients
			}{
				err:     err,
				id:      ctrl.Id,
				clients: clients,
			}
		}()
	}

	// Preserve component order so reAuth can pair components with clients by
	// index without depending on the result-channel arrival order.
	componentIdx := map[string]int{}
	for i, ctrl := range ctrls {
		componentIdx[ctrl.Id] = i
	}
	self.ctrls = make([]*zitirest.Clients, len(ctrls))
	for range ctrls {
		result := <-resultC
		if result.err != nil {
			return result.err
		}
		self.ctrls[componentIdx[result.id]] = result.clients
		self.ctrlMap[result.id] = result.clients
	}

	interval := self.ReAuthInterval
	if interval <= 0 {
		interval = DefaultReAuthInterval
	}
	go self.reAuthLoop(interval)
	return nil
}

// Close stops the background re-auth loop. Safe to call multiple times.
// Callers should defer this after a successful Init.
func (self *CtrlClients) Close() {
	self.stopOnce.Do(func() {
		if self.stop != nil {
			close(self.stop)
		}
	})
}

// reAuthLoop periodically refreshes the API session on each controller client.
// A controller restart drops its in-memory session store, so the test's token
// becomes invalid; refreshing on a timer keeps the clients usable across chaos
// restarts without the test having to handle 401s in its retry path.
func (self *CtrlClients) reAuthLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-self.stop:
			return
		case <-ticker.C:
			self.reAuth()
		}
	}
}

func (self *CtrlClients) reAuth() {
	self.reAuthLock.Lock()
	defer self.reAuthLock.Unlock()

	resultC := make(chan error, len(self.components))
	for i, ctrl := range self.components {
		username := ctrl.MustStringVariable("credentials.edge.username")
		password := ctrl.MustStringVariable("credentials.edge.password")
		clients := self.ctrls[i]
		go func() {
			resultC <- clients.Authenticate(username, password)
		}()
	}

	for range self.components {
		if err := <-resultC; err != nil {
			pfxlog.Logger().WithError(err).Debug("background re-authentication failed for a controller; will retry next tick")
		}
	}
}

func (self *CtrlClients) GetRandomCtrl() *zitirest.Clients {
	return self.ctrls[rand.IntN(len(self.ctrls))]
}

func (self *CtrlClients) GetCtrl(id string) *zitirest.Clients {
	return self.ctrlMap[id]
}
