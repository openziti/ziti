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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/common/eid"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

type simpleSimAction struct {
	verbose      bool
	logFormatter string
	configFile   string
}

func newSimpleSimCmd() *cobra.Command {
	action := &simpleSimAction{}

	cmd := &cobra.Command{
		Use:   "simple-sim <config file or directory>",
		Short: "Generates traffic to one more ziti identities",
		Args:  cobra.ExactArgs(1),
		Run:   action.run,
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().BoolVarP(&action.verbose, "verbose", "v", false, "Enable verbose logging")
	cmd.Flags().StringVar(&action.logFormatter, "log-formatter", "", "Specify log formatter [json|pfxlog|text]")
	cmd.Flags().StringVarP(&action.configFile, "identity", "i", "", "Specify the Ziti identity to use. If not specified the Ziti listener won't be started")
	cmd.Flags().SetInterspersed(true)

	return cmd
}

func (self *simpleSimAction) initLogging() {
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

func (self *simpleSimAction) run(_ *cobra.Command, args []string) {
	self.initLogging()

	log := pfxlog.Logger()

	fsInfo, err := os.Stat(args[0])
	if err != nil {
		log.WithError(err).Fatalf("unable to stat config file/dir %s", args[0])
		return
	}

	if fsInfo.IsDir() {
		files, err := os.ReadDir(args[0])
		if err != nil {
			log.WithError(err).Fatalf("failed to scan directory %s:", args[0])
			return
		}

		for _, file := range files {
			if filepath.Ext(file.Name()) == ".json" {
				fn, err := filepath.Abs(filepath.Join(args[0], file.Name()))
				if err != nil {
					log.Fatalf("failed to listing file %s: %v", file.Name(), err)
				}
				self.startSimForIdentity(fn)
			}
		}
	}
}

func (self *simpleSimAction) startSimForIdentity(path string) {
	log := pfxlog.Logger().WithField("config", path)

	zitiConfig, err := ziti.NewConfigFromFile(path)
	if err != nil {
		log.WithError(err).Fatal("unable to load ziti identity")
		return
	}

	zitiContext, err := ziti.NewContext(zitiConfig)
	if err != nil {
		log.WithError(err).Fatal("could not create sdk context from config")
	}

	go self.runSim(path, zitiContext)
}

func (self *simpleSimAction) runSim(path string, ctx ziti.Context) {
	log := pfxlog.Logger().WithField("config", path)

	var lastLog time.Time
	cycle := 0

	for {
		cycle++
		services, err := ctx.GetServices()
		if err != nil {
			log.WithError(err).Error("unable to list services")
			time.Sleep(time.Second * 5)
			continue
		}

		for _, service := range services {
			conn, err := ctx.Dial(*service.Name)
			if err != nil {
				log.WithField("svc", *service.Name).WithError(err).Error("unable to dial service")
			} else {
				randomString := eid.New()
				if _, err = conn.Write([]byte(randomString)); err != nil {
					log.WithField("svc", *service.Name).WithError(err).Error("unable to write")
				} else {
					result := make([]byte, len(randomString))
					n, err := conn.Read(result)
					if err != nil {
						log.WithField("svc", *service.Name).WithError(err).Error("unable to read")
					} else {
						result = result[:n]
						if string(result) != randomString {
							log.WithField("svc", *service.Name).WithError(err).Error("%s != %s", randomString, result)
						}
					}
				}
			}

			if conn != nil {
				if err = conn.Close(); err != nil {
					log.WithField("svc", *service.Name).WithError(err).Error("error closing conn")
				}
			}
		}

		if time.Since(lastLog) > time.Minute {
			log.WithField("serviceCount", len(services)).WithField("cycle", cycle).Info("completed")
			lastLog = time.Now()
		}

		time.Sleep(time.Duration(500 + rand.Intn(1000)))
	}
}

func main() {
	cli := newSimpleSimCmd()
	if err := cli.Execute(); err != nil {
		pfxlog.Logger().Fatal(err.Error())
	}
}
