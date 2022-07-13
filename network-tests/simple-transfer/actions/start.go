package actions

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/semaphore"
	"github.com/openziti/fablab/kernel/model"
	zitilib_actions "github.com/openziti/zitilab/actions"
	"github.com/openziti/zitilab/models"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

func NewStartAction(metricbeatConfigPath, metricbeatDataPath, metricbeatLogPath string) model.ActionBinder {
	action := &startAction{
		metricbeatConfigPath: metricbeatConfigPath,
		metricbeatDataPath:   metricbeatDataPath,
		metricbeatLogPath:    metricbeatLogPath,
	}
	return action.bind
}

func (a *startAction) bind(m *model.Model) model.Action {
	workflow := actions.Workflow()
	workflow.AddAction(component.Start("#ctrl"))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))
	//workflow.AddAction(component.Start("#router-east"))
	workflow.AddAction(component.StartInParallel(models.EdgeRouterTag, 25))
	//workflow.AddAction(test())
	workflow.AddAction(semaphore.Sleep(2 * time.Second))
	workflow.AddAction(zitilib_actions.StartMetricbeat("*", a.metricbeatConfigPath, a.metricbeatDataPath, a.metricbeatLogPath))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))
	workflow.AddAction(StartEchoServers("#echo-server"))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	fmt.Println("Done starting!")
	return workflow
}

type startAction struct {
	metricbeatConfigPath string
	metricbeatDataPath   string
	metricbeatLogPath    string
}

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

		if out, err := zitilib_actions.EdgeExecWithOutput(c.GetModel(), "policy-advisor", "identities", "-q", "echo-client", "echo"); err != nil {
			return err
		} else {
			logrus.Error(out)
		}

		if out, err := zitilib_actions.EdgeExecWithOutput(c.GetModel(), "list", "service-policies", "name contains \"echo\""); err != nil {
			return err
		} else {
			logrus.Error(out)
		}
		if out, err := zitilib_actions.EdgeExecWithOutput(c.GetModel(), "policy-advisor", "identities", "-q", "echo-server", "echo"); err != nil {
			return err
		} else {
			logrus.Error(out)
		}

		//if out, err := zitilib_actions.FabricExecWithOutput(c.GetModel(), "list", "routers"); err != nil {
		//	return err
		//} else {
		//	logrus.Error(out)
		//}
		//logFile := fmt.Sprintf("/home/%s/logs/echo-server.log", ssh.User())
		echoServerCmd := fmt.Sprintf("nohup /home/%s/fablab/bin/ziti-echo --identity %s 2>&1 &",
			ssh.User(), remoteConfigFile)

		//echoServerCmd = "echo test &"

		//if _, err := zitilib_cli.Exec(c.GetModel(), "edge", "tutorial", "ziti-echo-server", "--identity", remoteConfigFile); err != nil {
		//	return err
		//}
		//if output, err := lib.RemoteExec(ssh, fmt.Sprintf("ls /home")); err != nil {
		//	logrus.Errorf("error ls [%s] (%v)", output, err)
		//	return err
		//}
		if output, err := RemoteExec(ssh, echoServerCmd); err != nil {
			logrus.Errorf("error starting echo server [%s] (%v)", output, err)
			return err
		}
		logrus.Error("After remote starting echo server")
		return nil
	})
}

type testAction struct{}

func test() model.Action {
	return &testAction{}
}

func (t *testAction) Execute(m *model.Model) error {
	return m.ForEachComponent("#router-east", 1, func(c *model.Component) error {
		sshConfigFactory := lib.NewSshConfigFactory(c.GetHost())

		sudoCmd := ""
		if true {
			sudoCmd = " sudo "
		}
		serviceCmd := fmt.Sprintf("nohup%v /home/%s/fablab/bin/%s --log-formatter pfxlog run /home/%s/fablab/cfg/%s 2>&1 &",
			sudoCmd, sshConfigFactory.User(), c.BinaryName, sshConfigFactory.User(), c.ConfigName)
		if value, err := RemoteExec(sshConfigFactory, serviceCmd); err == nil {
			if len(value) > 0 {
				logrus.Infof("output [%s]", strings.Trim(value, " \t\r\n"))
			}
		} else {
			return fmt.Errorf("error starting component [%s] on [%s] [%s] (%s)", c.BinaryName, c.GetHost().PublicIp, value, err)
		}
		return nil
		if err := lib.LaunchService(sshConfigFactory, c.BinaryName, c.ConfigName, c.RunWithSudo); err != nil {
			return fmt.Errorf("error starting component [%s] on [%s] (%s)", c.BinaryName, c.GetHost().PublicIp, err)
		}
		return nil
	})
}

func RemoteExec(sshConfig lib.SshConfigFactory, cmd string) (string, error) {
	return RemoteExecAll(sshConfig, cmd)
}

func RemoteExecAll(sshConfig lib.SshConfigFactory, cmds ...string) (string, error) {
	var b bytes.Buffer
	err := RemoteExecAllTo(sshConfig, &b, cmds...)
	return b.String(), err
}

func RemoteExecAllTo(sshConfig lib.SshConfigFactory, out io.Writer, cmds ...string) error {
	if len(cmds) == 0 {
		return nil
	}
	config := sshConfig.Config()

	logrus.Infof("executing [%s]: '%s'", sshConfig.Address(), cmds[0])

	client, err := ssh.Dial("tcp", sshConfig.Address(), config)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	for idx, cmd := range cmds {
		session, err := client.NewSession()
		if err != nil {
			return err
		}
		session.Stdout = out

		if idx > 0 {
			logrus.Infof("executing [%s]: '%s'", sshConfig.Address(), cmd)
		}
		err = session.Run(cmd)
		_ = session.Close()

		if err != nil {
			return err
		}
	}

	return nil
}
