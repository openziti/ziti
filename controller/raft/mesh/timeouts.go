package mesh

import (
	"sync"
	"time"
)

func newDeadline() *deadline {
	return &deadline{
		C: make(chan struct{}, 1),
	}
}

type deadline struct {
	C     chan struct{}
	timer *time.Timer
	lock  sync.Mutex
}

func (self *deadline) Trigger() {
	select {
	case self.C <- struct{}{}:
	default:
	}
}

func (self *deadline) SetTimeout(t time.Duration) {
	self.lock.Lock()
	defer self.lock.Unlock()

	if self.timer != nil {
		self.timer.Stop()
		self.timer = nil
	}

	select {
	case <-self.C:
	default:
	}

	self.timer = time.AfterFunc(t, self.Trigger)
}
