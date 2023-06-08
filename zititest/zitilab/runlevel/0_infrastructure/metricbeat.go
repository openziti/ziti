package zitilib_runlevel_0_infrastructure

import (
	"fmt"

	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/model"
	"github.com/sirupsen/logrus"
)

const RECCOMENDED_METRICBEAT_VERSION = "8.3.2"

type installMetricbeat struct {
	hostSpec string
	version  string
}

func InstallMetricbeat(hostSpec, version string) model.InfrastructureStage {
	return &installMetricbeat{
		hostSpec: hostSpec,
		version:  version,
	}
}

func (imb *installMetricbeat) Express(run model.Run) error {
	return run.GetModel().ForEachHost(imb.hostSpec, 25, func(host *model.Host) error {
		ssh := lib.NewSshConfigFactory(host)

		if output, err := lib.RemoteExec(ssh, "wget -qO - https://artifacts.elastic.co/GPG-KEY-elasticsearch | sudo apt-key add -"); err != nil {
			return fmt.Errorf("error getting elastic gpg key on host [%s] %s (%s)", host.PublicIp, output, err)
		}

		if output, err := lib.RemoteExec(ssh, "echo \"deb https://artifacts.elastic.co/packages/8.x/apt stable main\" | sudo tee -a /etc/apt/sources.list.d/elastic-8.x.list"); err != nil {
			return fmt.Errorf("error adding elastic repo to apt on host [%s] %s (%s)", host.PublicIp, output, err)
		}

		cmd := fmt.Sprintf("sudo apt-get update && sudo apt-get install metricbeat%s -y", func() string {
			if imb.version != "" {
				return fmt.Sprintf("=%s", imb.version)
			}
			return ""
		}())

		if output, err := lib.RemoteExec(ssh, cmd); err != nil {
			return fmt.Errorf("error installing metricbeat on host [%s] %s (%s)", host.PublicIp, output, err)
		}
		logrus.Infof("%s => %s", host.PublicIp, "installing metricbeat")
		return nil
	})
}
