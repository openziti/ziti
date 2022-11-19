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

package enrollment

import (
	"encoding/json"
	"fmt"
	"github.com/openziti/ziti/ziti/cmd/common"
	"io/ioutil"
	"os"
	"strings"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/term"
	"github.com/openziti/identity/certtools"
	"github.com/openziti/sdk-golang/ziti/config"
	"github.com/openziti/sdk-golang/ziti/enroll"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// global state used by all subcommands are located here for easy discovery

const verboseDesc = "Enable verbose logging."
const outpathDesc = "Output configuration file."
const jwtpathDesc = "Enrollment token (JWT file). Required"
const certDesc = "The certificate to present when establishing a connection."
const idnameDesc = "Names the identity. Ignored if not 3rd party auto enrollment"

const outFlag = "out"

// EnrollOptions contains the command line options
type EnrollOptions struct {
	common.CommonOptions
	RemoveJwt  bool
	KeyAlg     config.KeyAlgVar
	JwtPath    string
	OutputPath string
	KeyPath    string
	CertPath   string
	IdName     string
	CaOverride string
	Username   string
	Password   string
}

type EnrollAction struct {
	EnrollOptions
}

func NewEnrollCommand(p common.OptionsProvider) *cobra.Command {
	action := &EnrollAction{
		EnrollOptions: EnrollOptions{
			CommonOptions: p(),
		},
	}
	var enrollSubCmd = &cobra.Command{
		SilenceErrors: true,
		SilenceUsage:  false,
		Use:           "enroll path/to/jwt",
		Short:         "enroll an identity",
		Args:          cobra.MaximumNArgs(1),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if action.Verbose {
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
				action.JwtPath = args[0]
			}
			if action.JwtPath == "" {
				defer fmt.Printf("\nERROR: no jwt provided\n")
				return cmd.Help()
			}
			action.Cmd = cmd
			action.Args = args
			return action.Run()
		},
	}

	//enrollSubCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, verboseDesc)
	enrollSubCmd.Flags().StringVarP(&action.JwtPath, "jwt", "j", "", jwtpathDesc)
	enrollSubCmd.Flags().StringVarP(&action.OutputPath, outFlag, "o", "", outpathDesc)
	enrollSubCmd.Flags().StringVarP(&action.IdName, "idname", "n", "", idnameDesc)
	enrollSubCmd.Flags().StringVarP(&action.CertPath, "cert", "c", "", certDesc)
	enrollSubCmd.Flags().StringVarP(&action.CaOverride, "ca", "", "", "Additional trusted certificates")
	enrollSubCmd.Flags().StringVarP(&action.Username, "username", "u", "", "Username for updb enrollment, prompted if not provided and necessary")
	enrollSubCmd.Flags().StringVarP(&action.Password, "password", "p", "", "Password for updb enrollment, prompted if not provided and necessary")
	enrollSubCmd.Flags().BoolVar(&action.RemoveJwt, "rm", false, "Remove the JWT on success")
	enrollSubCmd.Flags().BoolVarP(&action.Verbose, "verbose", "v", false, "Enable verbose logging")

	action.KeyAlg.Set("RSA") // set default
	enrollSubCmd.Flags().VarP(&action.KeyAlg, "keyAlg", "a", "Crypto algorithm to use when generating private key")

	var keyDesc = ""
	engines := certtools.ListEngines()
	if len(engines) > 0 {
		keyDesc = fmt.Sprintf("The key to use with the certificate. Optionally specify the engine to use. supported engines: %v", engines)
	} else {
		keyDesc = "The key to use with the certificate."
	}

	enrollSubCmd.Flags().StringVarP(&action.KeyPath, "key", "k", "", keyDesc)
	return enrollSubCmd
}

func (e *EnrollAction) Run() error {
	if strings.TrimSpace(e.OutputPath) == "" {
		out, outErr := outPathFromJwt(e.JwtPath)
		if outErr != nil {
			return fmt.Errorf("could not set the output path: %s", outErr)
		}
		e.OutputPath = out
	}

	if e.JwtPath != "" {
		if _, err := os.Stat(e.JwtPath); os.IsNotExist(err) {
			return fmt.Errorf("the provided jwt file does not exist: %s", e.JwtPath)
		}
	}

	if e.CaOverride != "" {
		if _, err := os.Stat(e.CaOverride); os.IsNotExist(err) {
			return fmt.Errorf("the provided ca file does not exist: %s", e.CaOverride)
		}
	}

	if strings.TrimSpace(e.OutputPath) == strings.TrimSpace(e.JwtPath) {
		return fmt.Errorf("the output path must not be the same as the jwt path")
	}

	tokenStr, _ := ioutil.ReadFile(e.JwtPath)

	pfxlog.Logger().Debugf("jwt to parse: %s", tokenStr)
	tkn, _, err := enroll.ParseToken(string(tokenStr))

	if err != nil {
		return fmt.Errorf("failed to parse JWT: %s", err.Error())
	}

	flags := enroll.EnrollmentFlags{
		CertFile:      e.CertPath,
		KeyFile:       e.KeyPath,
		KeyAlg:        e.KeyAlg,
		Token:         tkn,
		IDName:        e.IdName,
		AdditionalCAs: e.CaOverride,
		Username:      e.Username,
		Password:      e.Password,
		Verbose:       e.Verbose,
	}

	if tkn.EnrollmentMethod == "updb" {
		if e.Password == "" {
			e.Password, err = term.PromptPassword("updb enrollment requires a password, please enter one: ", false)
			e.Password = strings.TrimSpace(e.Password)

			if err != nil {
				return fmt.Errorf("failed to complete enrollment, updb requires a non-empty password: %v", err)
			}

			confirm, err := term.PromptPassword("please confirm what you entered: ", false)

			if err != nil {
				return fmt.Errorf("failed to complete enrollment, updb password confirmation failed: %v", err)
			}

			confirm = strings.TrimSpace(confirm)

			if e.Password != confirm {
				return fmt.Errorf("failed to complete enrollment, passwords did not match")
			}

			flags.Password = e.Password
		}

		err = enroll.EnrollUpdb(flags)
		if err == nil {
			if rmErr := os.Remove(e.JwtPath); rmErr != nil {
				pfxlog.Logger().WithError(rmErr).Warnf("unable to remove JWT file as requested: %v", e.JwtPath)
			}
		}
		return err
	}

	conf, err := enroll.Enroll(flags)
	if err != nil {
		return fmt.Errorf("failed to enroll: %v", err)
	}

	output, err := os.Create(e.OutputPath)
	if err != nil {
		return fmt.Errorf("failed to open file '%s': %s", e.OutputPath, err.Error())
	}
	defer func() { _ = output.Close() }()

	enc := json.NewEncoder(output)
	enc.SetEscapeHTML(false)
	encErr := enc.Encode(&conf)

	if err = os.Remove(e.JwtPath); err != nil {
		pfxlog.Logger().WithError(err).Warnf("unable to remove JWT file as requested: %v", e.JwtPath)
	}

	if encErr == nil {
		pfxlog.Logger().Infof("enrolled successfully. identity file written to: %s", e.OutputPath)
		return nil
	} else {
		return fmt.Errorf("enrollment successful but the identity file was not able to be written to: %s [%s]", e.OutputPath, encErr)
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
