package actions

import (
	"fmt"

	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/model"
	"github.com/sirupsen/logrus"
)

type echoServerStart struct {
	componentSpec string
}

func StartEchoServers(componentSpec string) model.Action {
	return &echoServerStart{
		componentSpec: componentSpec,
	}
}

func (esi *echoServerStart) Execute(m *model.Model) error {
	return m.ForEachComponent(esi.componentSpec, 1, func(c *model.Component) error {
		ssh := lib.NewSshConfigFactory(c.GetHost())
		remoteConfigFile := "/home/ubuntu/fablab/cfg/" + c.PublicIdentity + ".json"

		echoServerCmd := fmt.Sprintf("nohup /home/%s/fablab/bin/ziti-echo server --identity %s > /home/ubuntu/logs/ziti-echo.log 2>&1 &",
			ssh.User(), remoteConfigFile)

		if output, err := lib.RemoteExec(ssh, echoServerCmd); err != nil {
			logrus.Errorf("error starting echo server [%s] (%v)", output, err)
			return err
		}
		logrus.Info("echo server started")
		return nil
	})
}
