package main

import (
	"embed"
	_ "embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab"
	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/semaphore"
	"github.com/openziti/fablab/kernel/lib/binding"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/aws_ssh_key"
	semaphore_0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/semaphore"
	terraform_0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/terraform"
	distribution "github.com/openziti/fablab/kernel/lib/runlevel/3_distribution"
	"github.com/openziti/fablab/kernel/lib/runlevel/3_distribution/rsync"
	fablibOps "github.com/openziti/fablab/kernel/lib/runlevel/5_operation"
	awsSshKeyDispose "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/kernel/model/aws"
	"github.com/openziti/fablab/resources"
	"github.com/openziti/ziti/v2/ziti/util"
	"github.com/openziti/ziti/zititest/models/test_resources"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/openziti/ziti/zititest/zitilab/models"
	zitilibOps "github.com/openziti/ziti/zititest/zitilab/runlevel/5_operation"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
	"github.com/sirupsen/logrus"
)

var BaselineVersion = "v2.0.0-pre3"

//go:embed configs
var configResource embed.FS

var throughputWorkload = "" +
	`concurrency:  1
    iterations:   2
    dialer:
      txRequests:       80000
      rxTimeout:        5s
      payloadMinBytes:  10000
      payloadMaxBytes:  10000
    listener:
      rxTimeout:        5s
`

var latencyWorkload = "" +
	`concurrency:  5
    iterations:  100
    dialer:
      txRequests:       1
      rxTimeout:        5s
      payloadMinBytes:  64
      payloadMaxBytes:  256
      latencyFrequency: 1
    listener:
      txRequests:       1
      txAfterRx:        true
      rxTimeout:        5s
      payloadMinBytes:  2048
      payloadMaxBytes:  10000
`

var perfReport = NewPerfDiffReport()
var metricsDumper = NewMetricsDumper()

var m = &model.Model{
	Id: "circuit-perf-diff",
	Scope: model.Scope{
		Defaults: model.Variables{
			"environment":      "circuit-perf-diff",
			"baseline_version": BaselineVersion,
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
			"throughputWorkload": throughputWorkload,
			"latencyWorkload":    latencyWorkload,

			"testErtClient":   true,
			"testSdkClient":   false,
			"testSdkXgClient": true,

			"testErtHost":   true,
			"testSdkHost":   false,
			"testSdkXgHost": true,
		},
	},
	StructureFactories: []model.Factory{
		model.FactoryFunc(func(m *model.Model) error {
			// Set controller to baseline version
			return m.ForEachComponent(".ctrl", 1, func(c *model.Component) error {
				c.Type.(*zitilab.ControllerType).Version = BaselineVersion
				return nil
			})
		}),
		model.FactoryFunc(func(m *model.Model) error {
			err := m.ForEachHost("*", 1, func(host *model.Host) error {
				if host.InstanceType == "" {
					host.InstanceType = "c5.xlarge"
				}
				return nil
			})
			if err != nil {
				return err
			}

			return m.ForEachHost("component.ctrl", 1, func(host *model.Host) error {
				host.InstanceType = "t3.micro"
				return nil
			})
		}),
		model.FactoryFunc(func(m *model.Model) error {
			for _, host := range m.SelectHosts("*") {
				host.InstanceResourceType = "ondemand_iops"
				host.AWS.Volume = aws.EC2Volume{
					Type:   "gp3",
					SizeGB: 20,
					IOPS:   1000,
				}
				for _, component := range host.Components {
					if rc, ok := component.Type.(*zitilab.Loop4SimType); ok && rc.Mode == zitilab.Loop4RemoteControlled {
						if component.HasTag("loop-client-xg") && !m.BoolVariable("testSdkXgClient") {
							delete(host.Components, component.Id)
						} else if component.HasTag("loop-client-ert") && !m.BoolVariable("testErtClient") {
							delete(host.Components, component.Id)
						}
					}
				}
			}
			return nil
		}),
	},
	Factories: []model.Factory{
		model.FactoryFunc(func(m *model.Model) error {
			simServices := zitilibOps.NewSimServices(func(s string) string {
				return "component#" + s
			})

			m.AddActivationStageF(simServices.SetupSimControllerIdentity)
			m.AddOperatingStage(simServices.CollectSimMetricStage("metrics"))

			perfReport.AddToModel(m)
			metricsDumper.AddToModel(m)

			m.AddActionF("runSimScenario", func(run model.Run) error {
				return RunSimScenarios(run, simServices)
			})

			m.AddActionF("startSimMetrics", func(run model.Run) error {
				return simServices.CollectSimMetrics(run, "metrics")
			})

			m.AddActionF("enableMetrics", perfReport.StartCollecting)

			m.AddActionF("stageBaselineTrafficTest", func(run model.Run) error {
				return stageBaselineTrafficTest(run, BaselineVersion)
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
					InstanceType: "t3.micro",
					Components: model.Components{
						"ctrl1": {
							Scope: model.Scope{Tags: model.Tags{"ctrl"}},
							Type:  &zitilab.ControllerType{},
						},
					},
				},
				"router-client": {
					Components: model.Components{
						"router-client": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "tunneler", "client", "test", "client-router", "data-plane"}},
							Type:  &zitilab.RouterType{},
						},
					},
				},
				"ert": {
					Components: model.Components{
						"ert": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "tunneler", "test", "loop-client", "client-router", "data-plane"}},
							Type:  &zitilab.RouterType{},
						},
						"loop-client-ert": {
							Scope: model.Scope{Tags: model.Tags{"loop-client", "loop-client-ert", "sdk-app", "client", "sim-services-client", "data-plane"}},
							Type: &zitilab.Loop4SimType{
								Mode: zitilab.Loop4RemoteControlled,
							},
						},
					},
				},
				"router-metrics": {
					InstanceType: "t3.micro",
					Components: model.Components{
						"router-metrics": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "no-traversal", "sim-services"}},
							Type: &zitilab.RouterType{
								Version: BaselineVersion,
							},
						},
					},
				},
				"loop-client-xg": {
					Components: model.Components{
						"loop-client-xg": {
							Scope: model.Scope{Tags: model.Tags{"loop-client", "loop-client-xg", "sdk-app", "client", "sim-services-client", "data-plane"}},
							Type: &zitilab.Loop4SimType{
								ConfigSource: "loop-client-xg.yml.tmpl",
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
				"router-host": {
					Components: model.Components{
						"router-host": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "host", "test", "host-router", "data-plane"}},
							Type:  &zitilab.RouterType{},
						},
					},
				},
				"ert-host": {
					Components: model.Components{
						"ert-host": {
							Scope: model.Scope{Tags: model.Tags{"edge-router", "tunneler", "loop-host-ert", "test", "host-router", "data-plane"}},
							Type:  &zitilab.RouterType{},
						},
						"loop-host-ert": {
							Scope: model.Scope{Tags: model.Tags{"loop-host", "sdk-app", "host", "sim-services-host", "data-plane"}},
							Type: &zitilab.Loop4SimType{
								Mode: zitilab.Loop4Listener,
							},
						},
					},
				},
				"loop-host-xg": {
					Components: model.Components{
						"loop-host-xg": {
							Scope: model.Scope{Tags: model.Tags{"loop-host-xg", "sdk-app", "host", "sim-services-host", "data-plane"}},
							Type: &zitilab.Loop4SimType{
								ConfigSource: "loop-host-xg.yml.tmpl",
								Mode:         zitilab.Loop4Listener,
							},
						},
					},
				},
			},
		},
	},

	Actions: model.ActionBinders{
		"deleteLogs": model.BindF(func(run model.Run) error {
			return run.GetModel().ForEachHost("*", 10, func(host *model.Host) error {
				return host.ExecLogOnlyOnError("rm -rf ./logs/*")
			})
		}),
		"bootstrap": NewBootstrapAction(),
		"stop":      model.Bind(component.StopInParallelHostExclusive("*", 15)),
		"login":     model.Bind(edge.Login("#ctrl1")),
		"activateBaseline": model.BindF(func(run model.Run) error {
			return activateBaseline(run, BaselineVersion)
		}),
		"stopDataPlane":  model.Bind(component.StopInParallel(".data-plane", 10)),
		"startDataPlane": model.Bind(component.StartInParallel(".data-plane", 10)),
		"swapToCandidate": model.BindF(func(run model.Run) error {
			return swapToCandidate(run)
		}),
		"runFullComparison": model.BindF(func(run model.Run) error {
			return runFullComparison(run)
		}),
		"validateUp": model.Bind(model.ActionFunc(func(run model.Run) error {
			if err := chaos.ValidateUp(run, ".ctrl", 3, 15*time.Second); err != nil {
				return err
			}
			err := run.GetModel().ForEachComponent(".ctrl", 3, func(c *model.Component) error {
				return edge.ControllerAvailable(c.Id, 30*time.Second).Execute(run)
			})
			if err != nil {
				return err
			}
			if err := chaos.ValidateUp(run, ".router", 100, time.Minute); err != nil {
				pfxlog.Logger().WithError(err).Error("validate up failed, trying to start all routers again")
				return component.StartInParallel(".router", 100).Execute(run)
			}
			return nil
		})),
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
		model.RunAction("deleteLogs"),
		distribution.DistributeSshKey("*"),
		model.StageActionF(func(run model.Run) error {
			return stageBaselineTrafficTest(run, BaselineVersion)
		}),
		model.StageActionF(func(run model.Run) error {
			return stageBaselineZiti(run, BaselineVersion)
		}),
		rsync.RsyncStaged(),
	},

	Activation: model.Stages{
		model.RunAction("stop"),
		model.RunAction("bootstrap"),
	},

	Operation: model.Stages{
		model.RunAction("login"),
		edge.SyncModelRouterIds(models.EdgeRouterTag),
		edge.SyncModelControllerIds(models.ControllerTag),

		fablibOps.StreamSarMetrics("*", 5, 1, nil),

		zitilibOps.ModelMetricsWithIdMapper(nil, func(id string) string {
			if id == "ctrl" {
				return "#ctrl"
			}
			id = strings.ReplaceAll(id, ".", ":")
			return "component.edgeId:" + id
		}),

		model.StageActionF(func(run model.Run) error {
			return runFullComparison(run)
		}),
	},

	Disposal: model.Stages{
		terraform.Dispose(),
		awsSshKeyDispose.Dispose(),
	},
}

func stageBaselineTrafficTest(run model.Run, version string) error {
	return run.DoOnce("install.ziti-traffic-test-"+version, func() error {
		srcName := "ziti-traffic-test-" + version
		src := filepath.Join(os.Getenv("GOPATH"), "bin", srcName)
		dst := filepath.Join(run.GetBinDir(), srcName)
		logrus.Infof("staging baseline traffic test: [%s] => [%s]", src, dst)
		return util.CopyFile(src, dst)
	})
}

func stageBaselineZiti(run model.Run, version string) error {
	// StageZitiOnce with version downloads the released binary as ziti-{version}
	ctrls := run.GetModel().SelectComponents(".ctrl")
	if len(ctrls) == 0 {
		return fmt.Errorf("no controller components found")
	}
	return stageziti.StageZitiOnce(run, ctrls[0], version, "")
}

// activateBaseline saves candidate binaries and activates baseline on all data-plane hosts.
func activateBaseline(run model.Run, version string) error {
	log := pfxlog.Logger()
	log.Info("activating baseline binaries on data-plane hosts")

	return run.GetModel().ForEachHost("component.data-plane", 10, func(host *model.Host) error {
		binDir := fmt.Sprintf("/home/%s/fablab/bin", host.GetSshUser())

		cmds := fmt.Sprintf(
			"cp %[1]s/ziti %[1]s/ziti-candidate && "+
				"cp %[1]s/ziti-traffic-test %[1]s/ziti-traffic-test-candidate && "+
				"cp %[1]s/ziti-%[2]s %[1]s/ziti && "+
				"cp %[1]s/ziti-traffic-test-%[2]s %[1]s/ziti-traffic-test",
			binDir, version)

		return host.ExecLogOnlyOnError(cmds)
	})
}

// swapToBaseline replaces binaries with baseline and restarts data-plane components.
// Unlike activateBaseline, this does not save candidate copies (already done on first pair).
func swapToBaseline(run model.Run, version string) error {
	log := pfxlog.Logger()
	log.Info("swapping to baseline binaries")

	if err := run.GetModel().ForEachHost("component.data-plane", 10, func(host *model.Host) error {
		binDir := fmt.Sprintf("/home/%s/fablab/bin", host.GetSshUser())
		cmds := fmt.Sprintf(
			"cp %[1]s/ziti-%[2]s %[1]s/ziti && "+
				"cp %[1]s/ziti-traffic-test-%[2]s %[1]s/ziti-traffic-test",
			binDir, version)
		return host.ExecLogOnlyOnError(cmds)
	}); err != nil {
		return fmt.Errorf("failed to swap binaries: %w", err)
	}

	return restartDataPlane(run)
}

// swapToCandidate replaces binaries with candidate and restarts data-plane components.
func swapToCandidate(run model.Run) error {
	log := pfxlog.Logger()
	log.Info("swapping to candidate binaries")

	// Replace binaries on data-plane hosts
	if err := run.GetModel().ForEachHost("component.data-plane", 10, func(host *model.Host) error {
		binDir := fmt.Sprintf("/home/%s/fablab/bin", host.GetSshUser())
		cmds := fmt.Sprintf(
			"cp %[1]s/ziti-candidate %[1]s/ziti && "+
				"cp %[1]s/ziti-traffic-test-candidate %[1]s/ziti-traffic-test",
			binDir)
		return host.ExecLogOnlyOnError(cmds)
	}); err != nil {
		return fmt.Errorf("failed to swap binaries: %w", err)
	}

	return restartDataPlane(run)
}

func restartDataPlane(run model.Run) error {
	restartWorkflow := actions.Workflow()
	restartWorkflow.AddAction(component.StartInParallel(".data-plane.edge-router", 25))
	restartWorkflow.AddAction(semaphore.Sleep(5 * time.Second))

	if err := restartWorkflow.Execute(run); err != nil {
		return fmt.Errorf("failed to restart routers: %w", err)
	}

	if err := chaos.ValidateUp(run, ".data-plane.edge-router", 100, time.Minute); err != nil {
		pfxlog.Logger().WithError(err).Warn("routers not all up after swap, continuing anyway")
	}

	hostClientWorkflow := actions.Workflow()
	hostClientWorkflow.AddAction(component.StartInParallel(".sim-services-host", 50))
	hostClientWorkflow.AddAction(semaphore.Sleep(2 * time.Second))
	hostClientWorkflow.AddAction(component.StartInParallel(".sim-services-client", 50))

	if err := hostClientWorkflow.Execute(run); err != nil {
		return fmt.Errorf("failed to restart loop components: %w", err)
	}

	time.Sleep(5 * time.Second)
	return nil
}

func runFullComparison(run model.Run) error {
	log := pfxlog.Logger()

	baselineLabel := fmt.Sprintf("baseline (%s)", BaselineVersion)
	candidateLabel := "candidate (local)"

	const targetDuration = 8 * time.Hour
	start := time.Now()

	perfReport.OpenRefFile()
	defer perfReport.CloseRefFile()

	// First pair starts with activateBaseline (saves candidate copies and activates baseline).
	// Subsequent pairs use swapToBaseline/swapToCandidate which just copy the saved binaries.
	firstPair := true

	for pairIdx := 1; time.Since(start) < targetDuration; pairIdx++ {
		log.Infof("=== Run Pair #%d (elapsed: %s) ===", pairIdx, time.Since(start).Truncate(time.Second))

		// === Baseline run ===
		log.Info("--- Starting baseline run ---")
		if firstPair {
			if err := run.GetModel().Exec(run, "stopDataPlane", "activateBaseline", "startDataPlane"); err != nil {
				return fmt.Errorf("pair %d: failed to activate baseline: %w", pairIdx, err)
			}
			firstPair = false
		} else {
			if err := run.GetModel().Exec(run, "stopDataPlane"); err != nil {
				return fmt.Errorf("pair %d: failed to stop data plane: %w", pairIdx, err)
			}
			if err := swapToBaseline(run, BaselineVersion); err != nil {
				return fmt.Errorf("pair %d: failed to swap to baseline: %w", pairIdx, err)
			}
		}

		time.Sleep(5 * time.Second)

		perfReport.SetRunLabel(baselineLabel)
		metricsDumper.SetRunLabel(baselineLabel)
		if err := run.GetModel().Exec(run, "enableMetrics"); err != nil {
			return fmt.Errorf("pair %d: failed to enable metrics for baseline: %w", pairIdx, err)
		}
		if err := run.GetModel().Exec(run, "runSimScenario"); err != nil {
			return fmt.Errorf("pair %d: baseline scenario failed: %w", pairIdx, err)
		}
		log.Info("draining final metrics...")
		time.Sleep(7 * time.Second)
		perfReport.StopCollecting()
		log.Info("--- Baseline run complete ---")

		// === Candidate run ===
		log.Info("--- Swapping to candidate ---")
		if err := run.GetModel().Exec(run, "stopDataPlane", "swapToCandidate"); err != nil {
			return fmt.Errorf("pair %d: failed to swap to candidate: %w", pairIdx, err)
		}

		log.Info("--- Starting candidate run ---")
		perfReport.SetRunLabel(candidateLabel)
		metricsDumper.SetRunLabel(candidateLabel)
		if err := run.GetModel().Exec(run, "enableMetrics"); err != nil {
			return fmt.Errorf("pair %d: failed to enable metrics for candidate: %w", pairIdx, err)
		}
		if err := run.GetModel().Exec(run, "runSimScenario"); err != nil {
			return fmt.Errorf("pair %d: candidate scenario failed: %w", pairIdx, err)
		}
		log.Info("draining final metrics...")
		time.Sleep(7 * time.Second)
		perfReport.StopCollecting()
		log.Info("--- Candidate run complete ---")

		// Record comparison for this pair, emit running summary, and reset for next
		perfReport.RecordComparison(pairIdx, baselineLabel, candidateLabel)
		perfReport.EmitSummary(baselineLabel, candidateLabel, time.Since(start))
		perfReport.ResetRuns()
	}

	metricsDumper.Close()

	return nil
}

func RunSimScenarios(run model.Run, services *zitilibOps.SimServices) error {
	if _, err := chaos.NewCtrlClients(run, "#ctrl1"); err != nil {
		return err
	}

	if err := run.GetModel().Exec(run, "startSimMetrics"); err != nil {
		return err
	}

	simControl, err := services.GetSimController(run, "sim-control", nil)
	if err != nil {
		return err
	}

	sims := run.GetModel().FilterComponents(".loop-client", func(c *model.Component) bool {
		t, ok := c.Type.(*zitilab.Loop4SimType)
		return ok && t.Mode == zitilab.Loop4RemoteControlled
	})

	err = simControl.WaitForAllConnected(time.Second*30, sims)
	if err != nil {
		return err
	}

	results, err := simControl.StartSimScenarios()
	if err != nil {
		return err
	}

	return results.GetResults(7 * time.Minute)
}

func main() {
	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)

	fablab.InitModel(m)
	fablab.Run()
}
