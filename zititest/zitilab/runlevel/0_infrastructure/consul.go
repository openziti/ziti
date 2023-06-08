package zitilib_runlevel_0_infrastructure

import (
	"fmt"

	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/model"
	"github.com/sirupsen/logrus"
)

type installConsul struct {
	hostSpec string
}

func InstallConsul(hostSpec string) model.InfrastructureStage {
	return &installConsul{
		hostSpec: hostSpec,
	}
}

func (imb *installConsul) Express(run model.Run) error {
	return run.GetModel().ForEachHost(imb.hostSpec, 25, func(host *model.Host) error {
		ssh := lib.NewSshConfigFactory(host)

		if output, err := lib.RemoteExec(ssh, "curl --fail --silent --show-error --location https://apt.releases.hashicorp.com/gpg | gpg --dearmor | sudo dd of=/usr/share/keyrings/hashicorp-archive-keyring.gpg"); err != nil {
			return fmt.Errorf("error getting hashicorp gpg key on host [%s] %s (%s)", host.PublicIp, output, err)
		}

		if output, err := lib.RemoteExec(ssh, "echo \"deb [arch=amd64 signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main\" | sudo tee -a /etc/apt/sources.list.d/hashicorp.list"); err != nil {
			return fmt.Errorf("error adding hashicorp repo to apt on host [%s] %s (%s)", host.PublicIp, output, err)
		}

		cmd := "sudo apt-get update && sudo apt-get install consul -y"

		if output, err := lib.RemoteExec(ssh, cmd); err != nil {
			return fmt.Errorf("error installing Consul on host [%s] %s (%s)", host.PublicIp, output, err)
		}
		logrus.Infof("%s => %s", host.PublicIp, "installing Consul")
		return nil
	})
}
