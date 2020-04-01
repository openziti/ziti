/*
	Copyright NetFoundry, Inc.

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

package loop3

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	loop3_pb "github.com/netfoundry/ziti-cmd/ziti-fabric-test/subcmd/loop3/pb"
	"github.com/netfoundry/ziti-fabric/router/xgress_transport"
	"github.com/netfoundry/ziti-foundation/identity/dotziti"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-foundation/transport"
	"github.com/netfoundry/ziti-sdk-golang/ziti"
	"github.com/spf13/cobra"
	"net"
	"strings"
	"time"
)

func init() {
	dialerCmd.Flags().StringVarP(&dialerCmdIdentity, "identity", "i", "default", ".ziti/identities.yml name")
	dialerCmd.Flags().StringVarP(&dialerCmdEndpoint, "endpoint", "e", "tls:127.0.0.1:7001", "Endpoint address")
	dialerCmd.Flags().BoolVarP(&dialerCmdDirect, "direct", "d", false, "Transmit direct (no ingress)")
	dialerCmd.Flags().StringVarP(&dialerCmdService, "service", "s", "loop", "Service name for ingress")
	loop3Cmd.AddCommand(dialerCmd)
}

var dialerCmd = &cobra.Command{
	Use:   "dialer <scenarioFile>",
	Short: "Start loop3 dialer",
	Args:  cobra.ExactArgs(1),
	Run:   dialer,
}
var dialerCmdIdentity string
var dialerCmdEndpoint string
var dialerCmdDirect bool
var dialerCmdService string

func dialer(_ *cobra.Command, args []string) {
	log := pfxlog.Logger()

	scenario, err := LoadScenario(args[0])
	if err != nil {
		panic(err)
	}
	log.Debug(scenario)

	resultChs := make(map[string]chan *loop3_pb.Result)
	for _, workload := range scenario.Workloads {
		log.Infof("executing workload [%s] with concurrency [%d]", workload.Name, workload.Concurrency)

		for i := 0; i < int(workload.Concurrency); i++ {
			name := fmt.Sprintf("%s:%d", workload.Name, i)
			resultCh := make(chan *loop3_pb.Result, 1)
			resultChs[name] = resultCh

			go func() {
				var conn net.Conn
				if strings.HasPrefix(dialerCmdEndpoint, "edge:") {
					context := ziti.NewContext()
					service := strings.TrimPrefix(dialerCmdEndpoint, "edge:")
					conn, err = context.Dial(service)
					if err != nil {
						panic(err)
					}
				} else {
					endpoint, err := transport.ParseAddress(dialerCmdEndpoint)
					if err != nil {
						panic(err)
					}

					_, id, err := dotziti.LoadIdentity(dialerCmdIdentity)
					if err != nil {
						panic(err)
					}

					if dialerCmdDirect {
						if conn, err = dialDirect(endpoint, id); err != nil {
							panic(err)
						}
					} else {
						serviceId := &identity.TokenId{Token: dialerCmdService}
						if conn, err = dialIngress(endpoint, id, serviceId); err != nil {
							panic(err)
						}
					}
				}

				workload := scenario.Workloads[0]
				local := &loop3_pb.Test{
					Name:            name,
					TxRequests:      workload.Dialer.TxRequests,
					TxPacing:        workload.Dialer.TxPacing,
					TxMaxJitter:     workload.Dialer.TxMaxJitter,
					RxRequests:      workload.Listener.TxRequests,
					RxTimeout:       workload.Dialer.RxTimeout,
					PayloadMinBytes: workload.Dialer.PayloadMinBytes,
					PayloadMaxBytes: workload.Dialer.PayloadMaxBytes,
				}
				remote := &loop3_pb.Test{
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

func dialDirect(endpoint transport.Address, id *identity.TokenId) (net.Conn, error) {
	peer, err := endpoint.Dial("loop", id)
	if err != nil {
		return nil, err
	}

	return peer.Conn(), nil
}

func dialIngress(endpoint transport.Address, id, serviceId *identity.TokenId) (net.Conn, error) {
	peer, err := xgress_transport.ClientDial(endpoint, id, serviceId)
	if err != nil {
		return nil, err
	}

	return peer.Conn(), nil
}
