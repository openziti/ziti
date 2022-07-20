package runlevel_5_operation

import (
	"fmt"
	"strings"

	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/model"
	zitilib_actions "github.com/openziti/zitilab/actions"
	"github.com/sirupsen/logrus"
)

type echoClient struct {
	componentSpec string
	message       string
}

func AssertEcho(componentSpec, message string) model.OperatingStage {
	return &echoClient{
		componentSpec: componentSpec,
		message:       message,
	}
}

func (ec *echoClient) Operate(run model.Run) error {
	return run.GetModel().ForEachComponent(ec.componentSpec, 1, func(c *model.Component) error {
		ssh := lib.NewSshConfigFactory(c.GetHost())
		remoteConfigFile := "/home/ubuntu/fablab/cfg/" + c.PublicIdentity + ".json"

		if err := zitilib_actions.EdgeExec(c.GetModel(), "list", "terminators"); err != nil {
			return err
		}

		echoClientCmd := fmt.Sprintf("/home/%s/fablab/bin/ziti-echo client --identity %s %s 2>&1",
			ssh.User(), remoteConfigFile, ec.message)

		if output, err := lib.RemoteExec(ssh, echoClientCmd); err != nil {
			logrus.Errorf("error starting echo client [%s] (%v)", output, err)
			return err
		} else {
			//trim the newline ssh added
			output = strings.TrimRight(output, "\n")
			if output != ec.message {
				return fmt.Errorf("Got message [%s] expected [%s]", output, ec.message)
			}
		}
		return nil
	})
}
