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

package main

import (
	"embed"
	"fmt"
	"github.com/openziti/fablab"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/binding"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/aws_ssh_key"
	semaphore0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/semaphore"
	terraform_0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/terraform"
	distribution "github.com/openziti/fablab/kernel/lib/runlevel/3_distribution"
	"github.com/openziti/fablab/kernel/lib/runlevel/3_distribution/rsync"
	aws_ssh_key2 "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/resources"
	"github.com/openziti/ziti/zititest/models/ha/actions"
	"github.com/openziti/ziti/zititest/models/test_resources"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
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

var m = &model.Model{
	Id: "ha",
	Scope: model.Scope{
		Defaults: model.Variables{
			"environment": "ha-smoketest",
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

	Resources: model.Resources{
		resources.Configs:   resources.SubFolder(configResource, "configs"),
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
							Scope: model.Scope{Tags: model.Tags{"ctrl"}},
							Type:  &zitilab.ControllerType{},
						},
					},
				},
				"ctrl2": {
					InstanceType: "t3.micro",
					Components: model.Components{
						"ctrl2": {
							Scope: model.Scope{Tags: model.Tags{"ctrl"}},
							Type:  &zitilab.ControllerType{},
						},
					},
				},

				"router-east": {
					InstanceType: "t2.micro",
					Components: model.Components{
						"router-east": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "terminator"}},
							Type:  &zitilab.RouterType{},
						},
						"echo-server": {
							Scope: model.Scope{Tags: model.Tags{"sdk-app", "service"}},
							Type:  &zitilab.EchoServerType{},
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
							Scope: model.Scope{Tags: model.Tags{"ctrl"}},
							Type:  &zitilab.ControllerType{},
						},
					},
				},

				"router-west": {
					Scope:        model.Scope{Tags: model.Tags{}},
					InstanceType: "t2.micro",
					Components: model.Components{
						"router-west": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "terminator"}},
							Type:  &zitilab.RouterType{},
						},
						"zcat": {
							Scope: model.Scope{Tags: model.Tags{"sdk-app"}},
							Type:  &zitilab.ZCatType{},
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
				LogPath:    "consul/log.out",
			}),
		"stop":  model.Bind(component.StopInParallel("*", 15)),
		"login": model.Bind(edge.Login("#ctrl1")),
	},

	Infrastructure: model.Stages{
		aws_ssh_key.Express(),
		terraform_0.Express(),
		semaphore0.Ready(time.Minute),
	},

	Distribution: model.Stages{
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

	Disposal: model.Stages{
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
