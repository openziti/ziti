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

package loop2

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/identity"
	"github.com/openziti/identity/dotziti"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/router/xgress_transport"
	loop2_pb "github.com/openziti/ziti/zititest/ziti-fabric-test/subcmd/loop2/pb"
	"github.com/spf13/cobra"
)

func init() {
	dialerCmd := newDialerCmd()
	loop2Cmd.AddCommand(dialerCmd.cmd)
}

type dialerCmd struct {
	cmd            *cobra.Command
	identity       string
	endpoint       string
	direct         bool
	service        string
	edgeConfigFile string
}

func newDialerCmd() *dialerCmd {
	result := &dialerCmd{
		cmd: &cobra.Command{
			Use:   "dialer <scenarioFile>",
			Short: "Start loop2 dialer",
			Args:  cobra.ExactArgs(1),
		},
	}

	result.cmd.Run = result.run

	flags := result.cmd.Flags()
	flags.StringVarP(&result.identity, "identity", "i", "default", ".ziti/identities.yml name")
	flags.StringVarP(&result.endpoint, "endpoint", "e", "tls:127.0.0.1:7001", "Endpoint address")
	flags.BoolVarP(&result.direct, "direct", "d", false, "Transmit direct (no ingress)")
	flags.StringVarP(&result.service, "service", "s", "loop", "Service name for ingress")
	flags.StringVarP(&result.edgeConfigFile, "config-file", "c", "", "Edge SDK config file")

	return result
}

func (cmd *dialerCmd) run(_ *cobra.Command, args []string) {
	log := pfxlog.Logger()

	scenario, err := LoadScenario(args[0])
	if err != nil {
		panic(err)
	}
	log.Debug(scenario)

	resultChs := make(map[string]chan *loop2_pb.Result)
	for _, workload := range scenario.Workloads {
		log.Infof("executing workload [%s] with concurrency [%d]", workload.Name, workload.Concurrency)

		var conns []net.Conn
		for i := 0; i < int(workload.Concurrency); i++ {
			conns = append(conns, cmd.connect())
		}

		for i, conn := range conns {
			name := fmt.Sprintf("%s:%d", workload.Name, i)
			resultCh := make(chan *loop2_pb.Result, 1)
			resultChs[name] = resultCh

			go func() {
				workload := scenario.Workloads[0]
				local := &loop2_pb.Test{
					Name:            name,
					TxRequests:      workload.Dialer.TxRequests,
					TxPacing:        workload.Dialer.TxPacing,
					TxMaxJitter:     workload.Dialer.TxMaxJitter,
					RxRequests:      workload.Listener.TxRequests,
					RxTimeout:       workload.Dialer.RxTimeout,
					PayloadMinBytes: workload.Dialer.PayloadMinBytes,
					PayloadMaxBytes: workload.Dialer.PayloadMaxBytes,
				}
				remote := &loop2_pb.Test{
					Name:            name,
					TxRequests:      workload.Listener.TxRequests,
					TxPacing:        workload.Listener.TxPacing,
					TxMaxJitter:     workload.Listener.TxMaxJitter,
					RxRequests:      workload.Dialer.TxRequests,
					RxTimeout:       workload.Listener.RxTimeout,
					PayloadMinBytes: workload.Listener.PayloadMinBytes,
					PayloadMaxBytes: workload.Listener.PayloadMaxBytes,
				}

				if proto, err := newProtocol(conn); err == nil {
					if err := proto.txTest(remote); err == nil {
						if err := proto.run(local); err == nil {
							if result, err := proto.rxResult(); err == nil {
								resultCh <- result
							} else {
								panic(err)
							}
						} else {
							panic(err)
						}
					} else {
						panic(err)
					}
				} else {
					panic(err)
				}
			}()

			time.Sleep(time.Duration(scenario.ConnectionDelay) * time.Millisecond)
		}
	}

	failed := false
	for name, resultCh := range resultChs {
		result := <-resultCh
		if !result.Success {
			failed = true
			log.Errorf("[%s] -> %s", name, result.Message)
		} else {
			log.Infof("[%s] -> success", name)
		}
	}
	if failed {
		panic("failures detected")
	} else {
		log.Info("success")
	}
}

func (cmd *dialerCmd) connect() net.Conn {
	log := pfxlog.Logger()

	var conn net.Conn
	var err error
	if strings.HasPrefix(cmd.endpoint, "edge:") {

		var context ziti.Context
		if cmd.edgeConfigFile != "" {
			zitiCfg, err := ziti.NewConfigFromFile(cmd.edgeConfigFile)
			if err != nil {
				log.Fatalf("failed to load ziti configuration from %s: %v", cmd.edgeConfigFile, err)
			}

			context, err = ziti.NewContext(zitiCfg)
			if err != nil {
				log.Fatalf("failed to load ziti context from configuration: %v", err)
			}
		} else {
			log.Fatal("no configuration file provided")
		}

		service := strings.TrimPrefix(cmd.endpoint, "edge:")
		conn, err = context.Dial(service)
		if err != nil {
			panic(err)
		}
	} else {
		endpoint, err := transport.ParseAddress(cmd.endpoint)
		if err != nil {
			panic(err)
		}

		_, id, err := dotziti.LoadIdentity(cmd.identity)
		if err != nil {
			panic(err)
		}

		if cmd.direct {
			if conn, err = dialDirect(endpoint, id); err != nil {
				panic(err)
			}
		} else {
			serviceId := &identity.TokenId{Token: cmd.service}
			if conn, err = dialIngress(endpoint, id, serviceId); err != nil {
				panic(err)
			}
		}
	}
	return conn
}

func dialDirect(endpoint transport.Address, id *identity.TokenId) (net.Conn, error) {
	peer, err := endpoint.Dial("loop", id, 0, nil)
	if err != nil {
		return nil, err
	}

	return peer, nil
}

func dialIngress(endpoint transport.Address, id, serviceId *identity.TokenId) (net.Conn, error) {
	peer, err := xgress_transport.ClientDial(endpoint, id, serviceId, nil)
	if err != nil {
		return nil, err
	}

	return peer, nil
}
