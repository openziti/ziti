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
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/internal"
	ziticobra "github.com/openziti/ziti/internal/cobra"
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
	loginOpts          edge.LoginOptions
	client             *edge_apis.ManagementApiClient
	reader             Reader
	ofJson             bool
	ofYaml             bool
	configCache        map[string]any
	serviceCache       map[string]any
	edgeRouterCache    map[string]any
	authPolicyCache    map[string]any
	extJwtSignersCache map[string]any
	identityCache      map[string]any
}

func NewImportCmd(out io.Writer, errOut io.Writer) *cobra.Command {

	importer := &Importer{}
	importer.loginOpts = edge.LoginOptions{}

	cmd := &cobra.Command{
		Use:   "import filename [entity]",
		Short: "Import entities",
		Long: "Import all or selected entities from the specified file.\n" +
			"Valid entities are: [all|ca/certificate-authority|identity|edge-router|service|config|config-type|service-policy|edge-router-policy|service-edge-router-policy|external-jwt-signer|auth-policy|posture-check] (default all)",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, executeErr := importer.Execute(args)
			if executeErr != nil {
				return executeErr
			}
			log.WithField("results", result).Debug("Finished")
			return nil
		},
		Hidden: true,
	}

	v := viper.New()

	viper.SetEnvPrefix(constants.ZITI) // All env vars we seek will be prefixed with "ZITI_"
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	edge.AddLoginFlags(cmd, &importer.loginOpts)
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().BoolVar(&importer.ofJson, "json", true, "Input parsed as JSON")
	cmd.Flags().BoolVar(&importer.ofYaml, "yaml", false, "Input parsed as YAML")
	cmd.Flags().StringVar(&importer.loginOpts.ControllerUrl, "controller-url", "", "The url of the controller")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")
	ziticobra.SetHelpTemplate(cmd)

	importer.loginOpts.Out = out
	importer.loginOpts.Err = errOut

	return cmd
}

func (importer *Importer) Execute(input []string) (map[string]any, error) {

	logLvl := logrus.InfoLevel
	if importer.loginOpts.Verbose {
		logLvl = logrus.DebugLevel
	}

	pfxlog.GlobalInit(logLvl, pfxlog.DefaultOptions().Color())
	internal.ConfigureLogFormat(logLvl)

	var err error
	importer.client, err = importer.loginOpts.NewManagementApiClient()
	if err != nil {
		return nil, err
	}
	importer.reader = FileReader{}

	raw, err := importer.reader.read(input[0])
	if err != nil {
		return nil, errors.Join(errors.New("unable to read input"), err)
	}
	data := map[string][]interface{}{}

	if importer.ofYaml {
		err = yaml.Unmarshal(raw, &data)
		if err != nil {
			return nil, errors.Join(errors.New("unable to parse input data as yaml"), err)
		}
	} else {
		err = json.Unmarshal(raw, &data)
		if err != nil {
			return nil, errors.Join(errors.New("unable to parse input data as json"), err)
		}
	}

	args := arrayutils.Map(input[1:], strings.ToLower)

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
		cas, err = importer.ProcessCertificateAuthorities(data)
		if err != nil {
			return nil, err
		}
		log.
			WithField("certificateAuthorities", cas).
			Debug("CertificateAuthorities created")
	}
	result["certificateAuthorities"] = cas
	_, _ = internal.FPrintfReusingLine(importer.loginOpts.Err, "Created %d CertificateAuthorities\r\n", len(cas))

	externalJwtSigners := map[string]string{}
	if importer.IsExtJwtSignerImportRequired(args) {
		log.Debug("Processing ExtJWTSigners")
		var err error
		externalJwtSigners, err = importer.ProcessExternalJwtSigners(data)
		if err != nil {
			return nil, err
		}
		log.WithField("externalJwtSigners", externalJwtSigners).Debug("ExtJWTSigners created")
	}
	_, _ = internal.FPrintfReusingLine(importer.loginOpts.Err, "Created %d ExtJWTSigners\r\n", len(externalJwtSigners))
	result["externalJwtSigners"] = externalJwtSigners

	authPolicies := map[string]string{}
	if importer.IsAuthPolicyImportRequired(args) {
		log.Debug("Processing AuthPolicies")
		var err error
		authPolicies, err = importer.ProcessAuthPolicies(data)
		if err != nil {
			return nil, err
		}
		log.WithField("authPolicies", authPolicies).Debug("AuthPolicies created")
	}
	_, _ = internal.FPrintfReusingLine(importer.loginOpts.Err, "Created %d AuthPolicies\r\n", len(authPolicies))
	result["authPolicies"] = authPolicies

	identities := map[string]string{}
	if importer.IsIdentityImportRequired(args) {
		log.Debug("Processing Identities")
		var err error
		identities, err = importer.ProcessIdentities(data)
		if err != nil {
			return nil, err
		}
		log.WithField("identities", identities).Debug("Identities created")
	}
	_, _ = internal.FPrintfReusingLine(importer.loginOpts.Err, "Created %d Identities\r\n", len(identities))
	result["identities"] = identities

	configTypes := map[string]string{}
	if importer.IsConfigTypeImportRequired(args) {
		log.Debug("Processing ConfigTypes")
		var err error
		configTypes, err = importer.ProcessConfigTypes(data)
		if err != nil {
			return nil, err
		}
		log.WithField("configTypes", configTypes).Debug("ConfigTypes created")
	}
	_, _ = internal.FPrintfReusingLine(importer.loginOpts.Err, "Created %d ConfigTypes\r\n", len(configTypes))
	result["configTypes"] = configTypes

	configs := map[string]string{}
	if importer.IsConfigImportRequired(args) {
		log.Debug("Processing Configs")
		var err error
		configs, err = importer.ProcessConfigs(data)
		if err != nil {
			return nil, err
		}
		log.WithField("configs", configs).Debug("Configs created")
	}
	_, _ = internal.FPrintfReusingLine(importer.loginOpts.Err, "Created %d Configs\r\n", len(configs))
	result["configs"] = configs

	services := map[string]string{}
	if importer.IsServiceImportRequired(args) {
		log.Debug("Processing Services")
		var err error
		services, err = importer.ProcessServices(data)
		if err != nil {
			return nil, err
		}
		log.WithField("services", services).Debug("Services created")
	}
	_, _ = internal.FPrintfReusingLine(importer.loginOpts.Err, "Created %d Services\r\n", len(services))
	result["services"] = services

	postureChecks := map[string]string{}
	if importer.IsPostureCheckImportRequired(args) {
		log.Debug("Processing PostureChecks")
		var err error
		postureChecks, err = importer.ProcessPostureChecks(data)
		if err != nil {
			return nil, err
		}
		log.WithField("postureChecks", postureChecks).Debug("PostureChecks created")
	}
	_, _ = internal.FPrintfReusingLine(importer.loginOpts.Err, "Created %d PostureChecks\r\n", len(postureChecks))
	result["postureChecks"] = postureChecks

	routers := map[string]string{}
	if importer.IsEdgeRouterImportRequired(args) {
		log.Debug("Processing EdgeRouters")
		var err error
		routers, err = importer.ProcessEdgeRouters(data)
		if err != nil {
			return nil, err
		}
		log.WithField("edgeRouters", routers).Debug("EdgeRouters created")
	}
	_, _ = internal.FPrintfReusingLine(importer.loginOpts.Err, "Created %d EdgeRouters\r\n", len(routers))
	result["edgeRouters"] = routers

	serviceEdgeRouterPolicies := map[string]string{}
	if importer.IsServiceEdgeRouterPolicyImportRequired(args) {
		log.Debug("Processing ServiceEdgeRouterPolicies")
		var err error
		serviceEdgeRouterPolicies, err = importer.ProcessServiceEdgeRouterPolicies(data)
		if err != nil {
			return nil, err
		}
		log.WithField("serviceEdgeRouterPolicies", serviceEdgeRouterPolicies).Debug("ServiceEdgeRouterPolicies created")
	}
	_, _ = internal.FPrintfReusingLine(importer.loginOpts.Err, "Created %d ServiceEdgeRouterPolicies\r\n", len(serviceEdgeRouterPolicies))
	result["serviceEdgeRouterPolicies"] = serviceEdgeRouterPolicies

	servicePolicies := map[string]string{}
	if importer.IsServicePolicyImportRequired(args) {
		log.Debug("Processing ServicePolicies")
		var err error
		servicePolicies, err = importer.ProcessServicePolicies(data)
		if err != nil {
			return nil, err
		}
		log.WithField("servicePolicies", servicePolicies).Debug("ServicePolicies created")
	}
	_, _ = internal.FPrintfReusingLine(importer.loginOpts.Err, "Created %d ServicePolicies\r\n", len(servicePolicies))
	result["servicePolicies"] = servicePolicies

	routerPolicies := map[string]string{}
	if importer.IsEdgeRouterPolicyImportRequired(args) {
		log.Debug("Processing EdgeRouterPolicies")
		var err error
		routerPolicies, err = importer.ProcessEdgeRouterPolicies(data)
		if err != nil {
			return nil, err
		}
		log.WithField("routerPolicies", routerPolicies).Debug("EdgeRouterPolicies created")
	}
	_, _ = internal.FPrintfReusingLine(importer.loginOpts.Err, "Created %d EdgeRouterPolicies\r\n", len(routerPolicies))
	result["edgeRouterPolicies"] = routerPolicies

	log.Info("Upload complete")

	return result, nil
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
	read(input any) ([]byte, error)
}

type FileReader struct {
}

func (i FileReader) read(input any) ([]byte, error) {
	file, err := os.ReadFile(input.(string))
	if err != nil {
		return nil, err
	}

	return file, nil
}
