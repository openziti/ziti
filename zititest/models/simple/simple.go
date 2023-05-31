/*
	(c) Copyright NetFoundry Inc.

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

package simple

import (
	"embed"
	"fmt"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/binding"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/aws_ssh_key"
	semaphore0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/semaphore"
	terraform_0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/terraform"
	"github.com/openziti/fablab/kernel/lib/runlevel/1_configuration/config"
	"github.com/openziti/fablab/kernel/lib/runlevel/2_kitting/devkit"
	distribution "github.com/openziti/fablab/kernel/lib/runlevel/3_distribution"
	"github.com/openziti/fablab/kernel/lib/runlevel/3_distribution/rsync"
	aws_ssh_key2 "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/resources"
	actions2 "github.com/openziti/ziti/zititest/models/simple/actions"
	"github.com/openziti/ziti/zititest/models/simple/stages/5_operation"
	"github.com/openziti/ziti/zititest/models/test_resources"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	zitilib_runlevel_1_configuration "github.com/openziti/ziti/zititest/zitilab/runlevel/1_configuration"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
	"github.com/sirupsen/logrus"
	"os"
	"time"
)

//go:embed configs
var configResource embed.FS

func getConfigData(filePath string) []byte {
	data, err := configResource.ReadFile(fmt.Sprintf("configs/%s", filePath))
	if err != nil {
		logrus.Errorf("Unable to read config data from %s: [%s]", filePath, err)
	}
	return data
}

var Model = &model.Model{
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

	Factories: []model.Factory{
		model.FactoryFunc(func(m *model.Model) error {
			m.AddActivationActions("stop", "bootstrap", "start")
			m.AddOperatingStage(runlevel_5_operation.AssertEcho("#echo-client"))

			return nil
		}),
	},

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
					InstanceType: "t2.micro",
					Components: model.Components{
						"ctrl": {
							Scope:          model.Scope{Tags: model.Tags{"ctrl"}},
							BinaryName:     "ziti controller",
							ConfigSrc:      "ctrl.yml",
							ConfigName:     "ctrl.yml",
							PublicIdentity: "ctrl",
						},
						"consul": {
							BinaryName: "consul",
						},
					},
				},
				"router-east-1": {
					Scope:        model.Scope{Tags: model.Tags{"ert-client"}},
					InstanceType: "t2.micro",
					Components: model.Components{
						"router-east-1": {
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "terminator", "tunneler", "client"}},
							BinaryName:     "ziti router",
							ConfigSrc:      "router.yml",
							ConfigName:     "router-east-1.yml",
							PublicIdentity: "router-east-1",
							RunWithSudo:    true,
						},
						"echo-client": {
							Scope:          model.Scope{Tags: model.Tags{"sdk-app", "client"}},
							BinaryName:     "echo-client",
							PublicIdentity: "echo-client",
						},
						"consul": {
							BinaryName: "consul",
						},
					},
				},
				"router-east-2": {
					InstanceType: "t2.micro",
					Components: model.Components{
						"router-east-2": {
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "initiator"}},
							BinaryName:     "ziti router",
							ConfigSrc:      "router.yml",
							ConfigName:     "router-east-2.yml",
							PublicIdentity: "router-east-2",
						},
						"consul": {
							BinaryName: "consul",
						},
					},
				},
				"ziti-edge-tunnel-client": {
					Scope:        model.Scope{Tags: model.Tags{"zet-client"}},
					InstanceType: "t2.micro",
					Components: model.Components{
						"ziti-edge-tunnel-client": {
							Scope:          model.Scope{Tags: model.Tags{"sdk-app", "client"}},
							BinaryName:     "ziti-edge-tunnel",
							PublicIdentity: "ziti-edge-tunnel-client",
							RunWithSudo:    true,
						},
						"consul": {
							BinaryName: "consul",
						},
					},
				},
				"ziti-tunnel-client": {
					Scope:        model.Scope{Tags: model.Tags{"ziti-tunnel-client"}},
					InstanceType: "t2.micro",
					Components: model.Components{
						"ziti-tunnel-client": {
							Scope:          model.Scope{Tags: model.Tags{"ziti-tunnel", "sdk-app", "client"}},
							BinaryName:     "ziti tunnel",
							PublicIdentity: "ziti-tunnel-client",
							RunWithSudo:    true,
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
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "tunneler", "host", "ert-host"}},
							BinaryName:     "ziti router",
							ConfigSrc:      "router.yml",
							ConfigName:     "router-west.yml",
							PublicIdentity: "router-west",
							RunWithSudo:    true,
						},
						"echo-server": {
							Scope:          model.Scope{Tags: model.Tags{"sdk-app", "service"}},
							BinaryName:     "echo-server",
							PublicIdentity: "echo-server",
						},
						"consul": {
							BinaryName: "consul",
						},
					},
				},
				"ziti-edge-tunnel-host": {
					InstanceType: "t2.micro",
					Components: model.Components{
						"ziti-edge-tunnel-host": {
							Scope:          model.Scope{Tags: model.Tags{"sdk-app", "host", "zet-host"}},
							BinaryName:     "ziti-edge-tunnel",
							PublicIdentity: "ziti-edge-tunnel-host",
							RunWithSudo:    true,
						},
						"consul": {
							BinaryName: "consul",
						},
					},
				},
				"ziti-tunnel-host": {
					InstanceType: "t2.micro",
					Components: model.Components{
						"ziti-tunnel-host": {
							Scope:          model.Scope{Tags: model.Tags{"ziti-tunnel", "sdk-app", "host", "ziti-tunnel-host"}},
							BinaryName:     "ziti tunnel",
							PublicIdentity: "ziti-tunnel-host",
							RunWithSudo:    true,
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
		"bootstrap": actions2.NewBootstrapAction(),
		"start": actions2.NewStartAction(actions2.MetricbeatConfig{
			ConfigPath: "metricbeat",
			DataPath:   "metricbeat/data",
			LogPath:    "metricbeat/logs",
		},
			actions2.ConsulConfig{
				ServerAddr: os.Getenv("CONSUL_ENDPOINT"),
				ConfigDir:  "consul",
				DataPath:   "consul/data",
				LogPath:    "consul/log.out",
			}),
		"stop":  model.Bind(component.StopInParallel("*", 15)),
		"login": model.Bind(edge.Login("#ctrl")),
	},

	Infrastructure: model.InfrastructureStages{
		aws_ssh_key.Express(),
		terraform_0.Express(),
		semaphore0.Ready(time.Minute),
	},

	Configuration: model.ConfigurationStages{
		zitilib_runlevel_1_configuration.IfPkiNeedsRefresh(
			zitilib_runlevel_1_configuration.Fabric("simple-transfer.test", "#ctrl"),
		),
		config.Component(),
		devkit.DevKitF(zitilab.ZitiRoot, []string{"ziti", "ziti-echo"}),
		stageziti.FetchZitiEdgeTunnel("v0.21.4"),
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
				"${build_number}": func(h *model.Host) string {
					return os.Getenv("BUILD_NUMBER")
				},
				"${ziti_version}": func(h *model.Host) string {
					return h.MustStringVariable("ziti_version")
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
				"${build_number}": func(h *model.Host) string {
					return os.Getenv("BUILD_NUMBER")
				},
				"${ziti_version}": func(h *model.Host) string {
					return h.MustStringVariable("ziti_version")
				},
			},
		),
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

func InitBootstrapExtensions() {
	model.AddBootstrapExtension(
		zitilab.BootstrapWithFallbacks(
			&zitilab.BootstrapFromEnv{},
		))
	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)
}
