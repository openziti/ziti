/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package main

import (
	"embed"
	_ "embed"
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
	"github.com/openziti/ziti/zititest/zitilab/models"
	"os"
	"path"
	"time"
)

const TargetZitiVersion = ""

// const TargetZitiVersion = "v0.32.0"

const TargetZrokVersion = ""

//const TargetZrokVersion = "v0.4.22"

const iterations = 100_000
const pacing = 10 * time.Millisecond

//go:embed configs
var configResource embed.FS

type scaleStrategy struct{}

func (self scaleStrategy) IsScaled(entity model.Entity) bool {
	return entity.GetScope().HasTag("scaled")
}

func (self scaleStrategy) GetEntityCount(entity model.Entity) uint32 {
	if entity.GetType() == model.EntityTypeHost {
		if entity.GetScope().HasTag("router") {
			return 2
		} else if entity.GetScope().HasTag("client") {
			return 3
		}
	}

	if entity.GetType() == model.EntityTypeComponent {
		if entity.GetScope().HasTag("client") {
			return 50
		}
	}

	return 1
}

var m = &model.Model{
	Id: "zrok-test",
	Scope: model.Scope{
		Defaults: model.Variables{
			"environment": "zrok-test",
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
					host.InstanceType = "c5.large"
				}
				return nil
			})
		}),
		model.FactoryFunc(func(m *model.Model) error {
			return m.ForEachHost("component.router", 1, func(host *model.Host) error {
				host.InstanceType = "c5.large"
				return nil
			})
		}),

		model.FactoryFunc(func(m *model.Model) error {
			return m.ForEachHost("component.client", 1, func(host *model.Host) error {
				host.InstanceType = "c5.large"
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
								Version: TargetZitiVersion,
							},
						},
					},
				},
				"zrokCtrl": {
					InstanceType: "c5.large",
					Components: model.Components{
						"zrokCtrl": {
							Scope: model.Scope{Tags: model.Tags{"zrokCtrl"}},
							Type: &zitilab.ZrokControllerType{
								Version:          TargetZrokVersion,
								PreCreateClients: ".client",
							},
						},
					},
				},
				"zrokFront": {
					InstanceType: "c5.large",
					Components: model.Components{
						"zrokFront": {
							Scope: model.Scope{Tags: model.Tags{"zrokFront"}},
							Type: &zitilab.ZrokFrontendType{
								Version: TargetZrokVersion,
								DNS:     "paul.demo.openziti.org",
							},
						},
					},
				},
				"router-us-east-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"scaled", "router"}},
					Components: model.Components{
						"router-us-east-{{ .Host.ScaleIndex }}": {
							Scope: model.Scope{Tags: model.Tags{"router"}},
							Type: &zitilab.RouterType{
								Version: TargetZitiVersion,
							},
						},
					},
				},
				"zrok-us-east-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"client", "scaled"}},
					Components: model.Components{
						"zrok-us-east-{{ .Host.ScaleIndex }}.{{ .ScaleIndex }}": {
							Scope: model.Scope{Tags: model.Tags{"client", "scaled"}},
							Type: &zitilab.ZrokLoopTestType{
								Version:    TargetZrokVersion,
								Pacing:     pacing,
								Iterations: iterations,
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
				"router-us-west-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"scaled", "router"}},
					Components: model.Components{
						"router-us-west-{{ .Host.ScaleIndex }}": {
							Scope: model.Scope{Tags: model.Tags{"router"}},
							Type: &zitilab.RouterType{
								Version: TargetZitiVersion,
							},
						},
					},
				},

				"zrok-us-west-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"client", "scaled"}},
					Components: model.Components{
						"zrok-us-west-{{ .Host.ScaleIndex }}.{{ .ScaleIndex }}": {
							Scope: model.Scope{Tags: model.Tags{"client", "scaled"}},
							Type: &zitilab.ZrokLoopTestType{
								Version:    TargetZrokVersion,
								Pacing:     pacing,
								Iterations: iterations,
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
				"router-eu-west-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"scaled", "router"}},
					Components: model.Components{
						"router-eu-west-{{ .Host.ScaleIndex }}": {
							Scope: model.Scope{Tags: model.Tags{"router"}},
							Type: &zitilab.RouterType{
								Version: TargetZitiVersion,
							},
						},
					},
				},

				"zrok-eu-west-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"client", "scaled"}},
					Components: model.Components{
						"zrok-eu-west-{{ .Host.ScaleIndex }}.{{ .ScaleIndex }}": {
							Scope: model.Scope{Tags: model.Tags{"client", "scaled"}},
							Type: &zitilab.ZrokLoopTestType{
								Version:    TargetZrokVersion,
								Pacing:     pacing,
								Iterations: iterations,
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
					Scope: model.Scope{Tags: model.Tags{"scaled", "router"}},
					Components: model.Components{
						"router-eu-central-{{ .Host.ScaleIndex }}": {
							Scope: model.Scope{Tags: model.Tags{"router"}},
							Type: &zitilab.RouterType{
								Version: TargetZitiVersion,
							},
						},
					},
				},

				"zrok-eu-central-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"client", "scaled"}},
					Components: model.Components{
						"zrok-eu-central-{{ .Host.ScaleIndex }}.{{ .ScaleIndex }}": {
							Scope: model.Scope{Tags: model.Tags{"client", "scaled"}},
							Type: &zitilab.ZrokLoopTestType{
								Version:    TargetZrokVersion,
								Pacing:     pacing,
								Iterations: iterations,
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
			workflow.AddAction(component.StopInParallel("*", 100))
			workflow.AddAction(host.GroupExec("*", 50, "rm -rf logs/* .zrok/"))
			workflow.AddAction(host.GroupExec("*", 50, "find fablab -type d -exec chmod 755 {} \\;"))
			workflow.AddAction(edge.InitController("#ctrl1"))
			workflow.AddAction(component.Start(".ctrl"))
			workflow.AddAction(edge.ControllerAvailable("#ctrl1", 30*time.Second))

			workflow.AddAction(edge.Login("#ctrl1"))

			workflow.AddAction(component.StopInParallel(models.RouterTag, 50))
			workflow.AddAction(edge.InitEdgeRouters(models.RouterTag, 50))
			workflow.AddAction(component.StartInParallel(models.RouterTag, 50))

			workflow.AddAction(component.ExecF("#zrokCtrl", (*zitilab.ZrokControllerType).Init))
			workflow.AddAction(component.ExecF("#zrokCtrl", (*zitilab.ZrokControllerType).PreCreateAccounts))
			workflow.AddAction(component.Start("#zrokCtrl"))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))

			workflow.AddAction(component.ExecF("#zrokFront", (*zitilab.ZrokFrontendType).Init))
			workflow.AddAction(component.Start("#zrokFront"))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))

			workflow.AddAction(component.ExecInParallelF(".client", 200, (*zitilab.ZrokLoopTestType).Init))
			return workflow
		}),
		"clean": model.Bind(actions.Workflow(
			component.StopInParallelHostExclusive("*", 15),
			host.GroupExec("*", 25, "rm -f logs/*"),
		)),
		"login": model.Bind(edge.Login("#ctrl1")),
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

func main() {
	m.AddActivationActions("bootstrap")

	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)

	fablab.InitModel(m)
	fablab.Run()
}
