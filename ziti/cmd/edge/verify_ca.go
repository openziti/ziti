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

package edge

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/foundation/v2/term"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
	"gopkg.in/resty.v1"
	"io"
	"io/ioutil"
	"math/big"
	"time"
)

type verifyCaOptions struct {
	api.Options

	certPath     string
	certPemBytes []byte

	caNameOrId string
	caId       string

	caCertPath     string
	caCertPemBytes []byte

	caKeyPath     string
	caKeyPemBytes []byte
	caKeyPassword string

	isGenerateCert bool
}

// newVerifyCaCmd creates the 'edge controller verify ca' command for the given entity type
func newVerifyCaCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &verifyCaOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:   "ca <name> ( --cert <pemCertFile> | --cacert <signingCaCert> --cakey <signingCaKey> [--password <caKeyPassword>])",
		Short: "verifies a ca managed by the Ziti Edge Controller",
		Long: "verifies a ca managed by the Ziti Edge Controller. If --cert is supplied, it is expected that it is a certificate with the " +
			"common name set to the proper verificationToken value from the target CA. If not set, --cakey and --cacert can be provided to " +
			"generate the signed certificate and submit it.",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("requires at least %d arg(s), only received %d", 1, len(args))
			}

			options.caNameOrId = args[0]

			if options.certPath != "" {
				var err error
				options.certPemBytes, err = ioutil.ReadFile(options.certPath)

				if err != nil {
					return fmt.Errorf("could not read --cert file (%s): %v", options.certPath, err)
				}
				options.isGenerateCert = false
			} else if options.caCertPath != "" && options.caKeyPath != "" {
				var err error
				options.caKeyPemBytes, err = ioutil.ReadFile(options.caKeyPath)

				if err != nil {
					return fmt.Errorf("could not read --cakey file (%s): %v", options.caKeyPath, err)
				}

				options.caCertPemBytes, err = ioutil.ReadFile(options.caCertPath)

				if err != nil {
					return fmt.Errorf("could not read --cacert file (%s): %v", options.caCertPath, err)
				}
				options.isGenerateCert = true
			} else {
				return errors.New("expected either (--cert <pemCertFile) or (--cacert <signingCaCert> --cakey <signingCaKey>), some options were missing")
			}

			return nil

		},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runValidateCa(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringVarP(&options.certPath, "cert", "c", "", "The path to a cert with the CN set as the verification token and signed by the target CA")
	cmd.Flags().StringVarP(&options.caCertPath, "cacert", "a", "", "The path to the CA cert that should be used to generate and sign a verification cert")
	cmd.Flags().StringVarP(&options.caKeyPath, "cakey", "k", "", "The path to the CA key that should be used to generate and sign a verification cert")
	cmd.Flags().StringVarP(&options.caKeyPassword, "password", "p", "", "The password for the CA key if necessary")
	options.AddCommonFlags(cmd)

	return cmd
}

func runValidateCa(options *verifyCaOptions) error {
	var err error
	options.caId, err = mapCaNameToID(options.caNameOrId, options.Options)

	if err != nil {
		return err
	}

	if options.isGenerateCert {
		jsonContainer, err := util.EdgeControllerRequest("cas/"+options.caId, options.Out, options.OutputJSONResponse, options.Options.Timeout, options.Options.Verbose, func(request *resty.Request, url string) (*resty.Response, error) { return request.Get(url) })

		if err != nil {
			return fmt.Errorf("could not request ca [%s] (%v}", options.caId, err)
		}

		token := jsonContainer.Path("data.verificationToken").Data().(string)

		if token == "" {
			return fmt.Errorf("could not obtain verification token for ca [%s]", options.caId)
		}

		options.certPemBytes, _, err = generateCert(options, token)

		if err != nil {
			return fmt.Errorf("could not generate validation cert: %v", err)
		}
	}

	return util.EdgeControllerVerify("cas", options.caId, string(options.certPemBytes), options.Out, options.OutputJSONResponse, options.Options.Timeout, options.Options.Verbose)
}

func generateCert(options *verifyCaOptions, token string) ([]byte, crypto.Signer, error) {

	certBlocks := nfpem.PemToX509(string(options.caCertPemBytes))

	if len(certBlocks) == 0 {
		return nil, nil, errors.New("could not parse cert file, 0 PEM blocks detected1")
	}

	caCertBlock := certBlocks[0]
	caCert, err := x509.ParseCertificate(caCertBlock.Raw)

	if err != nil {
		return nil, nil, fmt.Errorf("could not parse ca cert (%v)", err)
	}
	keyBlocks := nfpem.DecodeAll(options.caKeyPemBytes)

	if len(keyBlocks) == 0 {
		return nil, nil, errors.New("could not parse key file, 0 PEM blocks detected")
	}

	caPrivateKeyBlock := keyBlocks[0]

	if x509.IsEncryptedPEMBlock(caPrivateKeyBlock) {
		if options.caKeyPassword == "" {
			options.caKeyPassword, err = term.PromptPassword("enter the password for the supplied ca key file: ", false)
			if err != nil {
				return nil, nil, fmt.Errorf("could not retrieve password for encrypted CA key file (%v)", err)
			}
		}

		caPrivateKeyBlock.Bytes, err = x509.DecryptPEMBlock(caPrivateKeyBlock, []byte(options.caKeyPassword))
		if err != nil {
			return nil, nil, fmt.Errorf("could not decrypt CA private key (%v)", err)
		}
	}

	var signerInterface crypto.Signer
	if caPrivateKeyBlock.Type == "EC PRIVATE KEY" {
		signerInterface, err = x509.ParseECPrivateKey(caPrivateKeyBlock.Bytes)
	} else {
		signerInterface, err = x509.ParsePKCS1PrivateKey(caPrivateKeyBlock.Bytes)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("could not parse key file (%s)", err)
	}

	caKey := signerInterface.(crypto.Signer)

	if caKey == nil {
		return nil, nil, fmt.Errorf("key was not of correct type, could not be used as signer")
	}

	id, _ := rand.Int(rand.Reader, big.NewInt(100000000000000000))
	verificationCert := &x509.Certificate{
		SerialNumber: id,
		Subject: pkix.Name{
			CommonName:   token,
			Organization: []string{"Ziti CLI Generated Validation Cert"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Minute * 5),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	verificationKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)

	if err != nil {
		return nil, nil, fmt.Errorf("could not generate private key for verification certificate (%v)", err)
	}

	signedCertBytes, err := x509.CreateCertificate(rand.Reader, verificationCert, caCert, verificationKey.Public(), caKey)

	if err != nil {
		return nil, nil, fmt.Errorf("could not sign verification certificate with CA (%v)", err)
	}

	verificationCert, _ = x509.ParseCertificate(signedCertBytes)

	verificationBlock := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: verificationCert.Raw,
	}

	verificationCertPemBytes := pem.EncodeToMemory(verificationBlock)

	return verificationCertPemBytes, verificationKey, nil
}
