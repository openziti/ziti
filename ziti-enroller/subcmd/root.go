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

package subcmd

import (
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/identity/certtools"
	"github.com/openziti/sdk-golang/ziti/enroll"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io/ioutil"
	"os"
	"strings"
)

// global state used by all subcommands are located here for easy discovery
var verbose bool
var jwtpath, outpath, keyPath, certPath, idname, caOverride string

const verboseDesc = "Enable verbose logging."
const outpathDesc = "Output configuration file."
const jwtpathDesc = "Enrollment token (JWT file). Required"
const certDesc = "The certificate to present when establishing a connection."
const idnameDesc = "Names the identity. Ignored if not 3rd party auto enrollment"

const outFlag = "out"

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, verboseDesc)
	rootCmd.Flags().StringVarP(&jwtpath, "jwt", "j", "", jwtpathDesc)
	rootCmd.Flags().StringVarP(&outpath, outFlag, "o", "", outpathDesc)
	rootCmd.Flags().StringVarP(&idname, "idname", "n", "", idnameDesc)
	rootCmd.Flags().StringVarP(&certPath, "cert", "c", "", certDesc)
	rootCmd.Flags().StringVarP(&caOverride, "ca", "", "", "Additional trusted certificates")

	var keyDesc = ""
	engines := certtools.ListEngines()
	if len(engines) > 0 {
		keyDesc = fmt.Sprintf("The key to use with the certificate. Optionally specify the engine to use. supported engines: %v", engines)
	} else {
		keyDesc = fmt.Sprintf("The key to use with the certificate.")
	}

	rootCmd.Flags().StringVarP(&keyPath, "key", "k", "", keyDesc)

	_ = rootCmd.MarkFlagRequired("jwt")
}

var rootCmd = &cobra.Command{
	Use:   "ziti-enroller",
	Short: "Ziti Enroller",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logrus.Warnf("ziti-enroller is DEPRECATED and will soon be removed. Please use 'ziti edge enroll' or 'ziti-tunnel enroll' instead.")
		if verbose {
			logrus.SetLevel(logrus.DebugLevel)
		}
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return processEnrollment()
	},
}

func Execute() {
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	if err := rootCmd.Execute(); err != nil {
		pfxlog.Logger().Errorf("%s\n", err)
	}
}

func processEnrollment() error {
	if strings.TrimSpace(outpath) == "" {
		out, outErr := outPathFromJwt(jwtpath)
		if outErr != nil {
			return fmt.Errorf("could not set the output path: %s", outErr)
		}
		outpath = out
	}

	if jwtpath != "" {
		if _, err := os.Stat(jwtpath); os.IsNotExist(err) {
			return fmt.Errorf("the provided jwt file does not exist: %s", jwtpath)
		}
	}

	if caOverride != "" {
		if _, err := os.Stat(caOverride); os.IsNotExist(err) {
			return fmt.Errorf("the provided ca file does not exist: %s", caOverride)
		}
	}

	if strings.TrimSpace(outpath) == strings.TrimSpace(jwtpath) {
		return fmt.Errorf("the output path must not be the same as the jwt path")
	}

	tokenStr, _ := ioutil.ReadFile(jwtpath)

	pfxlog.Logger().Debugf("jwt to parse: %s", tokenStr)
	tkn, _, err := enroll.ParseToken(string(tokenStr))

	if err != nil {
		return fmt.Errorf("failed to parse JWT: %s", err.Error())
	}

	flags := enroll.EnrollmentFlags{
		CertFile:      certPath,
		KeyFile:       keyPath,
		Token:         tkn,
		IDName:        idname,
		AdditionalCAs: caOverride,
		KeyAlg:        "RSA",
	}
	conf, err := enroll.Enroll(flags)
	if err != nil {
		return fmt.Errorf("failed to enroll: %v", err)
	}

	output, err := os.Create(outpath)
	defer output.Close()
	if err != nil {
		return fmt.Errorf("failed to open file '%s': %s", outpath, err.Error())
	}

	enc := json.NewEncoder(output)
	enc.SetEscapeHTML(false)
	encErr := enc.Encode(&conf)

	if encErr == nil {
		pfxlog.Logger().Infof("enrolled successfully. identity file written to: %s", outpath)
		return nil
	} else {
		return fmt.Errorf("enrollment successful but the identity file was not able to be written to: %s [%s]", outpath, encErr)
	}
}

func outPathFromJwt(jwt string) (string, error) {
	if strings.HasSuffix(jwt, ".jwt") {
		return jwt[:len(jwt)-len(".jwt")] + ".json", nil
	} else if strings.HasSuffix(jwt, ".json") {
		//ugh - so that makes things a bit uglier but ok fine. we'll return an error in this situation
		return "", errors.Errorf("unexpected configuration. cannot infer '%s' flag if the jwt file "+
			"ends in .json. rename jwt file or provide the '%s' flag", outFlag, outFlag)
	} else {
		//doesn't end with .jwt - so just slap a .json on the end and call it a day
		return jwt + ".json", nil
	}
}
