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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab"
	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/lib/actions/semaphore"
	"github.com/openziti/fablab/kernel/lib/binding"
	"github.com/openziti/fablab/kernel/lib/parallel"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/semaphore"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/terraform"
	distribution "github.com/openziti/fablab/kernel/lib/runlevel/3_distribution"
	"github.com/openziti/fablab/kernel/lib/runlevel/3_distribution/rsync"
	aws_ssh_key2 "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/resources"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/zititest/models/test_resources"
	"github.com/openziti/ziti/zititest/zitilab"
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/openziti/ziti/zititest/zitilab/cli"
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
	Id: "ert-hosting-test",
	Scope: model.Scope{
		Defaults: model.Variables{
			"environment": "ert-hosting-test",
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

		model.FactoryFunc(func(m *model.Model) error {
			return m.ForEachComponent(".router", 1, func(c *model.Component) error {
				if !stringz.Contains(c.Tags, "tunneler") {
					c.Tags = append(c.Tags, "tunneler")
				}
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
			workflow.AddAction(semaphore.Sleep(5 * time.Second))
			workflow.AddAction(edge.InitRaftController("#ctrl1"))
			workflow.AddAction(edge.ControllerAvailable("#ctrl1", 30*time.Second))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))

			workflow.AddAction(edge.Login("#ctrl1"))

			workflow.AddAction(component.StopInParallel(models.RouterTag, 50))
			workflow.AddAction(edge.InitEdgeRouters(models.RouterTag, 50))

			workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "all", "--edge-router-roles", "#all", "--identity-roles", "#all"))
			workflow.AddAction(zitilib_actions.Edge("create", "service-edge-router-policy", "all", "--service-roles", "#all", "--edge-router-roles", "#all"))

			workflow.AddAction(zitilib_actions.Edge("create", "config", "host-config", "host.v1", `
				{
					"address" : "localhost",
					"port" : 8080,
					"protocol" : "tcp"
				}`))

			workflow.AddAction(model.ActionFunc(func(run model.Run) error {
				var tasks []parallel.LabeledTask
				for i := 0; i < 2000; i++ {
					name := fmt.Sprintf("service-%04d", i)
					task := func() error {
						_, err := cli.Exec(run.GetModel(), "edge", "create", "service", name, "-c", "host-config", "--timeout", "15")
						return err
					}
					tasks = append(tasks, parallel.TaskWithLabel("create.service", "create service "+name, task))
				}

				return parallel.ExecuteLabeled(tasks, 5, parallel.AlwaysReport())
			}))

			workflow.AddAction(model.ActionFunc(func(run model.Run) error {
				identities := getRouterIdentities(m)
				serviceIdx := 0
				var tasks []parallel.Task
				for i, identity := range identities {
					name := fmt.Sprintf("service-policy-%03d", i)
					identityRoles := fmt.Sprintf("@%s", identity)
					servicesRoles := ""
					for j := 0; j < 10; j++ {
						idx := serviceIdx % 2000
						if j > 0 {
							servicesRoles += ","
						}
						servicesRoles += fmt.Sprintf("@service-%04d", idx)
						serviceIdx++
					}
					tasks = append(tasks, func() error {
						_, err := cli.Exec(run.GetModel(), "edge", "create", "service-policy", name, "Bind",
							"--identity-roles", identityRoles, "--service-roles", servicesRoles, "--timeout", "15")
						return err
					})
				}
				return parallel.Execute(tasks, 5)
			}))

			workflow.AddAction(edge.RaftJoin("ctrl1", ".ctrl"))

			workflow.AddAction(semaphore.Sleep(2 * time.Second))
			workflow.AddAction(component.StartInParallel(".router", 50))

			return workflow
		}),
		"stop": model.Bind(component.StopInParallelHostExclusive("*", 15)),
		"clean": model.Bind(actions.Workflow(
			component.StopInParallelHostExclusive("*", 15),
			host.GroupExec("*", 25, "rm -f logs/*"),
		)),
		"login":  model.Bind(edge.Login("#ctrl1")),
		"login2": model.Bind(edge.Login("#ctrl2")),
		"login3": model.Bind(edge.Login("#ctrl3")),
		"restart": model.ActionBinder(func(run *model.Model) model.Action {
			workflow := actions.Workflow()
			workflow.AddAction(component.StopInParallel("*", 100))
			workflow.AddAction(host.GroupExec("*", 25, "rm -f logs/*"))
			workflow.AddAction(component.Start(".ctrl"))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))
			workflow.AddAction(component.StartInParallel(".router", 50))
			return workflow
		}),
		"sowChaos": model.Bind(model.ActionFunc(sowChaos)),
		"validateUp": model.Bind(model.ActionFunc(func(run model.Run) error {
			if err := chaos.ValidateUp(run, ".ctrl", 3, 15*time.Second); err != nil {
				return err
			}
			err := run.GetModel().ForEachComponent(".ctrl", 3, func(c *model.Component) error {
				return edge.ControllerAvailable(c.Id, 30*time.Second).Execute(run)
			})
			if err != nil {
				return err
			}
			if err := chaos.ValidateUp(run, ".router", 100, time.Minute); err != nil {
				pfxlog.Logger().WithError(err).Error("validate up failed, trying to start all routers again")
				return component.StartInParallel(".router", 100).Execute(run)
			}
			return nil
		})),
		"validate": model.Bind(model.ActionFunc(validateTerminators)),
		"testIteration": model.Bind(model.ActionFunc(func(run model.Run) error {
			return run.GetModel().Exec(run,
				"sowChaos",
				"validateUp",
				"validate",
			)
		})),
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

func getRouterIdentities(m *model.Model) []string {
	var result []string
	for _, c := range m.SelectComponents(".router") {
		result = append(result, c.GetId())
	}
	return result
}

func main() {
	m.AddActivationActions("bootstrap")

	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)

	fablab.InitModel(m)
	fablab.Run()
}
