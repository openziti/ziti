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
	"github.com/michaelquigley/pfxlog"
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
	"github.com/openziti/ziti/zititest/models/simple/actions"
	"github.com/openziti/ziti/zititest/models/test_resources"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"os"
	"time"
)

const ZitiEdgeTunnelVersion = "v0.22.17"

//go:embed configs
var configResource embed.FS

func getUniqueId() string {
	if runId := os.Getenv("GITHUB_RUN_ID"); runId != "" {
		return "-" + runId + "." + os.Getenv("GITHUB_RUN_ATTEMPT")
	}
	return "-" + os.Getenv("USER")
}

var Model = &model.Model{
	Id: "simple-transfer",
	Scope: model.Scope{
		Defaults: model.Variables{
			"environment": "simple-transfer-smoketest" + getUniqueId(),
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
		},
	},

	StructureFactories: []model.Factory{
		model.FactoryFunc(func(m *model.Model) error {
			if val, _ := m.GetBoolVariable("ha"); !val {
				for _, host := range m.SelectHosts("component.ha") {
					delete(host.Region.Hosts, host.Id)
				}
			}
			return nil
		}),
	},

	Factories: []model.Factory{
		model.FactoryFunc(func(m *model.Model) error {
			pfxlog.Logger().Infof("environment [%s]", m.MustStringVariable("environment"))
			m.AddActivationActions("stop", "bootstrap", "start")
			return nil
		}),
		model.FactoryFunc(func(m *model.Model) error {
			return m.ForEachHost("*", 1, func(host *model.Host) error {
				host.InstanceType = "t2.micro"
				return nil
			})
		}),

		model.FactoryFunc(func(m *model.Model) error {
			zetPath, useLocalPath := m.GetStringVariable("local_zet_path")
			return m.ForEachComponent("*", 1, func(c *model.Component) error {
				if c.Type == nil {
					return nil
				}

				if zet, ok := c.Type.(*zitilab.ZitiEdgeTunnelType); ok {
					if useLocalPath {
						zet.Version = ""
						zet.LocalPath = zetPath
					} else {
						zet.Version = ZitiEdgeTunnelVersion
						zet.LocalPath = ""
					}
					zet.InitType(c)
					return nil
				}

				return nil
			})
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
				"ctrl1": {
					Components: model.Components{
						"ctrl1": {
							Scope: model.Scope{Tags: model.Tags{"ctrl"}},
							Type:  &zitilab.ControllerType{},
						},
					},
				},
				"ctrl2": {
					Components: model.Components{
						"ctrl2": {
							Scope: model.Scope{Tags: model.Tags{"ctrl", "ha"}},
							Type:  &zitilab.ControllerType{},
						},
					},
				},
				"router-east-1": {
					Scope: model.Scope{Tags: model.Tags{"ert-client"}},
					Components: model.Components{
						"router-east-1": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "terminator", "tunneler", "client"}},
							Type:  &zitilab.RouterType{},
						},
						"zcat": {
							Scope: model.Scope{Tags: model.Tags{"sdk-app", "client"}},
							Type:  &zitilab.ZCatType{},
						},
					},
				},
				"router-east-2": {
					Components: model.Components{
						"router-east-2": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "initiator"}},
							Type:  &zitilab.RouterType{},
						},
					},
				},
				"ziti-edge-tunnel-client": {
					Scope: model.Scope{Tags: model.Tags{"zet-client"}},
					Components: model.Components{
						"ziti-edge-tunnel-client": {
							Scope: model.Scope{Tags: model.Tags{"sdk-app", "client"}},
							Type: &zitilab.ZitiEdgeTunnelType{
								Version: ZitiEdgeTunnelVersion,
							},
						},
					},
				},
				"ziti-tunnel-client": {
					Scope: model.Scope{Tags: model.Tags{"ziti-tunnel-client"}},
					Components: model.Components{
						"ziti-tunnel-client": {
							Scope: model.Scope{Tags: model.Tags{"ziti-tunnel", "sdk-app", "client"}},
							Type:  &zitilab.ZitiTunnelType{},
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
					Components: model.Components{
						"ctrl3": {
							Scope: model.Scope{Tags: model.Tags{"ctrl", "ha"}},
							Type:  &zitilab.ControllerType{},
						},
					},
				},

				"router-west": {
					Components: model.Components{
						"router-west": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "tunneler", "host", "ert-host"}},
							Type:  &zitilab.RouterType{},
						},
						"echo-server": {
							Scope: model.Scope{Tags: model.Tags{"sdk-app", "service"}},
							Type:  &zitilab.EchoServerType{},
						},
						"iperf-server-ert": {
							Scope: model.Scope{Tags: model.Tags{"iperf", "service"}},
							Type:  &zitilab.IPerfServerType{},
						},
					},
				},
				"ziti-edge-tunnel-host": {
					Components: model.Components{
						"ziti-edge-tunnel-host": {
							Scope: model.Scope{Tags: model.Tags{"sdk-app", "host", "zet-host"}},
							Type: &zitilab.ZitiEdgeTunnelType{
								Version: ZitiEdgeTunnelVersion,
							},
						},
						"iperf-server-zet": {
							Scope: model.Scope{Tags: model.Tags{"iperf", "service"}},
							Type:  &zitilab.IPerfServerType{},
						},
					},
				},
				"ziti-tunnel-host": {
					Components: model.Components{
						"ziti-tunnel-host": {
							Scope: model.Scope{Tags: model.Tags{"ziti-tunnel", "sdk-app", "host", "ziti-tunnel-host"}},
							Type: &zitilab.ZitiTunnelType{
								Mode: zitilab.ZitiTunnelModeHost,
							},
						},
						"iperf-server-zt": {
							Scope: model.Scope{Tags: model.Tags{"iperf", "service"}},
							Type:  &zitilab.IPerfServerType{},
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
		"login":     model.Bind(edge.Login("#ctrl1")),
	},

	Infrastructure: model.Stages{
		aws_ssh_key.Express(),
		&terraform_0.Terraform{
			Retries: 3,
			ReadyCheck: &semaphore0.ReadyStage{
				MaxWait: 90 * time.Second,
			},
		},
	},

	Distribution: model.Stages{
		distribution.DistributeSshKey("*"),
		rsync.RsyncStaged(),
	},

	Disposal: model.Stages{
		terraform.Dispose(),
		aws_ssh_key2.Dispose(),
	},
}

func InitBootstrapExtensions() {
	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)
}
