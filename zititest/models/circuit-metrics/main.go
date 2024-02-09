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
	fablib_5_operation "github.com/openziti/fablab/kernel/lib/runlevel/5_operation"
	aws_ssh_key2 "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/resources"
	cmActions "github.com/openziti/ziti/zititest/models/circuit-metrics/actions"
	"github.com/openziti/ziti/zititest/models/test_resources"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	zitilib_5_operation "github.com/openziti/ziti/zititest/zitilab/runlevel/5_operation"
	"github.com/sirupsen/logrus"
	"os"
	"strings"
	"time"
)

//go:embed configs
var configResource embed.FS

const ZitiVersion = "latest"

const ZitiEdgeTunnelVersion = "0.22.19"

type StartIPerfTestAction struct{}

// Execute executes the StartIPerfTestAction for the given run.
func (a *StartIPerfTestAction) Execute(run model.Run) error {
	m := run.GetModel()
	hosts := m.SelectHosts("*") // modify to select correct hosts
	for _, host := range hosts {
		for _, c := range host.Components {
			if iperfComponent, ok := c.Type.(*zitilab.IPerfServerType); ok {
				err := iperfComponent.Start(run, c)
				if err != nil {
					logrus.Printf("underlying error: %v", err)
					return fmt.Errorf("error starting Iperf component %w", err)
				}
			}
		}
	}

	return nil
}

type StartZitiEdgeTunnelAction struct{}

func (a *StartZitiEdgeTunnelAction) Execute(run model.Run) error {
	m := run.GetModel()
	hosts := m.SelectHosts("*") // modify to select correct hosts
	for _, host := range hosts {
		for _, c := range host.Components {
			if zitiEdgeTunnelComponent, ok := c.Type.(*zitilab.ZitiEdgeTunnelType); ok {
				err := zitiEdgeTunnelComponent.Start(run, c)
				if err != nil {
					logrus.Printf("underlying error: %v", err)
					return fmt.Errorf("error starting ziti-edge-tunnel %w", err)
				}
			}
		}
	}

	return nil
}

type StartTCPDumpAction struct {
	scenarioName string
	host         string
	snaplen      int
	joiner       chan struct{}
}

func (a *StartTCPDumpAction) Execute(run model.Run) error {
	stage := fablib_5_operation.Tcpdump(a.scenarioName, a.host, a.snaplen, a.joiner)
	return stage.Execute(run)
}

type StopTCPDumpAction struct {
	host string
}

func (a *StopTCPDumpAction) Execute(run model.Run) error {
	stage := fablib_5_operation.TcpdumpCloser(a.host)
	return stage.Execute(run)
}

type StartTCPDumpStage struct{}

func (s *StartTCPDumpStage) Execute(run model.Run) error {
	action := StartTCPDumpAction{
		scenarioName: "Flow-Control", // replace with your scenario name
		host:         "router-eu",    // replace with your host
		snaplen:      128,            // replace with your actual snaplen value
		joiner:       make(chan struct{}),
	}
	return action.Execute(run)
}

type StopTCPDumpStage struct{}

func (s *StopTCPDumpStage) Execute(run model.Run) error {
	action := StopTCPDumpAction{
		host: "router-eu", // replace with your host
	}
	return action.Execute(run)
}

var m = &model.Model{
	Id: "flow-control",
	Scope: model.Scope{
		Defaults: model.Variables{
			"environment": "flow-control",
			"credentials": model.Variables{
				"ssh": model.Variables{
					"username": "ubuntu",
				},
				"edge": model.Variables{
					"username": "admin",
					"password": "admin",
				},
				"influxdb": model.Variables{
					"token": os.Getenv("INFLUX_TOKEN"), // These env variables are local to your machine
				},
				"aws": model.Variables{
					"managed_key": true,
					"access_key":  os.Getenv("AWS_ACCESS_KEY_ID"),     // These env variables are local to your machine
					"secret_key":  os.Getenv("AWS_SECRET_ACCESS_KEY"), // These env variables are local to your machine
				},
			},
			"metrics": model.Variables{
				"influxdb": model.Variables{
					"url":    os.Getenv("INFLUX_URL"),    // These env variables are local to your machine
					"db":     os.Getenv("INFLUX_DB"),     // These env variables are local to your machine
					"org":    os.Getenv("INFLUX_ORG"),    // These env variables are local to your machine
					"bucket": os.Getenv("INFLUX_BUCKET"), // These env variables are local to your machine
					"token":  os.Getenv("INFLUX_TOKEN"),  // These env variables are local to your machine
				},
			},
		},
	},
	StructureFactories: []model.Factory{},
	Factories:          []model.Factory{},
	Resources: model.Resources{
		resources.Configs:   resources.SubFolder(configResource, "configs"),
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
								Version:        ZitiVersion,
								LocalPath:      "",
								DNSNames:       nil,
							},
						},
					},
				},
				"router-us": {
					InstanceType: "c5.large",
					Components: model.Components{
						"edge-router-us": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "tunneler", "terminator", "iperf-server"}}, // These are identity attributes the identity is created automatically based on the 'edge-router' tag/attribute
							Type:  &zitilab.RouterType{Version: ZitiVersion},
						},
						"iperf-server": {
							Scope: model.Scope{Tags: model.Tags{"iperf", "service", "iperf-server"}},
							Type:  &zitilab.IPerfServerType{},
						},
					},
				},
			},
		},
		"eu-west-2": {
			Region: "eu-west-2",
			Site:   "eu-west-2a",
			Hosts: model.Hosts{
				"router-eu": {
					InstanceType: "c5.large",
					Components: model.Components{
						"edge-router-eu": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "iperf-client", "terminator", "tunneler"}}, // These are identity attributes the identity is created automatically based on the 'edge-router' tag/attribute
							Type:  &zitilab.RouterType{Version: ZitiVersion},
						},
						"iperf-client": {
							Scope: model.Scope{Tags: model.Tags{"iperf", "service", "iperf-client"}},
							Type:  &zitilab.IPerfServerType{},
						},
					},
				},
			},
		},
	},

	Actions: model.ActionBinders{
		"bootstrap": cmActions.NewBootstrapAction(),
		"start":     cmActions.NewStartAction(),
		//"start-ziti-edge-tunnel": model.Bind(&StartZitiEdgeTunnelAction{}),
		"stop":               model.Bind(component.StopInParallel("*", 15)),
		"login":              model.Bind(edge.Login("#ctrl")),
		"syncModelEdgeState": model.Bind(edge.SyncModelEdgeState(".terminator")),
		"start-iperf-tests":  model.Bind(&StartIPerfTestAction{}),
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
	},

	Disposal: model.Stages{
		terraform.Dispose(),
		aws_ssh_key2.Dispose(),
	},
}

func main() {
	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)
	//m.Actions["start-ziti-edge-tunnel"] = model.Bind(&StartZitiEdgeTunnelAction{})
	m.AddActivationActions("stop", "bootstrap", "start")
	m.AddOperatingActions("login", "syncModelEdgeState", "start-iperf-tests")
	runPhase := fablib_5_operation.NewPhase()
	circuitMetricsStage := zitilib_5_operation.CircuitMetrics(1*time.Second, runPhase.GetCloser(), func(id string) string {
		id = strings.ReplaceAll(id, ".", ":")
		return "component.edgeId:" + id
	})
	m.AddOperatingStage(circuitMetricsStage)

	// Add the Influx2MetricsReporter stage to the FabLab Model
	influxMetricsReporterStage := fablib_5_operation.InfluxMetricsReporter2()

	m.AddOperatingStage(influxMetricsReporterStage)

	var iPerfServerEndpoint = func(m *model.Model) string {
		return "iperf.service"
	}
	m.AddOperatingStage(fablib_5_operation.Iperf("Flow-Control", iPerfServerEndpoint, "component.iperf-server", "component.iperf-client", 30))

	m.AddOperatingStage(runPhase)
	m.AddOperatingStage(fablib_5_operation.Persist())
	fablab.InitModel(m)
	fablab.Run()
}
