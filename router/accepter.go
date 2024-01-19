package router

import (
	"github.com/openziti/ziti/router/forwarder"
	"github.com/openziti/ziti/router/xlink"
	"github.com/sirupsen/logrus"
)

func newXlinkAccepter(f *forwarder.Forwarder) xlink.Acceptor {
	return &xlinkAccepter{
		forwarder: f,
	}
}

func (self *xlinkAccepter) Accept(xlink xlink.Xlink) error {
	if err := self.forwarder.RegisterLink(xlink); err != nil {
		return err
	}
	logrus.WithField("linkId", xlink.Id()).
		WithField("destId", xlink.DestinationId()).
		WithField("iteration", xlink.Iteration()).
		Info("accepted new link")
	return nil
}

type xlinkAccepter struct {
	forwarder *forwarder.Forwarder
}
