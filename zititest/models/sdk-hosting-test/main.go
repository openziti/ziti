package main

import (
	"embed"
	_ "embed"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/fablab"
	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/lib/actions/semaphore"
	"github.com/openziti/fablab/kernel/lib/binding"
	"github.com/openziti/fablab/kernel/lib/parallel"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/semaphore"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/terraform"
	distribution "github.com/openziti/fablab/kernel/lib/runlevel/3_distribution"
	"github.com/openziti/fablab/kernel/lib/runlevel/3_distribution/rsync"
	awsSshKeyDispose "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/resources"
	"github.com/openziti/foundation/v2/util"
	errUtil "github.com/openziti/ziti/v2/ziti/util"
	"github.com/openziti/ziti/v2/zitirest"
	"github.com/openziti/ziti/zititest/models/test_resources"
	"github.com/openziti/ziti/zititest/zitilab"
	zitilibActions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/openziti/ziti/zititest/zitilab/cli"
	"github.com/openziti/ziti/zititest/zitilab/models"
	cmap "github.com/orcaman/concurrent-map/v2"
)

const (
	targetZitiVersion = ""
	useZde            = false

	routersPerRegion      = 2
	hostsPerRegion        = 5
	tunnelersPerHost      = 100
	servicesPerTunneler   = 5
	terminatorsPerService = 2
)

//go:embed configs
var configResource embed.FS

type scaleStrategy struct{}

func (self scaleStrategy) IsScaled(entity model.Entity) bool {
	if entity.GetType() == model.EntityTypeHost {
		return entity.GetScope().HasTag("router") || entity.GetScope().HasTag("host")
	}
	return entity.GetType() == model.EntityTypeComponent && entity.GetScope().HasTag("host")
}

func (self scaleStrategy) GetEntityCount(entity model.Entity) uint32 {
	if entity.GetType() == model.EntityTypeHost {
		if entity.GetScope().HasTag("router") {
			return routersPerRegion
		}
		if entity.GetScope().HasTag("host") {
			return hostsPerRegion
		}
	}
	if entity.GetType() == model.EntityTypeComponent {
		return tunnelersPerHost
	}
	return 1
}

var m = &model.Model{
	Id: "sdk-hosting-test",
	Scope: model.Scope{
		Defaults: model.Variables{
			"ha":          true,
			"environment": "sdk-hosting-test",
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
				host.InstanceType = "c5.2xlarge" // need larger cpu for all the tls handshaking with 200 hosts
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

			err = m.ForEachComponent(".host", 1, func(c *model.Component) error {
				if useZde {
					c.Type = &zitilab.ZitiEdgeTunnelType{
						Version: "1.9.5",
						Mode:    zitilab.ZitiEdgeTunnelModeHost,
					}
				} else {
					c.Type.(*zitilab.ZitiTunnelType).Mode = zitilab.ZitiTunnelModeHost
				}
				return nil
			})

			if err != nil {
				return err
			}

			return m.ForEachHost("component.host", 1, func(host *model.Host) error {
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
				"ctrl1": {
					Components: model.Components{
						"ctrl1": {
							Scope: model.Scope{Tags: model.Tags{"ctrl"}},
							Type: &zitilab.ControllerType{
								Version: targetZitiVersion,
							},
						},
					},
				},
				"router-us-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"router"}},
					Components: model.Components{
						"router-us-{{.Host.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"router"}},
							Type: &zitilab.RouterType{
								Version: targetZitiVersion,
							},
						},
					},
				},
				"host-us-{{ .ScaleIndex }}": {
					Scope: model.Scope{Tags: model.Tags{"host"}},
					Components: model.Components{
						"host-us-{{ .Host.ScaleIndex }}-{{ .ScaleIndex }}": {
							Scope: model.Scope{Tags: model.Tags{"host"}},
							Type: &zitilab.ZitiTunnelType{
								Version: targetZitiVersion,
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
				"ctrl2": {
					Components: model.Components{
						"ctrl2": {
							Scope: model.Scope{Tags: model.Tags{"ctrl", "ha"}},
							Type: &zitilab.ControllerType{
								Version: targetZitiVersion,
							},
						},
					},
				},
				"router-eu-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"router"}},
					Components: model.Components{
						"router-eu-{{.Host.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"router"}},
							Type: &zitilab.RouterType{
								Version: targetZitiVersion,
							},
						},
					},
				},
				"host-eu-{{ .ScaleIndex }}": {
					Scope: model.Scope{Tags: model.Tags{"host"}},
					Components: model.Components{
						"host-eu-{{ .Host.ScaleIndex }}-{{ .ScaleIndex }}": {
							Scope: model.Scope{Tags: model.Tags{"host"}},
							Type: &zitilab.ZitiTunnelType{
								Version: targetZitiVersion,
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
				"ctrl3": {
					Components: model.Components{
						"ctrl3": {
							Scope: model.Scope{Tags: model.Tags{"ctrl", "ha"}},
							Type: &zitilab.ControllerType{
								Version: targetZitiVersion,
							},
						},
					},
				},
				"router-ap-{{.ScaleIndex}}": {
					Scope: model.Scope{Tags: model.Tags{"router", "scaled"}},
					Components: model.Components{
						"router-ap-{{.Host.ScaleIndex}}": {
							Scope: model.Scope{Tags: model.Tags{"router"}},
							Type: &zitilab.RouterType{
								Version: targetZitiVersion,
							},
						},
					},
				},
				"host-ap-{{ .ScaleIndex }}": {
					Scope: model.Scope{Tags: model.Tags{"host", "scaled"}},
					Components: model.Components{
						"host-ap-{{ .Host.ScaleIndex }}-{{ .ScaleIndex }}": {
							Scope: model.Scope{Tags: model.Tags{"host"}},
							Type: &zitilab.ZitiTunnelType{
								Version: targetZitiVersion,
							},
						},
					},
				},
			},
		},
	},

	Actions: model.ActionBinders{
		"createBaseModel": model.ActionBinder(func(m *model.Model) model.Action {
			workflow := actions.Workflow()
			workflow.AddAction(zitilibActions.Edge("create", "edge-router-policy", "all", "--edge-router-roles", "#all", "--identity-roles", "#all"))
			workflow.AddAction(zitilibActions.Edge("create", "service-edge-router-policy", "all", "--service-roles", "#all", "--edge-router-roles", "#all"))

			hostConfig := fmt.Sprintf(`{
					"address" : "localhost",
					"port" : 8080,
					"protocol" : "tcp",
                    "listenOptions": {
                         "maxConnections": %d
                    }
				}`, terminatorsPerService)

			workflow.AddAction(zitilibActions.Edge("create", "config", "host-config", "host.v1", hostConfig))

			workflow.AddAction(model.ActionFunc(func(run model.Run) error {
				var tasks []parallel.Task
				for i := 0; i < 2000; i++ {
					name := fmt.Sprintf("service-%04d", i)
					task := func() error {
						_, err := cli.Exec(run.GetModel(), "edge", "create", "service", name, "-c", "host-config", "--timeout", "15")
						return err
					}
					tasks = append(tasks, task)
				}
				return parallel.Execute(tasks, 25)
			}))

			return workflow
		}),
		"createServicePolicies": model.ActionBinder(func(m *model.Model) model.Action {
			ctrls := models.CtrlClients{}
			var ctrl1 *zitirest.Clients

			workflow := actions.Workflow()

			workflow.AddAction(model.ActionFunc(func(run model.Run) error {
				if err := ctrls.Init(run, "#ctrl1"); err != nil {
					return err
				}
				ctrl1 = ctrls.GetCtrl("ctrl1")
				return nil
			}))

			svcIdCache := cmap.New[string]()

			workflow.AddAction(model.ActionFunc(func(run model.Run) error {
				identities := getHostNames()
				serviceIdx := 0
				var tasks []parallel.Task
				for i, identity := range identities {
					tasks = append(tasks, func() error {
						policyName := fmt.Sprintf("service-policy-%03d", i)
						identityId, err := models.GetIdentityId(ctrl1, identity, 5*time.Second)
						if err != nil {
							return err
						}
						identityRole := fmt.Sprintf("@%s", identityId)
						var serviceRoles []string
						for j := 0; j < servicesPerTunneler; j++ {
							idx := serviceIdx % 2000
							svcName := fmt.Sprintf("service-%04d", idx)
							svcId, ok := svcIdCache.Get(svcName)
							if !ok {
								svcId, err = models.GetServiceId(ctrl1, svcName, 5*time.Second)
								if err != nil {
									return err
								}
								svcIdCache.Set(svcName, svcId)
							}
							serviceRoles = append(serviceRoles, fmt.Sprintf("@%s", svcId))
							serviceIdx++
						}

						attempts := 0
						for {
							err := models.CreateServicePolicy(ctrl1, &rest_model.ServicePolicyCreate{
								IdentityRoles: []string{identityRole},
								Name:          util.Ptr(policyName),
								Semantic:      util.Ptr(rest_model.SemanticAnyOf),
								ServiceRoles:  serviceRoles,
								Type:          util.Ptr(rest_model.DialBindBind),
							}, 15*time.Second)

							if err == nil {
								fmt.Printf("creating service policy %s: OK\n", policyName)
								return nil
							}
							err = errUtil.WrapIfApiError(err)

							l, _ := models.ListServicePolicies(ctrl1, fmt.Sprintf(`name="%s"`, policyName), 5*time.Second)
							if len(l) > 0 {
								fmt.Printf("creating service policy %s: ALREADY PRESENT\n", policyName)
								return nil
							}

							fmt.Printf("creating service policy %s: FAILED (%+v)\n", policyName, err)

							if attempts > 3 {
								return err
							}
							attempts++
							time.Sleep(time.Duration(attempts) * time.Second)
						}
					})
				}
				return parallel.Execute(tasks, 25)
			}))

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

			workflow.AddAction(edge.InitEdgeRouters(models.RouterTag, 25))
			workflow.AddAction(edge.InitIdentities(".host", 10))
			workflow.AddAction(model.RunAction("createBaseModel"))
			workflow.AddAction(model.RunAction("createServicePolicies"))
			workflow.AddAction(model.RunAction("initHA"))

			workflow.AddAction(component.StartInParallel(".router", 10))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))
			workflow.AddAction(component.StartInParallel(".host", 1000))

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
		"restart": model.ActionBinder(func(run *model.Model) model.Action {
			workflow := actions.Workflow()
			workflow.AddAction(component.StopInParallel("*", 10000))
			workflow.AddAction(host.GroupExec("*", 100, "rm -f logs/*"))
			workflow.AddAction(component.Start(".ctrl"))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))
			workflow.AddAction(component.StartInParallel(".router", 10))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))
			workflow.AddAction(component.StartInParallel(".host", 10000))
			return workflow
		}),
		"sowChaos": model.BindF(sowChaos),
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
		"validate": model.BindF(validateTerminators),
		"testIteration": model.BindF(func(run model.Run) error {
			return run.GetModel().Exec(run,
				"sowChaos",
				"validateUp",
				"validate",
			)
		}),
		"testRouterFlap": model.BindF(func(run model.Run) error {
			routers := []string{"router-us-0", "router-eu-0", "router-ap-0"}
			for _, router := range routers {
				if err := component.Start(router).Execute(run); err != nil {
					return err
				}
			}

			time.Sleep(10 * time.Second)

			for _, router := range routers {
				if err := component.Stop(router).Execute(run); err != nil {
					return err
				}
			}

			return nil
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

func getHostNames() []string {
	var result []string
	for i := 0; i < hostsPerRegion; i++ {
		for j := 0; j < tunnelersPerHost; j++ {
			result = append(result, fmt.Sprintf("host-us-%d-%d", i, j))
			result = append(result, fmt.Sprintf("host-eu-%d-%d", i, j))
			result = append(result, fmt.Sprintf("host-ap-%d-%d", i, j))
		}
	}
	return result
}

func main() {
	m.AddActivationActions("bootstrap")

	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)

	fablab.InitModel(m)
	fablab.Run()
}
