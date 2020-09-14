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

package loop2

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/identity/dotziti"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/transport"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/ziti-fabric-test/subcmd/loop2/pb"
	"github.com/spf13/cobra"
	"net"
	"strings"
)

func init() {
	listenerCmd.Flags().StringVarP(&listenerCmdIdentity, "identity", "i", "default", ".ziti/identities.yml name")
	listenerCmd.Flags().StringVarP(&listenerCmdBindAddress, "bind", "b", "tcp:127.0.0.1:8171", "Listener bind address")
	loop2Cmd.AddCommand(listenerCmd)
}

var listenerCmd = &cobra.Command{
	Use:   "listener",
	Short: "Start loop2 listener",
	Args:  cobra.ExactArgs(0),
	Run:   listener,
}
var listenerCmdIdentity string
var listenerCmdBindAddress string

func listener(_ *cobra.Command, _ []string) {
	if strings.HasPrefix(listenerCmdBindAddress, "edge") {
		listenEdge()
	} else {
		_, id, err := dotziti.LoadIdentity(listenerCmdIdentity)
		if err != nil {
			panic(err)
		}

		bindAddress, err := transport.ParseAddress(listenerCmdBindAddress)
		if err != nil {
			panic(err)
		}

		listen(bindAddress, id)
	}
}

func listenEdge() {
	log := pfxlog.ContextLogger(dialerCmdEndpoint)
	defer log.Error("exited")
	log.Info("started")

	context := ziti.NewContext()
	service := strings.TrimPrefix(dialerCmdEndpoint, "edge:")
	listener, err := context.Listen(service)
	if err != nil {
		panic(err)
	}

	for {
		if conn, err := listener.Accept(); err != nil {
			panic(err)
		} else {
			go handle(conn, dialerCmdEndpoint)
		}
	}
}

func listen(bind transport.Address, i *identity.TokenId) {
	log := pfxlog.ContextLogger(bind.String())
	defer log.Error("exited")
	log.Info("started")

	incoming := make(chan transport.Connection)
	go func() {
		if _, err := bind.Listen("loop", i, incoming, nil); err != nil {
			panic(err)
		}
	}()
	for {
		select {
		case peer := <-incoming:
			if peer != nil {
				go handle(peer.Conn(), peer.Detail().String())
			} else {
				return
			}
		}
	}
}

func handle(conn net.Conn, context string) {
	log := pfxlog.ContextLogger(context)
	if proto, err := newProtocol(conn); err == nil {
		if test, err := proto.rxTest(); err == nil {
			var result *loop2_pb.Result
			if err := proto.run(test); err == nil {
				result = &loop2_pb.Result{Success: true}
			} else {
				result = &loop2_pb.Result{Success: false, Message: err.Error()}
			}
			if err := proto.txResult(result); err != nil {
				log.Errorf("unable to tx result (%s)", err)
			}
		}
	} else {
		log.Errorf("error creating new protocol (%s)", err)
	}
}
