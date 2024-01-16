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

// ziti edge secure 8080
// ziti edge secure localhost:8080
// ziti edge secure tcp:localhost:8080
// ziti edge secure udp:localhost:8080
// ziti edge secure udp:127.0.0.1:8080
// ...
// Protocol defaults to ["udp","tcp"] if not set
// default address to 127.0.0.1 if not provided
func TestZitiEdgeSecure(t *testing.T) {

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

	// Declare an outer function
	setupFunction := func(client *rest_management_api_client.ZitiEdgeManagement, dialAddress string, dialPort int, advPort string, advAddy string, serviceName string, hostingRouterName string, testerUsername string) {
		// Run ziti edge secure with the controller edge details
		zes := newSecureCmd(os.Stdout, os.Stderr)
		zes.SetArgs([]string{
			serviceName,
			fmt.Sprintf("tcp:%s:%s", ctrlAddy, ctrlPort),
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

	}

	performQuickstartTest(t, setupFunction)

	//
	//routerName := "quickstart-router"
	//ctx, cancel := context.WithCancel(context.Background())
	//_ = os.Setenv("ZITI_CTRL_EDGE_ADVERTISED_ADDRESS", "localhost") //force localhost
	//_ = os.Setenv("ZITI_ROUTER_NAME", routerName)
	//cmdComplete := make(chan bool)
	//qs := NewQuickStartCmd(os.Stdout, os.Stderr, ctx)
	//go func() {
	//	err := qs.Execute()
	//	if err != nil {
	//		log.Fatal(err)
	//	}
	//	cmdComplete <- true
	//}()
	//
	//wd, _ := os.Getwd()
	//testerName := "gotester"
	//ctrlAddy := helpers.GetCtrlEdgeAdvertisedAddress()
	//ctrlPort := helpers.GetCtrlEdgeAdvertisedPort()
	//ctrlUrl := fmt.Sprintf("https://%s:%s", ctrlAddy, ctrlPort)
	//
	//c := make(chan struct{})
	//go waitForController(ctrlUrl, c)
	//timeout, _ := time.ParseDuration("60s")
	//select {
	//case <-c:
	//	//completed normally
	//	log.Info("controller online")
	//case <-time.After(timeout):
	//	cancel()
	//	panic("timed out waiting for controller")
	//}
	//
	//routerPort, _ := strconv.Atoi(helpers.GetZitiEdgeRouterPort())
	//for {
	//	t := IsPortListening("localhost", routerPort, 10*time.Second)
	//	fmt.Printf("Waiting for router on port %d\n", routerPort)
	//	if t {
	//		break
	//	}
	//	time.Sleep(time.Second)
	//}
	//
	//// Log into the controller
	//loginCmd := NewLoginCmd(os.Stdout, os.Stderr)
	//loginCmd.SetArgs([]string{
	//	ctrlUrl,
	//	fmt.Sprintf("--username=%s", "admin"),
	//	fmt.Sprintf("--password=%s", "admin"),
	//	"-y",
	//})
	//loginErr := loginCmd.Execute()
	//if loginErr != nil {
	//	fmt.Printf("Login error: %s", loginErr)
	//}
	//
	//

	// test the connection

	cancel() //terminate the running ctrl/router

	select { //wait for quickstart to clean up
	case <-cmdComplete:
		fmt.Println("Operation completed")
	}
}
