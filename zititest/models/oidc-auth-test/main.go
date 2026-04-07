/*
	Copyright NetFoundry Inc.

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
	_ "embed"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab"
	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/lib/actions/semaphore"
	"github.com/openziti/fablab/kernel/lib/binding"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/semaphore"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/terraform"
	distribution "github.com/openziti/fablab/kernel/lib/runlevel/3_distribution"
	"github.com/openziti/fablab/kernel/lib/runlevel/3_distribution/rsync"
	awsSshKeyDispose "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/resources"
	"github.com/openziti/ziti/v2/zitirest"
	"github.com/openziti/ziti/zititest/models/test_resources"
	"github.com/openziti/ziti/zititest/zitilab"
	zitilibActions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/openziti/ziti/zititest/zitilab/models"
	zitilibOps "github.com/openziti/ziti/zititest/zitilab/runlevel/5_operation"
	"github.com/openziti/ziti/zititest/zitilab/validations"
)

const (
	targetZitiVersion = ""
	//targetZitiCSdkVersion = "1.14.3"
	targetZitiCSdkVersion = ""

	// Per region: 2 ER/T, 2 client-facing, 1 SDK-hosting = 5 routers
	ertRoutersPerRegion    = 2
	clientRoutersPerRegion = 2
	sdkRoutersPerRegion    = 1

	// Client hosts (scaled)
	goClientHostsPerRegion   = 2
	csdkClientHostsPerRegion = 18
	clientsPerHost           = 100

	// Service hosting: 1 host per region, each running Go + ZDE instances
	goHostsPerHostingBox  = 5
	zdeHostsPerHostingBox = 10

	// Expected terminators per service.
	// svc-ert: ER/T routers host on themselves, 1 terminator each.
	// svc-go/svc-zde: SDKs create up to 3 terminators per hosted service by
	// default (one per available edge router, capped at 3). With 3 regions
	// each having at least 1 SDK-hosting router, every hosting app reaches
	// the max of 3 terminators.
	expectedErtTerminators = 2 * 3      // 2 ER/T routers * 3 regions
	expectedGoTerminators  = 5 * 3 * 3  // 5 Go SDK * 3 regions * 3 per app
	expectedZdeTerminators = 10 * 3 * 3 // 10 ZDE * 3 regions * 3 per app

	// Expected OIDC sessions: Go direct clients + ZDE proxy OIDC clients.
	// Go direct: 2*3*100 = 600
	// C-SDK proxy: 18*3*100 = 5400
	// Total: 6000
	expectedOidcSessions = 6000
)

var expectedServiceTerminators = []validations.ServiceTerminatorExpectation{
	{ServiceName: "svc-ert", ExpectedCount: expectedErtTerminators},
	{ServiceName: "svc-go", ExpectedCount: expectedGoTerminators},
	{ServiceName: "svc-zde", ExpectedCount: expectedZdeTerminators},
}

//go:embed configs
var configResource embed.FS

var ctrlClients models.CtrlClients
var eventCollector zitilibOps.OidcEventCollector
var trafficCollector zitilibOps.TrafficResultsCollector

const (
	labelPrefixClient   = "identity:client:"
	labelPrefixProx     = "identity:prox:"
	labelPrefixGoClient = "identity:go-client:"
)

// clientIdentityIds is the authoritative set of identity IDs for all client
// components (prox-c and go-client), populated during bootstrap and persisted
// to the label for reuse across process invocations.
var clientIdentityIds map[string]bool

// proxIdentityIds is the subset of clientIdentityIds for C-SDK prox-c clients.
var proxIdentityIds map[string]bool

// goClientIdentityIds is the subset of clientIdentityIds for Go SDK clients.
var goClientIdentityIds map[string]bool

var identityRegistryLoaded sync.Once

// buildClientIdentityRegistry queries the controller for every client component's
// identity ID, populates the package-level maps, and persists them to the label.
func buildClientIdentityRegistry(run model.Run) error {
	log := pfxlog.Logger()
	clients, err := initCtrl1(run)
	if err != nil {
		return err
	}

	// Get all client + prox identities from the controller in one query.
	filter := `(anyOf(roleAttributes) = "client" or anyOf(roleAttributes) = "prox") limit none`
	identities, err := models.ListIdentities(clients, filter, 2*time.Minute)
	if err != nil {
		return fmt.Errorf("failed to list client identities: %w", err)
	}

	// Build name -> id map from API results.
	nameToId := make(map[string]string, len(identities))
	for _, ident := range identities {
		nameToId[*ident.Name] = *ident.ID
	}

	label := run.GetLabel()

	// Clear old entries.
	for k := range label.Bindings {
		if strings.HasPrefix(k, "identity:") {
			delete(label.Bindings, k)
		}
	}

	// Build the authoritative sets from model components and persist to label.
	clientIdentityIds = make(map[string]bool)
	proxIdentityIds = make(map[string]bool)
	goClientIdentityIds = make(map[string]bool)

	for _, c := range run.GetModel().SelectComponents(".prox") {
		if id, ok := nameToId[c.Id]; ok {
			clientIdentityIds[id] = true
			proxIdentityIds[id] = true
			label.Bindings[labelPrefixClient+id] = ""
			label.Bindings[labelPrefixProx+id] = ""
		} else {
			log.Warnf("identity for component %s not found in controller", c.Id)
		}
	}
	for _, c := range run.GetModel().SelectComponents(".go-client") {
		if id, ok := nameToId[c.Id]; ok {
			clientIdentityIds[id] = true
			goClientIdentityIds[id] = true
			label.Bindings[labelPrefixClient+id] = ""
			label.Bindings[labelPrefixGoClient+id] = ""
		} else {
			log.Warnf("identity for component %s not found in controller", c.Id)
		}
	}

	log.Infof("built client identity registry: %d total (%d prox, %d go-client, %d from API)",
		len(clientIdentityIds), len(proxIdentityIds), len(goClientIdentityIds), len(identities))

	return label.Save()
}

func initCtrl1(run model.Run) (*zitirest.Clients, error) {
	if err := ctrlClients.InitOidc(run, "#ctrl1", time.Minute); err != nil {
		return nil, err
	}
	return ctrlClients.GetCtrl("ctrl1"), nil
}

type scaleStrategy struct{}

func (self scaleStrategy) IsScaled(entity model.Entity) bool {
	if entity.GetType() == model.EntityTypeHost {
		return entity.GetScope().HasTag("ert-router") ||
			entity.GetScope().HasTag("client-router") ||
			entity.GetScope().HasTag("sdk-router") ||
			entity.GetScope().HasTag("go-client") ||
			entity.GetScope().HasTag("csdk-client")
	}
	if entity.GetType() == model.EntityTypeComponent {
		return entity.GetScope().HasTag("client") ||
			entity.GetScope().HasTag("go-host") ||
			entity.GetScope().HasTag("zde-host")
	}
	return false
}

func (self scaleStrategy) GetEntityCount(entity model.Entity) uint32 {
	if entity.GetType() == model.EntityTypeHost {
		if entity.GetScope().HasTag("ert-router") {
			return ertRoutersPerRegion
		}
		if entity.GetScope().HasTag("client-router") {
			return clientRoutersPerRegion
		}
		if entity.GetScope().HasTag("sdk-router") {
			return sdkRoutersPerRegion
		}
		if entity.GetScope().HasTag("go-client") {
			return goClientHostsPerRegion
		}
		if entity.GetScope().HasTag("csdk-client") {
			return csdkClientHostsPerRegion
		}
	}
	if entity.GetType() == model.EntityTypeComponent {
		if entity.GetScope().HasTag("go-host") {
			return goHostsPerHostingBox
		}
		if entity.GetScope().HasTag("zde-host") {
			return zdeHostsPerHostingBox
		}
		if entity.GetScope().HasTag("client") {
			return clientsPerHost
		}
	}
	return 1
}

var m = &model.Model{
	Id: "oidc-auth-test",
	Scope: model.Scope{
		Defaults: model.Variables{
			"ha":          true,
			"environment": "oidc-auth-test",
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
	StructureFactories: []model.Factory{
		model.FactoryFunc(func(m *model.Model) error {
			err := m.ForEachHost("component.ctrl", 1, func(host *model.Host) error {
				host.InstanceType = "c5.2xlarge"
				return nil
			})
			if err != nil {
				return err
			}

			err = m.ForEachHost("component.router", 1, func(host *model.Host) error {
				host.InstanceType = "c5.xlarge"
				return nil
			})
			if err != nil {
				return err
			}

			err = m.ForEachHost(".go-host", 1, func(host *model.Host) error {
				host.InstanceType = "c5.xlarge"
				return nil
			})
			if err != nil {
				return err
			}

			err = m.ForEachHost(".zde-host", 1, func(host *model.Host) error {
				host.InstanceType = "c5.xlarge"
				return nil
			})
			if err != nil {
				return err
			}

			return m.ForEachHost(".client,.hosting", 1, func(host *model.Host) error {
				host.InstanceType = "c5.xlarge"
				return nil
			})
		}),
		model.FactoryFunc(func(m *model.Model) error {
			if val, _ := m.GetBoolVariable("ha"); !val {
				for _, host := range m.SelectHosts("component.ha") {
					delete(host.Region.Hosts, host.Id)
				}
			}
			return nil
		}),
		model.NewScaleFactoryWithDefaultEntityFactory(&scaleStrategy{}),
	},
	Resources: model.Resources{
		resources.Configs:   resources.SubFolder(configResource, "configs"),
		resources.Binaries:  os.DirFS(path.Join(os.Getenv("GOPATH"), "bin")),
		resources.Terraform: test_resources.TerraformResources(),
	},
	Regions: model.Regions{
		"us-east-1": {
			Region: "us-east-1",
			Site:   "us-east-1a",
			Hosts: model.Hosts{
				// Controller
				"ctrl1": {
					Components: model.Components{
						"ctrl1": {
							Scope: model.Scope{Tags: model.Tags{"ctrl", "preferredLeader"}},
							Type: &zitilab.ControllerType{
								Version: targetZitiVersion,
							},
						},
						"event-fwd-1": {
							Scope: model.Scope{Tags: model.Tags{"event-forwarder"}},
							Type:  &zitilab.EventForwarderType{},
						},
					},
				},

				// ER/T routers - tunnel-capable, host svc-ert
				"ert-router-us-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"ert-router", "router", "tunneler"}},
					Components: model.Components{
						"ert-router-us-{{.Host.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"router", "tunneler", "ert-router"}},
							Type: &zitilab.RouterType{
								Version: targetZitiVersion,
							},
						},
						"echo-ert-us-{{.Host.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"echo-server", "echo-server-ert"}},
							Type: &zitilab.EchoServerType{
								Version: targetZitiVersion,
								Port:    8080,
							},
						},
					},
				},

				// Client-facing routers
				"client-router-us-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"client-router", "router"}},
					Components: model.Components{
						"client-router-us-{{.Host.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"router", "client-router"}},
							Type: &zitilab.RouterType{
								Version: targetZitiVersion,
							},
						},
					},
				},

				// SDK-hosting router
				"sdk-router-us-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"sdk-router", "router"}},
					Components: model.Components{
						"sdk-router-us-{{.Host.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"router", "sdk-router"}},
							Type: &zitilab.RouterType{
								Version: targetZitiVersion,
							},
						},
					},
				},

				// Go SDK client hosts (sdk-direct mode)
				"go-client-us-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"go-client", "client"}},
					Components: model.Components{
						"go-client-us-{{.Host.ScaleIndex}}-{{.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"client", "go-client"}},
							Type: &zitilab.OidcTestClientType{
								Mode:           zitilab.OidcTestClientSdkDirect,
								Services:       "svc-ert,svc-go,svc-zde",
								ResultsService: "traffic-results",
							},
						},
					},
				},

				// C-SDK client hosts: 100 prox-c instances + 1 traffic driver
				"csdk-client-us-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"csdk-client", "client"}},
					Components: model.Components{
						"prox-us-{{.Host.ScaleIndex}}-{{.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"prox", "client"}},
							Type: &zitilab.ZitiProxCType{
								Version:  targetZitiCSdkVersion,
								Services: []string{"svc-ert", "svc-go", "svc-zde"},
								BasePort: 9700,
								LogLevel: 4,
							},
						},
						"traffic-us-{{.Host.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"traffic-driver"}},
							Type: &zitilab.OidcTestClientType{
								Mode:               zitilab.OidcTestClientProxy,
								Services:           "svc-ert,svc-go,svc-zde",
								ProxyBasePort:      9700,
								ProxyInstanceCount: clientsPerHost,
								ResultsService:     "traffic-results",
							},
						},
					},
				},

				// Hosting box: 5 Go SDK + 10 ZDE instances + 1 echo server
				"hosting-us": {
					Scope: model.Scope{Tags: model.Tags{"hosting"}},
					Components: model.Components{
						"go-host-us-{{.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"go-host", "sdk-app"}},
							Type: &zitilab.ZitiTunnelType{
								Version: targetZitiVersion,
								Mode:    zitilab.ZitiTunnelModeHost,
							},
						},
						"zde-host-us-{{.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"zde-host", "sdk-app"}},
							Type: &zitilab.ZitiEdgeTunnelType{
								Mode: zitilab.ZitiEdgeTunnelModeHost,
							},
						},
						"echo-server-us": {
							Scope: model.Scope{Tags: model.Tags{"echo-server"}},
							Type: &zitilab.EchoServerType{
								Version: targetZitiVersion,
								Port:    8080,
							},
						},
					},
				},
			},
		},
		"eu-west-2": {
			Region: "eu-west-2",
			Site:   "eu-west-2a",
			Hosts: model.Hosts{
				// Controller
				"ctrl2": {
					Components: model.Components{
						"ctrl2": {
							Scope: model.Scope{Tags: model.Tags{"ctrl", "ha", "preferredLeader"}},
							Type: &zitilab.ControllerType{
								Version: targetZitiVersion,
							},
						},
						"event-fwd-2": {
							Scope: model.Scope{Tags: model.Tags{"event-forwarder"}},
							Type:  &zitilab.EventForwarderType{},
						},
					},
				},

				// ER/T routers
				"ert-router-eu-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"ert-router", "router", "tunneler"}},
					Components: model.Components{
						"ert-router-eu-{{.Host.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"router", "tunneler", "ert-router"}},
							Type: &zitilab.RouterType{
								Version: targetZitiVersion,
							},
						},
						"echo-ert-eu-{{.Host.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"echo-server", "echo-server-ert"}},
							Type: &zitilab.EchoServerType{
								Version: targetZitiVersion,
								Port:    8080,
							},
						},
					},
				},

				// Client-facing routers
				"client-router-eu-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"client-router", "router"}},
					Components: model.Components{
						"client-router-eu-{{.Host.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"router", "client-router"}},
							Type: &zitilab.RouterType{
								Version: targetZitiVersion,
							},
						},
					},
				},

				// SDK-hosting router
				"sdk-router-eu-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"sdk-router", "router"}},
					Components: model.Components{
						"sdk-router-eu-{{.Host.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"router", "sdk-router"}},
							Type: &zitilab.RouterType{
								Version: targetZitiVersion,
							},
						},
					},
				},

				// Go SDK client hosts
				"go-client-eu-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"go-client", "client"}},
					Components: model.Components{
						"go-client-eu-{{.Host.ScaleIndex}}-{{.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"client", "go-client"}},
							Type: &zitilab.OidcTestClientType{
								Mode:           zitilab.OidcTestClientSdkDirect,
								Services:       "svc-ert,svc-go,svc-zde",
								ResultsService: "traffic-results",
							},
						},
					},
				},

				// C-SDK client hosts: 100 prox-c instances + 1 traffic driver
				"csdk-client-eu-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"csdk-client", "client"}},
					Components: model.Components{
						"prox-eu-{{.Host.ScaleIndex}}-{{.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"prox", "client"}},
							Type: &zitilab.ZitiProxCType{
								Version:  targetZitiCSdkVersion,
								Services: []string{"svc-ert", "svc-go", "svc-zde"},
								BasePort: 9700,
								LogLevel: 4,
							},
						},
						"traffic-eu-{{.Host.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"traffic-driver"}},
							Type: &zitilab.OidcTestClientType{
								Mode:               zitilab.OidcTestClientProxy,
								Services:           "svc-ert,svc-go,svc-zde",
								ProxyBasePort:      9700,
								ProxyInstanceCount: clientsPerHost,
								ResultsService:     "traffic-results",
							},
						},
					},
				},

				// Hosting box: 5 Go SDK + 10 ZDE instances + 1 echo server
				"hosting-eu": {
					Scope: model.Scope{Tags: model.Tags{"hosting"}},
					Components: model.Components{
						"go-host-eu-{{.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"go-host", "sdk-app"}},
							Type: &zitilab.ZitiTunnelType{
								Version: targetZitiVersion,
								Mode:    zitilab.ZitiTunnelModeHost,
							},
						},
						"zde-host-eu-{{.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"zde-host", "sdk-app"}},
							Type: &zitilab.ZitiEdgeTunnelType{
								Mode: zitilab.ZitiEdgeTunnelModeHost,
							},
						},
						"echo-server-eu": {
							Scope: model.Scope{Tags: model.Tags{"echo-server"}},
							Type: &zitilab.EchoServerType{
								Version: targetZitiVersion,
								Port:    8080,
							},
						},
					},
				},
			},
		},
		"ap-southeast-2": {
			Region: "ap-southeast-2",
			Site:   "ap-southeast-2a",
			Hosts: model.Hosts{
				// Controller
				"ctrl3": {
					Components: model.Components{
						"ctrl3": {
							Scope: model.Scope{Tags: model.Tags{"ctrl", "ha"}},
							Type: &zitilab.ControllerType{
								Version: targetZitiVersion,
							},
						},
						"event-fwd-3": {
							Scope: model.Scope{Tags: model.Tags{"event-forwarder"}},
							Type:  &zitilab.EventForwarderType{},
						},
					},
				},

				// ER/T routers
				"ert-router-ap-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"ert-router", "router", "tunneler"}},
					Components: model.Components{
						"ert-router-ap-{{.Host.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"router", "tunneler", "ert-router"}},
							Type: &zitilab.RouterType{
								Version: targetZitiVersion,
							},
						},
						"echo-ert-ap-{{.Host.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"echo-server", "echo-server-ert"}},
							Type: &zitilab.EchoServerType{
								Version: targetZitiVersion,
								Port:    8080,
							},
						},
					},
				},

				// Client-facing routers
				"client-router-ap-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"client-router", "router"}},
					Components: model.Components{
						"client-router-ap-{{.Host.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"router", "client-router"}},
							Type: &zitilab.RouterType{
								Version: targetZitiVersion,
							},
						},
					},
				},

				// SDK-hosting router
				"sdk-router-ap-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"sdk-router", "router"}},
					Components: model.Components{
						"sdk-router-ap-{{.Host.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"router", "sdk-router"}},
							Type: &zitilab.RouterType{
								Version: targetZitiVersion,
							},
						},
					},
				},

				// Go SDK client hosts
				"go-client-ap-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"go-client", "client"}},
					Components: model.Components{
						"go-client-ap-{{.Host.ScaleIndex}}-{{.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"client", "go-client"}},
							Type: &zitilab.OidcTestClientType{
								Mode:           zitilab.OidcTestClientSdkDirect,
								Services:       "svc-ert,svc-go,svc-zde",
								ResultsService: "traffic-results",
							},
						},
					},
				},

				// C-SDK client hosts: 100 prox-c instances + 1 traffic driver
				"csdk-client-ap-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"csdk-client", "client"}},
					Components: model.Components{
						"prox-ap-{{.Host.ScaleIndex}}-{{.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"prox", "client"}},
							Type: &zitilab.ZitiProxCType{
								Version:  targetZitiCSdkVersion,
								Services: []string{"svc-ert", "svc-go", "svc-zde"},
								BasePort: 9700,
								LogLevel: 4,
							},
						},
						"traffic-ap-{{.Host.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"traffic-driver"}},
							Type: &zitilab.OidcTestClientType{
								Mode:               zitilab.OidcTestClientProxy,
								Services:           "svc-ert,svc-go,svc-zde",
								ProxyBasePort:      9700,
								ProxyInstanceCount: clientsPerHost,
								ResultsService:     "traffic-results",
							},
						},
					},
				},

				// Hosting box: 5 Go SDK + 10 ZDE instances + 1 echo server
				"hosting-ap": {
					Scope: model.Scope{Tags: model.Tags{"hosting"}},
					Components: model.Components{
						"go-host-ap-{{.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"go-host", "sdk-app"}},
							Type: &zitilab.ZitiTunnelType{
								Version: targetZitiVersion,
								Mode:    zitilab.ZitiTunnelModeHost,
							},
						},
						"zde-host-ap-{{.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"zde-host", "sdk-app"}},
							Type: &zitilab.ZitiEdgeTunnelType{
								Mode: zitilab.ZitiEdgeTunnelModeHost,
							},
						},
						"echo-server-ap": {
							Scope: model.Scope{Tags: model.Tags{"echo-server"}},
							Type: &zitilab.EchoServerType{
								Version: targetZitiVersion,
								Port:    8080,
							},
						},
					},
				},
			},
		},
	},

	Actions: model.ActionBinders{
		"bootstrap": model.ActionBinder(func(m *model.Model) model.Action {
			workflow := actions.Workflow()

			isHA := len(m.SelectComponents(".ctrl")) > 1

			workflow.AddAction(component.StopInParallel("*", 10000))
			workflow.AddAction(host.GroupExec("component.ctrl", 100, "rm -f logs/* ctrl.db"))
			workflow.AddAction(host.GroupExec("component.ctrl", 5, "rm -rf ./fablab/ctrldata"))

			if !isHA {
				workflow.AddAction(component.Exec("#ctrl1", zitilab.ControllerActionInitStandalone))
			}

			workflow.AddAction(component.Start("#ctrl1"))

			if isHA {
				workflow.AddAction(semaphore.Sleep(2 * time.Second))
				workflow.AddAction(edge.InitRaftController("#ctrl1"))
			}

			workflow.AddAction(edge.ControllerAvailable("#ctrl1", 30*time.Second))
			workflow.AddAction(edge.Login("#ctrl1"))

			// Init all routers, hosting identities, and event forwarder identities
			workflow.AddAction(edge.InitEdgeRoutersWithClients(models.RouterTag, 25, initCtrl1))
			workflow.AddAction(edge.InitIdentitiesWithClients(".sdk-app", 50, initCtrl1))
			workflow.AddAction(edge.InitIdentitiesWithClients(".prox", 50, initCtrl1))
			workflow.AddAction(edge.InitIdentitiesWithClients(".go-client", 50, initCtrl1))
			workflow.AddAction(edge.InitIdentitiesWithClients(".event-forwarder", 10, initCtrl1))
			workflow.AddAction(edge.InitIdentitiesWithClients(".traffic-driver", 100, initCtrl1))
			workflow.AddAction(model.ActionFunc(func(run model.Run) error {
				clients, err := initCtrl1(run)
				if err != nil {
					return err
				}
				return eventCollector.SetupCollectorIdentity(run, clients)
			}))
			workflow.AddAction(model.ActionFunc(func(run model.Run) error {
				clients, err := initCtrl1(run)
				if err != nil {
					return err
				}
				return trafficCollector.SetupCollectorIdentity(run, clients)
			}))

			workflow.AddAction(edge.Login("#ctrl1")) // Re-login; CLI session may have expired during identity creation
			workflow.AddAction(model.RunAction("createBaseModel"))
			workflow.AddAction(model.ActionFunc(buildClientIdentityRegistry))

			workflow.AddAction(model.RunAction("initHA"))

			workflow.AddAction(component.StartInParallel(".router", 10))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))

			// Start event and traffic collection before clients connect
			workflow.AddAction(model.ActionFunc(func(run model.Run) error {
				if err := eventCollector.StartCollecting(run, "oidc-events"); err != nil {
					return err
				}
				return trafficCollector.StartCollecting(run, "traffic-results")
			}))
			workflow.AddAction(component.StartInParallel(".event-forwarder", 10))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))

			// Start hosting components first, then proxies, then clients
			workflow.AddAction(component.StartInParallel(".sdk-app", 50))
			workflow.AddAction(component.StartInParallel(".echo-server", 50))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))
			workflow.AddAction(component.StartInParallel(".prox", 10000))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))
			workflow.AddAction(component.StartInParallel(".go-client", 10000))
			workflow.AddAction(component.StartInParallel(".traffic-driver", 100))

			return workflow
		}),
		"initHA": model.ActionBinder(func(m *model.Model) model.Action {
			workflow := actions.Workflow()
			isHA := len(m.SelectComponents(".ctrl")) > 1
			if isHA {
				workflow.AddAction(component.StartInParallel(".ctrl", 10))
				workflow.AddAction(semaphore.Sleep(2 * time.Second))
				workflow.AddAction(edge.RaftJoin("ctrl1", ".ctrl"))
				workflow.AddAction(semaphore.Sleep(5 * time.Second))
			}
			return workflow
		}),
		"createBaseModel": model.ActionBinder(func(m *model.Model) model.Action {
			workflow := actions.Workflow()

			// Edge router policies: clients use client-routers, hosting identities use sdk-routers
			workflow.AddAction(zitilibActions.Edge("create", "edge-router-policy", "client-erp",
				"--identity-roles", "#client,#prox",
				"--edge-router-roles", "#client-router"))
			workflow.AddAction(zitilibActions.Edge("create", "edge-router-policy", "hosting-erp",
				"--identity-roles", "#go-host,#zde-host",
				"--edge-router-roles", "#sdk-router"))

			// Service edge router policy: all services can use all edge routers
			workflow.AddAction(zitilibActions.Edge("create", "service-edge-router-policy", "all",
				"--service-roles", "#all",
				"--edge-router-roles", "#all"))

			// Host config for echo servers
			hostConfig := `{
				"address": "localhost",
				"port": 8080,
				"protocol": "tcp"
			}`
			workflow.AddAction(zitilibActions.Edge("create", "config", "host-config", "host.v1", hostConfig))

			// Create the three services
			workflow.AddAction(zitilibActions.Edge("create", "service", "svc-ert",
				"-c", "host-config",
				"-a", "ert-service"))
			workflow.AddAction(zitilibActions.Edge("create", "service", "svc-go",
				"-c", "host-config",
				"-a", "go-service"))
			workflow.AddAction(zitilibActions.Edge("create", "service", "svc-zde",
				"-c", "host-config",
				"-a", "zde-service"))

			// Bind service policies: ER/T routers bind svc-ert, Go hosts bind svc-go, ZDE hosts bind svc-zde
			workflow.AddAction(zitilibActions.Edge("create", "service-policy", "ert-bind", "Bind",
				"--identity-roles", "#ert-router",
				"--service-roles", "@svc-ert"))
			workflow.AddAction(zitilibActions.Edge("create", "service-policy", "go-bind", "Bind",
				"--identity-roles", "#go-host",
				"--service-roles", "@svc-go"))
			workflow.AddAction(zitilibActions.Edge("create", "service-policy", "zde-bind", "Bind",
				"--identity-roles", "#zde-host",
				"--service-roles", "@svc-zde"))

			// Dial service policies: clients can dial all three services
			workflow.AddAction(zitilibActions.Edge("create", "service-policy", "client-dial", "Dial",
				"--identity-roles", "#client,#prox",
				"--service-roles", "#ert-service,#go-service,#zde-service"))

			// Infrastructure services: event collection and traffic results.
			workflow.AddAction(zitilibActions.Edge("create", "service", "oidc-events"))
			workflow.AddAction(zitilibActions.Edge("create", "service", "traffic-results"))

			workflow.AddAction(zitilibActions.Edge("create", "service-policy", "events-bind", "Bind",
				"--identity-roles", "#oidc-event-collector",
				"--service-roles", "@oidc-events"))
			workflow.AddAction(zitilibActions.Edge("create", "service-policy", "events-dial", "Dial",
				"--identity-roles", "#event-forwarder",
				"--service-roles", "@oidc-events"))

			workflow.AddAction(zitilibActions.Edge("create", "service-policy", "traffic-bind", "Bind",
				"--identity-roles", "#traffic-collector",
				"--service-roles", "@traffic-results"))
			workflow.AddAction(zitilibActions.Edge("create", "service-policy", "traffic-dial", "Dial",
				"--identity-roles", "#traffic-driver,#go-client",
				"--service-roles", "@traffic-results"))

			workflow.AddAction(zitilibActions.Edge("create", "edge-router-policy", "infra-erp",
				"--identity-roles", "#oidc-event-collector,#event-forwarder,#traffic-collector,#traffic-driver",
				"--edge-router-roles", "#all"))

			return workflow
		}),
		"stop": model.Bind(component.StopInParallelHostExclusive("*", 10000)),
		"clean": model.Bind(actions.Workflow(
			component.StopInParallelHostExclusive("*", 10000),
			host.GroupExec("*", 100, "rm -f logs/*"),
		)),
		"login":  model.Bind(edge.Login("#ctrl1")),
		"login2": model.Bind(edge.Login("#ctrl2")),
		"login3": model.Bind(edge.Login("#ctrl3")),
		"restart": model.ActionBinder(func(m *model.Model) model.Action {
			workflow := actions.Workflow()
			workflow.AddAction(component.StopInParallel("*", 10000))
			workflow.AddAction(host.GroupExec("*", 100, "rm -f logs/*"))
			workflow.AddAction(component.Start(".ctrl"))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))
			workflow.AddAction(component.StartInParallel(".router", 10))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))
			workflow.AddAction(component.StartInParallel(".sdk-app", 50))
			workflow.AddAction(component.StartInParallel(".echo-server", 50))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))
			workflow.AddAction(component.StartInParallel(".prox", 10000))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))
			workflow.AddAction(component.StartInParallel(".go-client", 10000))
			workflow.AddAction(component.StartInParallel(".traffic-driver", 100))
			return workflow
		}),
		"validate": model.BindF(func(run model.Run) error {
			if err := ensureInitialized(run); err != nil {
				return err
			}
			if err := validations.ValidateServiceTerminators(run, 15*time.Minute, expectedServiceTerminators, validations.ValidateSdkTerminators|validations.ValidateErtTerminators); err != nil {
				return err
			}
			return validations.ValidateOidcAuthenticated(&eventCollector, clientIdentityIds, 15*time.Minute)
		}),
		"validateUp": model.BindF(func(run model.Run) error {
			if err := chaos.ValidateUp(run, ".ctrl", 3, 15*time.Second); err != nil {
				return err
			}
			err := run.GetModel().ForEachComponent(".ctrl", 3, func(c *model.Component) error {
				return edge.ControllerAvailable(c.Id, time.Minute).Execute(run)
			})
			if err != nil {
				return err
			}
			if err := chaos.ValidateUp(run, ".router", 100, time.Minute); err != nil {
				pfxlog.Logger().WithError(err).Error("validate up failed, trying to start all routers again")
				return component.StartInParallel(".router", 100).Execute(run)
			}
			return nil
		}),
		"sowChaos":               model.BindF(sowChaos),
		"testSteadyState":        model.BindF(testSteadyState),
		"testShortPartition":     model.BindF(testShortPartition),
		"testMediumPartition":    model.BindF(testMediumPartition),
		"testLongPartition":      model.BindF(testLongPartition),
		"testControllerFailover": model.BindF(testControllerFailover),
		"testChaosIteration":     model.BindF(testChaosIteration),
		"restartClients":         model.BindF(restartAllClients),
		"fullSuite": model.BindF(func(run model.Run) error {
			return run.GetModel().Exec(run,
				"restartClients",
				"testSteadyState",
				"testShortPartition",
				"testSteadyState",
				"testMediumPartition",
				"testSteadyState",
				"testLongPartition",
				"testSteadyState",
				"testControllerFailover",
				"testSteadyState",
				"testChaosIteration",
				"testSteadyState",
			)
		}),
	},

	Infrastructure: model.Stages{
		aws_ssh_key.Express(),
		&terraform_0.Terraform{
			Retries: 3,
			ReadyCheck: &semaphore_0.ReadyStage{
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
		awsSshKeyDispose.Dispose(),
	},
}

func main() {
	m.AddActivationActions("bootstrap")

	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)

	fablab.InitModel(m)
	fablab.Run()
}
