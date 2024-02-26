//go:build quickstart && (automated || manual)

package edge

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_management_api_client"
	"github.com/openziti/edge-api/rest_management_api_client/edge_router_policy"
	"github.com/openziti/edge-api/rest_management_api_client/service_edge_router_policy"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/ziti/ziti/cmd/testutil"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"io"
	"os"
	"strconv"
	"testing"
	"time"
)

// in order to share test code between quickstart_test.go and quickstart_test_manual.go, this function had to be
// created. I couldn't find a way to share the code any other way. Happy to learn a better way!
func performQuickstartTest(t *testing.T) {
	dialAddress := "web.test.ziti"

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
	dialPort := 1280

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

	t.Run("Basic Web Test", func(t *testing.T) {
		serviceName := testutil.GenerateRandomName("basic.web.smoke.test.service")

		// Allow dialing the service using an intercept config (intercept because we'll be using the SDK)
		dialSvcConfig := createInterceptV1ServiceConfig(client, "basic.smoke.dial", []string{"tcp"}, []string{dialAddress}, dialPort, dialPort)

		// Provide host config for the hostname
		bindPort, _ := strconv.Atoi(advPort)
		bindSvcConfig := createHostV1ServiceConfig(client, "basic.smoke.bind", "tcp", advAddy, bindPort)

		// Create a service that "links" the dial and bind configs
		createService(client, serviceName, []string{bindSvcConfig.ID, dialSvcConfig.ID})

		// Create a service policy to allow the router to host the web test service
		fmt.Println("finding hostingRouterName: ", hostingRouterName)
		hostRouterIdent := testutil.GetIdentityByName(client, hostingRouterName)
		webTestService := testutil.GetServiceByName(client, serviceName)
		bindSP := testutil.CreateServicePolicy(client, "basic.web.smoke.test.service.bind", rest_model.DialBindBind, rest_model.Roles{"@" + *hostRouterIdent.ID}, rest_model.Roles{"@" + *webTestService.ID})

		// Create a service policy to allow tester to dial the service
		fmt.Println("finding testerUsername: ", testerUsername)
		testerIdent := testutil.GetIdentityByName(client, testerUsername)
		dialSP := testutil.CreateServicePolicy(client, "basic.web.smoke.test.service.dial", rest_model.DialBindDial, rest_model.Roles{"@" + *testerIdent.ID}, rest_model.Roles{"@" + *webTestService.ID})

		testServiceEndpoint(t, client, hostingRouterName, serviceName, dialPort, testerUsername)

		// Cleanup
		testutil.DeleteServiceConfigByID(client, dialSvcConfig.ID)
		testutil.DeleteServiceConfigByID(client, bindSvcConfig.ID)
		testutil.DeleteServiceByID(client, *webTestService.ID)
		testutil.DeleteServicePolicyByID(client, bindSP.ID)
		testutil.DeleteServicePolicyByID(client, dialSP.ID)
	})

	t.Run("ZES Full Input", createZESTestFunc(t, fmt.Sprintf("tcp:%s:%s", advAddy, advPort), client, dialAddress, dialPort, testutil.GenerateRandomName("ZESTest1"), hostingRouterName, testerUsername))
	t.Run("ZES Only Port", createZESTestFunc(t, advPort, client, dialAddress, dialPort, testutil.GenerateRandomName("ZESTest2"), hostingRouterName, testerUsername))
	t.Run("ZES No Protocol", createZESTestFunc(t, fmt.Sprintf("%s:%s", advAddy, advPort), client, dialAddress, dialPort, testutil.GenerateRandomName("ZESTest3"), hostingRouterName, testerUsername))

	t.Run("ZES Multiple Calls", func(t *testing.T) {
		service1Name := testutil.GenerateRandomName("ZESTestMultiple1")
		service2Name := testutil.GenerateRandomName("ZESTestMultiple2")
		service1BindConfName := service1Name + ".host.v1"
		service2BindConfName := service2Name + ".host.v1"
		service1DialConfName := service1Name + ".intercept.v1"
		service2DialConfName := service2Name + ".intercept.v1"
		service1BindSPName := service1Name + ".bind"
		service2BindSPName := service2Name + ".bind"
		service1DialSPName := service1Name + ".dial"
		service2DialSPName := service2Name + ".dial"
		dialAddress1 := "dialAddress1"
		dialAddress2 := "dialAddress2"
		params := fmt.Sprintf("tcp:%s:%s", advAddy, advPort)

		// Run ZES once
		zes := NewSecureCmd(os.Stdout, os.Stderr)
		zes.SetArgs([]string{
			service1Name,
			params,
			fmt.Sprintf("--endpoint=%s", dialAddress1),
		})
		err := zes.Execute()
		if err != nil {
			fmt.Printf("Error: %s", err)
		}

		// Run ZES twice
		zes = NewSecureCmd(os.Stdout, os.Stderr)
		zes.SetArgs([]string{
			service2Name,
			params,
			fmt.Sprintf("--endpoint=%s", dialAddress2),
		})
		err = zes.Execute()
		if err != nil {
			fmt.Printf("Error: %s", err)
		}

		// Check network components for validity
		// Confirm the four configs exist
		service1BindConf := testutil.GetServiceConfigByName(client, service1BindConfName)
		service2BindConf := testutil.GetServiceConfigByName(client, service2BindConfName)
		service1DialConf := testutil.GetServiceConfigByName(client, service1DialConfName)
		service2DialConf := testutil.GetServiceConfigByName(client, service2DialConfName)

		assert.Equal(t, service1BindConfName, *service1BindConf.Name)
		assert.Equal(t, service2BindConfName, *service2BindConf.Name)
		assert.Equal(t, service1DialConfName, *service1DialConf.Name)
		assert.Equal(t, service2DialConfName, *service2DialConf.Name)

		// Confirm the two services exist
		service1 := testutil.GetServiceByName(client, service1Name)
		service2 := testutil.GetServiceByName(client, service2Name)

		assert.Equal(t, service1Name, *service1.Name)
		assert.Equal(t, service2Name, *service2.Name)

		// Confirm the four service policies exist
		service1BindPol := testutil.GetServicePolicyByName(client, service1BindSPName)
		service2BindPol := testutil.GetServicePolicyByName(client, service2BindSPName)
		service1DialPol := testutil.GetServicePolicyByName(client, service1DialSPName)
		service2DialPol := testutil.GetServicePolicyByName(client, service2DialSPName)

		assert.Equal(t, service1BindSPName, *service1BindPol.Name)
		assert.Equal(t, service2BindSPName, *service2BindPol.Name)
		assert.Equal(t, service1DialSPName, *service1DialPol.Name)
		assert.Equal(t, service2DialSPName, *service2DialPol.Name)

		// Cleanup
		testutil.DeleteServiceConfigByID(client, *service1BindConf.ID)
		testutil.DeleteServiceConfigByID(client, *service2BindConf.ID)
		testutil.DeleteServiceConfigByID(client, *service1DialConf.ID)
		testutil.DeleteServiceConfigByID(client, *service2DialConf.ID)
		testutil.DeleteServiceByID(client, *service1.ID)
		testutil.DeleteServiceByID(client, *service2.ID)
		testutil.DeleteServicePolicyByID(client, *service1BindPol.ID)
		testutil.DeleteServicePolicyByID(client, *service2BindPol.ID)
		testutil.DeleteServicePolicyByID(client, *service1DialPol.ID)
		testutil.DeleteServicePolicyByID(client, *service2DialPol.ID)
	})
}

func createZESTestFunc(t *testing.T, params string, client *rest_management_api_client.ZitiEdgeManagement, dialAddress string, dialPort int, serviceName string, hostingRouterName string, testerUsername string) func(*testing.T) {
	return func(t *testing.T) {
		// Run ziti edge secure with the controller edge details
		zes := NewSecureCmd(os.Stdout, os.Stderr)
		zes.SetArgs([]string{
			serviceName,
			params,
			fmt.Sprintf("--endpoint=%s", dialAddress),
		})
		err := zes.Execute()
		if err != nil {
			fmt.Printf("Error: %s", err)
		}

		// Update the router and user with the appropriate attributes
		zeui := newUpdateIdentityCmd(os.Stdout, os.Stderr)
		zeui.SetArgs([]string{
			hostingRouterName,
			fmt.Sprintf("-a=%s.%s", serviceName, "servers"),
		})
		err = zeui.Execute()
		if err != nil {
			fmt.Printf("Error: %s", err)
		}

		zeui = newUpdateIdentityCmd(os.Stdout, os.Stderr)
		zeui.SetArgs([]string{
			testerUsername,
			fmt.Sprintf("-a=%s.%s", serviceName, "clients"),
		})
		err = zeui.Execute()
		if err != nil {
			fmt.Printf("Error: %s", err)
		}

		testServiceEndpoint(t, client, hostingRouterName, serviceName, dialPort, testerUsername)

		// Cleanup
		serviceBindConfName := serviceName + ".host.v1"
		serviceDialConfName := serviceName + ".intercept.v1"
		serviceBindSPName := serviceName + ".bind"
		serviceDialSPName := serviceName + ".dial"
		testutil.DeleteServiceConfigByID(client, *testutil.GetServiceConfigByName(client, serviceBindConfName).ID)
		testutil.DeleteServiceConfigByID(client, *testutil.GetServiceConfigByName(client, serviceDialConfName).ID)
		testutil.DeleteServiceByID(client, *getServiceByName(client, serviceName).ID)
		testutil.DeleteServicePolicyByID(client, *getServicePolicyByName(client, serviceBindSPName).ID)
		testutil.DeleteServicePolicyByID(client, *getServicePolicyByName(client, serviceDialSPName).ID)
	}
}

func testServiceEndpoint(t *testing.T, client *rest_management_api_client.ZitiEdgeManagement, hostingRouterName string, serviceName string, dialPort int, testerUsername string) {
	// Test connectivity with private edge router, wait some time for the terminator to be created
	currentCount := testutil.GetTerminatorCountByRouterName(client, hostingRouterName)
	termCntReached := testutil.WaitForTerminatorCountByRouterName(client, hostingRouterName, currentCount+1, 30*time.Second)
	if !termCntReached {
		fmt.Println("Unable to detect a terminator for the edge router")
	}
	helloUrl := fmt.Sprintf("https://%s:%d", serviceName, dialPort)
	log.Infof("created url: %s", helloUrl)
	wd, _ := os.Getwd()
	httpClient := testutil.CreateZitifiedHttpClient(wd + "/" + testerUsername + ".json")

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
