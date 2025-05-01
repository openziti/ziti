package main

import (
	"embed"
	_ "embed"
	"github.com/michaelquigley/pfxlog"
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
	fablibOps "github.com/openziti/fablab/kernel/lib/runlevel/5_operation"
	awsSshKeyDispose "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/resources"
	"github.com/openziti/ziti/zititest/models/test_resources"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/openziti/ziti/zititest/zitilab/models"
	zitilibOps "github.com/openziti/ziti/zititest/zitilab/runlevel/5_operation"
	"os"
	"path"
	"strings"
	"time"
)

const TargetZitiVersion = ""

//go:embed configs
var configResource embed.FS

type scaleStrategy struct{}

func (self scaleStrategy) IsScaled(entity model.Entity) bool {
	if entity.GetType() == model.EntityTypeHost {
		return entity.GetScope().HasTag("router") || entity.GetScope().HasTag("host")
	}
	return entity.GetType() == model.EntityTypeComponent && entity.GetScope().HasTag("host")
}

func (self scaleStrategy) GetEntityCount(entity model.Entity) uint32 {
	if entity.GetType() == model.EntityTypeHost {
		if entity.GetScope().HasTag("router") {
			return 2
		}
		if entity.GetScope().HasTag("host") {
			h := entity.(*model.Host)
			if h.Region.Id == "us-east-1" {
				return 8
			}
			return 6
		}
	}
	if entity.GetType() == model.EntityTypeComponent {
		return 10
	}
	return 1
}

var m = &model.Model{
	Id: "circuit-test",
	Scope: model.Scope{
		Defaults: model.Variables{
			"environment": "circuit-scale-test",
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
		model.FactoryFunc(func(m *model.Model) error {
			err := m.ForEachHost("component.ctrl", 1, func(host *model.Host) error {
				host.InstanceType = "c5.xlarge"
				return nil
			})

			if err != nil {
				return err
			}

			err = m.ForEachHost("component.router", 1, func(host *model.Host) error {
				host.InstanceType = "c5.xlarge"
				return nil
			})

			if err != nil {
				return err
			}

			return m.ForEachComponent(".host", 1, func(c *model.Component) error {
				c.Type.(*zitilab.ZitiTunnelType).Mode = zitilab.ZitiTunnelModeHost
				return nil
			})
		}),
		model.FactoryFunc(func(m *model.Model) error {
			if val, _ := m.GetBoolVariable("ha"); !val {
				for _, host := range m.SelectHosts("component.ha") {
					delete(host.Region.Hosts, host.Id)
				}
			}

			for _, c := range m.SelectComponents(".router") {
				c.Tags = append(c.Tags, "tunneler")
			}

			return nil
		}),
		model.NewScaleFactoryWithDefaultEntityFactory(&scaleStrategy{}),
	},
	Factories: []model.Factory{
		model.FactoryFunc(func(m *model.Model) error {
			simServices := zitilibOps.NewSimServices(func(s string) string {
				return "component#" + s
			})

			m.AddActivationStageF(simServices.SetupSimControllerIdentity)
			m.AddOperatingStage(simServices.CollectSimMetricStage("metrics"))

			return nil
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
					InstanceType: "t3.micro",
					Components: model.Components{
						"ctrl": {
							Scope: model.Scope{Tags: model.Tags{"ctrl", "underTest"}},
							Type:  &zitilab.ControllerType{},
						},
					},
				},
				"ctrl2": {
					InstanceType: "t3.micro",
					Components: model.Components{
						"ctrl": {
							Scope: model.Scope{Tags: model.Tags{"ctrl", "underTest", "ha"}},
							Type:  &zitilab.ControllerType{},
						},
					},
				},
				"router-client-1": {
					Components: model.Components{
						"router-client-1": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "tunneler", "client", "test", "underTest"}},
							Type:  &zitilab.RouterType{},
						},
					},
				},
				"router-client-2": {
					Components: model.Components{
						"router-client-2": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "tunneler", "client", "test", "underTest"}},
							Type:  &zitilab.RouterType{},
						},
					},
				},
				"ert": {
					Components: model.Components{
						"ert": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "tunneler", "client", "test", "underTest"}},
							Type:  &zitilab.RouterType{},
						},
						"loop-client-ert": {
							Scope: model.Scope{Tags: model.Tags{"loop-client", "sdk-app", "client", "sim-services-client"}},
							Type: &zitilab.Loop4SimType{
								Mode: zitilab.Loop4RemoteControlled,
							},
						},
					},
				},
				"router-metrics": {
					InstanceType: "t3.micro",
					Components: model.Components{
						"router-metrics": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "no-traversal", "sim-services"}},
							Type:  &zitilab.RouterType{},
						},
					},
				},
				"loop-client": {
					Scope: model.Scope{Tags: model.Tags{"loop-client"}},
					Components: model.Components{
						"loop-client": {
							Scope: model.Scope{Tags: model.Tags{"loop-client", "sdk-app", "client", "sim-services-client"}},
							Type: &zitilab.Loop4SimType{
								Mode: zitilab.Loop4RemoteControlled,
							},
						},
					},
				},
				"loop-client-xg": {
					Scope: model.Scope{Tags: model.Tags{"loop-client"}},
					Components: model.Components{
						"loop-client-xg": {
							Scope: model.Scope{Tags: model.Tags{"loop-client", "sdk-app", "client", "sim-services-client"}},
							Type: &zitilab.Loop4SimType{
								Mode: zitilab.Loop4RemoteControlled,
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
				"ctrl3": {
					InstanceType: "t3.micro",
					Components: model.Components{
						"ctrl": {
							Scope: model.Scope{Tags: model.Tags{"ctrl", "underTest", "ha"}},
							Type:  &zitilab.ControllerType{},
						},
					},
				},
				"router-host-1": {
					Components: model.Components{
						"router-host-1": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "host", "test", "underTest"}},
							Type:  &zitilab.RouterType{},
						},
					},
				},
				"router-host-2": {
					Components: model.Components{
						"router-host-2": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "host", "test", "underTest"}},
							Type:  &zitilab.RouterType{},
						},
					},
				},
				"ert-host": {
					Components: model.Components{
						"ert-host": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "tunneler", "host", "ert-host", "test", "underTest"}},
							Type:  &zitilab.RouterType{},
						},
						"loop-host-ert": {
							Scope: model.Scope{Tags: model.Tags{"loop-host", "sdk-app", "host"}},
							Type: &zitilab.Loop4SimType{
								Mode: zitilab.Loop4Listener,
							},
						},
					},
				},
				"loop-host": {
					Components: model.Components{
						"loop-host": {
							Scope: model.Scope{Tags: model.Tags{"loop-host", "sdk-app", "host", "sim-services-host"}},
							Type: &zitilab.Loop4SimType{
								Mode: zitilab.Loop4Dialer,
							},
						},
					},
				},
				"loop-host-xg": {
					Components: model.Components{
						"loop-host-xg": {
							Scope: model.Scope{Tags: model.Tags{"loop-host-xg", "sdk-app", "host", "sim-services-host"}},
							Type: &zitilab.Loop4SimType{
								Mode: zitilab.Loop4Dialer,
							},
						},
					},
				},
			},
		},
	},

	Actions: model.ActionBinders{
		"bootstrap": NewBootstrapAction(),
		"stop":      model.Bind(component.StopInParallelHostExclusive("*", 15)),
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
			workflow.AddAction(component.StartInParallel(".router", 10))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))
			workflow.AddAction(component.StartInParallel(".host", 50))
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
		"validate": model.Bind(model.ActionFunc(validateRouterDataModel)),
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

	Operation: model.Stages{
		model.RunAction("login"),
		edge.SyncModelEdgeState(models.EdgeRouterTag),

		fablibOps.StreamSarMetrics("*", 5, 1, nil),

		fablibOps.InfluxMetricsReporter(),

		zitilibOps.ModelMetricsWithIdMapper(nil, func(id string) string {
			if id == "ctrl" {
				return "#ctrl"
			}
			id = strings.ReplaceAll(id, ".", ":")
			return "component.edgeId:" + id
		}),
	},

	Disposal: model.Stages{
		terraform.Dispose(),
		awsSshKeyDispose.Dispose(),
	},
}

func main() {
	m.AddActivationActions("stop", "bootstrap")

	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)

	fablab.InitModel(m)
	fablab.Run()
}
