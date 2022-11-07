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

package demo

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/info"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

type zcatAction struct {
	verbose      bool
	logFormatter string
	configFile   string
}

func newZcatCmd() *cobra.Command {
	action := &zcatAction{}

	cmd := &cobra.Command{
		Use:     "zcat <destination>",
		Example: "ziti demo zcat tcp:localhost:1234 or ziti demo zcat -i zcat.json ziti:echo",
		Short:   "A simple network cat facility which can run over tcp or ziti",
		Args:    cobra.ExactArgs(1),
		Run:     action.run,
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().BoolVarP(&action.verbose, "verbose", "v", false, "Enable verbose logging")
	cmd.Flags().StringVar(&action.logFormatter, "log-formatter", "", "Specify log formatter [json|pfxlog|text]")
	cmd.Flags().StringVarP(&action.configFile, "identity", "i", "", "Specify the Ziti identity to use. If not specified the Ziti listener won't be started")
	cmd.Flags().SetInterspersed(true)

	return cmd
}

func (self *zcatAction) initLogging() {
	logLevel := logrus.InfoLevel
	if self.verbose {
		logLevel = logrus.DebugLevel
	}

	options := pfxlog.DefaultOptions().SetTrimPrefix("github.com/openziti/").NoColor()
	pfxlog.GlobalInit(logLevel, options)

	switch self.logFormatter {
	case "pfxlog":
		pfxlog.SetFormatter(pfxlog.NewFormatter(pfxlog.DefaultOptions().SetTrimPrefix("github.com/openziti/").StartingToday()))
	case "json":
		pfxlog.SetFormatter(&logrus.JSONFormatter{TimestampFormat: "2006-01-02T15:04:05.000Z"})
	case "text":
		pfxlog.SetFormatter(&logrus.TextFormatter{})
	default:
		// let logrus do its own thing
	}
}

func (self *zcatAction) run(_ *cobra.Command, args []string) {
	self.initLogging()

	log := pfxlog.Logger()

	parts := strings.Split(args[0], ":")
	if len(parts) < 2 {
		log.Fatalf("destination %v is invalid. destination must be of the form network:address", args[0])
	}
	network := parts[0]
	addr := strings.Join(parts[1:], ":")

	var conn net.Conn
	var err error

	log.Debugf("dialing %v:%v", network, addr)
	if network == "tcp" || network == "udp" {
		conn, err = net.Dial(network, addr)
	} else if network == "ziti" {
		zitiConfig, cfgErr := config.NewFromFile(self.configFile)
		if cfgErr != nil {
			log.WithError(cfgErr).Fatalf("unable to load ziti identity from [%v]", self.configFile)
		}

		dialIdentifier := ""
		if atIdx := strings.IndexByte(addr, '@'); atIdx > 0 {
			dialIdentifier = addr[:atIdx]
			addr = addr[atIdx+1:]
		}

		zitiContext := ziti.NewContextWithConfig(zitiConfig)
		dialOptions := &ziti.DialOptions{
			ConnectTimeout: 5 * time.Second,
			Identity:       dialIdentifier,
		}
		conn, err = zitiContext.DialWithOptions(addr, dialOptions)
	} else {
		log.Fatalf("invalid network '%v'. valid values are ['ziti', 'tcp', 'udp']", network)
	}

	if err != nil {
		log.WithError(err).Fatalf("unable to dial %v:%v", network, addr)
	}

	log.Debugf("connected to %v:%v", network, addr)

	go self.copy(conn, os.Stdin)
	self.copy(os.Stdout, conn)
}

func (self *zcatAction) copy(writer io.Writer, reader io.Reader) {
	buf := make([]byte, info.MaxUdpPacketSize)
	bytesCopied, err := io.CopyBuffer(writer, reader, buf)
	pfxlog.Logger().Debugf("Copied %v bytes", bytesCopied)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("error while copying bytes")
	}
}
