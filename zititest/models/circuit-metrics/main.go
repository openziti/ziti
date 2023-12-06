package main

import (
	"embed"
	"github.com/openziti/fablab"
	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/host"
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

//go:embed configs
var configResource embed.FS

const zitiVersion = "latest"

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
					"username": "pete",
					"password": "oI5ns3X+R#aG",
				},
				"aws": model.Variables{
					"managed_key": true,
					"access_key":  os.Getenv("AWS_ACCESS_KEY_ID"),
					"secret_key":  os.Getenv("AWS_SECRET_ACCESS_KEY"),
				},
			},
			"metrics": model.Variables{
				"influxdb": model.Variables{
					"url": "http://3.90.26.17:8086/orgs/1690e1ae86127b82",
					"db":  "flow-control",
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
				"iperf-client": {
					InstanceType: "t3.micro",
					Components: model.Components{
						"iperf-client": {
							Scope: model.Scope{Tags: model.Tags{"iperf-client", "terminator", "tunneler", "edge-router"}},
							Type:  &zitilab.RouterType{Version: zitiVersion},
						},
					},
				},
			},
		},
		"eu-west-2": {
			Region: "eu-west-2",
			Site:   "eu-west-2a",
			Hosts: model.Hosts{
				"iperf-server": {
					InstanceType: "t3.micro",
					Components: model.Components{
						"iperf-server": {
							Scope: model.Scope{Tags: model.Tags{"iperf-server", "terminator", "tunneler", "edge-router"}},
							Type:  &zitilab.RouterType{Version: zitiVersion},
						},
					},
				},
			},
		},
	},

	Actions: model.ActionBinders{
		"bootstrap": cmActions.NewBootstrapAction(),
		"start":     cmActions.NewStartAction(),
		"stop":      model.Bind(component.StopInParallel("*", 15)),
		"clean": model.Bind(actions.Workflow(
			component.StopInParallel("*", 15),
			host.GroupExec("*", 25, "rm -f logs/*"),
		)),
		"login":              model.Bind(edge.Login("#ctrl")),
		"syncModelEdgeState": model.Bind(edge.SyncModelEdgeState(".terminator")),
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
	m.AddActivationActions("stop", "bootstrap")
	m.AddOperatingActions("login", "syncModelEdgeState")
	fablab.InitModel(m)
	runPhase := fablib_5_operation.NewPhase()

	// Add the CircuitMetrics stage to the FabLab Model - the func/last arguments is needed to map the router name to lookup expression that will return a host.
	circuitMetricsStage := zitilib_5_operation.CircuitMetrics(5*time.Second, runPhase.GetCloser(), func(id string) string {
		id = strings.ReplaceAll(id, ".", ":")
		return "component.edgeId:" + id
	})
	m.AddOperatingStage(circuitMetricsStage)

	// Add the InfluxMetricsReporter stage to the FabLab Model
	influxMetricsReporterStage := fablib_5_operation.InfluxMetricsReporter()
	m.AddOperatingStage(influxMetricsReporterStage)

	var iPerfServerEndpoint = func(m *model.Model) string {
		//return m.MustSelectHost("component.iperf-server").PublicIp
		return "iperf.service"
	}
	m.AddOperatingStage(fablib_5_operation.Iperf("Ziti_Overlay", iPerfServerEndpoint, "component.iperf-server", "component.iperf-client", 60))
	m.AddOperatingStage(runPhase)
	m.AddOperatingStage(fablib_5_operation.Persist())
	fablab.Run()
}
