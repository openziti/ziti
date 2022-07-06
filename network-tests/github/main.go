//go:build tests

package main

import (
	"embed"
	"time"

	"github.com/openziti/fablab"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/binding"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/aws_ssh_key"
	semaphore_0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/semaphore"
	terraform_0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/terraform"
	"github.com/openziti/fablab/kernel/lib/runlevel/1_configuration/config"
	distribution "github.com/openziti/fablab/kernel/lib/runlevel/3_distribution"
	"github.com/openziti/fablab/kernel/lib/runlevel/3_distribution/rsync"
	aws_ssh_key2 "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/resources"
	"github.com/openziti/ziti/network-tests/github/actions"
	"github.com/openziti/zitilab"
	zitilib_runlevel_1_configuration "github.com/openziti/zitilab/runlevel/1_configuration"
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

var m = &model.Model{
	Id: "test",
	Scope: model.Scope{
		Defaults: model.Variables{
			"environment": "ziti-github-test",
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
		resources.Configs: resources.SubFolder(configResource, "configs"),
		//resources.Binaries:  os.DirFS("/Users/cam/go/bin"),
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
						"loop-listener-{{ .Host.ScaleIndex }}": {
							Scope:          model.Scope{Tags: model.Tags{"sdk-app", "service"}},
							BinaryName:     "ziti-fabric-test",
							PublicIdentity: "loop-listener-{{ .Host.ScaleIndex }}",
						},
					},
				},
				"metrics-router": {
					InstanceType: "t2.micro",
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
					InstanceType: "t2.micro",
					Components: model.Components{
						"router-west-{{ .Host.ScaleIndex }}": {
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "tunneler", "terminator"}},
							BinaryName:     "ziti-router",
							ConfigSrc:      "router.yml",
							ConfigName:     "router-west-{{ .Host.ScaleIndex }}.yml",
							PublicIdentity: "router-west-{{ .Host.ScaleIndex }}",
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
		"bootstrap": actions.NewBootstrapAction(),
		"start":     actions.NewStartAction(),
		"stop":      model.Bind(component.StopInParallel("*", 15)),
		//"stopSdkApps": model.Bind(component.StopInParallel(".sdk-app", 15)),
		"test": actions.NewSyncModelEdgeStateAction(),
		//"clean": model.Bind(fabactions.Workflow(
		//	component.StopInParallel("*", 15),
		//	host.GroupExec("*", 25, "rm -f logs/*"),
		//)),
		//"login": model.Bind(edge.Login("#ctrl")),
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
	m.AddActivationActions("stop", "bootstrap", "start", "test")

	model.AddBootstrapExtension(
		zitilab.BootstrapWithFallbacks(
			//zitilab.BootstrapFromDir("/Users/cam/bin/linux/amd64", "/Users/cam/bin/linux/amd64"),
			//&zitilab.BootstrapFromFind{},
			&zitilab.BootstrapFromEnv{},
		))
	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)

	fablab.InitModel(m)
	fablab.Run()
}
