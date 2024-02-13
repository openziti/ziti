//go:build quickstart && (automated || manual)

package edge

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_management_api_client"
	api_client_config "github.com/openziti/edge-api/rest_management_api_client/config"
	"github.com/openziti/edge-api/rest_management_api_client/edge_router_policy"
	"github.com/openziti/edge-api/rest_management_api_client/identity"
	"github.com/openziti/edge-api/rest_management_api_client/service"
	"github.com/openziti/edge-api/rest_management_api_client/service_edge_router_policy"
	"github.com/openziti/edge-api/rest_management_api_client/service_policy"
	"github.com/openziti/edge-api/rest_management_api_client/terminator"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/enroll"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func enrollIdentity(client *rest_management_api_client.ZitiEdgeManagement, identityID string) *ziti.Config {
	// Get the identity object
	params := &identity.DetailIdentityParams{
		Context: context.Background(),
		ID:      identityID,
	}
	params.SetTimeout(30 * time.Second)
	resp, err := client.Identity.DetailIdentity(params, nil)

	if err != nil {
		log.Fatal(err)
	}

	// Enroll the identity
	tkn, _, err := enroll.ParseToken(resp.GetPayload().Data.Enrollment.Ott.JWT)
	if err != nil {
		log.Fatal(err)
	}

	flags := enroll.EnrollmentFlags{
		Token:  tkn,
		KeyAlg: "RSA",
	}
	conf, err := enroll.Enroll(flags)

	if err != nil {
		log.Fatal(err)
	}

	return conf
}

var zitiContext ziti.Context

func Dial(_ context.Context, _ string, addr string) (net.Conn, error) {
	service := strings.Split(addr, ":")[0] // will always get passed host:port
	return zitiContext.Dial(service)
}

func createZitifiedHttpClient(idFile string) http.Client {
	cfg, err := ziti.NewConfigFromFile(idFile)
	if err != nil {
		panic(err)
	}
	zitiContext, err = ziti.NewContext(cfg)
	if err != nil {
		panic(err)
	}
	zitiTransport := http.DefaultTransport.(*http.Transport).Clone() // copy default transport
	zitiTransport.DialContext = Dial                                 //zitiDialContext.Dial
	zitiTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	return http.Client{Transport: zitiTransport}
}

// #################### Test Utils #############################

func createIdentity(client *rest_management_api_client.ZitiEdgeManagement, name string,
	identType rest_model.IdentityType, isAdmin bool) *identity.CreateIdentityCreated {
	i := &rest_model.IdentityCreate{
		Enrollment: &rest_model.IdentityCreateEnrollment{
			Ott: true,
		},
		IsAdmin:                   &isAdmin,
		Name:                      &name,
		RoleAttributes:            nil,
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
		log.Fatal("Failed to create the identity")
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

func getConfigTypeByName(client *rest_management_api_client.ZitiEdgeManagement, name string) *rest_model.ConfigTypeDetail {
	interceptFilter := "name=\"" + name + "\""
	configTypeParams := &api_client_config.ListConfigTypesParams{
		Filter:  &interceptFilter,
		Context: context.Background(),
	}
	interceptCTResp, err := client.Config.ListConfigTypes(configTypeParams, nil)
	if err != nil {
		log.Fatalf("Could not obtain %s config type", name)
		fmt.Println(err)
	}
	return interceptCTResp.GetPayload().Data[0]
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
		log.Fatalf("Could not obtain an ID for the identity named %s", name)
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
		log.Fatalf("Could not obtain an ID for the service named %s", name)
		fmt.Println(err)
	}
	return resp.GetPayload().Data[0]
}

func createInterceptV1ServiceConfig(client *rest_management_api_client.ZitiEdgeManagement, name string, protocols []string, addresses []string, portRangeLow int, portRangeHigh int) rest_model.CreateLocation {
	configTypeID := *getConfigTypeByName(client, "intercept.v1").ID
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
		log.Fatal("Could not create intercept.v1 service config")
	}
	return *resp.GetPayload().Data
}

func createHostV1ServiceConfig(client *rest_management_api_client.ZitiEdgeManagement, name string, protocol string, address string, port int) rest_model.CreateLocation {
	hostID := getConfigTypeByName(client, "host.v1").ID
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
		log.Fatal("Could not create host.v1 service config")
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
		log.Fatal("Failed to create " + name + " service")
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
		log.Fatal("Failed to create the " + name + " service policy")
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
		log.Fatal("An error occurred during terminator query")
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
		time.Sleep(100 * time.Millisecond)
	}
	return false
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

func deleteServiceEdgeRouterPolicyById(client *rest_management_api_client.ZitiEdgeManagement, id string) *service_edge_router_policy.DeleteServiceEdgeRouterPolicyOK {
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

func deleteEdgeRouterPolicyById(client *rest_management_api_client.ZitiEdgeManagement, id string) *edge_router_policy.DeleteEdgeRouterPolicyOK {
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

// in order to share test code between quickstart_test.go and quickstart_test_manual.go, this function had to be
// created. I couldn't find a way to share the code any other way. Happy to learn a better way!
func performQuickstartTest(t *testing.T) {
	// Wait for the controller to become available
	zitiAdminUsername := os.Getenv("ZITI_USER")
	if zitiAdminUsername == "" {
		zitiAdminUsername = "admin"
	}
	zitiAdminPassword := os.Getenv("ZITI_PWD")
	if zitiAdminPassword == "" {
		zitiAdminPassword = "admin"
	}
	testerUsername := "gotester"
	advAddy := os.Getenv("ZITI_CTRL_EDGE_ADVERTISED_ADDRESS")
	advPort := os.Getenv("ZITI_CTRL_EDGE_ADVERTISED_PORT")
	if advAddy == "" {
		advAddy = "ziti-edge-controller"
	}
	if advPort == "" {
		advPort = "1280"
	}
	erName := os.Getenv("ZITI_ROUTER_NAME")
	if erName == "" {
		erName = "ziti-edge-router"
	}

	ctrlAddress := "https://" + advAddy + ":" + advPort
	//bindHostAddress := os.Getenv("ZITI_QUICKSTART_TEST_ADDRESS")
	//if bindHostAddress == "" {
	//	bindHostAddress = ctrlAddress
	//}
	hostingRouterName := erName
	dialAddress := "simple.web.smoke.test"
	dialPort := 80
	serviceName := "basic.web.smoke.test.service"
	wd, _ := os.Getwd()

	log.Infof("connecting user: %s to %s", zitiAdminUsername, ctrlAddress)
	// Authenticate with the controller
	caCerts, err := rest_util.GetControllerWellKnownCas(ctrlAddress)
	if err != nil {
		log.Fatal(err)
	}
	caPool := x509.NewCertPool()
	for _, ca := range caCerts {
		caPool.AddCert(ca)
	}
	client, err := rest_util.NewEdgeManagementClientWithUpdb(zitiAdminUsername, zitiAdminPassword, ctrlAddress, caPool)
	if err != nil {
		log.Fatal(err)
	}

	// Create the tester identity
	ident := createIdentity(client, testerUsername, rest_model.IdentityTypeUser, false)
	defer func() { _ = deleteIdentityByID(client, ident.GetPayload().Data.ID) }()

	// Enroll the identity
	identConfig := enrollIdentity(client, ident.Payload.Data.ID)

	// Create a json config file
	output, err := os.Create(testerUsername + ".json")
	if err != nil {
		fmt.Println(err)
		log.Fatal("Failed to create output config file")
	}
	defer func() {
		_ = output.Close()
		err = os.Remove(testerUsername + ".json")
		if err != nil {
			fmt.Println(err)
			log.Fatal("Failed to delete json config file")
		}
	}()
	enc := json.NewEncoder(output)
	enc.SetEscapeHTML(false)
	encErr := enc.Encode(&identConfig)
	if encErr != nil {
		fmt.Println(err)
		log.Fatal("Failed to generate encoded output")
	}

	// Verify all identities can access all routers
	allIdRoles := rest_model.Roles{"#all"}
	serpParams := service_edge_router_policy.NewCreateServiceEdgeRouterPolicyParams()
	serpParams.Policy = &rest_model.ServiceEdgeRouterPolicyCreate{
		ServiceRoles:    allIdRoles,
		EdgeRouterRoles: allIdRoles,
		Name:            toPtr(uuid.NewString()),
		Semantic:        toPtr(rest_model.SemanticAnyOf),
	}
	serpParams.SetTimeout(30 * time.Second)
	serp, err := client.ServiceEdgeRouterPolicy.CreateServiceEdgeRouterPolicy(serpParams, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = deleteServiceEdgeRouterPolicyById(client, serp.Payload.Data.ID) }()

	p := &rest_model.EdgeRouterPolicyCreate{
		EdgeRouterRoles: allIdRoles,
		IdentityRoles:   allIdRoles,
		Name:            toPtr("all-erps"),
		Semantic:        toPtr(rest_model.SemanticAnyOf),
		Tags:            nil,
	}
	erpParams := &edge_router_policy.CreateEdgeRouterPolicyParams{
		Policy: p,
	}
	erpParams.SetTimeout(30 * time.Second)
	erp, err := client.EdgeRouterPolicy.CreateEdgeRouterPolicy(erpParams, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = deleteEdgeRouterPolicyById(client, erp.Payload.Data.ID) }()

	// Allow dialing the service using an intercept config (intercept because we'll be using the SDK)
	dialSvcConfig := createInterceptV1ServiceConfig(client, "basic.smoke.dial", []string{"tcp"}, []string{dialAddress}, dialPort, dialPort)
	defer func() { _ = deleteServiceConfigByID(client, dialSvcConfig.ID) }()

	// Provide host config for the hostname
	bindPort, _ := strconv.Atoi(advPort)
	bindSvcConfig := createHostV1ServiceConfig(client, "basic.smoke.bind", "tcp", advAddy, bindPort)
	defer func() { _ = deleteServiceConfigByID(client, bindSvcConfig.ID) }()

	// Create a service that "links" the dial and bind configs
	createService(client, serviceName, []string{bindSvcConfig.ID, dialSvcConfig.ID})

	// Create a service policy to allow the router to host the web test service
	fmt.Println("finding hostingRouterName: ", hostingRouterName)
	hostRouterIdent := getIdentityByName(client, hostingRouterName)
	webTestService := getServiceByName(client, serviceName)
	defer func() { _ = deleteServiceByID(client, *webTestService.ID) }()
	bindSP := createServicePolicy(client, "basic.web.smoke.test.service.bind", rest_model.DialBindBind, rest_model.Roles{"@" + *hostRouterIdent.ID}, rest_model.Roles{"@" + *webTestService.ID})
	defer func() { _ = deleteServicePolicyByID(client, bindSP.ID) }()

	// Create a service policy to allow tester to dial the service
	fmt.Println("finding testerUsername: ", testerUsername)
	testerIdent := getIdentityByName(client, testerUsername)
	dialSP := createServicePolicy(client, "basic.web.smoke.test.service.dial", rest_model.DialBindDial, rest_model.Roles{"@" + *testerIdent.ID}, rest_model.Roles{"@" + *webTestService.ID})
	defer func() { _ = deleteServicePolicyByID(client, dialSP.ID) }()

	// Test connectivity with private edge router, wait some time for the terminator to be created
	currentCount := getTerminatorCountByRouterName(client, hostingRouterName)
	termCntReached := waitForTerminatorCountByRouterName(client, hostingRouterName, currentCount+1, 30*time.Second)
	if !termCntReached {
		fmt.Println("Unable to detect a terminator for the edge router")
	}
	helloUrl := fmt.Sprintf("https://%s:%d", serviceName, dialPort)
	log.Infof("created url: %s", helloUrl)
	httpClient := createZitifiedHttpClient(wd + "/" + testerUsername + ".json")

	resp, e := httpClient.Get(helloUrl)
	if e != nil {
		panic(e)
	}

	assert.Equal(t, 200, resp.StatusCode, fmt.Sprintf("Expected successful HTTP status code 200, received %d instead", resp.StatusCode))
	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))
}

func toPtr[T any](in T) *T {
	return &in
}
