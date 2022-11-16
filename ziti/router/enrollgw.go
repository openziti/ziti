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

package router

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/router/enroll"
	"github.com/openziti/fabric/router"
	"github.com/openziti/sdk-golang/ziti/config"
	"github.com/spf13/cobra"
	"io/ioutil"
)

var jwtPath *string
var engine *string
var keyAlg config.KeyAlgVar

func NewEnrollGwCmd() *cobra.Command {
	var enrollEdgeRouterCmd = &cobra.Command{
		Use:   "enroll <config>",
		Short: "Enroll a router as an edge router",
		Args:  cobra.ExactArgs(1),
		Run:   enrollGw,
	}

	jwtPath = enrollEdgeRouterCmd.Flags().StringP("jwt", "j", "", "The path to a JWT file")
	engine = enrollEdgeRouterCmd.Flags().StringP("engine", "e", "", "An engine")
	keyAlg.Set("RSA") // set default
	enrollEdgeRouterCmd.Flags().VarP(&keyAlg, "keyAlg", "a", "Crypto algorithm to use when generating private key")

	return enrollEdgeRouterCmd
}

func enrollGw(cmd *cobra.Command, args []string) {
	log := pfxlog.Logger()
	if cfgmap, err := router.LoadConfigMap(args[0]); err == nil {
		router.SetConfigMapFlags(cfgmap, getFlags(cmd))

		enroller := enroll.NewRestEnroller()
		err := enroller.LoadConfig(cfgmap)

		if err != nil {
			log.Panicf("could not load config: %s", err)
		}

		jwtBuf, err := ioutil.ReadFile(*jwtPath)
		if err != nil {
			log.Panicf("could not load JWT file from path [%s]", *jwtPath)
		}

		if err := enroller.Enroll(jwtBuf, true, *engine, keyAlg); err != nil {
			log.Fatalf("enrollment failure: (%v)", err)
		}
	} else {
		panic(err)
	}
}
