package router

import (
	forwarder2 "github.com/openziti/fabric/router/forwarder"
	"github.com/openziti/fabric/router/xlink"
	"github.com/sirupsen/logrus"
)

func newXlinkAccepter(f *forwarder2.Forwarder) xlink.Accepter {
	return &xlinkAccepter{forwarder: f}
}

func (self *xlinkAccepter) Accept(xlink xlink.Xlink) error {
	self.forwarder.RegisterLink(xlink)
	logrus.Infof("accepted new link [l/%s]", xlink.Id().Token)
	return nil
}

type xlinkAccepter struct {
	forwarder *forwarder2.Forwarder
}
