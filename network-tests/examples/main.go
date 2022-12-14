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
	"github.com/openziti/fablab/kernel/lib/runlevel/1_configuration/config"
	"github.com/openziti/fablab/kernel/lib/runlevel/2_kitting/devkit"
	distribution "github.com/openziti/fablab/kernel/lib/runlevel/3_distribution"
	"github.com/openziti/fablab/kernel/lib/runlevel/3_distribution/rsync"
	fablib_5_operation "github.com/openziti/fablab/kernel/lib/runlevel/5_operation"
	aws_ssh_key2 "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/resources"
	"github.com/openziti/ziti/network-tests/examples/actions"
	"github.com/openziti/zitilab"
	"github.com/openziti/zitilab/actions/edge"
	zitilib_runlevel_1_configuration "github.com/openziti/zitilab/runlevel/1_configuration"
	"github.com/sirupsen/logrus"
	"os"
	"time"
)

//go:embed configs
var configResource embed.FS

type scaleStrategy struct{}

func (s scaleStrategy) IsScaled(entity model.Entity) bool {
	return entity.GetType() == model.EntityTypeHost && entity.GetScope().HasTag("scaled")
}

func (s scaleStrategy) GetEntityCount(entity model.Entity) uint32 {
	if entity.GetType() == model.EntityTypeHost && entity.GetScope().HasTag("scaled") {
		return 4
	}
	return 1
}

// This is used to pull in custom config for example Terraform HCL Files or Metricbeat YML files as seen in the model.
func getConfigData(filePath string) []byte {
	data, err := configResource.ReadFile(fmt.Sprintf("configs/%s", filePath))
	if err != nil {
		logrus.Errorf("Unable to read config data from %s: [%s]", filePath, err)
	}
	return data
}

// Definition of the model, which houses most you need to run things.
var m = &model.Model{
	Id: "example-pete",
	Scope: model.Scope{
		Defaults: model.Variables{
			"environment": "example-pete-test",
			"credentials": model.Variables{
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
		model.NewScaleFactoryWithDefaultEntityFactory(scaleStrategy{}),
	},

	Factories: []model.Factory{
		newStageFactory(),
	},

	Resources: model.Resources{
		resources.Configs:   resources.SubFolder(configResource, "configs"),
		resources.Terraform: resources.DefaultTerraformResources(),
	},

	Regions: model.Regions{
		"us-east-1": {
			Region: "us-east-1",
			Site:   "us-east-1a",
			Hosts: model.Hosts{
				"ctrl": {
					InstanceType: "c5.large",
					Components: model.Components{
						"ctrl": {
							Scope:          model.Scope{Tags: model.Tags{"ctrl"}},
							BinaryName:     "ziti-controller",
							ConfigSrc:      "ctrl.yml",
							ConfigName:     "ctrl.yml",
							PublicIdentity: "ctrl",
							RunWithSudo:    true,
						},
					},
				},
				"router-east-server": {
					InstanceType: "c5.large",
					Components: model.Components{
						"router-east-server": {
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "terminator", "iperf-server", "tunneler"}},
							BinaryName:     "ziti-router",
							ConfigSrc:      "router.yml",
							ConfigName:     "router-east-server.yml",
							PublicIdentity: "router-east-server",
							RunWithSudo:    true,
						},
					},
				},
				"router-east-client": {
					Scope:        model.Scope{Tags: model.Tags{}},
					InstanceType: "c5.large",
					Components: model.Components{
						"router-east-client": {
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "terminator", "iperf-client", "tunneler"}},
							BinaryName:     "ziti-router",
							ConfigSrc:      "router.yml",
							ConfigName:     "router-east-client.yml",
							PublicIdentity: "router-east-client",
							RunWithSudo:    true,
						},
						//"tun-east-client": {
						//	Scope:          model.Scope{Tags: model.Tags{"terminator", "iperf-client", "sdk-app"}},
						//	BinaryName:     "ziti-edge-tunnel",
						//	PublicIdentity: "tun-east-client",
						//	RunWithSudo:    true,
						//},
					},
				},
			},
		},
		"us-west-2": {
			Region: "us-west-2",
			Site:   "us-west-2b",
			Hosts: model.Hosts{
				"router-west-client": {
					Scope:        model.Scope{Tags: model.Tags{}},
					InstanceType: "c5.large",
					Components: model.Components{
						"router-west-client": {
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "terminator", "iperf-client", "tunneler"}},
							BinaryName:     "ziti-router",
							ConfigSrc:      "router.yml",
							ConfigName:     "router-west-client.yml",
							PublicIdentity: "router-west-client",
							RunWithSudo:    true,
						},
						//"tun-west-client": {
						//	Scope:          model.Scope{Tags: model.Tags{"terminator", "iperf-client", "sdk-app"}},
						//	BinaryName:     "ziti-edge-tunnel",
						//	PublicIdentity: "tun-west-client",
						//	RunWithSudo:    true,
						//},
					},
				},
			},
		},
		"ca-central-1": {
			Region: "ca-central-1",
			Site:   "ca-central-1b",
			Hosts: model.Hosts{
				"router-canada-client": {
					Scope:        model.Scope{Tags: model.Tags{}},
					InstanceType: "c5.large",
					Components: model.Components{
						"router-canada-client": {
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "terminator", "iperf-client", "tunneler"}},
							BinaryName:     "ziti-router",
							ConfigSrc:      "router.yml",
							ConfigName:     "router-canada-client.yml",
							PublicIdentity: "router-canada-client",
							RunWithSudo:    true,
						},
						//"tun-canada-client": {
						//	Scope:          model.Scope{Tags: model.Tags{"terminator", "iperf-client", "sdk-app"}},
						//	BinaryName:     "ziti-edge-tunnel",
						//	PublicIdentity: "tun-canada-client",
						//	RunWithSudo:    true,
						//},
					},
				},
			},
		},
		"ap-northeast-1": {
			Region: "ap-northeast-1",
			Site:   "ap-northeast-1a",
			Hosts: model.Hosts{
				"router-tokyo-client": {
					Scope:        model.Scope{Tags: model.Tags{}},
					InstanceType: "c5.large",
					Components: model.Components{
						"router-tokyo-client": {
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "terminator", "iperf-client", "tunneler"}},
							BinaryName:     "ziti-router",
							ConfigSrc:      "router.yml",
							ConfigName:     "router-tokyo-client.yml",
							PublicIdentity: "router-tokyo-client",
							RunWithSudo:    true,
						},
						//"tun-tokyo-client": {
						//	Scope:          model.Scope{Tags: model.Tags{"terminator", "iperf-client", "sdk-app"}},
						//	BinaryName:     "ziti-edge-tunnel",
						//	PublicIdentity: "tun-tokyo-client",
						//	RunWithSudo:    true,
						//},
					},
				},
			},
		},
		"ap-southeast-2": {
			Region: "ap-southeast-2",
			Site:   "ap-southeast-2a",
			Hosts: model.Hosts{
				"router-sydney-client": {
					Scope:        model.Scope{Tags: model.Tags{}},
					InstanceType: "c5.large",
					Components: model.Components{
						"router-sydney-client": {
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "terminator", "iperf-client", "tunneler"}},
							BinaryName:     "ziti-router",
							ConfigSrc:      "router.yml",
							ConfigName:     "router-sydney-client.yml",
							PublicIdentity: "router-sydney-client",
							RunWithSudo:    true,
						},
						//"tun-sydney-client": {
						//	Scope:          model.Scope{Tags: model.Tags{"terminator", "iperf-client", "sdk-app"}},
						//	BinaryName:     "ziti-edge-tunnel",
						//	PublicIdentity: "tun-sydney-client",
						//	RunWithSudo:    true,
						//},
					},
				},
			},
		},
		"sa-east-1": {
			Region: "sa-east-1",
			Site:   "sa-east-1a",
			Hosts: model.Hosts{
				"router-brazil-client": {
					Scope:        model.Scope{Tags: model.Tags{}},
					InstanceType: "c5.large",
					Components: model.Components{
						"router-brazil-client": {
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "terminator", "iperf-client", "tunneler"}},
							BinaryName:     "ziti-router",
							ConfigSrc:      "router.yml",
							ConfigName:     "router-brazil-client.yml",
							PublicIdentity: "router-brazil-client",
							RunWithSudo:    true,
						},
						//"tun-brazil-client": {
						//	Scope:          model.Scope{Tags: model.Tags{"terminator", "iperf-client", "sdk-app"}},
						//	BinaryName:     "ziti-edge-tunnel",
						//	PublicIdentity: "tun-brazil-client",
						//	RunWithSudo:    true,
						//},
					},
				},
			},
		},
		"eu-central-1": {
			Region: "eu-central-1",
			Site:   "eu-central-1a",
			Hosts: model.Hosts{
				"router-frankfurt-client": {
					Scope:        model.Scope{Tags: model.Tags{}},
					InstanceType: "c5.large",
					Components: model.Components{
						"router-frankfurt-client": {
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "terminator", "iperf-client", "tunneler"}},
							BinaryName:     "ziti-router",
							ConfigSrc:      "router.yml",
							ConfigName:     "router-frankfurt-client.yml",
							PublicIdentity: "router-frankfurt-client",
							RunWithSudo:    true,
						},
						//"tun-frankfurt-client": {
						//	Scope:          model.Scope{Tags: model.Tags{"terminator", "iperf-client", "sdk-app"}},
						//	BinaryName:     "ziti-edge-tunnel",
						//	PublicIdentity: "tun-frankfurt-client",
						//	RunWithSudo:    true,
						//},
					},
				},
			},
		},
		"af-south-1": {
			Region: "af-south-1",
			Site:   "af-south-1a",
			Hosts: model.Hosts{
				"router-cape_town-client": {
					Scope:        model.Scope{Tags: model.Tags{}},
					InstanceType: "c5.large",
					Components: model.Components{
						"router-cape_town-client": {
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "terminator", "iperf-client", "tunneler"}},
							BinaryName:     "ziti-router",
							ConfigSrc:      "router.yml",
							ConfigName:     "router-cape_town-client.yml",
							PublicIdentity: "router-cape_town-client",
							RunWithSudo:    true,
						},
						//"tun-cape_town-client": {
						//	Scope:          model.Scope{Tags: model.Tags{"terminator", "iperf-client", "sdk-app"}},
						//	BinaryName:     "ziti-edge-tunnel",
						//	PublicIdentity: "tun-cape_town-client",
						//	RunWithSudo:    true,
						//},
					},
				},
			},
		},
	},

	Actions: model.ActionBinders{
		"bootstrap": actions.NewBootstrapAction(),
		"start":     actions.NewStartAction(),
		"stop":      model.Bind(component.StopInParallel("*", 15)),
		"login":     model.Bind(edge.Login("#ctrl")),
	},

	Infrastructure: model.InfrastructureStages{
		aws_ssh_key.Express(),
		terraform_0.Express(),
		semaphore_0.Restart(90 * time.Second),
	},

	Configuration: model.ConfigurationStages{
		zitilib_runlevel_1_configuration.IfPkiNeedsRefresh(
			zitilib_runlevel_1_configuration.Fabric(),
			zitilib_runlevel_1_configuration.DotZiti(),
		),
		config.Component(),
		zitilab.DefaultZitiBinaries(),
		devkit.DevKitF(zitilab.ZitiRoot, []string{"ziti-edge-tunnel"}),
	},

	Distribution: model.DistributionStages{
		distribution.DistributeSshKey("*"),
		distribution.Locations("*", "logs"),
		distribution.DistributeDataWithReplaceCallbacks(
			"#ctrl",
			string(getConfigData("ziti.hcl")),
			"consul/ziti.hcl",
			os.FileMode(0644),
			map[string]func(*model.Host) string{
				"${build_number}": func(h *model.Host) string {
					return os.Getenv("BUILD_NUMBER")
				},
				"${ziti_version}": func(h *model.Host) string {
					return h.MustStringVariable("ziti_version")
				},
			}),
		rsync.RsyncStaged(),
	},

	Disposal: model.DisposalStages{
		terraform.Dispose(),
		aws_ssh_key2.Dispose(),
	},
}

func main() {
	//m.AddActivationActions("stop", "bootstrap", "start")
	m.AddActivationActions("stop", "bootstrap")
	model.AddBootstrapExtension(
		zitilab.BootstrapWithFallbacks(
			&zitilab.BootstrapFromEnv{},
		))
	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)

	endpoint := func(m *model.Model) string {
		return m.MustSelectHost("component.iperf-server").PublicIp
	}

	m.AddOperatingStage(fablib_5_operation.Iperf("Ziti_Overlay", endpoint, "component.iperf-server", "component.iperf-client", 60, true))
	m.AddOperatingStage(fablib_5_operation.Persist())
	m.AddOperatingStage(fablib_5_operation.Iperf("Ziti_Underlay_Only", endpoint, "component.iperf-server", "component.iperf-client", 60, false))
	m.AddOperatingStage(fablib_5_operation.Persist())

	fablab.InitModel(m)
	fablab.Run()

}
