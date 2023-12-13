package main

import (
	"embed"
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
	"os"
	"strings"
	"time"
)

// configResource is a variable of type embed.FS used to store the configuration resources.
// Example usage:
// To access the configuration resources, you can use the configResource variable like this:
// configPath := "configs/myconfig.json"
// configFile, err := configResource.Open(configPath)
//
//	if err != nil {
//	    // handle error
//	}
//
// defer configFile.Close()
// Note: The exact usage and behavior of the configResource variable depend on the context in which it is used, and the specific methods and functions that interact
//
//go:embed configs
var configResource embed.FS

const zitiVersion = "latest"

type StartIPerfTestAction struct{}

func (a *StartIPerfTestAction) Execute(run model.Run) error {
	m := run.GetModel()
	hosts := m.SelectHosts("*") // modify to select correct hosts
	for _, host := range hosts {
		for _, c := range host.Components {
			if iperfComponent, ok := c.Type.(*zitilab.IPerfServerType); ok {
				err := iperfComponent.Start(run, c)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
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
					//"username": os.Getenv("INFLUX_USERNAME"), // These env variables are local to your machine
					//"password": os.Getenv("INFLUX_PASSWORD"), // These env variables are local to your machine
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
		resources.Configs: resources.SubFolder(configResource, "configs"),
		//resources.Binaries:  os.DirFS(path.Join(os.Getenv("GOPATH"), "bin")),
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
								Version:        zitiVersion,
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
							Scope: model.Scope{Tags: model.Tags{"terminator", "tunneler", "edge-router", "iperf-server"}}, // These are identity attributes the identity is created automatically based on the 'edge-router' tag/attribute
							Type:  &zitilab.RouterType{Version: zitiVersion},
						},
						"iperf-server": {
							Scope: model.Scope{Tags: model.Tags{"iperf", "service", "iperf-server", "sdk-app"}},
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
							Scope: model.Scope{Tags: model.Tags{"terminator", "tunneler", "edge-router", "iperf-client"}}, // These are identity attributes the identity is created automatically based on the 'edge-router' tag/attribute
							Type:  &zitilab.RouterType{Version: zitiVersion},
						},
						"iperf-client": {
							Scope: model.Scope{Tags: model.Tags{"iperf", "service", "iperf-client", "sdk-app"}},
							Type:  &zitilab.IPerfServerType{},
						},
					},
				},
			},
		},
	},

	Actions: model.ActionBinders{
		"bootstrap":          cmActions.NewBootstrapAction(),
		"start":              cmActions.NewStartAction(),
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
	m.AddActivationActions("stop", "bootstrap", "start")
	m.AddOperatingActions("login", "syncModelEdgeState", "start-iperf-tests")
	runPhase := fablib_5_operation.NewPhase()

	// Add the CircuitMetrics stage to the FabLab Model - the func/last arguments is needed to map the router name to lookup expression that will return a host.
	circuitMetricsStage := zitilib_5_operation.CircuitMetrics(5*time.Second, runPhase.GetCloser(), func(id string) string {
		id = strings.ReplaceAll(id, ".", ":")
		return "component.edgeId:" + id
	})
	m.AddOperatingStage(circuitMetricsStage)

	// Add the InfluxMetricsReporter2 stage to the FabLab Model
	influxMetricsReporterStage := fablib_5_operation.InfluxMetricsReporter2()
	m.AddOperatingStage(influxMetricsReporterStage)

	var iPerfServerEndpoint = func(m *model.Model) string {
		//return m.MustSelectHost("component.iperf-server").PublicIp
		return "iperf.service"
	}
	m.AddOperatingStage(fablib_5_operation.Iperf("Ziti_Overlay", iPerfServerEndpoint, "component.iperf-server", "component.iperf-client", 60))

	m.AddOperatingStage(runPhase)
	m.AddOperatingStage(fablib_5_operation.Persist())
	fablab.InitModel(m)
	fablab.Run()
}
