package main

import (
	"embed"
	_ "embed"
	"fmt"
	"github.com/openziti/fablab"
	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/lib/actions/semaphore"
	"github.com/openziti/fablab/kernel/lib/binding"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/semaphore"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/terraform"
	distribution "github.com/openziti/fablab/kernel/lib/runlevel/3_distribution"
	"github.com/openziti/fablab/kernel/lib/runlevel/3_distribution/rsync"
	aws_ssh_key2 "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/resources"
	"github.com/openziti/ziti/zititest/models/test_resources"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/openziti/ziti/zititest/zitilab/models"
	"os"
	"path"
	"time"
)

const TargetZitiVersion = ""

//go:embed configs
var configResource embed.FS

type scaleStrategy struct{}

func (self scaleStrategy) IsScaled(entity model.Entity) bool {
	return entity.GetScope().HasTag("scaled")
}

func (self scaleStrategy) GetEntityCount(entity model.Entity) uint32 {
	if entity.GetType() == model.EntityTypeComponent {
		return 20
	}
	return 5
}

var m = &model.Model{
	Id: "links-test",
	Scope: model.Scope{
		Defaults: model.Variables{
			"environment": "links-test",
			"credentials": model.Variables{
				"aws": model.Variables{
					"managed_key": true,
				},
				"ssh": model.Variables{
					"username": "ubuntu",
				},
				"edge": model.Variables{
					"username": "admin",
					"password": "admin",
				},
			},
			"metrics": model.Variables{
				"influxdb": model.Variables{
					"url": "http://localhost:8086",
					"db":  "ziti",
				},
			},
		},
	},
	StructureFactories: []model.Factory{
		model.NewScaleFactoryWithDefaultEntityFactory(scaleStrategy{}),
		model.FactoryFunc(func(m *model.Model) error {
			return m.ForEachHost("component.ctrl", 1, func(host *model.Host) error {
				if host.InstanceType == "" {
					host.InstanceType = "t3.medium"
				}
				return nil
			})
		}),
		model.FactoryFunc(func(m *model.Model) error {
			return m.ForEachHost("component.router", 1, func(host *model.Host) error {
				host.InstanceType = "c5.xlarge"
				return nil
			})
		}),
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
				"ctrl1": {
					InstanceType: "t3.medium",
					Components: model.Components{
						"ctrl1": {
							Scope: model.Scope{Tags: model.Tags{"ctrl"}},
							Type: &zitilab.ControllerType{
								Version: TargetZitiVersion,
							},
						},
					},
				},
				"router-us-east-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"scaled"}},
					Components: model.Components{
						"router-us-east-{{ .Host.ScaleIndex }}.{{ .ScaleIndex }}": {
							Scope: model.Scope{Tags: model.Tags{"router", "scaled"}},
							Type: &zitilab.RouterType{
								Version: TargetZitiVersion,
							},
						},
					},
				},
			},
		},
		"us-west-2": {
			Region: "us-west-2",
			Site:   "us-west-2b",
			Hosts: model.Hosts{
				"ctrl2": {
					Components: model.Components{
						"ctrl2": {
							Scope: model.Scope{Tags: model.Tags{"ctrl"}},
							Type: &zitilab.ControllerType{
								Version: TargetZitiVersion,
							},
						},
					},
				},
				"router-us-west-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"scaled"}},
					Components: model.Components{
						"router-us-west-{{ .Host.ScaleIndex }}.{{ .ScaleIndex }}": {
							Scope: model.Scope{Tags: model.Tags{"router", "scaled"}},
							Type: &zitilab.RouterType{
								Version: TargetZitiVersion,
							},
						},
					},
				},
			},
		},
		"eu-west-2": {
			Region: "eu-west-2",
			Site:   "eu-west-2a",
			Hosts: model.Hosts{
				"ctrl3": {
					InstanceType: "c5.large",
					Components: model.Components{
						"ctrl3": {
							Scope: model.Scope{Tags: model.Tags{"ctrl"}},
							Type: &zitilab.ControllerType{
								Version: TargetZitiVersion,
							},
						},
					},
				},
				"router-eu-west-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"scaled"}},
					Components: model.Components{
						"router-eu-west-{{ .Host.ScaleIndex }}.{{ .ScaleIndex }}": {
							Scope: model.Scope{Tags: model.Tags{"router", "scaled"}},
							Type: &zitilab.RouterType{
								Version: TargetZitiVersion,
							},
						},
					},
				},
			},
		},

		"eu-central-1": {
			Region: "eu-central-1",
			Site:   "eu-central-1a",
			Hosts: model.Hosts{
				"router-eu-central-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"scaled"}},
					Components: model.Components{
						"router-eu-central-{{ .Host.ScaleIndex }}.{{ .ScaleIndex }}": {
							Scope: model.Scope{Tags: model.Tags{"router", "scaled"}},
							Type: &zitilab.RouterType{
								Version: TargetZitiVersion,
							},
						},
					},
				},
			},
		},
	},

	Actions: model.ActionBinders{
		"bootstrap": model.ActionBinder(func(m *model.Model) model.Action {
			workflow := actions.Workflow()

			workflow.AddAction(host.GroupExec("*", 50, "touch .hushlogin"))
			workflow.AddAction(component.Stop(".ctrl"))
			workflow.AddAction(host.GroupExec("*", 50, "rm -f logs/*"))
			workflow.AddAction(host.GroupExec("component.ctrl", 5, "rm -rf ./fablab/ctrldata"))

			workflow.AddAction(component.Start(".ctrl"))
			workflow.AddAction(edge.RaftJoin(".ctrl"))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))
			workflow.AddAction(edge.InitRaftController("#ctrl1"))
			workflow.AddAction(edge.ControllerAvailable("#ctrl1", 30*time.Second))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))

			workflow.AddAction(edge.Login("#ctrl1"))

			workflow.AddAction(component.StopInParallel(models.RouterTag, 50))
			workflow.AddAction(edge.InitEdgeRouters(models.RouterTag, 2))

			return workflow
		}),
		"clean": model.Bind(actions.Workflow(
			component.StopInParallelHostExclusive("*", 15),
			host.GroupExec("*", 25, "rm -f logs/*"),
		)),
		"login":    model.Bind(edge.Login("#ctrl1")),
		"login2":   model.Bind(edge.Login("#ctrl2")),
		"login3":   model.Bind(edge.Login("#ctrl3")),
		"sowChaos": model.Bind(model.ActionFunc(sowChaos)),
	},

	Infrastructure: model.Stages{
		aws_ssh_key.Express(),
		&terraform_0.Terraform{
			Retries: 3,
			ReadyCheck: &semaphore_0.ReadyStage{
				MaxWait: 90 * time.Second,
			},
		},
	},

	Distribution: model.Stages{
		distribution.DistributeSshKey("*"),
		rsync.RsyncStaged(),
	},

	Disposal: model.Stages{
		terraform.Dispose(),
		aws_ssh_key2.Dispose(),
	},
}

func sowChaos(run model.Run) error {
	controllers, err := chaos.SelectRandom(run, ".ctrl", chaos.RandomOfTotal())
	if err != nil {
		return err
	}
	routers, err := chaos.SelectRandom(run, ".router", chaos.Percentage(15))
	if err != nil {
		return err
	}
	toRestart := append(routers, controllers...)
	fmt.Printf("restarting %v controllers and %v routers\n", len(controllers), len(routers))
	return chaos.RestartSelected(run, toRestart, 50)
}

func main() {
	m.AddActivationActions("stop", "bootstrap")

	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)

	fablab.InitModel(m)
	fablab.Run()
}
