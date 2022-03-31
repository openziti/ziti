package testutil

import (
	"github.com/openziti/channel"
	"github.com/openziti/transport"
	"github.com/pkg/errors"
	"time"
)

func NewTimeoutUnderlayFactory(factory channel.UnderlayFactory, timeout time.Duration) *UnderlayFactoryWrapper {
	return &UnderlayFactoryWrapper{
		timeout: timeout,
		wrapped: factory,
	}
}

type UnderlayFactoryWrapper struct {
	timeout time.Duration
	wrapped channel.UnderlayFactory
}

func (self *UnderlayFactoryWrapper) Create(timeout time.Duration, tcfg transport.Configuration) (channel.Underlay, error) {
	underlayC := make(chan channel.Underlay, 1)
	errC := make(chan error, 1)
	go func() {
		u, err := self.wrapped.Create(timeout, tcfg)
		if err != nil {
			errC <- err
		} else {
			underlayC <- u
		}
	}()

	select {
	case underlay := <-underlayC:
		return underlay, nil
	case err := <-errC:
		return nil, err
	case <-time.After(self.timeout):
		return nil, errors.New("timed out")
	}
}
