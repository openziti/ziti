package test

/*
These tests are designed to test an OpenZiti network that was generated with the Express Install script. The default
values will test the quickstart-test docker container, use dockerExpressInstall.sh to establish the default network.
Some of these tests will require that ziti-edge-controller be added to your hosts file if running the docker container
express install.
*/

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/edge/tunnel/entities"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"
)

// Adjust these values as needed if not using the default test environment
var IdentityPrefix = "@"
var AttributePrefix = "#"
var ExpressCtrlAddress = "https://ziti-edge-controller:1280"
var ExpressAdminUsername = "admin"
var ExpressAdminPassword = "kvqzGyUj2oJ3Jmql_1YgM4ao61tVg-vw"
var ExpressCtrlTimeout = 10 * time.Second
var ExpressEdgeRouterName = "ziti-edge-router"
var ExpressControllerName = "ziti-edge-controller"

/*
Test that the controller is live and responding
*/
func TestController(t *testing.T) {
	assert.True(t, waitForController(ExpressCtrlAddress, ExpressCtrlTimeout), "The controller could not be reached")
}

/*
Test that the expected router is present and online
*/
func TestRouter(t *testing.T) {
	// Wait for the controller to be available
	result := waitForController(ExpressCtrlAddress, ExpressCtrlTimeout)
	if !result {
		log.Errorf("Controller cannot be reached")
	}

	// Log into the controller
	client, err := connectToController(ExpressCtrlAddress, ExpressAdminUsername, ExpressAdminPassword)
	require.Nilf(t, err, "An error occurred attempting to connect with the controller.\n%s", err)

	// Query routers
	erDetail := getEdgeRouterByName(client, ExpressEdgeRouterName)

	// Make sure it is online
	assert.Truef(t, *erDetail.IsOnline, "Expected Edge Router %s to be online but was not", ExpressEdgeRouterName)
}

/*
This is a manually run test to confirm expected values are appearing in the .env file that is generated after the
quickstart script is run.
*/
func TestEnvFileContents(t *testing.T) {
	containerName := "quickstart-test"
	envFileName := "localhost.env"
	expectedValues := []string{
		"export ZITI_EDGE_ROUTER_RAWNAME=\"" + ExpressEdgeRouterName + "\"",
		"export ZITI_EDGE_CONTROLLER_RAWNAME=\"" + ExpressControllerName + "\"",
		"export ZITI_HOME_OS_SPECIFIC=\"/persistent\"",
		"export ZITI_HOME=\"/persistent\"",
		"export ZITI_BIN_DIR=\"/var/openziti/ziti-bin\"",
		"export ZITI_EDGE_CTRL_ADVERTISED=\"" + ExpressControllerName + ":1280\"",
		"export ZITI_USER=\"" + ExpressAdminUsername + "\"",
		"export ZITI_PWD=\"" + ExpressAdminPassword + "\"",
		"export ZITI_PKI_OS_SPECIFIC=\"/persistent/pki\"",
		"export ZITI_EDGE_CONTROLLER_ROOTCA_NAME=\"" + ExpressControllerName + "-root-ca\"",
		"export ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME=\"" + ExpressControllerName + "-intermediate\"",
	}

	cpString := containerName + ":/persistent/" + envFileName
	cmd := exec.Command("docker", "cp", cpString, ".")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error copying env file: %s\n", err)
	}

	// Check env file for expected values
	file, err := os.Open(envFileName)
	if err != nil {
		log.Error(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		for i := 0; i < len(expectedValues); i++ {
			if expectedValues[i] == scanner.Text() {
				// If found, stop looking for it by removing it from the array
				expectedValues = append(expectedValues[:i], expectedValues[i+1:]...)
			}
		}
	}

	// Anything still in the array wasn't found
	if len(expectedValues) > 0 {
		for i := 0; i < len(expectedValues); i++ {
			fmt.Printf("Could not find expected value (%s)\n", expectedValues[i])
		}
	}
	if err = scanner.Err(); err != nil {
		log.Error(err)
	}

	// Test Cleanup
	if file != nil {
		err = os.Remove(file.Name())
		if err != nil {
			log.Errorf("Error removing test file %s", file.Name())
		}
	}

	// Test
	assert.Equal(t, 0, len(expectedValues), "Not all expected env file values were found")
}

/*
Test that the REST API service is configured properly
*/
func TestRestAPIService(t *testing.T) {

	RestServiceBindConfigName := "ziti.rest.bind"
	RestServiceDialConfigName := "ziti.rest.dial"
	RestServiceDialPort := 443
	RestServiceName := "ziti.rest.service"
	RestServiceDialAttribute := "quickstart.rest.user"
	RestServiceBindAttribute := "ziti.rest.host"
	RestServiceBindPolicyName := "ziti.rest.service.bind"
	RestServiceDialPolicyName := "ziti.rest.service.dial"

	// Wait for the controller to be available
	result := waitForController(ExpressCtrlAddress, ExpressCtrlTimeout)
	if !result {
		log.Errorf("Controller cannot be reached")
	}

	// Log into the controller
	client, err := connectToController(ExpressCtrlAddress, ExpressAdminUsername, ExpressAdminPassword)
	require.Nilf(t, err, "An error occurred attempting to connect with the controller.\n%s", err)

	/* ----Test Router Identity Attributes---- */

	t.Run("TestRESTAPIEdgeRouterAttributes", func(t *testing.T) {
		erIdent := getIdentityByName(client, ExpressEdgeRouterName)
		// Make sure the router identity has the proper attribute
		require.NotNilf(t, erIdent, "Edge Router Identity could not be found, cannot test for identity attributes")
		require.NotNilf(t, erIdent.RoleAttributes, "Expected to find Role Attribute <%s> on identity <%s> but no attributes were found", RestServiceBindAttribute, ExpressEdgeRouterName)
		assert.Containsf(t, *erIdent.RoleAttributes, RestServiceBindAttribute, "Expected to find Role Attribute <%s> on router <%s> but it was not found", RestServiceBindAttribute, ExpressEdgeRouterName)
	})

	/* ----Test Service Configs---- */

	// Query bind and dial configs
	bindConfig := getConfigByName(client, RestServiceBindConfigName)
	t.Run("TestRESTAPIServiceHostConfig", func(t *testing.T) {
		// Ensure the config type is correct
		bindConfigTypeID := *getConfigTypeByName(client, entities.HostConfigV1).ID

		require.NotNilf(t, bindConfig, "Expected <%s> config <%s> not found", entities.HostConfigV1, RestServiceBindConfigName)
		assert.Equalf(t, bindConfigTypeID, bindConfig.ConfigType.ID, "Expected host config type ID of %s but found %s", bindConfigTypeID, bindConfig.ConfigType.ID)
	})

	dialConfig := getConfigByName(client, RestServiceDialConfigName)
	t.Run("TestRESTAPIServiceDialConfig", func(t *testing.T) {
		// Ensure the config type is correct
		dialConfigTypeID := *getConfigTypeByName(client, entities.InterceptV1).ID

		require.NotNilf(t, dialConfig, "Expected <%s> config <%s> not found", entities.InterceptV1, RestServiceDialConfigName)
		assert.Equalf(t, dialConfigTypeID, dialConfig.ConfigType.ID, "Expected dial config type ID of %s but found %s", dialConfigTypeID, dialConfig.ConfigType.ID)
	})

	/* ----Test Service---- */

	// Query the service and service configs
	restService := getServiceByName(client, RestServiceName)

	t.Run("TestRESTAPIServiceForBindConfig", func(t *testing.T) {
		// Ensure the bind config ID is present on the service
		require.NotNilf(t, restService, "Expected service <%s> not found, cannot test for service bind config", RestServiceName)
		require.NotNilf(t, restService.Configs, "Expected to find service config ID <%s> but the service <%s> has no configs", RestServiceBindConfigName, RestServiceName)

		// Bind Config
		require.NotNilf(t, bindConfig, "Expected <%s> config <%s> not found", entities.HostConfigV1, RestServiceBindConfigName)
		assert.Containsf(t, restService.Configs, *bindConfig.ID, "Expected to find bind config %s for service %s but none was found", *bindConfig.ID, RestServiceName)
	})

	t.Run("TestRESTAPIServiceForDialConfig", func(t *testing.T) {
		// Ensure the dial config ID is present on the service
		require.NotNilf(t, restService, "Expected service <%s> not found, cannot test for service configs", RestServiceName)
		require.NotNilf(t, restService.Configs, "Expected to find service config ID <%s> but the service <%s> has no configs", RestServiceDialConfigName, RestServiceName)

		// Dial config
		require.NotNilf(t, dialConfig, "Expected <%s> config <%s> not found", entities.InterceptV1, RestServiceDialConfigName)
		assert.Containsf(t, restService.Configs, *dialConfig.ID, "Expected to find dial config %s for service %s but none was found", *dialConfig.ID, RestServiceName)
	})

	/* ----Test Service Policies---- */

	t.Run("TestRESTAPIServiceBindPolicy", func(t *testing.T) {
		// Query the service policies
		bindPolicy := getServicePolicyByName(client, RestServiceBindPolicyName)
		expectedBindPolicyIDRole := AttributePrefix + RestServiceBindAttribute

		require.NotNilf(t, restService, "Expected service <%s> not found, cannot test for <%s> service policy", RestServiceName, RestServiceBindPolicyName)
		require.NotNilf(t, bindPolicy, "Expected a bind policy <%s> but no policy with that name was found", RestServiceBindPolicyName)

		// Ensure the bind policy is linked via service roles
		expectedServiceRoleID := IdentityPrefix + *restService.ID
		require.Falsef(t, len(bindPolicy.ServiceRoles) < 1, "Expected to find service role ID <%s> but service policy <%s> has no service roles", expectedServiceRoleID, RestServiceBindPolicyName)
		assert.Containsf(t, bindPolicy.ServiceRoles, expectedServiceRoleID, "Expected to find service role %s in service policy %s but it was not found", RestServiceName, RestServiceBindPolicyName)

		// Ensure the bind attribute is found in Identity Roles
		require.Falsef(t, len(bindPolicy.ServiceRoles) < 1, "Expected to find identity role ID <%s> but service policy <%s> has no identity roles", expectedBindPolicyIDRole, RestServiceBindPolicyName)
		assert.Containsf(t, bindPolicy.IdentityRoles, expectedBindPolicyIDRole, "Expected to find identity role %s in service policy %s but it was not found", RestServiceBindAttribute, RestServiceBindPolicyName)
	})

	t.Run("TestRESTAPIServiceDialPolicy", func(t *testing.T) {
		// Query the service policies
		dialPolicy := getServicePolicyByName(client, RestServiceDialPolicyName)
		expectedDialPolicyIDRole := AttributePrefix + RestServiceDialAttribute

		require.NotNilf(t, restService, "Expected service <%s> not found, cannot test for <%s> service policy", RestServiceName, RestServiceDialPolicyName)
		require.NotNilf(t, dialPolicy, "Expected a bind policy <%s> but no policy with that name was found", RestServiceDialPolicyName)

		// Ensure the bind policy is linked via service roles
		expectedServiceIDRole := IdentityPrefix + *restService.ID
		require.Falsef(t, len(dialPolicy.ServiceRoles) < 1, "Expected to find service role ID <%s> but service policy <%s> has no service roles", expectedServiceIDRole, RestServiceDialPolicyName)
		assert.Containsf(t, dialPolicy.ServiceRoles, expectedServiceIDRole, "Expected to find service role %s in service policy %s but it was not found", RestServiceName, RestServiceDialPolicyName)

		// Ensure the bind attribute is found in Identity Roles
		require.Falsef(t, len(dialPolicy.ServiceRoles) < 1, "Expected to find identity role ID <%s> but service policy <%s> has no identity roles", expectedDialPolicyIDRole, RestServiceDialPolicyName)
		assert.Containsf(t, dialPolicy.IdentityRoles, expectedDialPolicyIDRole, "Expected to find identity role %s in service policy %s but it was not found", RestServiceBindAttribute, RestServiceDialPolicyName)
	})

	/* ----Test Service Functionality---- */
	t.Run("TestRESTAPIServiceQuery", func(t *testing.T) {
		testerUsername := "quickstart.user"
		wd, _ := os.Getwd()

		// Create the tester identity
		attributes := rest_model.Attributes{
			RestServiceDialAttribute,
		}
		ident := createIdentity(client, testerUsername, rest_model.IdentityTypeUser, false, attributes)
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

		helloUrl := fmt.Sprintf("https://%s:%d", RestServiceName, RestServiceDialPort)
		httpClient := createZitifiedHttpClient(wd + "/" + testerUsername + ".json")
		resp, err := httpClient.Get(helloUrl)
		if err != nil {
			log.Error(err)
			assert.Failf(t, "Query to service failed", "An error occurred while trying to reach the service")
		} else {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Error(err)
				assert.Failf(t, "Query to service failed", "An error occurred while trying to reach the service <%s>", RestServiceName)
			}
			assert.Equal(t, http.StatusOK, resp.StatusCode, fmt.Sprintf("Expected successful HTTP status code 200, received %d instead", resp.StatusCode))
			assert.Containsf(t, string(body), "https://ziti-edge-controller:1280/edge/management/v1", "Expected message not found in response body")
		}
	})
}
