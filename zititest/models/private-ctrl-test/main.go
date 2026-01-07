package main

import (
	"embed"
	_ "embed"
	"os"
	"path"
	"time"

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
	awsSshKeyDispose "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/resources"
	"github.com/openziti/ziti/zititest/models/test_resources"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/models"
)

const (
	targetZitiVersion = ""
)

//go:embed configs
var configResource embed.FS

var m = &model.Model{
	Id: "private-ctrl-test",
	Scope: model.Scope{
		Defaults: model.Variables{
			"ha":          true,
			"environment": "private-ctrl-test",
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
		},
	},
	StructureFactories: []model.Factory{
		model.FactoryFunc(func(m *model.Model) error {
			return m.ForEachHost("*", 1, func(host *model.Host) error {
				host.InstanceType = "t3.medium" // need larger cpu for all the tls handshaking with 200 hosts
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
					Components: model.Components{
						"ctrl1": {
							Scope: model.Scope{Tags: model.Tags{"ctrl"}},
							Type: &zitilab.ControllerType{
								Version: targetZitiVersion,
							},
						},
					},
				},
				"router-east-1": {
					Scope: model.Scope{Tags: model.Tags{"router"}},
					Components: model.Components{
						"router-east-1": {
							Scope: model.Scope{Tags: model.Tags{"router"}},
							Type: &zitilab.RouterType{
								Version: targetZitiVersion,
							},
						},
					},
				},
			},
		},
		"us-west-2": {
			Region: "eu-west-2",
			Site:   "eu-west-2a",
			Hosts: model.Hosts{
				"ctrl2": {
					Components: model.Components{
						"ctrl2": {
							Scope: model.Scope{Tags: model.Tags{"ctrl", "ha"}},
							Type: &zitilab.ControllerType{
								Version: targetZitiVersion,
							},
						},
					},
				},
				"ctrl3": {
					Components: model.Components{
						"ctrl3": {
							Scope: model.Scope{Tags: model.Tags{"ctrl", "ha"}},
							Type: &zitilab.ControllerType{
								Version: targetZitiVersion,
							},
						},
					},
				},
				"router-west-1": {
					Scope: model.Scope{Tags: model.Tags{"router"}},
					Components: model.Components{
						"router-west-1": {
							Scope: model.Scope{Tags: model.Tags{"router"}},
							Type: &zitilab.RouterType{
								Version: targetZitiVersion,
							},
						},
					},
				},
			},
		},
	},

	Actions: model.ActionBinders{
		"initHA": model.ActionBinder(func(m *model.Model) model.Action {
			workflow := actions.Workflow()
			workflow.AddAction(component.StartInParallel(".ctrl", 10))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))
			workflow.AddAction(edge.RaftJoin("ctrl1", ".ctrl"))
			workflow.AddAction(semaphore.Sleep(5 * time.Second))
			return workflow
		}),
		"bootstrap": model.ActionBinder(func(m *model.Model) model.Action {
			workflow := actions.Workflow()

			workflow.AddAction(component.StopInParallel("*", 10000))
			workflow.AddAction(host.GroupExec("component.ctrl", 100, "rm -f logs/* ctrl.db"))
			workflow.AddAction(host.GroupExec("component.ctrl", 5, "rm -rf ./fablab/ctrldata"))

			workflow.AddAction(component.Start("#ctrl1"))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))
			workflow.AddAction(edge.InitRaftController("#ctrl1"))

			workflow.AddAction(edge.ControllerAvailable("#ctrl1", 30*time.Second))

			workflow.AddAction(edge.Login("#ctrl1"))

			workflow.AddAction(edge.InitEdgeRouters(models.RouterTag, 25))
			workflow.AddAction(model.RunAction("initHA"))

			workflow.AddAction(component.StartInParallel(".router", 10))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))

			return workflow
		}),
		"stop": model.Bind(component.StopInParallelHostExclusive("*", 10000)),
		"clean": model.Bind(actions.Workflow(
			component.StopInParallelHostExclusive("*", 10000),
			host.GroupExec("*", 100, "rm -f logs/*"),
		)),
		"login":  model.Bind(edge.Login("#ctrl1")),
		"login2": model.Bind(edge.Login("#ctrl2")),
		"login3": model.Bind(edge.Login("#ctrl3")),
		"restart": model.ActionBinder(func(run *model.Model) model.Action {
			workflow := actions.Workflow()
			workflow.AddAction(component.StopInParallel("*", 10000))
			workflow.AddAction(host.GroupExec("*", 100, "rm -f logs/*"))
			workflow.AddAction(component.Start(".ctrl"))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))
			workflow.AddAction(component.StartInParallel(".router", 10))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))
			return workflow
		}),
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
		awsSshKeyDispose.Dispose(),
	},
}

func main() {
	m.AddActivationActions("bootstrap")

	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)

	fablab.InitModel(m)
	fablab.Run()
}
