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

package enroll

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/router/enroll"
	"github.com/openziti/ziti/router/env"
	"github.com/spf13/cobra"
	"os"
)

type enrollEdgeRouterAction struct {
	jwtPath string
	engine  string
	keyAlg  ziti.KeyAlgVar
}

func NewEnrollEdgeRouterCmd() *cobra.Command {
	action := &enrollEdgeRouterAction{}
	var enrollEdgeRouterCmd = &cobra.Command{
		Use:   "enroll <config>",
		Short: "Enroll a router as an edge router",
		Args:  cobra.ExactArgs(1),
		Run:   action.enrollEdgeRouter,
	}

	enrollEdgeRouterCmd.Flags().StringVarP(&action.jwtPath, "jwt", "j", "", "The path to a JWT file")
	enrollEdgeRouterCmd.Flags().StringVarP(&action.engine, "engine", "e", "", "An engine")
	if err := action.keyAlg.Set("RSA"); err != nil { // set default
		panic(err)
	}
	enrollEdgeRouterCmd.Flags().VarP(&action.keyAlg, "keyAlg", "a", "Crypto algorithm to use when generating private key")

	return enrollEdgeRouterCmd
}

func (self *enrollEdgeRouterAction) enrollEdgeRouter(cmd *cobra.Command, args []string) {
	log := pfxlog.Logger()
	if cfg, err := env.LoadConfigWithOptions(args[0], false); err == nil {
		enroller := enroll.NewRestEnroller(cfg)

		jwtBuf, err := os.ReadFile(self.jwtPath)
		if err != nil {
			log.Panicf("could not load JWT file from path [%s]", self.jwtPath)
		}

		if err := enroller.Enroll(jwtBuf, true, self.engine, self.keyAlg); err != nil {
			log.Fatalf("enrollment failure: (%v)", err)
		}
	} else {
		panic(err)
	}
}
