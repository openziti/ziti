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
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
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
	Use:   "sdk-echo-server <service>",
	Short: "Echos incoming data",
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

	service := "echo"
	if len(args) > 0 {
		service = args[0]
		log.Infof("Using service %v", service)
	} else {
		log.Infof("No service provided, using service %v: ", service)
	}

	listener, err := ztContext.Listen(service)
	if err != nil {
		log.Fatalf("failed to host service %v: %v", service, err)
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			panic(err)
		}
		log.Infof("Connection received: %v", conn)
		go echo(conn)
	}
}

func echo(conn io.ReadWriteCloser) {
	log := pfxlog.Logger().WithField("conn", conn)

	defer func() {
		if err := conn.Close(); err != nil {
			log.WithError(err).Error("Error while closing connection")
		}
		log.Info("Connection closed")
	}()

	for {
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			log.WithError(err).Error("Error while reading")
			break
		}
		log.Infof("Read %v bytes\n", n)
		writeBuf := buf[:n]
		if _, err := conn.Write(writeBuf); err != nil {
			log.WithError(err).Error("Error while writing")
			break
		}
		log.Infof("Wrote %v bytes\n", n)
	}
}
