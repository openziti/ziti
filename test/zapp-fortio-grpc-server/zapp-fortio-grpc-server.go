// +build utils

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

package main

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/debugz"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"
	"time"
)

func init() {
	pfxlog.Global(logrus.InfoLevel)
	pfxlog.SetPrefix("github.com/openziti/")
	pfxlog.SetDefaultNoColor()
}

func init() {
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	root.PersistentFlags().StringVar(&logFormatter, "log-formatter", "", "Specify log formatter [json|pfxlog|text]")

	root.Flags().SetInterspersed(true)
}

var root = &cobra.Command{
	Use:   "zapp-fortio-grpc-server <service>",
	Short: "Fortio GRPC Ping Server Ziti Application",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if verbose {
			logrus.SetLevel(logrus.DebugLevel)
		}

		switch logFormatter {
		case "pfxlog":
			logrus.SetFormatter(pfxlog.NewFormatterStartingToday())
		case "json":
			logrus.SetFormatter(&logrus.JSONFormatter{TimestampFormat: "2006-01-02T15:04:05.000Z"})
		case "text":
			logrus.SetFormatter(&logrus.TextFormatter{})
		default:
			// let logrus do its own thing
		}
	},
	Args: cobra.RangeArgs(0, 1),
	Run:  runFunc,
}

var verbose bool
var logFormatter string

func main() {
	debugz.AddStackDumpHandler()
	if err := root.Execute(); err != nil {
		fmt.Printf("error: %s", err)
	}
}

func runFunc(_ *cobra.Command, args []string) {
	log := pfxlog.Logger()
	ztContext := ziti.NewContext()

	service := "fortio-grpc"
	if len(args) > 0 {
		service = args[0]
	}

	listener, err := ztContext.Listen(service)
	if err != nil {
		log.WithError(err).Fatalf("failed to host %v service", service)
	}

	grpcServer := grpc.NewServer()
	reflection.Register(grpcServer)
	grpcServer.RegisterService(&_PingServer_serviceDesc, &pingSrv{})

	err = grpcServer.Serve(listener)

	if err != nil {
		log.WithError(err).Fatal("failed to start fortio grpc server")
	}
}

type pingSrv struct {
}

type PingServerServer interface {
	Ping(context.Context, *PingMessage) (*PingMessage, error)
}

var _PingServer_serviceDesc = grpc.ServiceDesc{
	ServiceName: "fgrpc.PingServer",
	HandlerType: (*PingServerServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Ping",
			Handler:    _PingServer_Ping_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "ping.proto",
}

func _PingServer_Ping_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(PingMessage)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PingServerServer).Ping(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/fgrpc.PingServer/Ping",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PingServerServer).Ping(ctx, req.(*PingMessage))
	}
	return interceptor(ctx, in, info, handler)
}

func (s *pingSrv) Ping(c context.Context, in *PingMessage) (*PingMessage, error) {
	log := pfxlog.Logger()
	var connId uint32
	if p, ok := peer.FromContext(c); ok {
		var msgChanIf interface{}
		msgChanIf = p.Addr
		if msgChan, ok := msgChanIf.(edge.Identifiable); ok {
			connId = msgChan.Id()
		} else {
			log.Error("Unable to cast peer address to edge.MsgChannel")
		}
	} else {
		log.Error("Unable to get peer from context")
	}

	log.WithField("connId", connId).Infof("Ping called %+v (ctx %+v)", *in, c)
	out := *in // copy the input including the payload etc
	out.Ts = time.Now().UnixNano()
	if in.DelayNanos > 0 {
		s := time.Duration(in.DelayNanos)
		log.Debugf("GRPC ping: sleeping for %v", s)
		time.Sleep(s)
	}
	return &out, nil
}
