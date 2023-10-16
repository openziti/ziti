package main

import (
	"embed"
	"github.com/openziti/fablab"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/host"
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
	"github.com/openziti/ziti/zititest/models/db-creation/actions"
	"github.com/openziti/ziti/zititest/models/test_resources"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"os"
	"path"
	"time"
)

//go:embed configs
var configResource embed.FS

// Definition of the model, which houses most you need to run things.
var m = &model.Model{
	Id: "db-creation",
	Scope: model.Scope{
		Defaults: model.Variables{
			"environment": "db-creation",
			"credentials": model.Variables{
				"ssh": model.Variables{
					"username": "ubuntu",
				},
				"edge": model.Variables{
					"username": "admin",
					"password": "admin",
				},
				"aws": model.Variables{
					"managed_key": true,
				},
			},
		},
	},

	StructureFactories: []model.Factory{
		//model.NewScaleFactoryWithDefaultEntityFactory(scaleStrategy{}),
	},

	Factories: []model.Factory{
		//newStageFactory(),
	},

	Resources: model.Resources{
		resources.Configs:   resources.SubFolder(configResource, "configs"),
		resources.Binaries:  os.DirFS(path.Join(os.Getenv("GOPATH"), "bin")),
		resources.Terraform: test_resources.TerraformResources(),
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
								Version:        os.Getenv("ZITI_VERSION"),
								LocalPath:      "",
								DNSNames:       []string{actions.DomainName},
							},
						},
					},
				},
			},
		},
	},

	Actions: model.ActionBinders{
		"bootstrap": actions.NewBootstrapAction(),
		"stop":      model.Bind(component.StopInParallel("ctrl", 1)),
		"login":     model.Bind(edge.Login("#ctrl")),
	},

	Infrastructure: model.Stages{
		aws_ssh_key.Express(),
		terraform_0.Express(),
		semaphore_0.Restart(90 * time.Second),
	},

	Distribution: model.Stages{
		distribution.DistributeSshKey("*"),
		distribution.Locations("*", "logs"),
		rsync.RsyncStaged(),
		//rsync.NewRsyncHost("#ctrl", "resources/ctrl.db", "/home/ubuntu/fablab/ctrl.db"),
		//rsync.NewRsyncHost("#ctrl", "resources/pki/", "/home/ubuntu/fablab/pki/"),
		rsync.NewRsyncHost("#ctrl", "resources/aws_setup.sh", "/home/ubuntu/fablab/bin/aws_setup.sh"),
		rsync.NewRsyncHost("#ctrl", "resources/db_creator_script_external.sh", "/home/ubuntu/fablab/bin/db_creator_script_external.sh"),
	},

	Disposal: model.Stages{
		model.StageActionF(func(run model.Run) error {
			m := run.GetModel()
			s := actions.Route53StringCreator(m, actions.Delete)
			return host.Exec(m.MustSelectHost("#ctrl"), s).Execute(run)
		}),
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
