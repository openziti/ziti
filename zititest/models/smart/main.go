/*
	Copyright 2019 NetFoundry Inc.

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

	"github.com/openziti/fablab"
	"github.com/openziti/fablab/kernel/lib/binding"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/aws_ssh_key"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/resources"
	"github.com/openziti/ziti/zititest/zitilab"
)

//go:embed config
var configs embed.FS

func init() {
	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)
}

var smartrouting = &model.Model{
	Id: "smart-routing",
	Resources: model.Resources{
		resources.Configs:   resources.SubFolder(configs, "config"),
		resources.Terraform: resources.DefaultTerraformResources(),
	},
	Scope: model.Scope{
		Defaults: model.Variables{
			"zitilib.fabric.data_plane_protocol": "tls",
			"credentials.ssh.username":           "ubuntu",
			"distribution": model.Variables{
				"rsync_bin": "rsync",
				"ssh_bin":   "ssh",
			},
			"ziti_fabric_path": "ziti-fabric",
			"instance_type":    "t2.micro",
		},
	},

	Factories: []model.Factory{
		newHostsFactory(),
		newActionsFactory(),
		newStageFactory(),
	},

	Regions: model.Regions{
		"initiator": {
			Region: "us-east-1",
			Site:   "us-east-1a",
			Hosts: model.Hosts{
				"ctrl": {
					Components: model.Components{
						"ctrl": {
							Scope: model.Scope{Tags: model.Tags{"ctrl"}},
							Type:  &zitilab.ControllerType{},
						},
					},
				},
				"001": {
					Scope: model.Scope{Tags: model.Tags{"iperf_server"}},
					Components: model.Components{
						"001": {
							Scope: model.Scope{Tags: model.Tags{"initiator", "router"}},
							Type: &zitilab.RouterType{
								ConfigSource: "ingress_router.yml.tmpl",
							},
						},
					},
				},
				"loop0": {
					Scope: model.Scope{Tags: model.Tags{"loop-dialer"}},
				},
				"loop1": {
					Scope: model.Scope{Tags: model.Tags{"loop-dialer"}},
				},
				"loop2": {
					Scope: model.Scope{Tags: model.Tags{"loop-dialer"}},
				},
				"loop3": {
					Scope: model.Scope{Tags: model.Tags{"loop-dialer"}},
				},
			},
		},
		"transitA": {
			Region: "us-east-1",
			Site:   "us-east-1b",
			Hosts: model.Hosts{
				"002": {
					Scope: model.Scope{Tags: model.Tags{"router"}},
					Components: model.Components{
						"002": {
							Type: &zitilab.RouterType{
								ConfigSource: "transit_router.yml.tmpl",
							},
						},
					},
				},
			},
		},
		"transitB": {
			Region: "us-east-1",
			Site:   "us-east-1c",
			Hosts: model.Hosts{
				"004": {
					Components: model.Components{
						"004": {
							Scope: model.Scope{Tags: model.Tags{"router"}},
							Type: &zitilab.RouterType{
								ConfigSource: "transit_router.yml.tmpl",
							},
						},
					},
				},
			},
		},
		"terminator": {
			Region: "us-west-2",
			Site:   "us-west-2b",
			Hosts: model.Hosts{
				"003": {
					Components: model.Components{
						"003": {
							Scope: model.Scope{Tags: model.Tags{"router", "terminator"}},
							Type: &zitilab.RouterType{
								ConfigSource: "egress_router.yml.tmpl",
							},
						},
					},
				},
				"loop0": {
					Scope: model.Scope{Tags: model.Tags{"loop-listener"}},
				},
				"loop1": {
					Scope: model.Scope{Tags: model.Tags{"loop-listener"}},
				},
				"loop2": {
					Scope: model.Scope{Tags: model.Tags{"loop-listener"}},
				},
				"loop3": {
					Scope: model.Scope{Tags: model.Tags{"loop-listener"}},
				},
			},
		},
	},
}

func main() {
	//smartrouting.VarConfig.EnableDebugLogger()

	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)

	fablab.InitModel(smartrouting)
	fablab.Run()
}
