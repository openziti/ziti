//go:build quickstart && automated

package edge

import (
	"context"
	"fmt"
	"github.com/openziti/edge-api/rest_management_api_client"
	"github.com/openziti/ziti/ziti/cmd/helpers"
	log "github.com/sirupsen/logrus"
	"os"
	"testing"
	"time"
)

func TestEdgeQuickstart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_ = os.Setenv("ZITI_CTRL_EDGE_ADVERTISED_ADDRESS", "localhost") //force localhost
	_ = os.Setenv("ZITI_ROUTER_NAME", "quickstart-router")
	cmdComplete := make(chan bool)
	qs := NewQuickStartCmd(os.Stdout, os.Stderr, ctx)
	go func() {
		err := qs.Execute()
		if err != nil {
			log.Fatal(err)
		}
		cmdComplete <- true
	}()

	ctrlAddy := helpers.GetCtrlEdgeAdvertisedAddress()
	ctrlPort := helpers.GetCtrlEdgeAdvertisedPort()
	ctrlUrl := fmt.Sprintf("https://%s:%s", ctrlAddy, ctrlPort)

	c := make(chan struct{})
	go waitForController(ctrlUrl, c)
	timeout, _ := time.ParseDuration("60s")
	select {
	case <-c:
		//completed normally
		log.Info("controller online")
	case <-time.After(timeout):
		cancel()
		panic("timed out waiting for controller")
	}

	performQuickstartTest(t, defaultSetupFunction)

	cancel() //terminate the running ctrl/router

	select { //wait for quickstart to cleanup
	case <-cmdComplete:
		fmt.Println("Operation completed")
	}
}

func TestZESFullParams(t *testing.T) {
	ctrlAddy := helpers.GetCtrlEdgeAdvertisedAddress()
	ctrlPort := helpers.GetCtrlEdgeAdvertisedPort()
	RunZitiEdgeSecureTest(t, createZESSetup(fmt.Sprintf("tcp:%s:%s", ctrlAddy, ctrlPort)))
}

func TestZESOnlyPort(t *testing.T) {
	ctrlPort := helpers.GetCtrlEdgeAdvertisedPort()
	RunZitiEdgeSecureTest(t, createZESSetup(ctrlPort))
}

func TestZESNoProtocol(t *testing.T) {
	ctrlAddy := helpers.GetCtrlEdgeAdvertisedAddress()
	ctrlPort := helpers.GetCtrlEdgeAdvertisedPort()
	RunZitiEdgeSecureTest(t, createZESSetup(fmt.Sprintf("%s:%s", ctrlAddy, ctrlPort)))
}

func RunZitiEdgeSecureTest(t *testing.T, setupFunc SetupFunction) {

	ctx, cancel := context.WithCancel(context.Background())
	_ = os.Setenv("ZITI_CTRL_EDGE_ADVERTISED_ADDRESS", "localhost") //force localhost
	_ = os.Setenv("ZITI_ROUTER_NAME", "quickstart-router")
	cmdComplete := make(chan bool)
	qs := NewQuickStartCmd(os.Stdout, os.Stderr, ctx)
	go func() {
		err := qs.Execute()
		if err != nil {
			log.Fatal(err)
		}
		cmdComplete <- true
	}()

	ctrlAddy := helpers.GetCtrlEdgeAdvertisedAddress()
	ctrlPort := helpers.GetCtrlEdgeAdvertisedPort()
	ctrlUrl := fmt.Sprintf("https://%s:%s", ctrlAddy, ctrlPort)

	c := make(chan struct{})
	go waitForController(ctrlUrl, c)
	timeout, _ := time.ParseDuration("60s")
	select {
	case <-c:
		//completed normally
		log.Info("controller online")
	case <-time.After(timeout):
		cancel()
		panic("timed out waiting for controller")
	}

	performQuickstartTest(t, setupFunc)

	cancel() //terminate the running ctrl/router

	select { //wait for quickstart to clean up
	case <-cmdComplete:
		fmt.Println("Operation completed")
	}
}

func TestMultipleZitiEdgeSecure(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	_ = os.Setenv("ZITI_CTRL_EDGE_ADVERTISED_ADDRESS", "localhost") //force localhost
	_ = os.Setenv("ZITI_ROUTER_NAME", "quickstart-router")
	cmdComplete := make(chan bool)
	qs := NewQuickStartCmd(os.Stdout, os.Stderr, ctx)
	go func() {
		err := qs.Execute()
		if err != nil {
			log.Fatal(err)
		}
		cmdComplete <- true
	}()

	ctrlAddy := helpers.GetCtrlEdgeAdvertisedAddress()
	ctrlPort := helpers.GetCtrlEdgeAdvertisedPort()
	ctrlUrl := fmt.Sprintf("https://%s:%s", ctrlAddy, ctrlPort)

	service1Name := "service1"
	service2Name := "service2"
	dialAddress1 := "dialAddress1"
	dialAddress2 := "dialAddress2"
	params := fmt.Sprintf("tcp:%s:%s", ctrlAddy, ctrlPort)

	c := make(chan struct{})
	go waitForController(ctrlUrl, c)
	timeout, _ := time.ParseDuration("60s")
	select {
	case <-c:
		//completed normally
		log.Info("controller online")
	case <-time.After(timeout):
		cancel()
		panic("timed out waiting for controller")
	}

	// Wait for the controller to become available
	zitiAdminUsername := os.Getenv("ZITI_USER")
	if zitiAdminUsername == "" {
		zitiAdminUsername = "admin"
	}
	zitiAdminPassword := os.Getenv("ZITI_PWD")
	if zitiAdminPassword == "" {
		zitiAdminPassword = "admin"
	}

	// Authenticate with the controller
	zel := NewLoginCmd(os.Stdout, os.Stderr)
	zel.SetArgs([]string{
		"https://127.0.0.1:1280/edge/management/v1",
		"--username=admin",
		"--password=admin",
		"-y",
	})
	err := zel.Execute()
	if err != nil {
		log.Fatal(err)
	}

	// Run ZES once
	zes := newSecureCmd(os.Stdout, os.Stderr)
	zes.SetArgs([]string{
		service1Name,
		params,
		fmt.Sprintf("--endpoint=%s", dialAddress1),
	})
	err = zes.Execute()
	if err != nil {
		fmt.Printf("Error: %s", err)
	}

	// Run ZES twice
	zes = newSecureCmd(os.Stdout, os.Stderr)
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

	cancel() //terminate the running ctrl/router

	select { //wait for quickstart to clean up
	case <-cmdComplete:
		fmt.Println("Operation completed")
	}
}

func createZESSetup(params string) SetupFunction {
	return func(client *rest_management_api_client.ZitiEdgeManagement, dialAddress string, dialPort int, advPort string, advAddy string, serviceName string, hostingRouterName string, testerUsername string) CleanupFunction {
		// Run ziti edge secure with the controller edge details
		zes := newSecureCmd(os.Stdout, os.Stderr)
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

		return func() {
			// TODO: Cleanup
		}
	}
}
