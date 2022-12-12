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

package cmd

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"io"
	"io/ioutil"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/spf13/viper"
)

var viperLock sync.Mutex

// PKICreateOptions the options for the create spring command
type PKICreateOptions struct {
	PKIOptions
}

// NewCmdPKICreate creates a command object for the "create" command
func NewCmdPKICreate(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &PKICreateOptions{
		PKIOptions: PKIOptions{
			CommonOptions: CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use: "create",
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.AddCommand(NewCmdPKICreateCA(out, errOut))
	cmd.AddCommand(NewCmdPKICreateIntermediate(out, errOut))
	cmd.AddCommand(NewCmdPKICreateKey(out, errOut))
	cmd.AddCommand(NewCmdPKICreateServer(out, errOut))
	cmd.AddCommand(NewCmdPKICreateClient(out, errOut))
	cmd.AddCommand(NewCmdPKICreateCSR(out, errOut))

	options.addPKICreateFlags(cmd)
	return cmd
}

func (options *PKICreateOptions) addPKICreateFlags(cmd *cobra.Command) {
	viperLock.Lock()
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
	viperLock.Unlock()
}

// Run implements this command
func (o *PKICreateOptions) Run() error {
	return o.Cmd.Help()
}

// ObtainPKIRoot returns the value for pki-root
func (o *PKICreateOptions) ObtainPKIRoot() (string, error) {
	pkiroot := o.Flags.PKIRoot
	if pkiroot == "" {
		pkiroot = viper.GetString("pki-root")
		if pkiroot == "" {
			pkiRootDir, err := util.PKIRootDir()
			if err != nil {
				return "", err
			}
			pkiroot, err = util.PickValue("Required flag 'pki-root' not specified; Enter PKI Root now:", pkiRootDir, true)
			if err != nil {
				return "", err
			}
		}
	}
	return pkiroot, nil
}

// ObtainCAFile returns the value for ca-file
func (o *PKICreateOptions) ObtainCAFile() (string, error) {
	cafile := o.Flags.CAFile
	if cafile == "" {
		cafile = viper.GetString("ca-file")
		if cafile == "" {
			var err error
			cafile, err = util.PickValue("Required flag 'ca-file' not specified; Enter CA name now:", "ca", true)
			if err != nil {
				return "", err
			}
		}
	}
	return cafile, nil
}

// ObtainIntermediateCAFile returns the value for intermediate-file
func (o *PKICreateOptions) ObtainIntermediateCAFile() (string, error) {
	intermediatefile := o.Flags.IntermediateFile
	if intermediatefile == "" {
		intermediatefile = viper.GetString("intermediate-file")
		if intermediatefile == "" {
			var err error
			intermediatefile, err = util.PickValue("Required flag 'intermediate-file' not specified; Enter Intermediate CA name now:", "intermediate", true)
			if err != nil {
				return "", err
			}
		}
	}
	return intermediatefile, nil
}

// ObtainIntermediateCSRFile returns the value for intermediate-file
func (o *PKICreateOptions) ObtainIntermediateCSRFile() (string, error) {
	intermediatecsrfile := viper.GetString("intermediate-csr-file")
	if intermediatecsrfile == "" {
		var err error
		intermediatecsrfile, err = util.PickValue("Required flag 'intermediate--csr-file' not specified; Enter Intermediate CSR file name now:", "intermediate-csr", true)
		if err != nil {
			return "", err
		}
	}
	return intermediatecsrfile, nil
}

// ObtainCSRFile returns the value for csr-file
func (o *PKICreateOptions) ObtainCSRFile() (string, error) {
	csrfile := viper.GetString("csr-file")
	if csrfile == "" {
		var err error
		csrfile, err = util.PickValue("Required flag 'csr-file' not specified; Enter CSR name now:", "csr", true)
		if err != nil {
			return "", err
		}
	}
	return csrfile, nil
}

// ObtainServerCertFile returns the value for server-file
func (o *PKICreateOptions) ObtainServerCertFile() (string, error) {
	serverfile := o.Flags.ServerFile
	if serverfile == "" {
		serverfile = viper.GetString("server-file")
		if serverfile == "" {
			var err error
			serverfile, err = util.PickValue("Required flag 'server-file' not specified; Enter name now:", "server", true)
			if err != nil {
				return "", err
			}
		}
	}
	return serverfile, nil
}

// ObtainClientCertFile returns the value for client-file
func (o *PKICreateOptions) ObtainClientCertFile() (string, error) {
	clientfile := o.Flags.ClientFile
	if clientfile == "" {
		clientfile = viper.GetString("client-file")
		if clientfile == "" {
			var err error
			clientfile, err = util.PickValue("Required flag 'client-file' not specified; Enter name now:", "client", true)
			if err != nil {
				return "", err
			}
		}
	}
	return clientfile, nil
}

// ObtainKeyFile returns the value for key-file
func (o *PKICreateOptions) ObtainKeyFile(required bool) (string, error) {
	keyfile := o.Flags.KeyFile
	if keyfile == "" {
		keyfile = viper.GetString("key-file")
		if keyfile == "" {
			if required {
				var err error
				keyfile, err = util.PickValue("Required flag 'key-file' not specified; Enter name now:", "key", true)
				if err != nil {
					return "", err
				}
			}
		}
	}
	return keyfile, nil
}

// ObtainCAName returns the value for ca-name
func (o *PKICreateOptions) ObtainCAName(pkiroot string) (string, error) {
	caname := o.Flags.CAName
	if caname == "" {
		caname = viper.GetString("ca-name")
		if caname == "" {
			var err error
			files, err := ioutil.ReadDir(pkiroot)
			if err != nil {
				return "", err
			}
			names := make([]string, 0)
			for _, f := range files {
				if f.IsDir() {
					if f.Name() != "ca" {
						names = append(names, f.Name())
					}
				}
			}
			caname, err = util.PickName(names, "Required flag 'ca-name' not specified; choose from below (dirs seen in your ZITI_PKI_ROOT):")
			if err != nil {
				return "", err
			}
		}
	}
	fmt.Println("Using CA name: ", caname)
	return caname, nil
}

// ObtainCommonName returns the value for CN
func (o *PKICreateOptions) ObtainCommonName() (string, error) {
	var commonName string
	if o.Flags.CommonName == "" {
		commonName = strings.Join(o.Args, " ")
	}
	if commonName == "" {
		var err error
		commonName, err = util.PickValue("CN not specified; Enter CN now:", "", true)
		if err != nil {
			return "", err
		}
	}
	return commonName, nil
}

// ObtainFileName returns the value for the 'name' used in the PKI request
func (o *PKICreateOptions) ObtainFileName(cafile string, commonName string) string {
	var filename string
	if filename = cafile; len(cafile) == 0 {
		filename = strings.Replace(commonName, " ", "_", -1)
		filename = strings.Replace(filename, "*", "wildcard", -1)
	}
	return filename
}

// ObtainPKIRequestTemplate returns the 'template' used in the PKI request
func (o *PKICreateOptions) ObtainPKIRequestTemplate(commonName string) *x509.Certificate {

	subject := pkix.Name{CommonName: commonName}
	if str := viper.GetString("pki-organization"); str != "" {
		subject.Organization = []string{str}
	}
	if str := viper.GetString("pki-locality"); str != "" {
		subject.Locality = []string{str}
	}
	if str := viper.GetString("pki-country"); str != "" {
		subject.Country = []string{str}
	}
	if str := viper.GetString("pki-state"); str != "" {
		subject.Province = []string{str}
	}
	if str := viper.GetString("pki-organizational-unit"); str != "" {
		subject.OrganizationalUnit = []string{str}
	}

	template := &x509.Certificate{
		Subject:    subject,
		NotAfter:   time.Now().AddDate(0, 0, o.Flags.CAExpire),
		MaxPathLen: o.Flags.CAMaxpath,
	}

	return template
}

// ObtainKeyName returns the private key from the key-file
func (o *PKICreateOptions) ObtainKeyName(pkiroot string) (string, error) {
	keyname := viper.GetString("key-name")
	if keyname == "" {
		var err error
		files, err := ioutil.ReadDir(pkiroot)
		if err != nil {
			return "", err
		}
		names := make([]string, 0)
		for _, f := range files {
			if f.IsDir() {
				names = append(names, f.Name())
			}
		}
		keyname, err = util.PickName(names, "Required flag 'key-name' not specified; choose from below (dirs seen in your ZITI_PKI_ROOT):")
		if err != nil {
			return "", err
		}
	}

	return keyname, nil
}

// ObtainPKICSRRequestTemplate returns the CSR 'template' used in the PKI request
func (o *PKICreateOptions) ObtainPKICSRRequestTemplate(commonName string) *x509.CertificateRequest {

	subject := pkix.Name{CommonName: commonName}
	if str := viper.GetString("pki-organization"); str != "" {
		subject.Organization = []string{str}
	}
	if str := viper.GetString("pki-locality"); str != "" {
		subject.Locality = []string{str}
	}
	if str := viper.GetString("pki-country"); str != "" {
		subject.Country = []string{str}
	}
	if str := viper.GetString("pki-state"); str != "" {
		subject.Province = []string{str}
	}
	if str := viper.GetString("pki-organizational-unit"); str != "" {
		subject.OrganizationalUnit = []string{str}
	}

	type basicConstraints struct {
		IsCA       bool `asn1:"optional"`
		MaxPathLen int  `asn1:"optional,default:-1"`
	}

	val, _ := asn1.Marshal(basicConstraints{true, 0})

	csrTemplate := &x509.CertificateRequest{
		Subject:            subject,
		SignatureAlgorithm: x509.SHA512WithRSA,
		ExtraExtensions: []pkix.Extension{
			{
				Id:       asn1.ObjectIdentifier{2, 5, 29, 19},
				Value:    val,
				Critical: true,
			},
		},
	}

	return csrTemplate
}

// ObtainIPsAndDNSNames returns the IP addrs and/or DNS names used in the PKI request template
func (o *PKICreateOptions) ObtainIPsAndDNSNames() ([]net.IP, []string, error) {

	if (len(o.Flags.IP) == 0) && (len(o.Flags.DNSName) == 0) {
		return nil, nil, errors.New("neither --ip or --dns were specified (either one, or both, must be specified)")
	}

	IPs := make([]net.IP, 0, len(o.Flags.IP))
	for _, ipStr := range o.Flags.IP {
		if i := net.ParseIP(ipStr); i != nil {
			IPs = append(IPs, i)
		}
	}

	return IPs, o.Flags.DNSName, nil
}
