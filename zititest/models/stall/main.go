package main

import (
	"embed"
	_ "embed"
	"github.com/openziti/fablab"
	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/lib/binding"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/semaphore"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/terraform"
	"github.com/openziti/fablab/kernel/lib/runlevel/1_configuration/config"
	distribution "github.com/openziti/fablab/kernel/lib/runlevel/3_distribution"
	"github.com/openziti/fablab/kernel/lib/runlevel/3_distribution/rsync"
	aws_ssh_key2 "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/resources"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	zitilib_runlevel_1_configuration "github.com/openziti/ziti/zititest/zitilab/runlevel/1_configuration"
	"os"
	"time"
)

//go:embed configs
var configResource embed.FS

type scaleStrategy struct{}

func (self scaleStrategy) IsScaled(entity model.Entity) bool {
	return entity.GetType() == model.EntityTypeHost && entity.GetScope().HasTag("scaled")
}

func (self scaleStrategy) GetEntityCount(entity model.Entity) uint32 {
	if entity.GetType() == model.EntityTypeHost && entity.GetScope().HasTag("scaled") {
		return 4
	}
	return 1
}

var m = &model.Model{
	Id: "stall",
	Scope: model.Scope{
		Defaults: model.Variables{
			"environment": "ziti-stall-test",
			"credentials": model.Variables{
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
	},
	Factories: []model.Factory{
		newStageFactory(),
	},
	Resources: model.Resources{
		resources.Configs:   resources.SubFolder(configResource, "configs"),
		resources.Binaries:  os.DirFS("/home/plorenz/go/bin"),
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
						},
					},
				},
				"metrics-router": {
					InstanceType: "c5.large",
					Components: model.Components{
						"metrics-router": {
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "no-traversal"}},
							BinaryName:     "ziti-router",
							ConfigSrc:      "router.yml",
							ConfigName:     "metrics-router.yml",
							PublicIdentity: "metrics-router",
						},
					},
				},
			},
		},
		"us-west-2": {
			Region: "us-west-2",
			Site:   "us-west-2b",
			Hosts: model.Hosts{
				"router-west-{{ .ScaleIndex }}": {
					Scope:        model.Scope{Tags: model.Tags{"scaled"}},
					InstanceType: "c5.large",
					Components: model.Components{
						"router-west-{{ .Host.ScaleIndex }}": {
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "tunneler", "terminator"}},
							BinaryName:     "ziti-router",
							ConfigSrc:      "router.yml",
							ConfigName:     "router-west-{{ .Host.ScaleIndex }}.yml",
							PublicIdentity: "router-west-{{ .Host.ScaleIndex }}",
							RunWithSudo:    true,
						},
						"loop-listener-{{ .Host.ScaleIndex }}": {
							Scope:          model.Scope{Tags: model.Tags{"sdk-app", "service"}},
							BinaryName:     "ziti-fabric-test",
							PublicIdentity: "loop-listener-{{ .Host.ScaleIndex }}",
						},
					},
				},
			},
		},
		"ap-southeast-1": {
			Region: "ap-southeast-1",
			Site:   "ap-southeast-1a",
			Hosts: model.Hosts{
				"router-ap-{{ .ScaleIndex }}": {
					Scope:        model.Scope{Tags: model.Tags{"scaled"}},
					InstanceType: "c5.large",
					Components: model.Components{
						"router-ap-{{ .Host.ScaleIndex }}": {
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "tunneler", "initiator"}},
							BinaryName:     "ziti-router",
							ConfigSrc:      "router.yml",
							ConfigName:     "router-ap-{{ .Host.ScaleIndex }}.yml",
							PublicIdentity: "router-ap-{{ .Host.ScaleIndex }}",
							RunWithSudo:    true,
						},
						"loop-client-{{ .Host.ScaleIndex }}": {
							Scope:          model.Scope{Tags: model.Tags{"sdk-app", "client"}},
							BinaryName:     "ziti-fabric-test",
							ConfigSrc:      "test.loop3.yml",
							ConfigName:     "test.loop3.yml",
							PublicIdentity: "loop-client-{{ .Host.ScaleIndex }}",
						},
					},
				},
			},
		},
	},

	Actions: model.ActionBinders{
		"bootstrap":          NewBootstrapAction(),
		"start":              NewStartAction(),
		"stop":               model.Bind(component.StopInParallel("*", 15)),
		"stopSdkApps":        model.Bind(component.StopInParallel(".sdk-app", 15)),
		"syncModelEdgeState": NewSyncModelEdgeStateAction(),
		"clean": model.Bind(actions.Workflow(
			component.StopInParallel("*", 15),
			host.GroupExec("*", 25, "rm -f logs/*"),
		)),
		"login": model.Bind(edge.Login("#ctrl")),
	},

	Infrastructure: model.InfrastructureStages{
		aws_ssh_key.Express(),
		terraform_0.Express(),
		semaphore_0.Restart(90 * time.Second),
	},

	Configuration: model.ConfigurationStages{
		zitilib_runlevel_1_configuration.IfPkiNeedsRefresh(
			zitilib_runlevel_1_configuration.Fabric("stall.test"),
		),
		config.Component(),
		zitilab.DefaultZitiBinaries(),
	},

	Distribution: model.DistributionStages{
		distribution.DistributeSshKey("*"),
		distribution.Locations("*", "logs"),
		rsync.RsyncStaged(),
	},

	Disposal: model.DisposalStages{
		terraform.Dispose(),
		aws_ssh_key2.Dispose(),
	},
}

func main() {
	m.AddActivationActions("stop", "bootstrap", "start", "syncModelEdgeState")
	// m.VarConfig.EnableDebugLogger()

	model.AddBootstrapExtension(&zitilab.BootstrapFromEnv{})
	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)

	fablab.InitModel(m)
	fablab.Run()
}
