package test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"github.com/openziti/edge/rest_management_api_client"
	api_client_config "github.com/openziti/edge/rest_management_api_client/config"
	"github.com/openziti/edge/rest_management_api_client/edge_router"
	"github.com/openziti/edge/rest_management_api_client/edge_router_policy"
	"github.com/openziti/edge/rest_management_api_client/identity"
	"github.com/openziti/edge/rest_management_api_client/router"
	"github.com/openziti/edge/rest_management_api_client/service"
	"github.com/openziti/edge/rest_management_api_client/service_edge_router_policy"
	"github.com/openziti/edge/rest_management_api_client/service_policy"
	"github.com/openziti/edge/rest_management_api_client/terminator"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/edge/rest_util"
	"github.com/openziti/edge/tunnel/entities"
	"github.com/openziti/sdk-golang/ziti"
	sdk_config "github.com/openziti/sdk-golang/ziti/config"
	"github.com/openziti/sdk-golang/ziti/enroll"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

var zitiContext ziti.Context
var DockerAdminUsername = "admin"
var DockerAdminPassword = "admin"

//"kvqzGyUj2oJ3Jmql_1YgM4ao61tVg-vw"

/*
This is a manually run test that will, with the default values except the admin password, confirm the docker-compose
ziti network is running as expected. The values can be edited to confirm other ziti networks but will require an http
server on the back end.
*/
func TestSimpleWebService(t *testing.T) {

	testerUsername := "gotester"
	ctrlAddress := "https://ziti-edge-controller:1280"
	hostingRouterName := "ziti-edge-router"
	dialAddress := "simple.web.smoke.test"
	dialPort := 80
	bindHostAddress := "web-test-blue"
	bindHostPort := 8000
	serviceName := "basic.web.smoke.test.service"
	wd, _ := os.Getwd()

	// Wait for the controller to become available
	waitForController(ctrlAddress, 10*time.Second)

	client, err := connectToController(ctrlAddress, DockerAdminUsername, DockerAdminPassword)
	require.Nilf(t, err, "An error occurred attempting to connect with the controller.\n%s", err)

	// Create the tester identity
	ident := createIdentity(client, testerUsername, rest_model.IdentityTypeUser, false, nil)
	defer func() { _ = deleteIdentityByID(client, ident.GetPayload().Data.ID) }()

	// Enroll the identity
	identConfig := enrollIdentity(client, ident.Payload.Data.ID)

	// Create a json config file
	output, err := os.Create(testerUsername + ".json")
	if err != nil {
		fmt.Println(err)
		log.Error("Failed to create output config file")
	}
	defer func() {
		_ = output.Close()
		err = os.Remove(testerUsername + ".json")
		if err != nil {
			fmt.Println(err)
			log.Error("Failed to delete json config file")
		}
	}()
	enc := json.NewEncoder(output)
	enc.SetEscapeHTML(false)
	encErr := enc.Encode(&identConfig)
	if encErr != nil {
		fmt.Println(err)
		log.Error("Failed to generate encoded output")
	}

	// Allow dialing the service using an intercept config (intercept because we'll be using the SDK)
	dialSvcConfig := createInterceptV1ServiceConfig(client, "basic.smoke.dial", []string{"tcp"}, []string{dialAddress}, dialPort, dialPort)
	defer func() { _ = deleteServiceConfigByID(client, dialSvcConfig.ID) }()

	// Provide host config for the hostname
	bindSvcConfig := createHostV1ServiceConfig(client, "basic.smoke.bind", "tcp", bindHostAddress, bindHostPort)
	defer func() { _ = deleteServiceConfigByID(client, bindSvcConfig.ID) }()

	// Create a service that "links" the dial and bind configs
	createService(client, serviceName, []string{bindSvcConfig.ID, dialSvcConfig.ID})

	// Create a service policy to allow the router to host the web test service
	hostRouterIdent := getIdentityByName(client, hostingRouterName)
	webTestService := getServiceByName(client, serviceName)
	defer func() { _ = deleteServiceByID(client, *webTestService.ID) }()
	bindSP := createServicePolicy(client, "basic.web.smoke.test.service.bind", rest_model.DialBindBind, rest_model.Roles{"@" + *hostRouterIdent.ID}, rest_model.Roles{"@" + *webTestService.ID})
	defer func() { _ = deleteServicePolicyByID(client, bindSP.ID) }()

	// Create a service policy to allow tester to dial the service
	testerIdent := getIdentityByName(client, testerUsername)
	dialSP := createServicePolicy(client, "basic.web.smoke.test.service.dial", rest_model.DialBindDial, rest_model.Roles{"@" + *testerIdent.ID}, rest_model.Roles{"@" + *webTestService.ID})
	defer func() { _ = deleteServicePolicyByID(client, dialSP.ID) }()

	// Test connectivity with private edge router, wait some time for the terminator to be created
	currentCount := getTerminatorCountByRouterName(client, hostingRouterName)
	termCntReached := waitForTerminatorCountByRouterName(client, hostingRouterName, currentCount+1, 30*time.Second)
	if !termCntReached {
		fmt.Println("Unable to detect a terminator for the edge router")
	}
	helloUrl := fmt.Sprintf("http://%s:%d", serviceName, dialPort)
	httpClient := createZitifiedHttpClient(wd + "/" + testerUsername + ".json")
	resp, e := httpClient.Get(helloUrl)
	if e != nil {
		log.Error(e)
	}

	assert.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("Expected successful HTTP status code 200, received %d instead", resp.StatusCode))
}

func enrollIdentity(client *rest_management_api_client.ZitiEdgeManagement, identityID string) *sdk_config.Config {
	// Get the identity object
	params := &identity.DetailIdentityParams{
		Context: context.Background(),
		ID:      identityID,
	}
	params.SetTimeout(30 * time.Second)
	resp, err := client.Identity.DetailIdentity(params, nil)

	if err != nil {
		log.Error(err)
	}

	// Enroll the identity
	tkn, _, err := enroll.ParseToken(resp.GetPayload().Data.Enrollment.Ott.JWT)
	if err != nil {
		log.Error(err)
	}

	flags := enroll.EnrollmentFlags{
		Token:  tkn,
		KeyAlg: "RSA",
	}
	conf, err := enroll.Enroll(flags)

	if err != nil {
		log.Error(err)
	}

	return conf
}

func Dial(_ context.Context, _ string, addr string) (net.Conn, error) {
	service := strings.Split(addr, ":")[0] // will always get passed host:port
	return zitiContext.Dial(service)
}

func createZitifiedHttpClient(idFile string) http.Client {
	cfg, err := sdk_config.NewFromFile(idFile)
	if err != nil {
		log.Error(err)
	}
	zitiContext = ziti.NewContextWithConfig(cfg)
	zitiTransport := http.DefaultTransport.(*http.Transport).Clone() // copy default transport
	zitiTransport.DialContext = Dial                                 //zitiDialContext.Dial
	return http.Client{Transport: zitiTransport}
}

// #################### Test Utils #############################

func createIdentity(client *rest_management_api_client.ZitiEdgeManagement, name string,
	identType rest_model.IdentityType, isAdmin bool, roleAttr rest_model.Attributes) *identity.CreateIdentityCreated {
	i := &rest_model.IdentityCreate{
		Enrollment: &rest_model.IdentityCreateEnrollment{
			Ott: true,
		},
		IsAdmin:                   &isAdmin,
		Name:                      &name,
		RoleAttributes:            &roleAttr,
		ServiceHostingCosts:       nil,
		ServiceHostingPrecedences: nil,
		Tags:                      nil,
		Type:                      &identType,
	}
	p := identity.NewCreateIdentityParams()
	p.Identity = i

	// Create the identity
	ident, err := client.Identity.CreateIdentity(p, nil)
	if err != nil {
		fmt.Println(err)
		log.Error("Failed to create the identity")
	}

	return ident
}

func deleteIdentityByID(client *rest_management_api_client.ZitiEdgeManagement, id string) *identity.DeleteIdentityOK {
	deleteParams := &identity.DeleteIdentityParams{
		ID: id,
	}
	deleteParams.SetTimeout(30 * time.Second)
	resp, err := client.Identity.DeleteIdentity(deleteParams, nil)
	if err != nil {
		fmt.Println(err)
	}
	return resp
}

func getConfigByName(client *rest_management_api_client.ZitiEdgeManagement, name string) *rest_model.ConfigDetail {
	filter := "name=\"" + name + "\""
	configTypeParams := &api_client_config.ListConfigsParams{
		Filter:  &filter,
		Context: context.Background(),
	}
	resp, err := client.Config.ListConfigs(configTypeParams, nil)
	if err != nil {
		log.Errorf("Could not obtain a config with the name %s", name)
		fmt.Println(err)
	}
	if len(resp.GetPayload().Data) < 1 {
		return nil
	}
	return resp.GetPayload().Data[0]
}

func getConfigTypeByName(client *rest_management_api_client.ZitiEdgeManagement, name string) *rest_model.ConfigTypeDetail {
	filter := "name=\"" + name + "\""
	configTypeParams := &api_client_config.ListConfigTypesParams{
		Filter:  &filter,
		Context: context.Background(),
	}
	resp, err := client.Config.ListConfigTypes(configTypeParams, nil)
	if err != nil {
		log.Errorf("Could not obtain %s config type", name)
		fmt.Println(err)
	}
	return resp.GetPayload().Data[0]
}

func getIdentityByName(client *rest_management_api_client.ZitiEdgeManagement, name string) *rest_model.IdentityDetail {
	filter := "name=\"" + name + "\""
	params := &identity.ListIdentitiesParams{
		Filter:  &filter,
		Context: context.Background(),
	}
	params.SetTimeout(30 * time.Second)
	resp, err := client.Identity.ListIdentities(params, nil)
	if err != nil {
		log.Errorf("Could not obtain an ID for the identity named %s", name)
		fmt.Println(err)
		return nil
	}
	return resp.GetPayload().Data[0]
}

func getRouterByName(client *rest_management_api_client.ZitiEdgeManagement, name string) *rest_model.RouterDetail {
	filter := "name=\"" + name + "\""
	params := &router.ListRoutersParams{
		Filter:  &filter,
		Context: context.Background(),
	}
	params.SetTimeout(30 * time.Second)
	resp, err := client.Router.ListRouters(params, nil)
	if err != nil {
		log.Errorf("Could not obtain an detail for the router named %s", name)
		fmt.Println(err)
	}
	return resp.GetPayload().Data[0]
}

func getServiceByName(client *rest_management_api_client.ZitiEdgeManagement, name string) *rest_model.ServiceDetail {
	filter := "name=\"" + name + "\""
	params := &service.ListServicesParams{
		Filter:  &filter,
		Context: context.Background(),
	}
	params.SetTimeout(30 * time.Second)
	resp, err := client.Service.ListServices(params, nil)
	if err != nil {
		log.Errorf("Could not obtain an ID for the service named %s", name)
		fmt.Println(err)
	}
	if len(resp.GetPayload().Data) <= 0 {
		return nil
	}
	return resp.GetPayload().Data[0]
}

func getServicePolicyByName(client *rest_management_api_client.ZitiEdgeManagement, name string) *rest_model.ServicePolicyDetail {
	filter := "name=\"" + name + "\""
	params := &service_policy.ListServicePoliciesParams{
		Filter:  &filter,
		Context: context.Background(),
	}
	params.SetTimeout(30 * time.Second)
	resp, err := client.ServicePolicy.ListServicePolicies(params, nil)
	if err != nil {
		log.Errorf("Could not obtain an ID for the service named %s", name)
		fmt.Println(err)
	}
	if len(resp.GetPayload().Data) <= 0 {
		return nil
	}
	return resp.GetPayload().Data[0]
}

func getEdgeRouterByName(client *rest_management_api_client.ZitiEdgeManagement, name string) rest_model.EdgeRouterDetail {
	filterValues := "name=\"" + name + "\""
	listParams := &edge_router.ListEdgeRoutersParams{
		Filter: &filterValues,
	}
	listParams.SetTimeout(30 * time.Second)
	resp, err := client.EdgeRouter.ListEdgeRouters(listParams, nil)
	if err != nil {
		fmt.Println(err)
		log.Errorf("Could not get edge router filtered by name")
	}
	return *resp.GetPayload().Data[0]
}

func createEdgeRouterPolicy(client *rest_management_api_client.ZitiEdgeManagement, name string, roles rest_model.Roles) service_edge_router_policy.CreateServiceEdgeRouterPolicyCreated {
	defaultSemantic := rest_model.SemanticAllOf
	serp := &rest_model.ServiceEdgeRouterPolicyCreate{
		EdgeRouterRoles: roles,
		Name:            &name,
		Semantic:        &defaultSemantic,
		ServiceRoles:    roles,
	}
	serpParams := &service_edge_router_policy.CreateServiceEdgeRouterPolicyParams{
		Policy:  serp,
		Context: context.Background(),
	}
	serpParams.SetTimeout(30 * time.Second)
	resp, err := client.ServiceEdgeRouterPolicy.CreateServiceEdgeRouterPolicy(serpParams, nil)
	if err != nil {
		fmt.Println(err)
		log.Errorf("Could not create service edge router policy")
	}
	return *resp
}

func createInterceptV1ServiceConfig(client *rest_management_api_client.ZitiEdgeManagement, name string, protocols []string, addresses []string, portRangeLow int, portRangeHigh int) rest_model.CreateLocation {
	configTypeID := *getConfigTypeByName(client, entities.InterceptV1).ID
	interceptData := map[string]interface{}{
		"protocols": protocols,
		"addresses": addresses,
		"portRanges": []map[string]interface{}{
			{
				"low":  portRangeLow,
				"high": portRangeHigh,
			},
		},
	}
	confCreate := &rest_model.ConfigCreate{
		ConfigTypeID: &configTypeID,
		Data:         &interceptData,
		Name:         &name,
	}
	confParams := &api_client_config.CreateConfigParams{
		Config:  confCreate,
		Context: context.Background(),
	}
	confParams.SetTimeout(30 * time.Second)
	resp, err := client.Config.CreateConfig(confParams, nil)
	if err != nil {
		fmt.Println(err)
		log.Errorf("Could not create %s service config", entities.InterceptV1)
	}
	return *resp.GetPayload().Data
}

func createHostV1ServiceConfig(client *rest_management_api_client.ZitiEdgeManagement, name string, protocol string, address string, port int) rest_model.CreateLocation {
	hostID := getConfigTypeByName(client, entities.HostConfigV1).ID
	hostData := map[string]interface{}{
		"protocol": protocol,
		"address":  address,
		"port":     port,
	}
	confCreate := &rest_model.ConfigCreate{
		ConfigTypeID: hostID,
		Data:         &hostData,
		Name:         &name,
	}
	confParams := &api_client_config.CreateConfigParams{
		Config:  confCreate,
		Context: context.Background(),
	}
	confParams.SetTimeout(30 * time.Second)
	resp, err := client.Config.CreateConfig(confParams, nil)
	if err != nil {
		fmt.Println(err)
		log.Errorf("Could not create %s service config", entities.HostConfigV1)
	}
	return *resp.GetPayload().Data
}

func createService(client *rest_management_api_client.ZitiEdgeManagement, name string, serviceConfigs []string) rest_model.CreateLocation {
	encryptOn := true // Default
	serviceCreate := &rest_model.ServiceCreate{
		Configs:            serviceConfigs,
		EncryptionRequired: &encryptOn,
		Name:               &name,
	}
	serviceParams := &service.CreateServiceParams{
		Service: serviceCreate,
		Context: context.Background(),
	}
	serviceParams.SetTimeout(30 * time.Second)
	resp, err := client.Service.CreateService(serviceParams, nil)
	if err != nil {
		fmt.Println(err)
		log.Errorf("Failed to create " + name + " service")
	}
	return *resp.GetPayload().Data
}

func createServicePolicy(client *rest_management_api_client.ZitiEdgeManagement, name string, servType rest_model.DialBind, identityRoles rest_model.Roles, serviceRoles rest_model.Roles) rest_model.CreateLocation {

	defaultSemantic := rest_model.SemanticAllOf
	servicePolicy := &rest_model.ServicePolicyCreate{
		IdentityRoles: identityRoles,
		Name:          &name,
		Semantic:      &defaultSemantic,
		ServiceRoles:  serviceRoles,
		Type:          &servType,
	}
	params := &service_policy.CreateServicePolicyParams{
		Policy:  servicePolicy,
		Context: context.Background(),
	}
	params.SetTimeout(30 * time.Second)
	resp, err := client.ServicePolicy.CreateServicePolicy(params, nil)
	if err != nil {
		fmt.Println(err)
		log.Errorf("Failed to create the " + name + " service policy")
	}

	return *resp.GetPayload().Data
}

func getTerminatorCountByRouterName(client *rest_management_api_client.ZitiEdgeManagement, routerName string) int {
	filter := "router.name=\"" + routerName + "\""
	params := &terminator.ListTerminatorsParams{
		Filter:  &filter,
		Context: context.Background(),
	}

	resp, err := client.Terminator.ListTerminators(params, nil)
	if err != nil {
		fmt.Println(err)
		log.Errorf("An error occurred during terminator query")
	}

	return len(resp.GetPayload().Data)
}

func waitForTerminatorCountByRouterName(client *rest_management_api_client.ZitiEdgeManagement, routerName string, count int, timeout time.Duration) bool {
	startTime := time.Now()
	for {
		if getTerminatorCountByRouterName(client, routerName) == count {
			return true
		}
		if time.Since(startTime) >= timeout {
			break
		}
		time.Sleep(1 * time.Second)
	}
	return false
}

func deleteEdgeRouterPolicyByID(client *rest_management_api_client.ZitiEdgeManagement, id string) *edge_router_policy.DeleteEdgeRouterPolicyOK {
	deleteParams := &edge_router_policy.DeleteEdgeRouterPolicyParams{
		ID: id,
	}
	deleteParams.SetTimeout(30 * time.Second)
	resp, err := client.EdgeRouterPolicy.DeleteEdgeRouterPolicy(deleteParams, nil)
	if err != nil {
		fmt.Println(err)
	}
	return resp
}

func deleteServiceEdgeRouterPolicyByID(client *rest_management_api_client.ZitiEdgeManagement, id string) *service_edge_router_policy.DeleteServiceEdgeRouterPolicyOK {
	deleteParams := &service_edge_router_policy.DeleteServiceEdgeRouterPolicyParams{
		ID: id,
	}
	deleteParams.SetTimeout(30 * time.Second)
	resp, err := client.ServiceEdgeRouterPolicy.DeleteServiceEdgeRouterPolicy(deleteParams, nil)
	if err != nil {
		fmt.Println(err)
	}
	return resp
}

func deleteServiceConfigByID(client *rest_management_api_client.ZitiEdgeManagement, id string) *api_client_config.DeleteConfigOK {
	deleteParams := &api_client_config.DeleteConfigParams{
		ID: id,
	}
	deleteParams.SetTimeout(30 * time.Second)
	resp, err := client.Config.DeleteConfig(deleteParams, nil)
	if err != nil {
		fmt.Println(err)
	}
	return resp
}

func deleteServiceByID(client *rest_management_api_client.ZitiEdgeManagement, id string) *service.DeleteServiceOK {
	deleteParams := &service.DeleteServiceParams{
		ID: id,
	}
	deleteParams.SetTimeout(30 * time.Second)
	resp, err := client.Service.DeleteService(deleteParams, nil)
	if err != nil {
		fmt.Println(err)
	}
	return resp
}

func deleteServicePolicyByID(client *rest_management_api_client.ZitiEdgeManagement, id string) *service_policy.DeleteServicePolicyOK {
	deleteParams := &service_policy.DeleteServicePolicyParams{
		ID: id,
	}
	deleteParams.SetTimeout(30 * time.Second)
	resp, err := client.ServicePolicy.DeleteServicePolicy(deleteParams, nil)
	if err != nil {
		fmt.Println(err)
	}
	return resp
}

func connectToController(ctrlAddress string, adminUsername string, adminPassword string) (*rest_management_api_client.ZitiEdgeManagement, error) {
	// Authenticate with the controller
	caCerts, err := rest_util.GetControllerWellKnownCas(ctrlAddress)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	caPool := x509.NewCertPool()
	for _, ca := range caCerts {
		caPool.AddCert(ca)
	}
	client, err := rest_util.NewEdgeManagementClientWithUpdb(adminUsername, adminPassword, ctrlAddress, caPool)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	return client, nil
}

func waitForController(hostport string, timeout time.Duration) bool {
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	startTime := time.Now()
	for {
		resp, _ := http.Get(hostport)
		if resp != nil && resp.StatusCode == 200 {
			return true
		}
		if time.Since(startTime) >= timeout {
			break
		}
		time.Sleep(1 * time.Second)
		fmt.Println("Waiting for controller...")
	}
	return false
}
