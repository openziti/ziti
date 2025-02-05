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

package importer

import (
	"encoding/json"
	"errors"
	"github.com/judedaryl/go-arrayutils"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_management_api_client"
	"github.com/openziti/ziti/internal"
	ziticobra "github.com/openziti/ziti/internal/cobra"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/cmd/edge"
	"github.com/openziti/ziti/ziti/constants"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"strings"
)

var log = pfxlog.Logger()

type Importer struct {
	Out                io.Writer
	Err                io.Writer
	Data               map[string][]interface{}
	configCache        map[string]any
	serviceCache       map[string]any
	edgeRouterCache    map[string]any
	authPolicyCache    map[string]any
	extJwtSignersCache map[string]any
	identityCache      map[string]any
	verbose            bool
}

func NewImportCmd(out io.Writer, errOut io.Writer) *cobra.Command {

	importer := &Importer{
		Out: out,
		Err: errOut,
	}

	var inputFormat string
	var loginOpts = edge.LoginOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{
				Out: os.Stdout,
				Err: os.Stderr,
			},
		},
	}

	cmd := &cobra.Command{
		Use:   "import filename [entity]",
		Short: "Import entities",
		Long: "Import all or comma separated list of selected entities from the specified file.\n" +
			"Valid entities are: [all|ca/certificate-authority|identity|edge-router|service|config|config-type|service-policy|edge-router-policy|service-edge-router-policy|external-jwt-signer|auth-policy|posture-check] (default all)",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {

			importer.verbose = loginOpts.Verbose

			var parsedInputFormat InputFormat
			if strings.ToUpper(inputFormat) == "JSON" {
				parsedInputFormat = JSON
			} else if strings.ToUpper(inputFormat) == "YAML" {
				parsedInputFormat = YAML
			} else {
				log.Fatalf("Invalid input format: %s", inputFormat)
			}

			raw, err := FileReader{
				filename: args[0],
			}.read()

			data := map[string][]interface{}{}
			if parsedInputFormat == YAML {
				err = yaml.Unmarshal(raw, &data)
				if err != nil {
					return errors.Join(errors.New("unable to parse input data as yaml"), err)
				}
			} else {
				err = json.Unmarshal(raw, &data)
				if err != nil {
					return errors.Join(errors.New("unable to parse input data as json"), err)
				}
			}
			importer.Data = data

			client, err := loginOpts.NewMgmtClient()
			if err != nil {
				log.Fatal(err)
			}

			executeErr := importer.Execute(client, args[1:])
			if executeErr != nil {
				return executeErr
			}
			return nil
		},
		Hidden: true,
	}

	v := viper.New()

	viper.SetEnvPrefix(constants.ZITI) // All env vars we seek will be prefixed with "ZITI_"
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	edge.AddLoginFlags(cmd, &loginOpts)
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringVar(&inputFormat, "input-format", "JSON", "Parse input as either JSON or YAML (default JSON)")
	cmd.Flags().StringVar(&loginOpts.ControllerUrl, "controller-url", "", "The url of the controller")
	ziticobra.SetHelpTemplate(cmd)

	return cmd
}

func (importer *Importer) Execute(client *rest_management_api_client.ZitiEdgeManagement, entities []string) error {

	logLvl := logrus.InfoLevel
	if importer.verbose {
		logLvl = logrus.DebugLevel
	}

	pfxlog.GlobalInit(logLvl, pfxlog.DefaultOptions().Color())
	internal.ConfigureLogFormat(logLvl)

	args := arrayutils.Map(entities, strings.ToLower)

	importer.configCache = map[string]any{}
	importer.serviceCache = map[string]any{}
	importer.edgeRouterCache = map[string]any{}
	importer.authPolicyCache = map[string]any{}
	importer.extJwtSignersCache = map[string]any{}
	importer.identityCache = map[string]any{}

	result := map[string]any{}

	cas := map[string]string{}
	if importer.IsCertificateAuthorityImportRequired(args) {
		log.Debug("Processing CertificateAuthorities")
		var err error
		cas, err = importer.ProcessCertificateAuthorities(client, importer.Data)
		if err != nil {
			return err
		}
		log.
			WithField("certificateAuthorities", cas).
			Debug("CertificateAuthorities created")
	}
	result["certificateAuthorities"] = cas
	_, _ = internal.FPrintfReusingLine(importer.Err, "Created %d CertificateAuthorities\r\n", len(cas))

	externalJwtSigners := map[string]string{}
	if importer.IsExtJwtSignerImportRequired(args) {
		log.Debug("Processing ExtJWTSigners")
		var err error
		externalJwtSigners, err = importer.ProcessExternalJwtSigners(client, importer.Data)
		if err != nil {
			return err
		}
		log.WithField("externalJwtSigners", externalJwtSigners).Debug("ExtJWTSigners created")
	}
	_, _ = internal.FPrintfReusingLine(importer.Err, "Created %d ExtJWTSigners\r\n", len(externalJwtSigners))
	result["externalJwtSigners"] = externalJwtSigners

	authPolicies := map[string]string{}
	if importer.IsAuthPolicyImportRequired(args) {
		log.Debug("Processing AuthPolicies")
		var err error
		authPolicies, err = importer.ProcessAuthPolicies(client, importer.Data)
		if err != nil {
			return err
		}
		log.WithField("authPolicies", authPolicies).Debug("AuthPolicies created")
	}
	_, _ = internal.FPrintfReusingLine(importer.Err, "Created %d AuthPolicies\r\n", len(authPolicies))
	result["authPolicies"] = authPolicies

	identities := map[string]string{}
	if importer.IsIdentityImportRequired(args) {
		log.Debug("Processing Identities")
		var err error
		identities, err = importer.ProcessIdentities(client, importer.Data)
		if err != nil {
			return err
		}
		log.WithField("identities", identities).Debug("Identities created")
	}
	_, _ = internal.FPrintfReusingLine(importer.Err, "Created %d Identities\r\n", len(identities))
	result["identities"] = identities

	configTypes := map[string]string{}
	if importer.IsConfigTypeImportRequired(args) {
		log.Debug("Processing ConfigTypes")
		var err error
		configTypes, err = importer.ProcessConfigTypes(client, importer.Data)
		if err != nil {
			return err
		}
		log.WithField("configTypes", configTypes).Debug("ConfigTypes created")
	}
	_, _ = internal.FPrintfReusingLine(importer.Err, "Created %d ConfigTypes\r\n", len(configTypes))
	result["configTypes"] = configTypes

	configs := map[string]string{}
	if importer.IsConfigImportRequired(args) {
		log.Debug("Processing Configs")
		var err error
		configs, err = importer.ProcessConfigs(client, importer.Data)
		if err != nil {
			return err
		}
		log.WithField("configs", configs).Debug("Configs created")
	}
	_, _ = internal.FPrintfReusingLine(importer.Err, "Created %d Configs\r\n", len(configs))
	result["configs"] = configs

	services := map[string]string{}
	if importer.IsServiceImportRequired(args) {
		log.Debug("Processing Services")
		var err error
		services, err = importer.ProcessServices(client, importer.Data)
		if err != nil {
			return err
		}
		log.WithField("services", services).Debug("Services created")
	}
	_, _ = internal.FPrintfReusingLine(importer.Err, "Created %d Services\r\n", len(services))
	result["services"] = services

	postureChecks := map[string]string{}
	if importer.IsPostureCheckImportRequired(args) {
		log.Debug("Processing PostureChecks")
		var err error
		postureChecks, err = importer.ProcessPostureChecks(client, importer.Data)
		if err != nil {
			return err
		}
		log.WithField("postureChecks", postureChecks).Debug("PostureChecks created")
	}
	_, _ = internal.FPrintfReusingLine(importer.Err, "Created %d PostureChecks\r\n", len(postureChecks))
	result["postureChecks"] = postureChecks

	routers := map[string]string{}
	if importer.IsEdgeRouterImportRequired(args) {
		log.Debug("Processing EdgeRouters")
		var err error
		routers, err = importer.ProcessEdgeRouters(client, importer.Data)
		if err != nil {
			return err
		}
		log.WithField("edgeRouters", routers).Debug("EdgeRouters created")
	}
	_, _ = internal.FPrintfReusingLine(importer.Err, "Created %d EdgeRouters\r\n", len(routers))
	result["edgeRouters"] = routers

	serviceEdgeRouterPolicies := map[string]string{}
	if importer.IsServiceEdgeRouterPolicyImportRequired(args) {
		log.Debug("Processing ServiceEdgeRouterPolicies")
		var err error
		serviceEdgeRouterPolicies, err = importer.ProcessServiceEdgeRouterPolicies(client, importer.Data)
		if err != nil {
			return err
		}
		log.WithField("serviceEdgeRouterPolicies", serviceEdgeRouterPolicies).Debug("ServiceEdgeRouterPolicies created")
	}
	_, _ = internal.FPrintfReusingLine(importer.Err, "Created %d ServiceEdgeRouterPolicies\r\n", len(serviceEdgeRouterPolicies))
	result["serviceEdgeRouterPolicies"] = serviceEdgeRouterPolicies

	servicePolicies := map[string]string{}
	if importer.IsServicePolicyImportRequired(args) {
		log.Debug("Processing ServicePolicies")
		var err error
		servicePolicies, err = importer.ProcessServicePolicies(client, importer.Data)
		if err != nil {
			return err
		}
		log.WithField("servicePolicies", servicePolicies).Debug("ServicePolicies created")
	}
	_, _ = internal.FPrintfReusingLine(importer.Err, "Created %d ServicePolicies\r\n", len(servicePolicies))
	result["servicePolicies"] = servicePolicies

	routerPolicies := map[string]string{}
	if importer.IsEdgeRouterPolicyImportRequired(args) {
		log.Debug("Processing EdgeRouterPolicies")
		var err error
		routerPolicies, err = importer.ProcessEdgeRouterPolicies(client, importer.Data)
		if err != nil {
			return err
		}
		log.WithField("routerPolicies", routerPolicies).Debug("EdgeRouterPolicies created")
	}
	_, _ = internal.FPrintfReusingLine(importer.Err, "Created %d EdgeRouterPolicies\r\n", len(routerPolicies))
	result["edgeRouterPolicies"] = routerPolicies

	log.Info("Upload complete")

	log.WithField("results", result).Debug("Finished")

	return nil
}

func FromMap[T interface{}](input interface{}, v T) *T {
	jsonData, _ := json.MarshalIndent(input, "", "  ")
	create := new(T)
	err := json.Unmarshal(jsonData, &create)
	if err != nil {
		log.
			WithField("err", err).
			Error("error converting input to object")
		return nil
	}
	return create
}

type Reader interface {
	read() ([]byte, error)
}

type FileReader struct {
	filename string
}

func (i FileReader) read() ([]byte, error) {
	content, err := os.ReadFile(i.filename)
	if err != nil {
		return nil, err
	}
	return content, nil
}

type InputFormat string

const (
	JSON InputFormat = "JSON"
	YAML InputFormat = "YAML"
)
