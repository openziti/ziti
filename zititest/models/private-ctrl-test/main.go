package main

import (
	"embed"
	_ "embed"
	"os"
	"path"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab"
	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/lib/actions/semaphore"
	"github.com/openziti/fablab/kernel/lib/binding"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/aws_ssh_key"
	semaphore_0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/semaphore"
	terraform_0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/terraform"
	distribution "github.com/openziti/fablab/kernel/lib/runlevel/3_distribution"
	"github.com/openziti/fablab/kernel/lib/runlevel/3_distribution/rsync"
	awsSshKeyDispose "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/kernel/model/aws"
	"github.com/openziti/fablab/resources"
	"github.com/openziti/ziti/zititest/models/test_resources"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/openziti/ziti/zititest/zitilab/models"
	zitilibOps "github.com/openziti/ziti/zititest/zitilab/runlevel/5_operation"
)

const (
	targetZitiVersion = ""
)

//go:embed configs
var configResource embed.FS

var throughputWorkload = "" +
	`concurrency:  1
    iterations:   2
    dialer:
      txRequests:       5000
      txPacing:         1ms
      rxTimeout:        10s
      payloadMinBytes:  1000
      payloadMaxBytes:  1000
    listener:
      rxTimeout:        10s
`

var latencyWorkload = "" +
	`concurrency:  2
    iterations:  100
    dialer:
      txRequests:       1
      rxTimeout:        10s
      payloadMinBytes:  64
      payloadMaxBytes:  256
      latencyFrequency: 1
    listener:
      txRequests:       1
      txAfterRx:        true
      rxTimeout:        10s
      payloadMinBytes:  64
      payloadMaxBytes:  256
`

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
			"throughputWorkload": throughputWorkload,
			"latencyWorkload":    latencyWorkload,
		},
	},
	StructureFactories: []model.Factory{
		model.FactoryFunc(func(m *model.Model) error {
			return m.ForEachHost("*", 1, func(host *model.Host) error {
				host.InstanceType = "t3.medium"
				return nil
			})
		}),
	},
	Factories: []model.Factory{
		model.FactoryFunc(func(m *model.Model) error {
			simServices := zitilibOps.NewSimServices(func(s string) string {
				return "component#" + s
			})

			m.AddActivationStageF(simServices.SetupSimControllerIdentity)
			m.AddOperatingStage(simServices.CollectSimMetricStage("metrics"))
			m.AddActionF("runSimScenario", func(run model.Run) error {
				return RunSimScenarios(run, simServices)
			})

			m.AddActionF("startSimMetrics", func(run model.Run) error {
				return simServices.CollectSimMetrics(run, "metrics")
			})

			return nil
		}),
	},
	Resources: model.Resources{
		resources.Configs:   resources.SubFolder(configResource, "configs"),
		resources.Binaries:  os.DirFS(path.Join(os.Getenv("GOPATH"), "bin")),
		resources.Terraform: test_resources.TerraformResources(),
	},
	AWS: aws.Model{
		SecurityGroups: aws.SecurityGroups{
			"public": {
				Rules: []*aws.NetworkRule{
					{
						Direction: aws.Ingress,
						Port:      1280,
						Protocol:  "tcp",
					},
					{
						Direction: aws.Ingress,
						Port:      6262,
						Protocol:  "tcp",
					},
					{
						Direction: aws.Ingress,
						Port:      6263,
						Protocol:  "tcp",
					},
				},
			},
			"private": {
				Rules: []*aws.NetworkRule{
					{
						Direction: aws.Ingress,
						Port:      1280,
						Protocol:  "tcp",
					},
					{
						Direction:  aws.Ingress,
						Port:       6262,
						Protocol:   "tcp",
						CidrBlocks: []string{"var.vpc_cidr"},
					},
					{
						Direction:  aws.Ingress,
						Port:       6263,
						Protocol:   "tcp",
						CidrBlocks: []string{"var.vpc_cidr"},
					},
				},
			},
		},
	},
	Regions: model.Regions{
		"us-east-1": {
			Region: "us-east-1",
			Site:   "us-east-1a",
			Hosts: model.Hosts{
				"ctrl-east": {
					Components: model.Components{
						"ctrl-east": {
							AWS: aws.Component{
								SecurityGroup: "public",
							},
							Scope: model.Scope{Tags: model.Tags{"ctrl"}},
							Type: &zitilab.ControllerType{
								Version: targetZitiVersion,
								Debug:   true,
							},
						},
					},
				},
				"router-east": {
					Scope: model.Scope{Tags: model.Tags{"router"}},
					Components: model.Components{
						"router-east": {
							AWS: aws.Component{
								SecurityGroup: "public",
							},
							Scope: model.Scope{Tags: model.Tags{"router", "test", "ctrl-listener", "east"}},
							Type: &zitilab.RouterType{
								Version: targetZitiVersion,
								Debug:   true,
							},
						},
					},
				},
				"router-metrics": {
					InstanceType: "t3.micro",
					Components: model.Components{
						"router-metrics": {
							Scope: model.Scope{Tags: model.Tags{"router", "no-traversal", "sim-services", "ctrl-listener"}},
							Type: &zitilab.RouterType{
								Version: targetZitiVersion,
							},
						},
					},
				},
				"sim-east": {
					Components: model.Components{
						"loop-host-east": {
							Scope: model.Scope{Tags: model.Tags{"loop-host", "loop-host-east", "sdk-app", "host", "sim-services-host", "east"}},
							Type: &zitilab.Loop4SimType{
								Mode:         zitilab.Loop4Listener,
								ConfigSource: "loop-host.yml.tmpl",
							},
						},
						"loop-client-east": {
							Scope: model.Scope{Tags: model.Tags{"loop-client", "loop-client-east", "sdk-app", "client", "sim-services-client", "east"}},
							Type: &zitilab.Loop4SimType{
								Mode:         zitilab.Loop4RemoteControlled,
								ConfigSource: "loop-client.yml.tmpl",
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
				"ctrl-west-1": {
					Components: model.Components{
						"ctrl-west-1": {
							AWS: aws.Component{
								SecurityGroup: "private",
							},
							Scope: model.Scope{Tags: model.Tags{"ctrl", "private", "west", "bootstrap"}},
							Type: &zitilab.ControllerType{
								Version: targetZitiVersion,
							},
						},
					},
				},
				"ctrl-west-2": {
					Components: model.Components{
						"ctrl-west-2": {
							AWS: aws.Component{
								SecurityGroup: "private",
							},
							Scope: model.Scope{Tags: model.Tags{"ctrl", "private", "west"}},
							Type: &zitilab.ControllerType{
								Version: targetZitiVersion,
							},
						},
					},
				},
				"router-west": {
					Scope: model.Scope{Tags: model.Tags{"router"}},
					Components: model.Components{
						"router-west": {
							AWS: aws.Component{
								SecurityGroup: "public",
							},
							Scope: model.Scope{Tags: model.Tags{"router", "tunneler", "test", "ctrl-listener", "west"}},
							Type: &zitilab.RouterType{
								Version: targetZitiVersion,
							},
						},
					},
				},
				"sim-west": {
					Components: model.Components{
						"loop-host-west": {
							Scope: model.Scope{Tags: model.Tags{"loop-host", "loop-host-west", "sdk-app", "host", "sim-services-host", "west"}},
							Type: &zitilab.Loop4SimType{
								Mode:         zitilab.Loop4Listener,
								ConfigSource: "loop-host.yml.tmpl",
							},
						},
						"loop-client-west": {
							Scope: model.Scope{Tags: model.Tags{"loop-client", "loop-client-west", "sdk-app", "client", "sim-services-client", "west"}},
							Type: &zitilab.Loop4SimType{
								Mode:         zitilab.Loop4RemoteControlled,
								ConfigSource: "loop-client.yml.tmpl",
							},
						},
					},
				},
			},
		},
	},

	Actions: model.ActionBinders{
		"bootstrap": NewBootstrapAction(),
		"stop":      model.Bind(component.StopInParallelHostExclusive("*", 10000)),
		"clean": model.Bind(actions.Workflow(
			component.StopInParallelHostExclusive("*", 10000),
			host.GroupExec("*", 100, "rm -f logs/*"),
		)),
		"login":    model.Bind(edge.Login("#ctrl-east")),
		"login2":   model.Bind(edge.Login("#ctrl-west-1")),
		"login3":   model.Bind(edge.Login("#ctrl-west-2")),
		"sowChaos": model.BindF(sowChaos),
		"validateUp": model.BindF(func(run model.Run) error {
			if err := chaos.ValidateUp(run, ".ctrl", 3, 15*time.Second); err != nil {
				return err
			}
			err := run.GetModel().ForEachComponent(".ctrl", 3, func(c *model.Component) error {
				return edge.ControllerAvailable(c.Id, time.Minute).Execute(run)
			})
			if err != nil {
				return err
			}
			if err := chaos.ValidateUp(run, ".router", 100, time.Minute); err != nil {
				pfxlog.Logger().WithError(err).Error("validate up failed, trying to start all routers again")
				return component.StartInParallel(".router", 100).Execute(run)
			}
			return nil
		}),
		"validateClusterConnectivity": model.BindF(validateClusterConnectivity),
		"validateTerminators":         model.BindF(validateTerminators),
		"validateCircuits":            model.BindF(validateCircuits),
		"testIteration": model.BindF(func(run model.Run) error {
			return run.GetModel().Exec(run,
				"sowChaos",
				"validateUp",
				"validateClusterConnectivity",
				"validateTerminators",
				"runSimScenario",
				"validateCircuits",
			)
		}),
		"restart": model.ActionBinder(func(run *model.Model) model.Action {
			workflow := actions.Workflow()
			workflow.AddAction(component.StopInParallel("*", 10000))
			workflow.AddAction(host.GroupExec("*", 100, "rm -f logs/*"))
			workflow.AddAction(component.Start(".ctrl"))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))
			workflow.AddAction(component.StartInParallel(models.RouterTag, 10))
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
