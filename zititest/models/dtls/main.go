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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/binding"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/aws_ssh_key"
	semaphore0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/semaphore"
	terraform_0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/terraform"
	distribution "github.com/openziti/fablab/kernel/lib/runlevel/3_distribution"
	"github.com/openziti/fablab/kernel/lib/runlevel/3_distribution/rsync"
	fablib_5_operation "github.com/openziti/fablab/kernel/lib/runlevel/5_operation"
	aws_ssh_key2 "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/resources"
	"github.com/openziti/ziti/zititest/models/dtls/actions"
	"github.com/openziti/ziti/zititest/models/test_resources"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/models"
	zitilib_5_operation "github.com/openziti/ziti/zititest/zitilab/runlevel/5_operation"
	"os"
	"strings"
	"time"
)

const ZitiEdgeTunnelVersion = "v2.0.0-alpha1"

//go:embed configs
var configResource embed.FS

func getUniqueId() string {
	if runId := os.Getenv("GITHUB_RUN_ID"); runId != "" {
		return "-" + runId + "." + os.Getenv("GITHUB_RUN_ATTEMPT")
	}
	return "-" + os.Getenv("USER")
}

var Model = &model.Model{
	Id: "dtls",
	Scope: model.Scope{
		Defaults: model.Variables{
			"environment": "dtls" + getUniqueId(),
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
			"metrics": model.Variables{
				"influxdb": model.Variables{
					"url": "http://localhost:8086",
					"db":  "ziti",
				},
			},
		},
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
					Components: model.Components{
						"ctrl": {
							Scope: model.Scope{Tags: model.Tags{"ctrl"}},
							Type:  &zitilab.ControllerType{},
						},
					},
				},
				"router-east-1": {
					Scope: model.Scope{Tags: model.Tags{"ert-client"}},
					Components: model.Components{
						"router-east-1": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "terminator", "tunneler", "client"}},
							Type: &zitilab.RouterType{
								Debug: false,
							},
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
					Components: model.Components{
						"router-west": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "tunneler", "host", "ert-host"}},
							Type: &zitilab.RouterType{
								Debug: false,
							},
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
		"login":     model.Bind(edge.Login("#ctrl")),
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

	Operation: model.Stages{
		edge.SyncModelEdgeState(models.EdgeRouterTag),
		fablib_5_operation.InfluxMetricsReporter(),
		zitilib_5_operation.ModelMetricsWithIdMapper(nil, func(id string) string {
			if id == "ctrl" {
				return "#ctrl"
			}
			id = strings.ReplaceAll(id, ".", ":")
			return "component.edgeId:" + id
		}),

		zitilib_5_operation.CircuitMetrics(5*time.Second, nil, func(id string) string {
			id = strings.ReplaceAll(id, ".", ":")
			return "component.edgeId:" + id
		}),
		model.StageActionF(func(run model.Run) error {
			time.Sleep(time.Hour * 24)
			return nil
		}),
	},
}

func InitBootstrapExtensions() {
	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)
}

func main() {
	InitBootstrapExtensions()
	fablab.InitModel(Model)
	fablab.Run()
}
