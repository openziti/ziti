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

package cmd

import (
	"github.com/spf13/cobra"
	"io"

	cmdutil "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/helpers"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/templates"
	"github.com/netfoundry/ziti-cmd/ziti/pki/pki"
	"github.com/spf13/viper"
)

// PKIOptions contains the command line options
type PKIOptions struct {
	CommonOptions

	Flags PKIFlags
}

type PKIFlags struct {
	PKIRoot               string
	PKIOrganization       string
	PKIOrganizationalUnit string
	PKICountry            string
	PKILocality           string
	PKIProvince           string
	CAFile                string
	CAName                string
	CommonName            string
	CAExpire              int
	CAMaxpath             int
	CAPrivateKeySize      int
	IntermediateFile      string
	IntermediateName      string
	ServerFile            string
	ServerName            string
	ClientFile            string
	ClientName            string
	KeyFile               string
	CSRFile               string
	CSRName               string
	KeyName               string
	DNSName               []string
	IP                    []string
	Email                 []string
	PKI                   *pki.ZitiPKI
}

var (
	pkiLong = templates.LongDesc(`
Provide the components needed to manage a Ziti PKI.
	`)
)

// NewCmdPKI PKIs a command object for the "PKI" command
func NewCmdPKI(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &PKIOptions{
		CommonOptions: CommonOptions{
			Factory: f,
			Out:     out,
			Err:     errOut,
		},
	}

	cmd := &cobra.Command{
		Use:   "pki",
		Short: "Manage a Ziti PKI",
		Long:  pkiLong,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.AddCommand(NewCmdPKICreate(f, out, errOut))
	// cmd.AddCommand(NewCmdPKIRevoke(f, out, errOut)) // coming soon :)

	options.addPKIFlags(cmd)

	return cmd
}

func (options *PKIOptions) addPKIFlags(cmd *cobra.Command) {

	cmd.PersistentFlags().StringVarP(&options.Flags.PKIRoot, "pki-root", "", "", "Directory in which to store CA")
	cmd.MarkFlagRequired("pki-root")
	viper.BindPFlag("pki_root", cmd.PersistentFlags().Lookup("pki-root"))

	cmd.PersistentFlags().StringVarP(&options.Flags.PKIOrganization, "pki-organization", "", "NetFoundry", "Organization")
	cmd.MarkFlagRequired("pki-organization")
	viper.BindPFlag("pki-organization", cmd.PersistentFlags().Lookup("pki-organization"))

	cmd.PersistentFlags().StringVarP(&options.Flags.PKIOrganizationalUnit, "pki-organizational-unit", "", "ADV-DEV", "Organization unit")
	cmd.MarkFlagRequired("pki-organizational-unit")
	viper.BindPFlag("pki-organizational-unit", cmd.PersistentFlags().Lookup("pki-organizational-unit"))

	cmd.PersistentFlags().StringVarP(&options.Flags.PKICountry, "pki-country", "", "US", "Country")
	cmd.MarkFlagRequired("pki-country")
	viper.BindPFlag("pki-country", cmd.PersistentFlags().Lookup("pki-country"))

	cmd.PersistentFlags().StringVarP(&options.Flags.PKILocality, "pki-locality", "", "Charlotte", "Locality/Location")
	cmd.MarkFlagRequired("pki-locality")
	viper.BindPFlag("pki-locality", cmd.PersistentFlags().Lookup("pki-locality"))

	// cmd.PersistentFlags().StringVarP(&options.Flags.PKILocality, "pki-location", "", "Charlotte", "Location/Locality")
	// cmd.MarkFlagRequired("pki-location")
	// viper.BindPFlag("pki-location", cmd.PersistentFlags().Lookup("pki-location"))

	cmd.PersistentFlags().StringVarP(&options.Flags.PKIProvince, "pki-province", "", "NC", "Province/State")
	cmd.MarkFlagRequired("pki-province")
	viper.BindPFlag("pki-province", cmd.PersistentFlags().Lookup("pki-province"))

	// cmd.PersistentFlags().StringVarP(&options.Flags.PKIProvince, "pki-state", "", "NC", "State/Province")
	// cmd.MarkFlagRequired("pki-state")
	// viper.BindPFlag("pki-state", cmd.PersistentFlags().Lookup("pki-state"))

}

// Run implements this command
func (o *PKIOptions) Run() error {
	return o.Cmd.Help()

}
