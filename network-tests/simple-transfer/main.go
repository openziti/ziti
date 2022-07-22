package main

import (
	"embed"
	"fmt"
	"os"
	"time"

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
	aws_ssh_key2 "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/resources"
	"github.com/openziti/ziti/network-tests/simple-transfer/actions"
	"github.com/openziti/zitilab"
	zitilib_runlevel_0_infrastructure "github.com/openziti/zitilab/runlevel/0_infrastructure"
	zitilib_runlevel_1_configuration "github.com/openziti/zitilab/runlevel/1_configuration"
	"github.com/sirupsen/logrus"
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

func getConfigData(filePath string) []byte {
	data, err := configResource.ReadFile(fmt.Sprintf("configs/%s", filePath))
	if err != nil {
		logrus.Errorf("Unable to read config data from %s: [%s]", filePath, err)
	}
	return data
}

var m = &model.Model{
	Id: "simple-transfer",
	Scope: model.Scope{
		Defaults: model.Variables{
			"environment": "simple-transfer-smoketest",
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
					InstanceType: "t2.micro",
					Components: model.Components{
						"ctrl": {
							Scope:          model.Scope{Tags: model.Tags{"ctrl"}},
							BinaryName:     "ziti-controller",
							ConfigSrc:      "ctrl.yml",
							ConfigName:     "ctrl.yml",
							PublicIdentity: "ctrl",
						},
						"consul": {
							BinaryName: "consul",
						},
					},
				},
				"router-east": {
					InstanceType: "t2.micro",
					Components: model.Components{
						"router-east": {
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "terminator"}},
							BinaryName:     "ziti-router",
							ConfigSrc:      "router.yml",
							ConfigName:     "router-east.yml",
							PublicIdentity: "router-east",
							RunWithSudo:    true,
						},
						"echo-server": {
							Scope:          model.Scope{Tags: model.Tags{"sdk-app", "service"}},
							PublicIdentity: "echo-server",
						},
						"consul": {
							BinaryName: "consul",
						},
					},
				},
			},
		},
		"us-west-2": {
			Region: "us-west-2",
			Site:   "us-west-2b",
			Hosts: model.Hosts{
				"router-west": {
					Scope:        model.Scope{Tags: model.Tags{}},
					InstanceType: "t2.micro",
					Components: model.Components{
						"router-west": {
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "terminator"}},
							BinaryName:     "ziti-router",
							ConfigSrc:      "router.yml",
							ConfigName:     "router-west.yml",
							PublicIdentity: "router-west",
							RunWithSudo:    true,
						},
						"echo-client": {
							Scope:          model.Scope{Tags: model.Tags{"sdk-app", "client"}},
							PublicIdentity: "echo-client",
						},
						"consul": {
							BinaryName: "consul",
						},
					},
				},
			},
		},
	},

	Actions: model.ActionBinders{
		"bootstrap": actions.NewBootstrapAction(),
		"start": actions.NewStartAction(actions.MetricbeatConfig{
			ConfigPath: "metricbeat",
			DataPath:   "metricbeat/data",
			LogPath:    "metricbeat/logs",
		},
			actions.ConsulConfig{
				ServerAddr: os.Getenv("CONSUL_ENDPOINT"),
				ConfigDir:  "consul",
				DataPath:   "consul/data",
			}),
		"stop": model.Bind(component.StopInParallel("*", 15)),
	},

	Infrastructure: model.InfrastructureStages{
		aws_ssh_key.Express(),
		terraform_0.Express(),
		zitilib_runlevel_0_infrastructure.InstallMetricbeat("*"),
		zitilib_runlevel_0_infrastructure.InstallConsul("*"),
		semaphore_0.Restart(90 * time.Second),
	},

	Configuration: model.ConfigurationStages{
		zitilib_runlevel_1_configuration.IfPkiNeedsRefresh(
			zitilib_runlevel_1_configuration.Fabric(),
			zitilib_runlevel_1_configuration.DotZiti(),
		),
		config.Component(),
		zitilab.DefaultZitiBinaries(),
		devkit.DevKitF(zitilab.ZitiDistBinaries, []string{"ziti-echo"}),
	},

	Distribution: model.DistributionStages{
		distribution.DistributeSshKey("*"),
		distribution.Locations("*", "logs"),
		distribution.DistributeDataWithReplaceCallbacks(
			"*",
			string(getConfigData("metricbeat.yml")),
			"metricbeat/metricbeat.yml",
			os.FileMode(0644),
			map[string]func(*model.Host) string{
				"${host}": func(h *model.Host) string {
					return os.Getenv("ELASTIC_ENDPOINT")
				},
				"${user}": func(h *model.Host) string {
					return os.Getenv("ELASTIC_USERNAME")
				},
				"${password}": func(h *model.Host) string {
					return os.Getenv("ELASTIC_PASSWORD")
				},
			},
		),

		distribution.DistributeDataWithReplaceCallbacks(
			"*",
			string(getConfigData("consul.hcl")),
			"consul/consul.hcl",
			os.FileMode(0644),
			map[string]func(*model.Host) string{
				"${public_ip}": func(h *model.Host) string {
					return h.PublicIp
				},
				"${encryption_key}": func(h *model.Host) string {
					return os.Getenv("CONSUL_ENCRYPTION_KEY")
				},
			},
		),
		distribution.DistributeData(
			"#ctrl",
			getConfigData("ziti.hcl"),
			"consul/ziti.hcl"),
		distribution.DistributeData(
			"*",
			[]byte(os.Getenv("CONSUL_AGENT_CERT")),
			"consul/consul-agent-ca.pem"),
		rsync.RsyncStaged(),
	},

	Disposal: model.DisposalStages{
		terraform.Dispose(),
		aws_ssh_key2.Dispose(),
	},
}

func main() {
	m.AddActivationActions("stop", "bootstrap", "start")

	model.AddBootstrapExtension(
		zitilab.BootstrapWithFallbacks(
			&zitilab.BootstrapFromEnv{},
		))
	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)

	fablab.InitModel(m)
	fablab.Run()
}
