package main

import (
	"embed"
	"fmt"
	"github.com/openziti/fablab"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/binding"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/aws_ssh_key"
	semaphore_0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/semaphore"
	terraform_0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/terraform"
	distribution "github.com/openziti/fablab/kernel/lib/runlevel/3_distribution"
	"github.com/openziti/fablab/kernel/lib/runlevel/3_distribution/rsync"
	aws_ssh_key2 "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/resources"
	"github.com/openziti/ziti/zititest/models/ha/actions"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/sirupsen/logrus"
	"os"
	"time"
)

//go:embed configs
var configResource embed.FS

type scaleStrategy struct{}

func (s scaleStrategy) IsScaled(entity model.Entity) bool {
	return entity.GetType() == model.EntityTypeHost && entity.GetScope().HasTag("scaled")
}

func (s scaleStrategy) GetEntityCount(entity model.Entity) uint32 {
	if entity.GetType() == model.EntityTypeHost && entity.GetScope().HasTag("scaled") {
		return 4
	}
	return 1
}

// This is used to pull in custom config for example Terraform HCL Files or Metricbeat YML files as seen in the model.
func getConfigData(filePath string) []byte {
	data, err := configResource.ReadFile(fmt.Sprintf("configs/%s", filePath))
	if err != nil {
		logrus.Errorf("Unable to read config data from %s: [%s]", filePath, err)
	}
	return data
}

// Definition of the model, which houses most you need to run things.
var m = &model.Model{
	Id: "endpointTesting",
	Scope: model.Scope{
		Defaults: model.Variables{
			"environment": "endpointTesting",
			"credentials": model.Variables{
				"ssh": model.Variables{
					"username": "ubuntu",
				},
				"edge": model.Variables{
					"username": "admin",
					"password": "admin",
				},
			},
		},
	},

	StructureFactories: []model.Factory{
		model.NewScaleFactoryWithDefaultEntityFactory(scaleStrategy{}),
	},

	Factories: []model.Factory{
		newStageFactory(),
	},

	Resources: model.Resources{
		resources.Configs:   resources.SubFolder(configResource, "configs"),
		resources.Terraform: resources.DefaultTerraformResources(),
	},

	Regions: model.Regions{
		"us-east-1": {
			Region: "us-east-1",
			Site:   "us-east-1a",
			Hosts: model.Hosts{
				"ctrl": {
					InstanceType: "t3.micro",
					Components: model.Components{
						"ctrl": {
							Scope: model.Scope{Tags: model.Tags{"ctrl"}},
							Type: &zitilab.ControllerType{
								ConfigSourceFS: nil,
								ConfigSource:   "",
								ConfigName:     "",
								Version:        "v0.28.4",
								LocalPath:      "/mnt/c/Users/padib/OneDrive/Documents/NF Files/NF_Repos/ziti/ziti",
								DNSNames:       nil,
							},
						},
					},
				},
			},
		},
	},

	Actions: model.ActionBinders{
		"bootstrap": actions.NewBootstrapAction(),
		"start": actions.NewStartAction(actions.MetricbeatConfig{
			ConfigPath: "metricbeat",
			DataPath:   "metricbeat/data",
			LogPath:    "metricbeat/logs",
		},
			actions.ConsulConfig{
				ServerAddr: os.Getenv("CONSUL_ENDPOINT"),
				ConfigDir:  "consul",
				DataPath:   "consul/data",
				LogPath:    "consul/log.out",
			}),
		"stop":  model.Bind(component.StopInParallel("*", 15)),
		"login": model.Bind(edge.Login("#ctrl")),
	},

	Infrastructure: model.Stages{
		aws_ssh_key.Express(),
		terraform_0.Express(),
		semaphore_0.Restart(90 * time.Second),
	},

	Distribution: model.Stages{
		distribution.DistributeSshKey("*"),
		distribution.Locations("*", "logs"),
		distribution.DistributeDataWithReplaceCallbacks(
			"#ctrl",
			string(getConfigData("ziti.hcl")),
			"consul/ziti.hcl",
			os.FileMode(0644),
			map[string]func(*model.Host) string{
				"${build_number}": func(h *model.Host) string {
					return os.Getenv("BUILD_NUMBER")
				},
				"${ziti_version}": func(h *model.Host) string {
					return h.MustStringVariable("ziti_version")
				},
			}),
		rsync.RsyncStaged(),
		//rsync.NewRsyncHost("#ctrl", "/home/padibona/.fablab/instances/services-pete/kit/0.27.9_Ziti_Full.db", "/home/ubuntu/fablab/ctrl.db"),
		rsync.NewRsyncHost("#ctrl", "/home/padibona/resources/db_creator_script_external.sh", "/home/ubuntu/fablab/bin/db_creator_script_external.sh"),
	},

	Disposal: model.Stages{
		terraform.Dispose(),
		aws_ssh_key2.Dispose(),
	},
}

func main() {
	m.AddActivationActions("stop", "bootstrap")
	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)
	fablab.InitModel(m)
	fablab.Run()
}
