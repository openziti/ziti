package runlevel_5_operation

import (
	"fmt"

	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/model"
	"github.com/sirupsen/logrus"
)

type echoServer struct {
	joiner chan struct{}
	host   *model.Host
}

func EchoServer(host *model.Host, joiner chan struct{}) model.OperatingStage {
	return &echoServer{
		joiner: joiner,
		host:   host,
	}
}

func (es *echoServer) Operate(run model.Run) error {
	go func() {
		defer func() {
			if es.joiner != nil {
				close(es.joiner)
				logrus.Debug("closed joiner")
			}
		}()

		ssh := lib.NewSshConfigFactory(es.host)

		echoServerCmd := fmt.Sprintf("ziti edge tutorial ziti-echo-server")

		if output, err := lib.RemoteExec(ssh, echoServerCmd); err != nil {
			logrus.Errorf("error starting echo server [%s] (%v)", output, err)
		}
	}()
	return nil
}
