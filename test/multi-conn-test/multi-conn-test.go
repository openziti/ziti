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
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
)

func init() {
	pfxlog.Global(logrus.InfoLevel)
	pfxlog.SetPrefix("github.com/openziti/")
	pfxlog.SetDefaultNoColor()
}

func init() {
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	root.PersistentFlags().StringVar(&logFormatter, "log-formatter", "", "Specify log formatter [json|pfxlog|text]")
	root.PersistentFlags().Uint32VarP(&connCount, "connections", "c", 2, "Number of concurrent connections")
	root.PersistentFlags().Uint32VarP(&iterations, "iterations", "i", 10, "Number of iterations")
	root.Flags().SetInterspersed(true)
}

var root = &cobra.Command{
	Use:   "multi-conn-test <service>",
	Short: "Multiplexed connection tester Ziti Application",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if verbose {
			logrus.SetLevel(logrus.DebugLevel)
		}

		switch logFormatter {
		case "pfxlog":
			logrus.SetFormatter(pfxlog.NewFormatterStartingToday())
		case "json":
			logrus.SetFormatter(&logrus.JSONFormatter{})
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
var connCount uint32
var iterations uint32

var iterCount uint32

var service = "echo"
var ztContext ziti.Context

var waitGroup sync.WaitGroup

func main() {
	debugz.AddStackDumpHandler()
	if err := root.Execute(); err != nil {
		fmt.Printf("error: %s", err)
	}
}

func runFunc(_ *cobra.Command, args []string) {
	ztContext = ziti.NewContext()

	if len(args) > 0 {
		service = args[0]
	}

	var conns []net.Conn

	waitGroup.Add(int(iterations))

	for i := uint32(0); i < connCount; i++ {
		conn, err := ztContext.Dial(service)
		if err != nil {
			panic(err)
		}
		conns = append(conns, conn)
	}

	for _, conn := range conns {
		iteration := atomic.AddUint32(&iterCount, 1)
		go runIteration(iteration, conn)
	}

	waitGroup.Wait()
}

func runIteration(iteration uint32, conn net.Conn) {
	defer waitGroup.Done()

	log := pfxlog.Logger()
	log.Infof("Starting iteration %v", iteration)
	bufferSize := 10 + rand.Int31n(1024)
	writeBuf := make([]byte, bufferSize)
	readBuf := make([]byte, bufferSize)
	for i := 0; i < 10; i++ {
		_, _ = rand.Read(writeBuf)
		if _, err := conn.Write(writeBuf); err != nil {
			panic(err)
		}
		if _, err := io.ReadFull(conn, readBuf); err != nil {
			panic(err)
		}
	}
	nextIteration()
}

func nextIteration() {
	iteration := atomic.AddUint32(&iterCount, 1)
	if iteration <= iterations {
		conn, err := ztContext.Dial(service)
		if err != nil {
			panic(err)
		}
		runIteration(iteration, conn)
	} else {
		log := pfxlog.Logger()
		log.Infof("Reached max iteration count, exiting")
	}
}
