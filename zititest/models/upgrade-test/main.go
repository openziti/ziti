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

// rotatingLogConfig is the log configuration applied to every ziti component in this model. The
// rotate strategy keeps each component's log size-bounded (via "ziti ops log-pipe") so a long
// multi-iteration run cannot fill the host disk. Combined with routers no longer running at debug,
// this keeps the per-host log footprint small across all iterations.
//
// The pipe binary is pinned to the locally-built ziti (the empty version) because the staged
// fromVersion/toVersion binaries predate "ops log-pipe". Without the pin, the pipe would run a binary
// that lacks the command and take the component's stdout down with it on most of the versions this
// run cycles through.
func rotatingLogConfig() zitilab.LogConfig {
	localBuild := ""
	return zitilab.LogConfig{
		Strategy:          zitilab.LogStrategyRotate,
		PipeBinaryVersion: &localBuild,
	}
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
			"fromVersion": "v1.6.20",
			"toVersion":   "v2.0.3",
			// zetVersion tracks the ziti-edge-tunnel bundled in the latest stable Ziti Desktop Edge for
			// Windows (WDE 2.11.2.5 ships v1.18.0). Pin to v1.11.4 (WDE 2.9.7.1) to reproduce the older
			// client's post-2.0-migration session-recovery failure.
			"zetVersion": "v1.18.0",
			// ziti-tunnel runs the locally-built ziti by default (so it exercises the current SDK,
			// e.g. to validate SDK fixes). Set to a release version to test a specific released build.
			"zitiTunnelVersion": "",
			// toVersionSource optionally points at a locally-built ziti binary to stage under the
			// toVersion name, so the upgrade runs a patched controller/router instead of the released
			// toVersion. Empty falls through to toVersionRef. The ZITI_TOVERSION_PATH env var overrides
			// this, and a source binary wins over a ref build wherever both are set.
			"toVersionSource": "",
			// toVersionRef optionally names a git ref (branch, tag, or SHA) on openziti/ziti to build
			// from source and stage under the toVersion name, so the upgrade can run a fix no release
			// carries yet. Empty downloads the released toVersion instead, which is the normal case.
			// The ZITI_TOVERSION_REF env var overrides this; toVersionSource wins over both.
			// Changing the ref alone is enough to get a new binary: the staging op is keyed on the ref as
			// well as the version, and acquire caches on the commit the ref resolves to, so a branch that
			// has moved rebuilds rather than serving the previous build.
			"toVersionRef": "",

			// nextVersion is the optional second upgrade hop: once the cluster is up on toVersion it is
			// upgraded again, cluster and all, to nextVersion. Leave it empty to end the iteration at
			// toVersion and skip the phase entirely. nextVersionSource/nextVersionRef work exactly like
			// their toVersion counterparts, with ZITI_NEXTVERSION_PATH and ZITI_NEXTVERSION_REF as the
			// environment overrides.
			//
			// 2.1 has no release yet, so the default builds it from main. That is a moving ref: what a
			// run tested is whatever main pointed at that day. Set nextVersionRef to a tag once 2.1
			// releases, at which point nextVersion alone selects a downloaded release.
			"nextVersion":       "v2.1.0",
			"nextVersionSource": "",
			"nextVersionRef":    "main",
			// clusterUpgradeMode picks how the cluster moves to nextVersion. "rolling" restarts one node
			// at a time, which leaves the cluster in mixed-version read-only mode until the last node is
			// done; "all-at-once" restarts every node together, so no node sees a version mismatch but
			// every node cold-starts simultaneously.
			"clusterUpgradeMode": clusterUpgradeRolling,

			// controller memory sampling. Every controller host records its controller's RSS once a
			// second for the whole iteration, so an upgrade's memory cost can be compared against the
			// node's own steady state and against its peers.
			"ctrlMemory": model.Variables{
				// heapDumpAtMb captures one heap profile per controller the first time RSS crosses this,
				// which is the artifact a memory regression is diagnosed from and the one thing that
				// cannot be recovered after the fact. Zero disables the capture.
				"heapDumpAtMb": 400,
				// failAtMb fails a phase whose peak exceeds it. Zero disables the check.
				"failAtMb": 1024,
				// failAtRatio fails the cluster upgrade when a controller's peak over the upgrade exceeds
				// this multiple of its own peak over the steady state just before it. This is the check
				// that bites at this model's scale, where the absolute ceiling is far out of reach; the
				// value is a first guess and wants calibrating against a run that is known good. Anything
				// at or below one disables it.
				"failAtRatio": 3,
			},

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
		// apply the bulk from/zet versions and the shared log config to every component
		model.FactoryFunc(func(m *model.Model) error {
			fromVersion := m.MustStringVariable("fromVersion")
			zetVersion := m.MustStringVariable("zetVersion")
			zitiTunnelVersion := m.MustStringVariable("zitiTunnelVersion")
			return m.ForEachComponent("*", 1, func(c *model.Component) error {
				switch t := c.Type.(type) {
				case *zitilab.ControllerType:
					t.Version = fromVersion
					t.LogConfig = rotatingLogConfig()
				case *zitilab.RouterType:
					t.Version = fromVersion
					t.LogConfig = rotatingLogConfig()
				case *zitilab.ZitiTunnelType:
					t.Version = zitiTunnelVersion
					t.LogConfig = rotatingLogConfig()
				case *zitilab.ZitiEdgeTunnelType:
					t.Version = zetVersion
					t.ZitiVersion = fromVersion
					t.LogConfig = rotatingLogConfig()
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
						// host mode (run-host) is required here: the default intercept mode would have both
						// instances contend for the shared box's tproxy/DNS state, including port 53
						"ziti-edge-tunnel-host-stable": {
							Scope: model.Scope{Tags: model.Tags{"sdk-app", "host", "zet-host", "zet", "stable"}},
							Type: &zitilab.ZitiEdgeTunnelType{
								Mode:           zitilab.ZitiEdgeTunnelModeHost,
								VerbosityLevel: 3,
							},
						},
						"ziti-edge-tunnel-host-restart": {
							Scope: model.Scope{Tags: model.Tags{"sdk-app", "host", "zet-host", "zet", "restart"}},
							Type: &zitilab.ZitiEdgeTunnelType{
								Mode:           zitilab.ZitiEdgeTunnelModeHost,
								VerbosityLevel: 3,
							},
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
		// the optional second upgrade hop, plus its pieces
		"upgradeToNextVersion":      model.BindF(upgradeToNextVersion),
		"upgradeClusterControllers": model.BindF(upgradeClusterControllers),
		"upgradeClusterRouters":     model.BindF(upgradeClusterRouters),
		// controller memory sampling, runnable on its own against a live instance
		"startCtrlMemory": model.BindF(startCtrlMemorySamplers),
		"stopCtrlMemory":  model.BindF(stopCtrlMemorySamplers),
		"reportCtrlMemory": model.BindF(func(run model.Run) error {
			return reportCtrlMemory(run, time.Time{}, "iteration")
		}),
		"ctrlMemorySummary": model.BindF(summarizeCtrlMemory),
		// testIteration is the exec-loop entry point (fablab exec-loop <until> testIteration). It runs
		// one full pass of the upgrade sequence. It begins by resetting to a fresh standalone fromVersion
		// baseline, so each pass is self-contained and repeated iterations (or a re-run after a failed
		// pass) start from a known state rather than re-upgrading an already upgraded controller.
		"testIteration": model.BindF(func(run model.Run) error {
			return run.GetModel().Exec(run,
				"resetToBaseline", // fresh standalone fromVersion baseline (undo any prior pass)
				"startSimMetrics",
				"startCtrlMemory",                    // per-second controller RSS sampling for the whole pass
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
				"upgradeToNextVersion",               // optional: whole system toVersion -> nextVersion
				"reportCtrlMemory",                   // controller memory over the whole pass
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
		// pre-stage the upgrade binaries so in-place upgrades need only a version swap + restart
		model.StageActionF(toTarget.stage),
		model.StageActionF(nextTarget.stage),
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
