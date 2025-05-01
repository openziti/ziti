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

package loop4

import (
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/spf13/cobra"
	"net"
	"sync/atomic"
	"time"
)

func init() {
	loop4Cmd.AddCommand(newDialerCmd())
}

type dialerCmd struct {
	*Sim
}

func newDialerCmd() *cobra.Command {
	dialer := &dialerCmd{
		Sim: NewSim(),
	}

	cmd := &cobra.Command{
		Use:   "dialer <scenarioFile>",
		Short: "Start loop4 dialer",
		Args:  cobra.ExactArgs(1),
		Run:   dialer.run,
	}

	return cmd
}

func (cmd *dialerCmd) run(_ *cobra.Command, args []string) {
	log := pfxlog.Logger()

	defer close(cmd.closeNotify)

	if err := cmd.InitScenario(args[0]); err != nil {
		panic(err)
	}

	if err := cmd.runScenario(cmd.scenario); err != nil {
		log.WithError(err).Fatal("error running scenario")
	}
}

func (sim *Sim) runScenario(scenario *Scenario) error {
	log := pfxlog.Logger()

	resultChs := make(map[string]chan *Result)
	for _, workload := range scenario.Workloads {
		if _, ok := sim.dialers[workload.Connector]; !ok {
			return fmt.Errorf("workload '%s' uses unknown connector '%s'", workload.Name, workload.Connector)
		}

		if workload.ConnectTimeout == 0 {
			workload.ConnectTimeout = 5 * time.Second
		}

		if workload.Iterations == 0 {
			workload.Iterations = 1
		}

		for i := 0; i < int(workload.Concurrency); i++ {
			resultCh := make(chan *Result, 1)
			resultChs[workload.GetRunnerName(i)] = resultCh
		}
	}

	for _, workload := range scenario.Workloads {
		log.Infof("executing workload [%s] with concurrency [%d]", workload.Name, workload.Concurrency)
		var active atomic.Int64
		for i := 0; i < int(workload.Concurrency); i++ {
			resultCh := resultChs[workload.GetRunnerName(i)]
			go sim.RunWorkload(scenario, workload, i+1, resultCh, &active)
		}
	}

	var errs []error

	for name, resultCh := range resultChs {
		result := <-resultCh
		if !result.Success {
			log.Errorf("[%s] -> error (%s)", name, result.Message)
			errs = append(errs, fmt.Errorf("[%s] -> error (%s)", name, result.Message))
		} else {
			log.Infof("[%s] -> success", name)
		}
	}

	return errors.Join(errs...)
}

func (sim *Sim) RunWorkload(scenario *Scenario, workload *Workload, idx int, resultCh chan *Result, active *atomic.Int64) {
	log := pfxlog.Logger()

	if workload.ConnectTimeout < time.Second {
		workload.ConnectTimeout = time.Second
	}

	var err error
	var conn net.Conn

	defer func() {
		if conn != nil {
			_ = conn.Close()
		}
	}()

	connectTimes := sim.metrics.Timer(workload.Name + ".connect.times")
	connectFailures := sim.metrics.Meter(workload.Name + ".connect.failures")
	connectSuccesses := sim.metrics.Meter(workload.Name + ".connect.successes")
	completed := sim.metrics.Meter(workload.Name + ".completed")
	sim.metrics.FuncGauge(workload.Name+".active", func() int64 {
		return active.Load()
	})

	var result *Result
	for i := int64(0); i < workload.Iterations || workload.Iterations == -1; i++ {
		if conn != nil {
			if err = conn.Close(); err != nil {
				log.Errorf("unable to close connection for workload [%s]: %v", workload.Name, err)
			}
			conn = nil
		}
		startConnect := time.Now()
		conn, err = sim.dialers[workload.Connector](workload)
		connectTimes.UpdateSince(startConnect)
		if err != nil {
			connectFailures.Mark(1)
			continue
		}

		active.Add(1)

		connectSuccesses.Mark(1)
		local, remote := workload.GetTests()

		if proto, err := newProtocol(conn, workload.Name, sim.metrics); err == nil {
			if local.IsTxRandomHashed() {
				if err = proto.txTest(remote); err != nil {
					sim.reportErr(resultCh, err)
					active.Add(-1)
					return
				}
			}

			if err = proto.run(local); err != nil {
				sim.reportErr(resultCh, err)
				active.Add(-1)
				return
			}

			if result, err = proto.rxResult(); err != nil {
				sim.reportErr(resultCh, err)
				active.Add(-1)
				return
			}

			if !result.Success {
				active.Add(-1)
				resultCh <- result
				return
			}
		}

		active.Add(-1)
		completed.Mark(1)
		pfxlog.Logger().Debugf("%s-%d completed iteration %d", workload.Name, idx, i+1)
		if scenario.ConnectionDelay > 0 {
			time.Sleep(time.Duration(sim.scenario.ConnectionDelay) * time.Millisecond)
		}
	}

	resultCh <- &Result{
		Success: true,
	}
}

func (sim *Sim) reportErr(resultCh chan *Result, err error) {
	result := &Result{
		Success: false,
		Message: err.Error(),
	}
	select {
	case resultCh <- result:
	default:
		pfxlog.Logger().WithError(err).Panicf("failed to send result")
	}
}
