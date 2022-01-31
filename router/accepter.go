package router

import (
	forwarder2 "github.com/openziti/fabric/router/forwarder"
	"github.com/openziti/fabric/router/xlink"
	"github.com/sirupsen/logrus"
)

func newXlinkAccepter(f *forwarder2.Forwarder) xlink.Acceptor {
	return &xlinkAccepter{forwarder: f}
}

func (self *xlinkAccepter) Accept(xlink xlink.Xlink) error {
	if err := self.forwarder.RegisterLink(xlink); err != nil {
		return err
	}
	logrus.Infof("accepted new link [l/%s]", xlink.Id().Token)
	return nil
}

type xlinkAccepter struct {
	forwarder *forwarder2.Forwarder
}
