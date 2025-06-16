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
	semaphore_0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/semaphore"
	terraform_0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/terraform"
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

var throughputWorkload = "" +
	`concurrency:  1
    iterations:   2
    dialer:
      txRequests:       80000
      rxTimeout:        5s
      payloadMinBytes:  10000
      payloadMaxBytes:  10000
    listener:
      rxTimeout:        5s
`

var gentleThroughputWorkload = "" +
	`concurrency:  2
    iterations:   2
    dialer:
      txRequests:       7000
      txPacing:         1ms
      txMaxJitter:      0
      rxTimeout:        5s
      payloadMinBytes:  10000
      payloadMaxBytes:  10000
    listener:
      rxTimeout:        5s
`

var latencyWorkload = "" +
	`concurrency:  5
    iterations:  400
    dialer:
      txRequests:       1
      rxTimeout:        5s
      payloadMinBytes:  64
      payloadMaxBytes:  256
      latencyFrequency: 1
    listener:
      txRequests:       1
      txAfterRx:        true
      rxTimeout:        5s
      payloadMinBytes:  2048
      payloadMaxBytes:  10000
`

var slowWorkload = "" +
	`concurrency:  1
    iterations:   1
    dialer:
      txRequests:       60
      rxTimeout:        10s
      payloadMinBytes:  64000
      payloadMaxBytes:  64000
    listener:
      txRequests:       60
      txAfterRx:        true
      rxTimeout:        10s
      rxPacing:         1s
      payloadMinBytes:  64
      payloadMaxBytes:  256`

var m = &model.Model{
	Id: "circuit-test",
	Scope: model.Scope{
		Defaults: model.Variables{
			"ha":          "true",
			"environment": "circuit-test",
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
			"throughputWorkload":       throughputWorkload,
			"gentleThroughputWorkload": gentleThroughputWorkload,
			"latencyWorkload":          latencyWorkload,
			"slowWorkload":             slowWorkload,
			"testErtHost":              true,
			"testErtClient":            true,
			"testSdkClient":            true,
			"testSdkHost":              true,
			"testSdkXgClient":          true,
			"testSdkXgHost":            true,
		},
	},
	StructureFactories: []model.Factory{
		model.FactoryFunc(func(m *model.Model) error {
			err := m.ForEachHost("*", 1, func(host *model.Host) error {
				if host.InstanceType == "" {
					host.InstanceType = "c5.xlarge"
				}
				return nil
			})

			if err != nil {
				return err
			}

			err = m.ForEachHost("component.ctrl", 1, func(host *model.Host) error {
				host.InstanceType = "t3.micro"
				return nil
			})

			if err != nil {
				return err
			}

			return nil
		}),
		model.FactoryFunc(func(m *model.Model) error {
			if val, _ := m.GetBoolVariable("ha"); !val {
				for _, host := range m.SelectHosts("component.ha") {
					delete(host.Region.Hosts, host.Id)
				}
			}

			return nil
		}),
		model.FactoryFunc(func(m *model.Model) error {
			for _, host := range m.SelectHosts("*") {
				for _, component := range host.Components {
					if rc, ok := component.Type.(*zitilab.Loop4SimType); ok && rc.Mode == zitilab.Loop4RemoteControlled {
						if component.Id == "loop-client" && !m.BoolVariable("testSdkClient") {
							delete(host.Components, component.Id)
						} else if component.Id == "loop-client-xg" && !m.BoolVariable("testSdkXgClient") {
							delete(host.Components, component.Id)
						} else if component.Id == "loop-client-ert" && !m.BoolVariable("testErtClient") {
							delete(host.Components, component.Id)
						}
					}
				}
			}
			return nil
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

			metricsValidator := &SimMetricsValidator{
				events: map[*model.Host][]*MetricsEvent{},
			}
			metricsValidator.AddToModel(m)

			m.AddActionF("validateSimMetrics", func(run model.Run) error {
				return metricsValidator.ValidateCollected()
			})

			m.AddActionF("enableMetrics", metricsValidator.StartCollecting)

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
						"ctrl1": {
							Scope: model.Scope{Tags: model.Tags{"ctrl", "underTest"}},
							Type:  &zitilab.ControllerType{},
						},
					},
				},
				"ctrl2": {
					InstanceType: "t3.micro",
					Components: model.Components{
						"ctrl2": {
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
							Scope: model.Scope{Tags: model.Tags{"edge-router", "tunneler", "test", "loop-client", "underTest"}},
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
						"ctrl3": {
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
							Scope: model.Scope{Tags: model.Tags{"edge-router", "tunneler", "loop-host-ert", "test", "underTest"}},
							Type:  &zitilab.RouterType{},
						},
						"loop-host-ert": {
							Scope: model.Scope{Tags: model.Tags{"loop-host", "sdk-app", "host", "sim-services-host"}},
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
								Mode: zitilab.Loop4Listener,
							},
						},
					},
				},
				"loop-host-xg": {
					Components: model.Components{
						"loop-host-xg": {
							Scope: model.Scope{Tags: model.Tags{"loop-host-xg", "sdk-app", "host", "sim-services-host"}},
							Type: &zitilab.Loop4SimType{
								Mode: zitilab.Loop4Listener,
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
		"validateCircuits": model.BindF(validateCircuits),
		"testIteration": model.BindF(func(run model.Run) error {
			return run.GetModel().Exec(run,
				"enableMetrics",
				"runSimScenario",
				"validateSimMetrics",
				"validateCircuits",
			)
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

	Activation: model.Stages{
		model.RunAction("stop"),
		model.RunAction("bootstrap"),
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
	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)

	fablab.InitModel(m)
	fablab.Run()
}
