package router

import (
	"github.com/openziti/fabric/router/forwarder"
	"github.com/openziti/fabric/router/xlink"
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
	logrus.Infof("accepted new link [l/%s]", xlink.Id())
	return nil
}

type xlinkAccepter struct {
	forwarder *forwarder.Forwarder
}
