package runlevel_5_operation

import (
	"crypto/rand"
	"fmt"
	"net/url"
	"strings"

	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/model"
	"github.com/sirupsen/logrus"
)

type echoClient struct {
	componentSpec string
	message       string
}

func AssertEcho(componentSpec string) model.OperatingStage {
	data := make([]byte, 10000)
	_, _ = rand.Read(data)

	return &echoClient{
		componentSpec: componentSpec,
		message:       url.QueryEscape(string(data)),
	}
}

func (ec *echoClient) Operate(run model.Run) error {
	return run.GetModel().ForEachComponent(ec.componentSpec, 1, func(c *model.Component) error {
		ssh := lib.NewSshConfigFactory(c.GetHost())
		remoteConfigFile := "/home/ubuntu/fablab/cfg/" + c.PublicIdentity + ".json"

		echoClientCmd := fmt.Sprintf("/home/%s/fablab/bin/ziti-echo client --identity %s %s 2>&1",
			ssh.User(), remoteConfigFile, ec.message)

		if output, err := lib.RemoteExec(ssh, echoClientCmd); err != nil {
			logrus.Errorf("error starting echo client [%s] (%v)", output, err)
			return err
		} else {
			//trim the newline ssh added
			output = strings.TrimRight(output, "\n")
			if output != ec.message {
				return fmt.Errorf("got message [%s] expected [%s]", output, ec.message)
			}
		}
		return nil
	})
}
