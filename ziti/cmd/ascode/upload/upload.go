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

package upload

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/judedaryl/go-arrayutils"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_management_api_client"
	"github.com/openziti/ziti/internal"
	"github.com/openziti/ziti/internal/rest/mgmt"
	"github.com/openziti/ziti/ziti/cmd/edge"
	c "github.com/openziti/ziti/ziti/constants"
	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"slices"
	"strings"
	"time"
)

type Upload struct {
	loginOpts edge.LoginOptions
	client    *rest_management_api_client.ZitiEdgeManagement
	reader    Reader

	verbose bool

	ofJson bool
	ofYaml bool

	configCache        *cache.Cache
	serviceCache       *cache.Cache
	edgeRouterCache    *cache.Cache
	authPolicyCache    *cache.Cache
	extJwtSignersCache *cache.Cache
	identityCache      *cache.Cache

	Out io.Writer
	Err io.Writer
}

var log = pfxlog.Logger()

func NewUploadCmd(out io.Writer, errOut io.Writer) *cobra.Command {

	u := &Upload{}
	u.Out = out
	u.Err = errOut
	u.loginOpts = edge.LoginOptions{}

	cmd := &cobra.Command{
		Use:   "import filename [entity]",
		Short: "Import ziti entities",
		Long: "Import all or selected ziti entities from the specified file.\n" +
			"Valid entities are: [all|ca/certificate-authority|identity|edge-router|service|config|config-type|service-policy|edgerouter-policy|service-edgerouter-policy|external-jwt-signer|auth-policy|posture-check] (default all)",
		Args: cobra.MinimumNArgs(1),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			u.Init()
		},
		Run: func(cmd *cobra.Command, args []string) {
			data, err := u.reader.read(args[0])
			if err != nil {
				panic(errors.Join(errors.New("unable to read input"), err))
			}
			m := map[string][]interface{}{}

			if u.ofYaml {
				err = yaml.Unmarshal(data, &m)
				if err != nil {
					panic(errors.Join(errors.New("unable to parse input data as yaml"), err))
				}
			} else {
				err = json.Unmarshal(data, &m)
				if err != nil {
					panic(errors.Join(errors.New("unable to parse input data as json"), err))
				}
			}

			result, executeErr := u.Execute(m, args[1:])
			if executeErr != nil {
				panic(executeErr)
			}
			if u.verbose {
				log.
					WithField("results", result).
					Debug("Finished")
			}
		},
		Hidden: true,
	}

	v := viper.New()

	viper.SetEnvPrefix(c.ZITI) // All env vars we seek will be prefixed with "ZITI_"
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	cmd.Flags().SetInterspersed(true)
	cmd.Flags().BoolVar(&u.ofJson, "json", true, "Input parsed as JSON")
	cmd.Flags().BoolVar(&u.ofYaml, "yaml", false, "Input parsed as YAML")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	edge.AddLoginFlags(cmd, &u.loginOpts)
	u.loginOpts.Out = out
	u.loginOpts.Err = errOut

	return cmd
}

func (u *Upload) Init() {
	u.verbose = u.loginOpts.Verbose

	logLvl := logrus.InfoLevel
	if u.verbose {
		logLvl = logrus.DebugLevel
	}

	pfxlog.GlobalInit(logLvl, pfxlog.DefaultOptions().Color())
	internal.ConfigureLogFormat(logLvl)

	client, err := mgmt.NewClient()
	if err != nil {
		loginErr := u.loginOpts.Run()
		if loginErr != nil {
			log.Fatal(err)
		}
		client, err = mgmt.NewClient()
		if err != nil {
			log.Fatal(err)
		}
	}
	u.client = client
	u.reader = FileReader{}
}

func (u *Upload) Execute(data map[string][]interface{}, inputArgs []string) (map[string]any, error) {

	args := arrayutils.Map(inputArgs, strings.ToLower)

	u.configCache = cache.New(time.Duration(-1), time.Duration(-1))
	u.serviceCache = cache.New(time.Duration(-1), time.Duration(-1))
	u.edgeRouterCache = cache.New(time.Duration(-1), time.Duration(-1))
	u.authPolicyCache = cache.New(time.Duration(-1), time.Duration(-1))
	u.extJwtSignersCache = cache.New(time.Duration(-1), time.Duration(-1))
	u.identityCache = cache.New(time.Duration(-1), time.Duration(-1))

	result := map[string]any{}
	all := slices.Contains(args, "all") || len(args) == 0

	cas := map[string]string{}
	if all ||
		slices.Contains(args, "ca") || slices.Contains(args, "cas") ||
		slices.Contains(args, "certificate-authority") || slices.Contains(args, "certificate-authorities") {
		if u.verbose {
			log.
				Debug("Processing CertificateAuthorities")
		}
		var err error
		cas, err = u.ProcessCertificateAuthorities(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("certificateAuthorities", cas).
				Debug("CertificateAuthorities created")
		}
	}
	result["certificateAuthorities"] = cas
	_, _ = fmt.Fprintf(u.Err, "\u001B[2KCreated %d CertificateAuthorities\r\n", len(cas))

	externalJwtSigners := map[string]string{}
	if all ||
		slices.Contains(args, "external-jwt-signer") || slices.Contains(args, "external-jwt-signers") ||
		slices.Contains(args, "auth-policy") || slices.Contains(args, "auth-policies") ||
		slices.Contains(args, "identity") || slices.Contains(args, "identities") {
		if u.verbose {
			log.
				Debug("Processing ExtJWTSigners")
		}
		var err error
		externalJwtSigners, err = u.ProcessExternalJwtSigners(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("externalJwtSigners", externalJwtSigners).
				Debug("ExtJWTSigners created")
		}
	}
	_, _ = fmt.Fprintf(u.Err, "\u001B[2KCreated %d ExtJWTSigners\r\n", len(externalJwtSigners))
	result["externalJwtSigners"] = externalJwtSigners

	authPolicies := map[string]string{}
	if all ||
		slices.Contains(args, "auth-policy") || slices.Contains(args, "auth-policies") ||
		slices.Contains(args, "identity") || slices.Contains(args, "identities") {
		if u.verbose {
			log.
				Debug("Processing AuthPolicies")
		}
		var err error
		authPolicies, err = u.ProcessAuthPolicies(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("authPolicies", authPolicies).
				Debug("AuthPolicies created")
		}
	}
	_, _ = fmt.Fprintf(u.Err, "\u001B[2KCreated %d AuthPolicies\r\n", len(authPolicies))
	result["authPolicies"] = authPolicies

	identities := map[string]string{}
	if all ||
		slices.Contains(args, "identity") || slices.Contains(args, "identities") {
		if u.verbose {
			log.
				Debug("Processing Identities")
		}
		var err error
		identities, err = u.ProcessIdentities(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("identities", identities).
				Debug("Identities created")
		}
	}
	_, _ = fmt.Fprintf(u.Err, "\u001B[2KCreated %d Identities\r\n", len(identities))
	result["identities"] = identities

	configTypes := map[string]string{}
	if all ||
		slices.Contains(args, "config-type") || slices.Contains(args, "config-types") ||
		slices.Contains(args, "config") || slices.Contains(args, "configs") ||
		slices.Contains(args, "service") || slices.Contains(args, "services") {
		if u.verbose {
			log.
				Debug("Processing ConfigTypes")
		}
		var err error
		configTypes, err = u.ProcessConfigTypes(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("configTypes", configTypes).
				Debug("ConfigTypes created")
		}
	}
	_, _ = fmt.Fprintf(u.Err, "\u001B[2KCreated %d ConfigTypes\r\n", len(configTypes))
	result["configTypes"] = configTypes

	configs := map[string]string{}
	if all ||
		slices.Contains(args, "config") || slices.Contains(args, "configs") ||
		slices.Contains(args, "service") || slices.Contains(args, "services") {
		if u.verbose {
			log.
				Debug("Processing Configs")
		}
		var err error
		configs, err = u.ProcessConfigs(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("configs", configs).
				Debug("Configs created")
		}
	}
	_, _ = fmt.Fprintf(u.Err, "\u001B[2KCreated %d Configs\r\n", len(configs))
	result["configs"] = configs

	services := map[string]string{}
	if all ||
		slices.Contains(args, "service") || slices.Contains(args, "services") {
		if u.verbose {
			log.
				Debug("Processing Services")
		}
		var err error
		services, err = u.ProcessServices(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("services", services).
				Debug("Services created")
		}
	}
	_, _ = fmt.Fprintf(u.Err, "\u001B[2KCreated %d Services\r\n", len(services))
	result["services"] = services

	postureChecks := map[string]string{}
	if all ||
		slices.Contains(args, "posture-check") || slices.Contains(args, "posture-checks") {
		if u.verbose {
			log.
				Debug("Processing PostureChecks")
		}
		var err error
		postureChecks, err = u.ProcessPostureChecks(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("postureChecks", postureChecks).
				Debug("PostureChecks created")
		}
	}
	_, _ = fmt.Fprintf(u.Err, "\u001B[2KCreated %d PostureChecks\r\n", len(postureChecks))
	result["postureChecks"] = postureChecks

	routers := map[string]string{}
	if all ||
		slices.Contains(args, "edge-router") || slices.Contains(args, "edge-routers") ||
		slices.Contains(args, "ers") || slices.Contains(args, "ers") {
		if u.verbose {
			log.
				Debug("Processing EdgeRouters")
		}
		var err error
		routers, err = u.ProcessEdgeRouters(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("edgeRouters", routers).
				Debug("EdgeRouters created")
		}
	}
	_, _ = fmt.Fprintf(u.Err, "\u001B[2KCreated %d EdgeRouters\r\n", len(routers))
	result["edgeRouters"] = routers

	serviceEdgeRouterPolicies := map[string]string{}
	if all ||
		slices.Contains(args, "service-edgerouter-policy") || slices.Contains(args, "service-edgerouter-policies") {
		if u.verbose {
			log.
				Debug("Processing ServiceRouterPolicies")
		}
		var err error
		serviceEdgeRouterPolicies, err = u.ProcessServiceEdgeRouterPolicies(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("serviceEdgeRouterPolicies", serviceEdgeRouterPolicies).
				Debug("ServiceEdgeRouterPolicies created")
		}
	}
	_, _ = fmt.Fprintf(u.Err, "\u001B[2KCreated %d ServiceEdgeRouterPolicies\r\n", len(serviceEdgeRouterPolicies))
	result["serviceEdgeRouterPolicies"] = serviceEdgeRouterPolicies

	servicePolicies := map[string]string{}
	if all ||
		slices.Contains(args, "service-policy") || slices.Contains(args, "service-policies") {
		if u.verbose {
			log.
				Debug("Processing ServicePolicies")
		}
		var err error
		servicePolicies, err = u.ProcessServicePolicies(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("servicePolicies", servicePolicies).
				Debug("ServicePolicies created")
		}
	}
	_, _ = fmt.Fprintf(u.Err, "\u001B[2KCreated %d ServicePolicies\r\n", len(servicePolicies))
	result["servicePolicies"] = servicePolicies

	routerPolicies := map[string]string{}
	if all ||
		slices.Contains(args, "edgerouter-policy") || slices.Contains(args, "edgerouter-policies") {
		if u.verbose {
			log.
				Debug("Processing EdgeRouterPolicies")
		}
		var err error
		routerPolicies, err = u.ProcessEdgeRouterPolicies(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("routerPolicies", routerPolicies).
				Debug("EdgeRouterPolicies created")
		}
	}
	_, _ = fmt.Fprintf(u.Err, "\u001B[2KCreated %d EdgeRouterPolicies\r\n", len(routerPolicies))
	result["edgeRouterPolicies"] = routerPolicies

	if u.verbose {
		log.
			Info("Upload complete")
	}

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
