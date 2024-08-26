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
package verify

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/openziti/ziti/common"
)

var log = pfxlog.Logger()

type network struct {
	controllerConfig string
	routerConfig     string
	verbose bool
}

type protoHostPort struct {
	proto string
	host  string
	port  string
}

type stringMapList []interface{}

type StringMap map[string]interface{}

func NewVerifyNetwork() *cobra.Command {
	n := &network{}

	cmd := &cobra.Command{
		Use:   "verify-network",
		Short: "Verifies the overlay is configured correctly",
		Long:  "A tool to verify network configurations, checking controller and router ports or other common problems",
		Run: func(cmd *cobra.Command, args []string) {
			logLvl := logrus.InfoLevel
			if n.verbose {
				logLvl = logrus.DebugLevel
			}

			pfxlog.GlobalInit(logLvl, pfxlog.DefaultOptions().Color())
			configureLogFormat(logLvl)

			anyFailure := false
			if n.controllerConfig != "" {
				log.Infof("Verifying controller config: %s", n.controllerConfig)
				anyFailure = verifyControllerConfig(n.controllerConfig) || anyFailure
				fmt.Println()
			}
			if n.routerConfig != "" {
				log.Infof("Verifying router config: %s", n.routerConfig)
				anyFailure = verifyRouterConfig(n.routerConfig) || anyFailure
				fmt.Println()
			}
			if anyFailure {
				log.Error("One or more error. Review the output above for errors.")
			} else {
				log.Info("All requested checks passed.")
			}
		},
	}

	cmd.Flags().StringVarP(&n.controllerConfig, "controller-config-file", "c", "", "the controller config file verify")
	cmd.Flags().StringVarP(&n.routerConfig, "router-config-file", "r", "", "the router config file to verify")
	cmd.Flags().BoolVar(&n.verbose, "verbose", false, "Show additional output.")

	return cmd
}

func (m StringMap) mapFromKey(key string) StringMap {
	if v, ok := m[key]; ok {
		return v.(StringMap)
	}
	log.Fatalf("map didn't contain key %s", key)
	return nil
}

func (m StringMap) listFromKey(key string) stringMapList {
	if v, ok := m[key]; ok {
		return v.([]interface{})
	}
	log.Fatalf("map didn't contain key %s", key)
	return nil
}

func (p protoHostPort) testPort(msg string) bool {
	conn, err := net.DialTimeout("tcp", p.address(), 3*time.Second)
	if err != nil {
		log.Errorf("%s at %s cannot be reached.", msg, p.address())
		return true
	}
	defer func(conn net.Conn) {
		_ = conn.Close()
	}(conn)
	log.Infof("%s at %s is available.", msg, p.address())
	return false
}

func (p protoHostPort) address() string {
	return p.host + ":" + p.port
}

func fromString(input string) *protoHostPort {
	// input is expected to be in either "proto:host:port" format or "host:port" format
	r := new(protoHostPort)
	parts := strings.Split(input, ":")
	if len(parts) > 2 {
		r.proto = parts[0]
		r.host = parts[1]
		r.port = parts[2]
	} else if len(parts) > 1 {
		r.proto = "none"
		r.host = parts[0]
		r.port = parts[1]
	} else {
		panic("input is invalid: " + input)
	}
	return r
}

func verifyControllerConfig(controllerConfigFile string) bool {
	if _, err := os.Stat(controllerConfigFile); err != nil {
		log.Errorf("controller config file %s does not exist", controllerConfigFile)
		return true
	}
	ctrlCfgBytes, err := os.ReadFile(controllerConfigFile)
	if err != nil {
		panic(err)
	}
	ctrlCfg := make(StringMap)
	err = yaml.Unmarshal(ctrlCfgBytes, &ctrlCfg)
	if err != nil {
		panic(err)
	}
	advertiseAddress := stringOrEmpty(ctrlCfg.mapFromKey("ctrl").mapFromKey("options")["advertiseAddress"])
	host := fromString(advertiseAddress)
	anyFailure := host.testPort("controller advertise address")

	web := ctrlCfg.listFromKey("web")

	log.Infof("verifying %d web entries", len(web))
	for _, item := range web {
		webEntry := item.(StringMap)
		webName := stringOrEmpty(webEntry["name"])
		bps := webEntry.listFromKey("bindPoints")
		log.Infof("verifying %d web bindPoints", len(bps))
		bpPos := 0
		for _, bpItem := range bps {
			bp := bpItem.(StringMap)
			bpInt := fromString(stringOrEmpty(bp["interface"]))
			bpAddr := fromString(stringOrEmpty(bp["address"]))
			if bpInt.port != bpAddr.port {
				log.Warnf("web entry[%s], bindPoint[%d] ports differ. make sure this is intentional. interface port: %s, address port: %s", webName, bpPos, bpInt.port, bpAddr.port)
			}

			if bpAddr.testPort(fmt.Sprintf("web entry[%s], bindPoint[%d] %s", webName, bpPos, "address")) {
				anyFailure = true
			} else {
				log.Infof("web entry[%s], bindPoint[%d] is valid", webName, bpPos)
			}
			bpPos++
		}
	}
	return anyFailure
}

func verifyRouterConfig(routerConfigFile string) bool {
	if _, err := os.Stat(routerConfigFile); err != nil {
		log.Errorf("router config file %s does not exist", routerConfigFile)
		return true
	}
	routerCfg := make(StringMap)
	routerCfgBytes, err := os.ReadFile(routerConfigFile)
	if err != nil {
		panic(err)
	}

	err = yaml.Unmarshal(routerCfgBytes, &routerCfg)
	if err != nil {
		panic(err)
	}

	controllerEndpoint := stringOrEmpty(routerCfg.mapFromKey("ctrl")["endpoint"])
	routerCtrl := fromString(controllerEndpoint)
	anyFailure := routerCtrl.testPort("ctrl endpoint")

	link := routerCfg.mapFromKey("link")
	linkListeners := link.listFromKey("listeners")
	log.Infof("verifying %d web link listeners", len(linkListeners))
	pos := 0
	for _, item := range linkListeners {
		listener := item.(StringMap)
		if verifyLinkListener(fmt.Sprintf("link listener[%d]", pos), listener) {
			anyFailure = true
		} else {
			log.Infof("link listener[%d] is valid", pos)
		}
		pos++
	}

	edgeListeners := routerCfg.listFromKey("listeners")
	log.Infof("verifying %d web edge listeners", len(edgeListeners))
	pos = 0
	for _, item := range edgeListeners {
		listener := item.(StringMap)
		if verifyEdgeListener(fmt.Sprintf("listener binding[%d]", pos), listener) {
			anyFailure = true
		} else {
			log.Infof("listener binding[%d] is valid", pos)
		}
		pos++
	}
	return anyFailure
}

func stringOrEmpty(input interface{}) string {
	if str, ok := input.(string); ok {
		return str
	}
	return ""
}

func verifyLinkListener(which string, listener StringMap) bool {
	bind := fromString(stringOrEmpty(listener["bind"]))
	adv := fromString(stringOrEmpty(listener["advertise"]))
	if bind.port != adv.port {
		log.Warnf("%s ports differ. make sure this is intentionalog. bind port: %s, advertise port: %s", which, bind.port, adv.port)
	}
	return adv.testPort(which)
}

func verifyEdgeListener(which string, listener StringMap) bool {
	binding := stringOrEmpty(listener["binding"])
	if binding == common.EdgeBinding {
		address := stringOrEmpty(listener["address"])
		opts := listener.mapFromKey("options")
		advertise := stringOrEmpty(opts["advertise"])

		add := fromString(address)
		adv := fromString(advertise)
		if add.port != adv.port {
			log.Warnf("%s ports differ. make sure this is intentionalog. address port: %s, advertise port: %s", which, add.port, adv.port)
		}
		return adv.testPort(which)
	} else {
		// only verify "edge" for now
		log.Infof("%s has binding %s and doesn't need to be verified", which, binding)
	}
	return false
}
