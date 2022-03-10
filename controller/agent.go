package controller

import (
	"bufio"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io"
)

const (
	AgentAppId             byte = 1
	AgentOpSnapshotDbSnaps byte = 1
)

func (self *Controller) RegisterDefaultAgentOps() {
	self.agentHandlers[AgentOpSnapshotDbSnaps] = self.agentOpSnapshotDb
}

func (self *Controller) HandleCustomAgentOp(conn io.ReadWriter) error {
	logrus.Debug("received agent operation request")
	bconn := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	appId, err := bconn.ReadByte()
	if err != nil {
		return err
	}

	if appId != AgentAppId {
		logrus.WithField("appId", appId).Debug("invalid app id on agent request")
		return errors.New("invalid operation for controller")
	}

	op, err := bconn.ReadByte()
	if err != nil {
		return err
	}

	if opF, ok := self.agentHandlers[op]; ok {
		if err := opF(bconn); err != nil {
			return err
		}
		return bconn.Flush()
	}
	return errors.Errorf("invalid operation %v", op)
}

func (self *Controller) agentOpSnapshotDb(c *bufio.ReadWriter) error {
	if err := self.network.SnapshotDatabase(); err != nil {
		return err
	}
	_, err := c.WriteString("success\n")
	return err
}
