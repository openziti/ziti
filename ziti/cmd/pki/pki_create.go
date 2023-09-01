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

package pki

import (
	"crypto/elliptic"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	cmd2 "github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/pki/pki"
	"github.com/openziti/ziti/ziti/util"
	"io"
	"net"
	"os"
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
			CommonOptions: cmd2.CommonOptions{
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
	defer viperLock.Unlock()

	cmd.PersistentFlags().StringVarP(&options.Flags.PKIRoot, "pki-root", "", "", "Directory in which to store CA")
	err := viper.BindPFlag("pki_root", cmd.PersistentFlags().Lookup("pki-root"))
	options.panicOnErr(err)

	cmd.PersistentFlags().StringVarP(&options.Flags.PKIOrganization, "pki-organization", "", "NetFoundry", "Organization")
	err = viper.BindPFlag("pki-organization", cmd.PersistentFlags().Lookup("pki-organization"))
	options.panicOnErr(err)

	cmd.PersistentFlags().StringVarP(&options.Flags.PKIOrganizationalUnit, "pki-organizational-unit", "", "ADV-DEV", "Organization unit")
	err = viper.BindPFlag("pki-organizational-unit", cmd.PersistentFlags().Lookup("pki-organizational-unit"))
	options.panicOnErr(err)

	cmd.PersistentFlags().StringVarP(&options.Flags.PKICountry, "pki-country", "", "US", "Country")
	err = viper.BindPFlag("pki-country", cmd.PersistentFlags().Lookup("pki-country"))
	options.panicOnErr(err)

	cmd.PersistentFlags().StringVarP(&options.Flags.PKILocality, "pki-locality", "", "Charlotte", "Locality/Location")
	err = viper.BindPFlag("pki-locality", cmd.PersistentFlags().Lookup("pki-locality"))
	options.panicOnErr(err)

	// cmd.PersistentFlags().StringVarP(&options.Flags.PKILocality, "pki-location", "", "Charlotte", "Location/Locality")
	// cmd.MarkFlagRequired("pki-location")
	// viper.BindPFlag("pki-location", cmd.PersistentFlags().Lookup("pki-location"))

	cmd.PersistentFlags().StringVarP(&options.Flags.PKIProvince, "pki-province", "", "NC", "Province/State")
	err = viper.BindPFlag("pki-province", cmd.PersistentFlags().Lookup("pki-province"))
	options.panicOnErr(err)

	// cmd.PersistentFlags().StringVarP(&options.Flags.PKIProvince, "pki-state", "", "NC", "State/Province")
	// cmd.MarkFlagRequired("pki-state")
	// viper.BindPFlag("pki-state", cmd.PersistentFlags().Lookup("pki-state"))
}

// Run implements this command
func (o *PKICreateOptions) Run() error {
	return o.Cmd.Help()
}

// ObtainPKIRoot returns the value for pki-root
func (o *PKICreateOptions) ObtainPKIRoot() (string, error) {
	pkiRoot := o.Flags.PKIRoot
	if pkiRoot == "" {
		pkiRoot = viper.GetString("pki-root")
		if pkiRoot == "" {
			pkiRootDir, err := util.PKIRootDir()
			if err != nil {
				return "", err
			}
			pkiRoot, err = util.PickValue("Required flag 'pki-root' not specified; Enter PKI Root now:", pkiRootDir, true)
			if err != nil {
				return "", err
			}
		}
	}
	return pkiRoot, nil
}

// ObtainCAFile returns the value for ca-file
func (o *PKICreateOptions) ObtainCAFile() (string, error) {
	caFile := o.Flags.CAFile
	if caFile == "" {
		caFile = viper.GetString("ca-file")
		if caFile == "" {
			var err error
			caFile, err = util.PickValue("Required flag 'ca-file' not specified; Enter CA name now:", "ca", true)
			if err != nil {
				return "", err
			}
		}
	}
	return caFile, nil
}

// ObtainIntermediateCAFile returns the value for intermediate-file
func (o *PKICreateOptions) ObtainIntermediateCAFile() (string, error) {
	intermediateFile := o.Flags.IntermediateFile
	if intermediateFile == "" {
		intermediateFile = viper.GetString("intermediate-file")
		if intermediateFile == "" {
			var err error
			intermediateFile, err = util.PickValue("Required flag 'intermediate-file' not specified; Enter Intermediate CA name now:", "intermediate", true)
			if err != nil {
				return "", err
			}
		}
	}
	return intermediateFile, nil
}

// ObtainIntermediateCSRFile returns the value for intermediate-file
func (o *PKICreateOptions) ObtainIntermediateCSRFile() (string, error) {
	intermediateCsrFile := viper.GetString("intermediate-csr-file")
	if intermediateCsrFile == "" {
		var err error
		intermediateCsrFile, err = util.PickValue("Required flag 'intermediate--csr-file' not specified; Enter Intermediate CSR file name now:", "intermediate-csr", true)
		if err != nil {
			return "", err
		}
	}
	return intermediateCsrFile, nil
}

// ObtainCSRFile returns the value for csr-file
func (o *PKICreateOptions) ObtainCSRFile() (string, error) {
	csrFile := viper.GetString("csr-file")
	if csrFile == "" {
		var err error
		csrFile, err = util.PickValue("Required flag 'csr-file' not specified; Enter CSR name now:", "csr", true)
		if err != nil {
			return "", err
		}
	}
	return csrFile, nil
}

// ObtainServerCertFile returns the value for server-file
func (o *PKICreateOptions) ObtainServerCertFile() (string, error) {
	serverFile := o.Flags.ServerFile
	if serverFile == "" {
		serverFile = viper.GetString("server-file")
		if serverFile == "" {
			var err error
			serverFile, err = util.PickValue("Required flag 'server-file' not specified; Enter name now:", "server", true)
			if err != nil {
				return "", err
			}
		}
	}
	return serverFile, nil
}

// ObtainClientCertFile returns the value for client-file
func (o *PKICreateOptions) ObtainClientCertFile() (string, error) {
	clientFile := o.Flags.ClientFile
	if clientFile == "" {
		clientFile = viper.GetString("client-file")
		if clientFile == "" {
			var err error
			clientFile, err = util.PickValue("Required flag 'client-file' not specified; Enter name now:", "client", true)
			if err != nil {
				return "", err
			}
		}
	}
	return clientFile, nil
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
func (o *PKICreateOptions) ObtainCAName(pkiRoot string) (string, error) {
	caName := o.Flags.CAName
	if caName == "" {
		caName = viper.GetString("ca-name")
		if caName == "" {
			var err error
			files, err := os.ReadDir(pkiRoot)
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
			caName, err = util.PickName(names, "Required flag 'ca-name' not specified; choose from below (dirs seen in your ZITI_PKI_ROOT):")
			if err != nil {
				return "", err
			}
		}
	}
	fmt.Println("Using CA name: ", caName)
	return caName, nil
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
func (o *PKICreateOptions) ObtainFileName(caFile string, commonName string) string {
	var filename string
	if filename = caFile; len(caFile) == 0 {
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
		MaxPathLen: o.Flags.CAMaxPath,
	}

	return template
}

// ObtainKeyName returns the private key from the key-file
func (o *PKICreateOptions) ObtainKeyName(pkiRoot string) (string, error) {
	keyName := viper.GetString("key-name")
	if keyName == "" {
		var err error
		files, err := os.ReadDir(pkiRoot)
		if err != nil {
			return "", err
		}
		names := make([]string, 0)
		for _, f := range files {
			if f.IsDir() {
				names = append(names, f.Name())
			}
		}
		keyName, err = util.PickName(names, "Required flag 'key-name' not specified; choose from below (dirs seen in your ZITI_PKI_ROOT):")
		if err != nil {
			return "", err
		}
	}

	return keyName, nil
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
		Subject: subject,
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

// ObtainIPsAndDNSNames returns the IP address and/or DNS names used in the PKI request template
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

// ObtainPrivateKeyOptions returns the private key options necessary to generate a private key
func (o *PKICreateOptions) ObtainPrivateKeyOptions() (pki.PrivateKeyOptions, error) {
	isEc := o.Flags.EcCurve != ""

	if isEc {
		return o.obtainEcOptions()
	}

	return o.obtainRsaOptions()
}

func (o *PKICreateOptions) obtainEcOptions() (pki.PrivateKeyOptions, error) {
	target := strings.Replace(strings.ToLower(o.Flags.EcCurve), "-", "", -1)

	validCurves := []elliptic.Curve{
		elliptic.P224(),
		elliptic.P256(),
		elliptic.P384(),
		elliptic.P521(),
	}
	for _, validCurve := range validCurves {
		curCurveName := strings.Replace(strings.ToLower(validCurve.Params().Name), "-", "", -1)

		if curCurveName == target {
			return &pki.EcPrivateKeyOptions{
				Curve: validCurve,
			}, nil
		}
	}

	validCurveString := ""

	for _, validCurve := range validCurves {
		if validCurveString != "" {
			validCurveString = validCurveString + ", "
		}

		validCurveString = validCurveString + strings.Replace(validCurve.Params().Name, "-", "", -1)
	}
	return nil, fmt.Errorf("unknown curve name '%s', valid curves (case and dash insensitive) %s", o.Flags.EcCurve, validCurveString)
}

func (o *PKICreateOptions) obtainRsaOptions() (pki.PrivateKeyOptions, error) {
	return &pki.RsaPrivateKeyOptions{
		Size: o.Flags.CAPrivateKeySize,
	}, nil
}

func (options *PKICreateOptions) panicOnErr(err error) {
	if err != nil {
		panic(err)
	}
}
