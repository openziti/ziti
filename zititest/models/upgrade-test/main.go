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
	"os"
	"path"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/binding"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/aws_ssh_key"
	semaphore "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/semaphore"
	terraformInit "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/terraform"
	distribution "github.com/openziti/fablab/kernel/lib/runlevel/3_distribution"
	"github.com/openziti/fablab/kernel/lib/runlevel/3_distribution/rsync"
	awsSshKeyDispose "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/resources"
	"github.com/openziti/ziti/zititest/models/test_resources"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/models"
	zitiLibOps "github.com/openziti/ziti/zititest/zitilab/runlevel/5_operation"
)

//go:embed configs
var configResource embed.FS

// livenessWorkload is a short loop4 workload used purely to prove a service's data path is
// currently healthy. A scenario run executes one of these per hosting flavor.
var livenessWorkload = "" +
	`concurrency:  1
    iterations:   1
    dialer:
      txRequests:       20
      rxTimeout:        5s
      payloadMinBytes:  512
      payloadMaxBytes:  512
    listener:
      rxTimeout:        5s
`

func getUniqueId() string {
	if runId := os.Getenv("GITHUB_RUN_ID"); runId != "" {
		return "-" + runId + "." + os.Getenv("GITHUB_RUN_ATTEMPT")
	}
	return "-" + os.Getenv("USER")
}

var m = &model.Model{
	Id: "upgrade-test",
	Scope: model.Scope{
		Defaults: model.Variables{
			// version knobs. The system starts on fromVersion and is upgraded to toVersion
			// in place. zetVersion is fixed for the whole run (ZET stays up, not upgraded).
			// toVersion is the label the built binary is staged under and, for ref builds, the version
			// stamped into it, so it must be a plain numeric semver (the SDK version parser rejects
			// pre-release/build-metadata suffixes). toVersionRef selects the actual source.
			"fromVersion": "v1.6.12",
			"toVersion":   "v2.0.2",
			// zetVersion tracks the ziti-edge-tunnel bundled in the latest stable Ziti Desktop Edge for
			// Windows (WDE 2.11.2.5 ships v1.18.0). Pin to v1.11.4 (WDE 2.9.7.1) to reproduce the older
			// client's post-2.0-migration session-recovery failure.
			"zetVersion": "v1.18.0",
			// ziti-tunnel runs the locally-built ziti by default (so it exercises the current SDK,
			// e.g. to validate SDK fixes). Set to a release version to test a specific released build.
			"zitiTunnelVersion": "",
			// toVersionSource optionally points at a locally-built ziti binary to stage under the
			// toVersion name, so the upgrade runs a patched controller/router instead of the released
			// toVersion. Empty downloads the release. The ZITI_TOVERSION_PATH env var overrides this.
			"toVersionSource": "",
			// toVersionRef names a git ref (branch, tag, or SHA) on openziti/ziti to build from source
			// and stage under the toVersion name, so the upgrade can run unmerged fixes without
			// hand-building a binary. Points at an integration branch: release-v2.0.x plus the two
			// not-yet-merged fixes this test needs (the legacy session-JWT id fix and the service-policy
			// enforcer type-query fix), the code the next 2.0.x release is expected to carry. Switch back
			// to release-v2.0.x once both merge. Empty falls back to toVersionSource, then the release
			// download. The ZITI_TOVERSION_REF env var overrides this; toVersionSource wins.
			// NOTE: the stager keys the built binary on toVersion (ziti-<toVersion>) and reuses an
			// existing one without rebuilding, so changing this ref alone will silently keep the old
			// binary. Bump toVersion (or delete the staged ziti-<toVersion>) whenever the ref changes.
			"toVersionRef": "test/upgrade-2.0.x",

			// auth/cluster flags. Phase 1 runs a single standalone controller on legacy auth,
			// so all three default to false. OIDC becomes available via the 2.0 controller's
			// auto-binding, not by setting these.
			"oidc":    false,
			"ha":      false,
			"cluster": false,

			// steady-state gate tuning
			"steadyState": model.Variables{
				"requiredCleanRuns": 3,
				"recoveryTimeout":   "1m",
				"stabilityWindow":   "2m",
			},

			// logStrategy=rotate keeps each component's log size-bounded (via "ziti ops log-pipe")
			// so a long multi-iteration run cannot fill the host disk. Combined with routers no longer
			// running at debug (see RouterType definitions below), this keeps the per-host log footprint
			// small across all iterations.
			"logStrategy": "rotate",
			// The staged fromVersion/toVersion binaries predate "ops log-pipe", so pin the log-pipe
			// binary to the locally-built ziti (version "" = local), which has the command. This keeps
			// rotate working across every version the run cycles through.
			"logPipeBinaryVersion": "",

			"livenessWorkload": livenessWorkload,

			"environment": "upgrade-test" + getUniqueId(),
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
		// apply the bulk from/zet versions to every component
		model.FactoryFunc(func(m *model.Model) error {
			fromVersion := m.MustStringVariable("fromVersion")
			zetVersion := m.MustStringVariable("zetVersion")
			zitiTunnelVersion := m.MustStringVariable("zitiTunnelVersion")
			return m.ForEachComponent("*", 1, func(c *model.Component) error {
				switch t := c.Type.(type) {
				case *zitilab.ControllerType:
					t.Version = fromVersion
				case *zitilab.RouterType:
					t.Version = fromVersion
				case *zitilab.ZitiTunnelType:
					t.Version = zitiTunnelVersion
				case *zitilab.ZitiEdgeTunnelType:
					t.Version = zetVersion
					t.ZitiVersion = fromVersion
					t.InitType(c)
				}
				return nil
			})
		}),
		// per-component version overrides, e.g. -l router_west_version=v1.6.10
		model.FactoryFunc(func(m *model.Model) error {
			return m.ForEachComponent("*", 1, func(c *model.Component) error {
				versioned, ok := c.Type.(interface{ SetVersion(string) })
				if !ok {
					return nil
				}
				varName := strings.ReplaceAll(c.Id, "-", "_") + "_version"
				if version, found := m.GetStringVariable(varName); found {
					versioned.SetVersion(version)
				}
				return nil
			})
		}),
		// instance sizing
		model.FactoryFunc(func(m *model.Model) error {
			return m.ForEachHost("*", 1, func(host *model.Host) error {
				if strings.HasPrefix(host.Id, "ctrl") {
					host.InstanceType = "t3.medium"
				} else {
					host.InstanceType = "c5.large"
				}
				return nil
			})
		}),
	},

	Factories: []model.Factory{
		model.FactoryFunc(func(m *model.Model) error {
			pfxlog.Logger().Infof("environment [%s]", m.MustStringVariable("environment"))
			return nil
		}),
		// sim harness: metrics collection and the steady-state validation gate
		model.FactoryFunc(func(m *model.Model) error {
			simServices := zitiLibOps.NewSimServices(func(s string) string {
				return "component#" + s
			})

			m.AddActivationStageF(simServices.SetupSimControllerIdentity)
			m.AddOperatingStage(simServices.CollectSimMetricStage("metrics"))

			m.AddActionF("startSimMetrics", func(run model.Run) error {
				return simServices.CollectSimMetrics(run, "metrics")
			})

			gate := newSteadyStateGate(simServices)
			m.AddActionF("validateSteadyState", gate.validate)
			m.AddActionF("validateSteadyStateAfterDisruption", gate.validateAfterDisruption)

			// resetToBaseline returns the system to a fresh standalone fromVersion baseline so a new
			// testIteration can run from a known state. It undoes the in-process mutations a pass makes
			// (controller/router binary versions, HA/migration flags) and drops the in-harness
			// sim-controller client's cached context/enrollment (stale once the controller is wiped),
			// then re-runs the standard configuration -> distribution -> activation pipeline. Re-rendering
			// the configs from the reset state is what brings the controller back up standalone rather
			// than reusing the prior pass's HA config; Activate also re-bootstraps and re-enrolls the
			// sim-controller identity (an activation stage).
			m.AddActionF("resetToBaseline", func(run model.Run) error {
				fromVersion := m.MustStringVariable("fromVersion")
				if err := m.ForEachComponent("*", 1, func(c *model.Component) error {
					switch t := c.Type.(type) {
					case *zitilab.ControllerType:
						t.SetVersion(fromVersion)
						c.PutVariable("cluster", false)
						c.PutVariable("migrateDb", false)
					case *zitilab.RouterType:
						t.SetVersion(fromVersion)
					}
					return nil
				}); err != nil {
					return err
				}

				simServices.Reset()

				if err := m.Build(run); err != nil {
					return err
				}
				if err := m.Sync(run); err != nil {
					return err
				}
				return m.Activate(run)
			})

			return nil
		}),
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
							Scope: model.Scope{Tags: model.Tags{"ctrl", "bootstrap-ctrl"}},
							Type:  &zitilab.ControllerType{},
						},
					},
				},
				// provisioned up front, started only during the add-nodes phase
				"ctrl2": {
					Components: model.Components{
						"ctrl2": {
							Scope: model.Scope{Tags: model.Tags{"ctrl", "ha", "cluster-node"}},
							Type:  &zitilab.ControllerType{},
						},
					},
				},
				"router-east-1": {
					Scope: model.Scope{Tags: model.Tags{"ert-client"}},
					Components: model.Components{
						"router-east-1": {
							// ert-proxy-client makes this ERT run its tunnel binding in proxy mode (local
							// loop-ert listener on :15390) so the co-located loop4 dialer below can push
							// traffic through the ERT client path.
							Scope: model.Scope{Tags: model.Tags{"edge-router", "terminator", "tunneler", "client", "ert-proxy-client"}},
							Type:  &zitilab.RouterType{},
						},
						"loop4-client-stable": {
							Scope: model.Scope{Tags: model.Tags{"sdk-app", "client", "loop-client", "sim-services-client", "stable"}},
							Type: &zitilab.Loop4SimType{
								ConfigSource: "loop4-client.yml.tmpl",
								Mode:         zitilab.Loop4RemoteControlled,
							},
						},
						"loop4-client-restart": {
							Scope: model.Scope{Tags: model.Tags{"sdk-app", "client", "loop-client", "sim-services-client", "restart"}},
							Type: &zitilab.Loop4SimType{
								ConfigSource: "loop4-client.yml.tmpl",
								Mode:         zitilab.Loop4RemoteControlled,
							},
						},
						// drives traffic through the ERT client's local proxy listener (router-east-1 is a
						// single router, not a stable/restart pair, so one dialer)
						"loop-ert-dialer": {
							Scope: model.Scope{
								Tags:     model.Tags{"sdk-app", "client", "loop-client", "sim-services-client"},
								Defaults: model.Variables{"loopTunnelAddress": "tcp:127.0.0.1:15390"},
							},
							Type: &zitilab.Loop4SimType{
								ConfigSource: "loop4-transport-dialer.yml.tmpl",
								Mode:         zitilab.Loop4RemoteControlled,
							},
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
				// Tunneler clients are grouped by stable/restart rather than by flavor. Each host runs one
				// Go ziti-tunnel (proxy mode, no :53) and one ZET (tproxy, needs :53). Splitting the ZET
				// pair across the two hosts keeps them off each other's :53, and the Go client's proxy mode
				// means it never contends for :53 with the co-located ZET. Each tunneler client has a
				// co-located loop4 dialer pushing traffic through its client-side data path.
				"tunnel-clients-stable": {
					Components: model.Components{
						"ziti-tunnel-client-stable": {
							Scope: model.Scope{Tags: model.Tags{"ziti-tunnel", "sdk-app", "client", "ziti-tunnel-client", "stable"}},
							Type: &zitilab.ZitiTunnelType{
								Mode:          zitilab.ZitiTunnelModeProxy,
								ProxyServices: []string{"loop-ziti-tunnel:15387"},
							},
						},
						"loop-zt-dialer-stable": {
							Scope: model.Scope{
								Tags:     model.Tags{"sdk-app", "client", "loop-client", "sim-services-client", "stable"},
								Defaults: model.Variables{"loopTunnelAddress": "tcp:127.0.0.1:15387"},
							},
							Type: &zitilab.Loop4SimType{
								ConfigSource: "loop4-transport-dialer.yml.tmpl",
								Mode:         zitilab.Loop4RemoteControlled,
							},
						},
						"ziti-edge-tunnel-client-stable": {
							Scope: model.Scope{Tags: model.Tags{"sdk-app", "client", "zet", "zet-client", "stable"}},
							Type:  &zitilab.ZitiEdgeTunnelType{VerbosityLevel: 3},
						},
						"loop-zet-dialer-stable": {
							Scope: model.Scope{
								Tags:     model.Tags{"sdk-app", "client", "loop-client", "sim-services-client", "stable"},
								Defaults: model.Variables{"loopTunnelAddress": "tcp:loop-zet.ziti:15391"},
							},
							Type: &zitilab.Loop4SimType{
								ConfigSource: "loop4-transport-dialer.yml.tmpl",
								Mode:         zitilab.Loop4RemoteControlled,
							},
						},
					},
				},
				"tunnel-clients-restart": {
					Components: model.Components{
						"ziti-tunnel-client-restart": {
							Scope: model.Scope{Tags: model.Tags{"ziti-tunnel", "sdk-app", "client", "ziti-tunnel-client", "restart"}},
							Type: &zitilab.ZitiTunnelType{
								Mode:          zitilab.ZitiTunnelModeProxy,
								ProxyServices: []string{"loop-ziti-tunnel:15388"},
							},
						},
						"loop-zt-dialer-restart": {
							Scope: model.Scope{
								Tags:     model.Tags{"sdk-app", "client", "loop-client", "sim-services-client", "restart"},
								Defaults: model.Variables{"loopTunnelAddress": "tcp:127.0.0.1:15388"},
							},
							Type: &zitilab.Loop4SimType{
								ConfigSource: "loop4-transport-dialer.yml.tmpl",
								Mode:         zitilab.Loop4RemoteControlled,
							},
						},
						"ziti-edge-tunnel-client-restart": {
							Scope: model.Scope{Tags: model.Tags{"sdk-app", "client", "zet", "zet-client", "restart"}},
							Type:  &zitilab.ZitiEdgeTunnelType{VerbosityLevel: 3},
						},
						"loop-zet-dialer-restart": {
							Scope: model.Scope{
								Tags:     model.Tags{"sdk-app", "client", "loop-client", "sim-services-client", "restart"},
								Defaults: model.Variables{"loopTunnelAddress": "tcp:loop-zet.ziti:15391"},
							},
							Type: &zitilab.Loop4SimType{
								ConfigSource: "loop4-transport-dialer.yml.tmpl",
								Mode:         zitilab.Loop4RemoteControlled,
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
				// provisioned up front, started only during the add-nodes phase
				"ctrl3": {
					Components: model.Components{
						"ctrl3": {
							Scope: model.Scope{Tags: model.Tags{"ctrl", "ha", "cluster-node"}},
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
						// paired Go SDK listeners that bind the loop-sdk service (two terminators)
						"loop4-sdk-host-stable": {
							Scope: model.Scope{Tags: model.Tags{"sdk-app", "host", "loop-sdk-host", "stable"}},
							Type: &zitilab.Loop4SimType{
								ConfigSource: "loop4-sdk-host.yml.tmpl",
								Mode:         zitilab.Loop4Listener,
							},
						},
						"loop4-sdk-host-restart": {
							Scope: model.Scope{Tags: model.Tags{"sdk-app", "host", "loop-sdk-host", "restart"}},
							Type: &zitilab.Loop4SimType{
								ConfigSource: "loop4-sdk-host.yml.tmpl",
								Mode:         zitilab.Loop4Listener,
							},
						},
						// plain-TCP backend the ERT router forwards loop-ert to (localhost:3456)
						"loop4-ert-backend": {
							Scope: model.Scope{Tags: model.Tags{"loop-backend"}},
							Type: &zitilab.Loop4SimType{
								ConfigSource: "loop4-backend-host.yml.tmpl",
								Mode:         zitilab.Loop4Listener,
							},
						},
					},
				},
				// The ziti-tunnel and ZET hosts share a box: both run in host mode (no :53 intercept), and
				// both forward their loop service to a single local loop4 backend on 127.0.0.1:3456.
				"tunnel-hosts": {
					Components: model.Components{
						"ziti-tunnel-host-stable": {
							Scope: model.Scope{Tags: model.Tags{"ziti-tunnel", "sdk-app", "host", "ziti-tunnel-host", "stable"}},
							Type: &zitilab.ZitiTunnelType{
								Mode:    zitilab.ZitiTunnelModeHost,
								Verbose: true,
							},
						},
						"ziti-tunnel-host-restart": {
							Scope: model.Scope{Tags: model.Tags{"ziti-tunnel", "sdk-app", "host", "ziti-tunnel-host", "restart"}},
							Type: &zitilab.ZitiTunnelType{
								Mode:    zitilab.ZitiTunnelModeHost,
								Verbose: true,
							},
						},
						"ziti-edge-tunnel-host-stable": {
							Scope: model.Scope{Tags: model.Tags{"sdk-app", "host", "zet-host", "zet", "stable"}},
							Type:  &zitilab.ZitiEdgeTunnelType{VerbosityLevel: 3},
						},
						"ziti-edge-tunnel-host-restart": {
							Scope: model.Scope{Tags: model.Tags{"sdk-app", "host", "zet-host", "zet", "restart"}},
							Type:  &zitilab.ZitiEdgeTunnelType{VerbosityLevel: 3},
						},
						// single plain-TCP loop4 backend both the ziti-tunnel and ZET hosts forward to
						"loop4-tunnel-backend": {
							Scope: model.Scope{Tags: model.Tags{"loop-backend"}},
							Type: &zitilab.Loop4SimType{
								ConfigSource: "loop4-backend-host.yml.tmpl",
								Mode:         zitilab.Loop4Listener,
							},
						},
					},
				},
			},
		},
	},

	Actions: model.ActionBinders{
		"bootstrap": newBootstrapAction(),
		"start":     newStartAction(),
		"stop":      model.Bind(component.StopInParallel("*", 15)),
		"login":     model.Bind(edge.Login("#ctrl1")),
		// individual upgrade steps, runnable on their own
		"upgradeController":     model.BindF(upgradeController),
		"upgradeRouters":        model.BindF(upgradeRouters),
		"restartAndVerifyOidc":  model.BindF(restartAndVerifyOidc),
		"restartZetWorkaround":  model.BindF(restartZetWorkaround),
		"upgradeControllerToHa": model.BindF(upgradeControllerToHa),
		"addClusterNodes":       model.BindF(addClusterNodes),
		// testIteration is the exec-loop entry point (fablab exec-loop <until> testIteration). It runs
		// one full pass of the upgrade sequence. It begins by resetting to a fresh standalone fromVersion
		// baseline, so each pass is self-contained and repeated iterations (or a re-run after a failed
		// pass) start from a known state rather than re-upgrading an already upgraded controller.
		"testIteration": model.BindF(func(run model.Run) error {
			return run.GetModel().Exec(run,
				"resetToBaseline",                    // fresh standalone fromVersion baseline (undo any prior pass)
				"startSimMetrics",
				"validateSteadyState",                // baseline: expected terminators present, clients healthy on legacy
				"upgradeController",                  // ctrl fromVersion -> toVersion (OIDC auto-binds)
				"restartZetWorkaround",               // old ziti-edge-tunnel can't recover sessions post-migration; restart to re-auth
				"validateSteadyStateAfterDisruption", // terminators re-establish, traffic recovers, clients still up
				"restartAndVerifyOidc",               // restart -restart instances, confirm OIDC on re-auth
				"validateSteadyStateAfterDisruption", // everything back and clean
				"upgradeRouters",                     // routers fromVersion -> toVersion, rolling one at a time
				"validateSteadyStateAfterDisruption", // routers back on toVersion, terminators + traffic recovered
				"upgradeControllerToHa",              // ctrl1 standalone -> single-node HA cluster (bbolt -> raft)
				"validateSteadyStateAfterDisruption", // cluster up, terminators + traffic recovered
				"addClusterNodes",                    // start ctrl2/ctrl3 and join them to the cluster
				"validateSteadyStateAfterDisruption", // 3-node cluster stable, terminators + traffic recovered
			)
		}),
	},

	Infrastructure: model.Stages{
		aws_ssh_key.Express(),
		&terraformInit.Terraform{
			Retries: 3,
			ReadyCheck: &semaphore.ReadyStage{
				MaxWait: 90 * time.Second,
			},
		},
	},

	Distribution: model.Stages{
		// Stop all components before pushing files. A leftover component from a prior run (e.g. a broken
		// controller spamming logs after a failed HA phase) can hold a large deleted log file open,
		// keeping the host disk full; the ssh-key and rsync steps below then fail on the full disk and
		// abort the whole refresh before it can reset anything. Stopping first releases those handles.
		model.RunAction("stop"),
		distribution.DistributeSshKey("*"),
		// pre-stage the toVersion binary so in-place upgrades need only a version swap + restart
		model.StageActionF(stageToVersionBinary),
		rsync.RsyncStaged(),
	},

	Activation: model.Stages{
		model.RunAction("stop"),
		model.RunAction("bootstrap"),
		model.RunAction("start"),
	},

	Operation: model.Stages{
		model.RunAction("login"),
		edge.SyncModelRouterIds(models.EdgeRouterTag),
	},

	Disposal: model.Stages{
		terraform.Dispose(),
		awsSshKeyDispose.Dispose(),
	},
}

func main() {
	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)

	fablab.InitModel(m)
	fablab.Run()
}
