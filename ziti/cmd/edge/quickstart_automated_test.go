// TODO: Uncomment this //go:build quickstart && automated

package edge

import (
	"context"
	"fmt"
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

	performQuickstartTest(t)

	cancel() //terminate the running ctrl/router

	select { //wait for quickstart to cleanup
	case <-cmdComplete:
		fmt.Println("Operation completed")
	}
}

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

	// Log into the controller
	loginCmd := NewLoginCmd(os.Stdout, os.Stderr)
	loginCmd.SetArgs([]string{
		ctrlUrl,
		fmt.Sprintf("--username=%s", "admin"),
		fmt.Sprintf("--password=%s", "admin"),
		"-y",
	})
	loginErr := loginCmd.Execute()
	if loginErr != nil {
		fmt.Printf("Login error: %s", loginErr)
	}

	// Run ziti edge secure
	zes := newSecureCmd(os.Stdout, os.Stderr)
	zes.SetArgs([]string{
		"gotest",
		fmt.Sprintf("tcp://%s:%s", ctrlAddy, ctrlPort)})
	err := zes.Execute()
	if err != nil {
		fmt.Printf("Error: %s", err)
	}

	cancel() //terminate the running ctrl/router

	select { //wait for quickstart to clean up
	case <-cmdComplete:
		fmt.Println("Operation completed")
	}
}
