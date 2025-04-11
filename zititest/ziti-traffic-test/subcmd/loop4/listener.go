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
	"github.com/michaelquigley/pfxlog"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"net"
	"net/http"
)

func init() {
	loop4Cmd.AddCommand(newListenerCmd())
}

type listenerCmd struct {
	*Sim

	healthCheckAddr string
}

func newListenerCmd() *cobra.Command {
	result := &listenerCmd{
		Sim: NewSim(),
	}

	cmd := &cobra.Command{
		Use:   "listener",
		Short: "Start loop3 listener",
		Args:  cobra.MaximumNArgs(1),
		Run:   result.run,
	}

	flags := cmd.Flags()
	flags.StringVar(&result.healthCheckAddr, "health-check-addr", "", "Edge SDK config file")

	return cmd
}

func (cmd *listenerCmd) run(_ *cobra.Command, args []string) {
	log := pfxlog.Logger()

	if err := cmd.InitScenario(args[0]); err != nil {
		panic(err)
	}

	var err error

	if cmd.healthCheckAddr != "" {
		go func() {
			err = http.ListenAndServe(cmd.healthCheckAddr, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(200)
			}))
			if err != nil {
				log.WithError(err).Fatalf("unable to start health check endpoint on addr [%v]", cmd.healthCheckAddr)
			}
		}()
	}

	for _, workload := range cmd.scenario.Workloads {
		cmd.runWorkload(workload)
	}
}

func (cmd *listenerCmd) runWorkload(workload *Workload) {
	log := pfxlog.Logger().WithField("workload", workload.Name)

	listenerF, ok := cmd.listeners[workload.Connector]
	if !ok {
		log.Fatalf("workload '%s' connector '%s' not defined", workload.Name, workload.Connector)
		return
	}

	listener, err := listenerF(workload)
	if err != nil {
		panic(err)
	}

	for {
		if conn, err := listener.Accept(); err != nil {
			panic(err)
		} else {
			go cmd.handle(conn, workload)
		}
	}
}

func (cmd *listenerCmd) handle(conn net.Conn, workload *Workload) {
	log := pfxlog.Logger().WithField("workload", workload.Name)

	if proto, err := newProtocol(conn, workload.Name, cmd.metrics); err == nil {
		_, test := workload.GetTests()

		if test == nil || !test.IsRxSequential() {
			if test, err = proto.rxTest(); err != nil {
				logrus.WithError(err).Error("failure receiving test parameters, closing")
				_ = conn.Close()
				return
			}
		}

		var result *Result
		if err = proto.run(test); err == nil {
			result = &Result{Success: true}
		} else {
			result = &Result{Success: false, Message: err.Error()}
		}

		if err = result.Tx(proto); err != nil {
			log.Errorf("unable to tx result (%s)", err)
		}

	} else {
		log.WithError(err).Error("error creating new protocol")
	}
}
