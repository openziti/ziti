//go:build quickstart && (automated || manual)

package run

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

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
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func enrollIdentity(client *rest_management_api_client.ZitiEdgeManagement, identityID string) (*ziti.Config, error) {
	// Get the identity object
	params := &identity.DetailIdentityParams{
		Context: context.Background(),
		ID:      identityID,
	}
	params.SetTimeout(30 * time.Second)
	resp, err := client.Identity.DetailIdentity(params, nil)

	if err != nil {
		return nil, err
	}

	// Enroll the identity
	tkn, _, err := enroll.ParseToken(resp.GetPayload().Data.Enrollment.Ott.JWT)
	if err != nil {
		return nil, err
	}

	flags := enroll.EnrollmentFlags{
		Token:  tkn,
		KeyAlg: "RSA",
	}
	conf, err := enroll.Enroll(flags)

	if err != nil {
		return nil, err
	}

	return conf, nil
}

var zitiContext ziti.Context

func Dial(_ context.Context, _ string, addr string) (net.Conn, error) {
	s := strings.Split(addr, ":")[0] // will always get passed host:port
	return zitiContext.Dial(s)
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
	// TODO: OIDC Remove when OIDC issues are resolved
	zitiContext.(*ziti.ContextImpl).CtrlClt.SetAllowOidcDynamicallyEnabled(false)
	zitiTransport := http.DefaultTransport.(*http.Transport).Clone() // copy default transport
	zitiTransport.DialContext = Dial                                 //zitiDialContext.Dial
	zitiTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	return http.Client{Transport: zitiTransport}
}

// #################### Test Utils #############################

func createIdentity(t *testing.T, client *rest_management_api_client.ZitiEdgeManagement, name string,
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
		t.Fatalf("Failed to create the identity: %v", err)
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

func getConfigTypeByName(t *testing.T, client *rest_management_api_client.ZitiEdgeManagement, name string) *rest_model.ConfigTypeDetail {
	interceptFilter := "name=\"" + name + "\""
	configTypeParams := &api_client_config.ListConfigTypesParams{
		Filter:  &interceptFilter,
		Context: context.Background(),
	}
	interceptCTResp, err := client.Config.ListConfigTypes(configTypeParams, nil)
	if err != nil {
		t.Fatalf("Could not obtain %s config type", name)
	}
	return interceptCTResp.GetPayload().Data[0]
}

func getIdentityByName(t *testing.T, client *rest_management_api_client.ZitiEdgeManagement, name string) *rest_model.IdentityDetail {
	filter := "name=\"" + name + "\""
	params := &identity.ListIdentitiesParams{
		Filter:  &filter,
		Context: context.Background(),
	}
	params.SetTimeout(30 * time.Second)

	timeout := time.Now().Add(3 * time.Second)

	for {
		resp, err := client.Identity.ListIdentities(params, nil)
		if err == nil && len(resp.GetPayload().Data) > 0 {
			return resp.GetPayload().Data[0]
		}

		if time.Now().After(timeout) {
			t.Fatalf("Could not obtain an ID for the identity named %s after retries", name)
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func getServiceByName(t *testing.T, client *rest_management_api_client.ZitiEdgeManagement, name string) *rest_model.ServiceDetail {
	filter := "name=\"" + name + "\""
	params := &service.ListServicesParams{
		Filter:  &filter,
		Context: context.Background(),
	}
	params.SetTimeout(30 * time.Second)
	resp, err := client.Service.ListServices(params, nil)
	if err != nil {
		t.Fatalf("Could not obtain an ID for the service named %s", name)
	}
	return resp.GetPayload().Data[0]
}

func createInterceptV1ServiceConfig(t *testing.T, client *rest_management_api_client.ZitiEdgeManagement, name string, protocols []string, addresses []string, portRangeLow int, portRangeHigh int) rest_model.CreateLocation {
	configTypeID := *getConfigTypeByName(t, client, "intercept.v1").ID
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
		t.Fatal("Could not create intercept.v1 service config")
	}
	return *resp.GetPayload().Data
}

func createHostV1ServiceConfig(t *testing.T, client *rest_management_api_client.ZitiEdgeManagement, name string, protocol string, address string, port int) rest_model.CreateLocation {
	hostID := getConfigTypeByName(t, client, "host.v1").ID
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
		t.Fatal("Could not create host.v1 service config")
	}
	return *resp.GetPayload().Data
}

func createService(t *testing.T, client *rest_management_api_client.ZitiEdgeManagement, name string, serviceConfigs []string) rest_model.CreateLocation {
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
		t.Fatal("Failed to create " + name + " service")
	}
	return *resp.GetPayload().Data
}

func createServicePolicy(t *testing.T, client *rest_management_api_client.ZitiEdgeManagement, name string, servType rest_model.DialBind, identityRoles rest_model.Roles, serviceRoles rest_model.Roles) rest_model.CreateLocation {

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
		t.Fatal("Failed to create the " + name + " service policy")
	}

	return *resp.GetPayload().Data
}

func getTerminatorsByFilter(t *testing.T, client *rest_management_api_client.ZitiEdgeManagement, filter string) rest_model.TerminatorList {
	params := &terminator.ListTerminatorsParams{
		Filter:  &filter,
		Context: context.Background(),
	}

	resp, err := client.Terminator.ListTerminators(params, nil)
	if err != nil {
		t.Fatal("An error occurred during terminator query")
	}

	return resp.GetPayload().Data
}

func waitForTerminatorCount(t *testing.T, client *rest_management_api_client.ZitiEdgeManagement, filter string, count int, timeout time.Duration) bool {
	startTime := time.Now()
	var found int
	for {
		found = len(getTerminatorsByFilter(t, client, filter))
		if found == count {
			return true
		}
		if time.Since(startTime) >= timeout {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	log.Infof("waitForTerminatorCount found %d, expected %d for %s", found, count, filter)
	return false
}

func deleteServiceConfigByID(t *testing.T, client *rest_management_api_client.ZitiEdgeManagement, id string) *api_client_config.DeleteConfigOK {
	deleteParams := &api_client_config.DeleteConfigParams{
		ID: id,
	}
	deleteParams.SetTimeout(30 * time.Second)
	resp, err := client.Config.DeleteConfig(deleteParams, nil)
	if err != nil {
		t.Fatal("Failed to delete " + id + " config")
	}
	return resp
}

func deleteServiceEdgeRouterPolicyById(t *testing.T, client *rest_management_api_client.ZitiEdgeManagement, id string) *service_edge_router_policy.DeleteServiceEdgeRouterPolicyOK {
	deleteParams := &service_edge_router_policy.DeleteServiceEdgeRouterPolicyParams{
		ID: id,
	}
	deleteParams.SetTimeout(30 * time.Second)
	resp, err := client.ServiceEdgeRouterPolicy.DeleteServiceEdgeRouterPolicy(deleteParams, nil)
	if err != nil {
		t.Fatal("Failed to delete " + id + " config")
	}
	return resp
}

func deleteEdgeRouterPolicyById(t *testing.T, client *rest_management_api_client.ZitiEdgeManagement, id string) *edge_router_policy.DeleteEdgeRouterPolicyOK {
	deleteParams := &edge_router_policy.DeleteEdgeRouterPolicyParams{
		ID: id,
	}
	deleteParams.SetTimeout(30 * time.Second)
	resp, err := client.EdgeRouterPolicy.DeleteEdgeRouterPolicy(deleteParams, nil)
	if err != nil {
		t.Fatal("Failed to delete " + id + " config")
	}
	return resp
}

func deleteServiceByID(t *testing.T, client *rest_management_api_client.ZitiEdgeManagement, id string) *service.DeleteServiceOK {
	deleteParams := &service.DeleteServiceParams{
		ID: id,
	}
	deleteParams.SetTimeout(30 * time.Second)
	resp, err := client.Service.DeleteService(deleteParams, nil)
	if err != nil {
		t.Fatal("Failed to delete " + id + " config")
	}

	return resp
}

func deleteServicePolicyByID(t *testing.T, client *rest_management_api_client.ZitiEdgeManagement, id string) *service_policy.DeleteServicePolicyOK {
	deleteParams := &service_policy.DeleteServicePolicyParams{
		ID: id,
	}
	deleteParams.SetTimeout(30 * time.Second)
	resp, err := client.ServicePolicy.DeleteServicePolicy(deleteParams, nil)
	if err != nil {
		t.Fatal("Failed to delete " + id + " config")
	}

	return resp
}

// in order to share test code between quickstart_test.go and quickstart_test_manual.go, this function had to be
// created. I couldn't find a way to share the code any other way. Happy to learn a better way!
func performQuickstartTest(t *testing.T) error {
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
	bindHostAddress := os.Getenv("ZITI_TEST_BIND_ADDRESS")
	// if bindHostAddress is not set, use the advertised address of the client API; this is useful if the client API is
	// behind a load balancer that routes by SNI because the request's server name indication in the TLS ClientHello
	// usually reflects the service's intercept address, not the address the load balancer is expecting
	if bindHostAddress == "" {
		bindHostAddress = advAddy
	}
	hostingRouterName := erName
	dialAddress := "simple.web.smoke.test"
	dialPort := 80
	serviceName := "basic.web.smoke.test.service"
	wd, _ := os.Getwd()

	log.Infof("connecting user: %s to %s", zitiAdminUsername, ctrlAddress)
	// Authenticate with the controller
	caCerts, err := rest_util.GetControllerWellKnownCas(ctrlAddress)
	if err != nil {
		return err
	}
	caPool := x509.NewCertPool()
	for _, ca := range caCerts {
		caPool.AddCert(ca)
	}
	var client *rest_management_api_client.ZitiEdgeManagement
	for i := 0; ; i++ {
		c, err := rest_util.NewEdgeManagementClientWithUpdb(zitiAdminUsername, zitiAdminPassword, ctrlAddress, caPool)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		log.Infof("created authenticated client after %d attempts (%f seconds)", i+1, float64(i)*0.5)
		client = c
		break
	}

	if client == nil {
		return fmt.Errorf("Failed to connect to Ziti")
	}

	// Create the tester identity
	ident := createIdentity(t, client, testerUsername, rest_model.IdentityTypeUser, false)
	defer func() { _ = deleteIdentityByID(client, ident.GetPayload().Data.ID) }()

	// Enroll the identity
	identConfig, err := enrollIdentity(client, ident.Payload.Data.ID)
	if err != nil {
		return err
	}

	// Create a json config file
	output, err := os.Create(testerUsername + ".json")
	if err != nil {
		return err
	}
	defer func() {
		_ = output.Close()
		err = os.Remove(testerUsername + ".json")
		if err != nil {
			logrus.Warnf("Failed to delete json config file: %s", err)
		}
	}()
	enc := json.NewEncoder(output)
	enc.SetEscapeHTML(false)
	encErr := enc.Encode(&identConfig)
	if encErr != nil {
		return err
	}

	// Verify all identities can access all routers
	allIdRoles := rest_model.Roles{"#all"}
	serpParams := service_edge_router_policy.NewCreateServiceEdgeRouterPolicyParams()
	serpParams.Policy = &rest_model.ServiceEdgeRouterPolicyCreate{
		ServiceRoles:    allIdRoles,
		EdgeRouterRoles: allIdRoles,
		Name:            toPtr("all-serps"),
		Semantic:        toPtr(rest_model.SemanticAnyOf),
	}
	serpParams.SetTimeout(30 * time.Second)
	serp, err := client.ServiceEdgeRouterPolicy.CreateServiceEdgeRouterPolicy(serpParams, nil)
	if err != nil {
		return err
	}
	defer func() { _ = deleteServiceEdgeRouterPolicyById(t, client, serp.Payload.Data.ID) }()

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
		return err
	}
	defer func() { _ = deleteEdgeRouterPolicyById(t, client, erp.Payload.Data.ID) }()

	// Allow dialing the service using an intercept config (intercept because we'll be using the SDK)
	dialSvcConfig := createInterceptV1ServiceConfig(t, client, "basic.smoke.dial", []string{"tcp"}, []string{dialAddress}, dialPort, dialPort)
	defer func() { _ = deleteServiceConfigByID(t, client, dialSvcConfig.ID) }()

	// Provide host config for the hostname
	bindPort, _ := strconv.Atoi(advPort)
	bindSvcConfig := createHostV1ServiceConfig(t, client, "basic.smoke.bind", "tcp", bindHostAddress, bindPort)
	defer func() { _ = deleteServiceConfigByID(t, client, bindSvcConfig.ID) }()

	// Create a service that "links" the dial and bind configs
	createService(t, client, serviceName, []string{bindSvcConfig.ID, dialSvcConfig.ID})

	// Create a service policy to allow the router to host the web test service
	fmt.Println("finding hostingRouterName: ", hostingRouterName)
	hostRouterIdent := getIdentityByName(t, client, hostingRouterName)
	webTestService := getServiceByName(t, client, serviceName)
	defer func() { _ = deleteServiceByID(t, client, *webTestService.ID) }()
	bindSP := createServicePolicy(t, client, "basic.web.smoke.test.service.bind", rest_model.DialBindBind, rest_model.Roles{"@" + *hostRouterIdent.ID}, rest_model.Roles{"@" + *webTestService.ID})
	defer func() { _ = deleteServicePolicyByID(t, client, bindSP.ID) }()

	// Create a service policy to allow tester to dial the service
	fmt.Println("finding testerUsername: ", testerUsername)
	testerIdent := getIdentityByName(t, client, testerUsername)
	dialSP := createServicePolicy(t, client, "basic.web.smoke.test.service.dial", rest_model.DialBindDial, rest_model.Roles{"@" + *testerIdent.ID}, rest_model.Roles{"@" + *webTestService.ID})
	defer func() { _ = deleteServicePolicyByID(t, client, dialSP.ID) }()

	// Test connectivity with private edge router, wait some time for the terminator to be created
	terminatorFilter := "service.name=\"" + serviceName + "\""
	termCntReached := waitForTerminatorCount(t, client, terminatorFilter, 1, 30*time.Second)
	if !termCntReached {
		t.Fatal("Unable to detect a terminator for the edge router")
	}
	helloUrl := fmt.Sprintf("https://%s:%d", serviceName, dialPort)
	log.Infof("created url: %s", helloUrl)
	httpClient := createZitifiedHttpClient(wd + "/" + testerUsername + ".json")

	resp, e := httpClient.Get(helloUrl)
	if e != nil {
		return err
	}

	assert.Equal(t, 200, resp.StatusCode, fmt.Sprintf("Expected successful HTTP status code 200, received %d instead", resp.StatusCode))
	body, _ := io.ReadAll(resp.Body)
	strBody := string(body)
	assert.Contains(t, strBody, "\"path\":\"/oidc\"")
	fmt.Println(strBody)
	return nil
}

func toPtr[T any](in T) *T {
	return &in
}
