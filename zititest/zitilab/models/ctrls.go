package models

import (
	"math/rand/v2"
	"sync/atomic"
	"time"

	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/v2/zitirest"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
)

type CtrlClients struct {
	ctrls       []*zitirest.Clients
	ctrlMap     map[string]*zitirest.Clients
	initialized atomic.Bool
}

func (self *CtrlClients) Init(run model.Run, selector string) error {
	if !self.initialized.CompareAndSwap(false, true) {
		return nil
	}

	self.ctrlMap = map[string]*zitirest.Clients{}
	ctrls := run.GetModel().SelectComponents(selector)
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

	for i := 0; i < len(ctrls); i++ {
		result := <-resultC
		if result.err != nil {
			return result.err
		}
		self.ctrls = append(self.ctrls, result.clients)
		self.ctrlMap[result.id] = result.clients
	}
	return nil
}

func (self *CtrlClients) GetRandomCtrl() *zitirest.Clients {
	return self.ctrls[rand.IntN(len(self.ctrls))]
}

func (self *CtrlClients) GetCtrl(id string) *zitirest.Clients {
	return self.ctrlMap[id]
}
