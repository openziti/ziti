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

package enrollment

import (
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/identity/certtools"
	"github.com/openziti/foundation/util/term"
	"github.com/openziti/sdk-golang/ziti/config"
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
var keyAlg config.KeyAlgVar
var jwtpath, outpath, keyPath, certPath, idname, caOverride, username, password string

const verboseDesc = "Enable verbose logging."
const outpathDesc = "Output configuration file."
const jwtpathDesc = "Enrollment token (JWT file). Required"
const certDesc = "The certificate to present when establishing a connection."
const idnameDesc = "Names the identity. Ignored if not 3rd party auto enrollment"

const outFlag = "out"

func NewEnrollCommand() *cobra.Command {
	var enrollSubCmd = &cobra.Command{
		SilenceErrors: true,
		SilenceUsage:  false,
		Use:           "enroll path/to/jwt",
		Short:         "enroll an identity",
		Args:          cobra.MaximumNArgs(1),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if verbose {
				logrus.SetLevel(logrus.DebugLevel)
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			//set the formatter for enrolling via ziti-tunnel
			logrus.SetFormatter(&logrus.TextFormatter{
				ForceColors:      true,
				DisableTimestamp: true,
				TimestampFormat:  "",
				PadLevelText:     true,
			})
			logrus.SetReportCaller(false) // for enrolling don't bother with this
			if len(args) > 0 {
				jwtpath = args[0]
			}
			if jwtpath == "" {
				defer fmt.Printf("\nERROR: no jwt provided\n")
				return cmd.Help()
			}
			return processEnrollment()
		},
	}

	enrollSubCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, verboseDesc)
	enrollSubCmd.Flags().StringVarP(&jwtpath, "jwt", "j", "", jwtpathDesc)
	enrollSubCmd.Flags().StringVarP(&outpath, outFlag, "o", "", outpathDesc)
	enrollSubCmd.Flags().StringVarP(&idname, "idname", "n", "", idnameDesc)
	enrollSubCmd.Flags().StringVarP(&certPath, "cert", "c", "", certDesc)
	enrollSubCmd.Flags().StringVarP(&caOverride, "ca", "", "", "Additional trusted certificates")
	enrollSubCmd.Flags().StringVarP(&username, "username", "u", "", "Username for updb enrollment, prompted if not provided and necessary")
	enrollSubCmd.Flags().StringVarP(&password, "password", "p", "", "Password for updb enrollment, prompted if not provided and necessary")

	keyAlg.Set("RSA") // set default
	enrollSubCmd.Flags().VarP(&keyAlg, "keyAlg", "a", "Crypto algorithm to use when generating private key")

	var keyDesc = ""
	engines := certtools.ListEngines()
	if len(engines) > 0 {
		keyDesc = fmt.Sprintf("The key to use with the certificate. Optionally specify the engine to use. supported engines: %v", engines)
	} else {
		keyDesc = fmt.Sprintf("The key to use with the certificate.")
	}

	enrollSubCmd.Flags().StringVarP(&keyPath, "key", "k", "", keyDesc)
	return enrollSubCmd
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
		KeyAlg:        keyAlg,
		Token:         tkn,
		IDName:        idname,
		AdditionalCAs: caOverride,
		Username:      username,
		Password:      password,
	}

	if tkn.EnrollmentMethod == "updb" {
		if password == "" {
			password, err = term.PromptPassword("updb enrollment requires a password", false)
			if err != nil {
				return fmt.Errorf("failed to complete enrollment, updb requires a non-empty password")
			}
		}

		return enroll.EnrollUpdb(flags)
	}

	conf, err := enroll.Enroll(flags)
	if err != nil {
		return fmt.Errorf("failed to enroll: %v", err)
	}

	output, err := os.Create(outpath)
	if err != nil {
		return fmt.Errorf("failed to open file '%s': %s", outpath, err.Error())
	}
	defer func() { _ = output.Close() }()

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
