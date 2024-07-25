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
	"bufio"
	"embed"
	_ "embed"
	"fmt"
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
	aws_ssh_key2 "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/resources"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/zititest/models/test_resources"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/models"
	"go.etcd.io/bbolt"
	"os"
	"path"
	"strings"
	"time"
)

const (
	EnrollIdentitiesAction = "enrollIdentities"
)

const (
	useLatestZitiOnRouters       = true
	useLatestZitiOnPublicRouters = true
)

//go:embed configs
var configResource embed.FS

var dbStrategyInstance = &dbStrategy{
	routerMappings: map[string]string{},
}

type dbStrategy struct {
	routerMappings map[string]string
}

func (self *dbStrategy) ProcessDbModel(tx *bbolt.Tx, m *model.Model, builder *models.ZitiDbBuilder) error {
	dbFile := self.GetDbFile(m)
	dbDir := path.Dir(dbFile)
	mapFile := path.Join(dbDir, "edge-router-mapping.csv")
	mapContents, err := os.ReadFile(mapFile)
	if err != nil {
		return fmt.Errorf("failed to read edge router mapping file (%w)", err)
	}

	if err = self.loadErMap(string(mapContents)); err != nil {
		return fmt.Errorf("unable to parse router mapping file (%w)", err)
	}

	if err = self.ProcessEdgeRouters(tx, m, builder); err != nil {
		return err
	}

	if err = self.CreateEnrollIdentitiesAction(tx, m, builder); err != nil {
		return err
	}

	return nil
}

func (self *dbStrategy) loadErMap(csv string) error {
	sc := bufio.NewScanner(strings.NewReader(csv))
	for sc.Scan() {
		parts := strings.Split(sc.Text(), ",")
		if len(parts) >= 2 {
			routerId := parts[0]
			nfVersion := parts[1]
			zitiVersion := nfZitiVersionMap[nfVersion]
			self.routerMappings[routerId] = zitiVersion
		}
	}
	return nil
}

func (self *dbStrategy) GetDbFile(m *model.Model) string {
	return m.MustStringVariable("db_file")
}

func (self *dbStrategy) ProcessEdgeRouters(tx *bbolt.Tx, m *model.Model, builder *models.ZitiDbBuilder) error {
	ids, _, err := builder.GetStores().EdgeRouter.QueryIds(tx, "true limit none")
	if err != nil {
		return err
	}

	versions := map[string]int{}
	hostsPerRegion := ((len(ids) / len(sites)) / 20) + 1

	hostIdF := func(regionId string, idx int) string {
		return fmt.Sprintf("%s-r%d", regionId[:len(regionId)-2], idx)
	}

	echoServerIdF := func(regionId string, idx int) string {
		return fmt.Sprintf("%s-echo%d", regionId[:len(regionId)-2], idx)
	}

	simCount := 0

	for _, site := range sites {
		regionId := site[:len(site)-1]
		region := m.Regions[regionId]

		if region != nil && region.Site != site {
			return fmt.Errorf("trying to add region for site %v, but one exists, with different site", site)
		}

		if region == nil {
			region = &model.Region{
				Id:     regionId,
				Scope:  model.Scope{Tags: model.Tags{}},
				Region: regionId,
				Site:   site,
				Hosts:  model.Hosts{},
			}
			m.Regions[regionId] = region
		}

		for i := 0; i < 10; i++ {
			simCount++
			simId := fmt.Sprintf("sim%02d", simCount)
			region.Hosts[simId] = &model.Host{
				InstanceType: "t3.small",
				Components: model.Components{
					simId: {
						Scope: model.Scope{Tags: model.Tags{"sim"}},
						Type: &zitilab.SimpleSimType{
							ConfigPath: "identities",
						},
					},
				},
			}
		}

		for i := 0; i < hostsPerRegion; i++ {
			hostId := hostIdF(regionId, i)
			routerHost := &model.Host{
				Id:     hostId,
				Scope:  model.Scope{Tags: model.Tags{}},
				Region: region,
				Components: model.Components{
					echoServerIdF(regionId, i): {
						Scope: model.Scope{Tags: model.Tags{"server"}},
						Type: &zitilab.EchoServerType{
							Port: 8888,
						},
					},
				},
				InstanceType: "t3.large",
				ScaleIndex:   uint32(i),
			}
			region.Hosts[hostId] = routerHost
		}
	}

	siteIdx := 0
	hostIdx := 0

	for _, id := range ids {
		er, err := builder.GetStores().EdgeRouter.LoadById(tx, id)
		if err != nil {
			return err
		}

		if siteIdx >= len(sites) {
			siteIdx = 0
			hostIdx++
			if hostIdx >= hostsPerRegion {
				hostIdx = 0
			}
		}

		site := sites[siteIdx]
		regionId := site[:len(site)-1]
		region := m.Regions[regionId]
		hostId := hostIdF(regionId, hostIdx)
		routerHost := region.Hosts[hostId]

		siteIdx++

		version := ""
		isPublicRouter := stringz.Contains(er.RoleAttributes, "public")
		useLatestVersion := useLatestZitiOnRouters || (useLatestZitiOnPublicRouters && isPublicRouter)
		if !useLatestVersion {
			version = self.routerMappings[id]
			if replacement, found := versionMapping[version]; found {
				version = replacement
			}
			versions[version] = versions[version] + 1
		}

		routerComponent := &model.Component{
			Scope: model.Scope{Tags: model.Tags{"router", "pre-created"}},
			Type: &zitilab.RouterType{
				Version: version,
			},
			Host:       routerHost,
			ScaleIndex: uint32(len(routerHost.Components)),
		}

		routerHost.Components[er.Id] = routerComponent

		if er.IsTunnelerEnabled {
			routerComponent.Scope.Tags = append(routerComponent.Scope.Tags, "tunneler")
		}

		if isPublicRouter {
			routerComponent.Scope.Tags = append(routerComponent.Scope.Tags, "public")
		}
	}

	return nil
}

func (self *dbStrategy) CreateEnrollIdentitiesAction(tx *bbolt.Tx, m *model.Model, builder *models.ZitiDbBuilder) error {
	ids, _, err := builder.GetStores().Identity.QueryIds(tx, `type != "Router" limit none`)
	if err != nil {
		return err
	}

	m.Actions[EnrollIdentitiesAction] = func(m *model.Model) model.Action {
		return model.ActionFunc(func(run model.Run) error {
			return provisionIdentities(ids, run)
		})
	}

	return nil
}

var m = &model.Model{
	Id: "db-ctrl-test",
	Scope: model.Scope{
		Defaults: model.Variables{
			"environment": "ctrl-test",
			"credentials": model.Variables{
				"aws": model.Variables{
					"managed_key": true,
				},
				"ssh": model.Variables{
					"username": "ubuntu",
				},
				"edge": model.Variables{
					"username": "admin",
				},
			},
		},
	},
	StructureFactories: []model.Factory{
		&models.ZitiDbBuilder{Strategy: dbStrategyInstance},
	},
	Resources: model.Resources{
		resources.Configs:   resources.SubFolder(configResource, "configs"),
		resources.Binaries:  os.DirFS(path.Join(os.Getenv("GOPATH"), "bin")),
		resources.Terraform: test_resources.TerraformResources(),
	},
	Regions: model.Regions{
		"us-east-1": {
			Region: "us-east-1",
			Site:   "us-east-1c",
			Hosts: model.Hosts{
				"ctrl": {
					InstanceType:         "c5.9xlarge",
					InstanceResourceType: "ondemand_iops",
					EC2: model.EC2Host{
						Volume: model.EC2Volume{
							Type:   "gp3",
							SizeGB: 16,
							IOPS:   3000,
						},
					},
					Components: model.Components{
						"ctrl": {
							Scope: model.Scope{Tags: model.Tags{"ctrl"}},
							Type:  &zitilab.ControllerType{},
						},
					},
				},
			},
		},
	},

	Actions: model.ActionBinders{
		"bootstrap": model.ActionBinder(func(m *model.Model) model.Action {
			workflow := actions.Workflow()

			workflow.AddAction(component.StopInParallel("*", 200))
			workflow.AddAction(host.GroupExec("*", 25, "rm -f logs/*"))

			workflow.AddAction(component.Start("#ctrl"))
			workflow.AddAction(semaphore.Sleep(2 * time.Second))

			workflow.AddAction(edge.Login("#ctrl"))

			workflow.AddAction(model.ActionFunc(provisionRouters))
			workflow.AddAction(m.ExecuteAction("syncRouterJwts"))
			workflow.AddAction(model.ActionFunc(enrollRouters))

			workflow.AddAction(m.ExecuteAction(EnrollIdentitiesAction))
			workflow.AddAction(model.ActionFunc(func(run model.Run) error {
				src := model.MakeBuildPath("identities")
				return rsync.RsyncSelected("component.sim", src, "identities").Execute(run)
			}))
			return workflow
		}),
		"syncRouterJwts": model.Bind(model.ActionFunc(func(run model.Run) error {
			src := model.MakeBuildPath("router-jwts")
			return rsync.RsyncSelected("component.router", src, "router-jwts").Execute(run)
		})),
		"stop": model.Bind(component.StopInParallel("*", 200)),
		"clean": model.Bind(actions.Workflow(
			component.StopInParallel("*", 200),
			host.GroupExec("*", 25, "rm -f logs/*"),
		)),
		"login": model.Bind(edge.Login("#ctrl")),
	},

	Infrastructure: model.Stages{
		aws_ssh_key.Express(),
		terraform_0.Express(),
		semaphore_0.Ready(90 * time.Second),
	},

	Distribution: model.Stages{
		distribution.DistributeSshKey("*"),
		rsync.RsyncStaged(),
		model.StageActionF(func(run model.Run) error {
			quickRun, _ := run.GetModel().GetBoolVariable("quick_run")
			_, targetedSync := run.GetModel().Scope.GetVariable("sync.target")

			if !quickRun && !targetedSync {
				dbFile := dbStrategyInstance.GetDbFile(run.GetModel())
				deferred := rsync.NewRsyncHost("#ctrl", dbFile, "/home/ubuntu/ctrl.db")
				return deferred.Execute(run)
			}
			return nil
		}),
	},

	Disposal: model.Stages{
		terraform.Dispose(),
		aws_ssh_key2.Dispose(),
	},
}

func main() {
	m.AddActivationActions("bootstrap")

	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)

	fablab.InitModel(m)
	fablab.Run()
}
