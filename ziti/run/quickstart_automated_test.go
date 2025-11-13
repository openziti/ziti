//go:build quickstart && automated

package run

import (
	"context"
	"fmt"
	"github.com/openziti/ziti/ziti/cmd/helpers"
	log "github.com/sirupsen/logrus"
	"os"
	"testing"
	"time"
)

func TestEdgeQuickstartAutomated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_ = os.Setenv("ZITI_CTRL_EDGE_ADVERTISED_ADDRESS", "localhost") //force localhost
	_ = os.Setenv("ZITI_ROUTER_NAME", "quickstart-router")
	qs := NewQuickStartCmd(os.Stdout, os.Stderr, ctx)
	qs.SetArgs([]string{})
	go func() {
		_ = qs.Execute()
	}()

	ctrlAddy := helpers.GetCtrlEdgeAdvertisedAddress()
	ctrlPort := helpers.GetCtrlEdgeAdvertisedPort()
	ctrlUrl := fmt.Sprintf("https://%s:%s", ctrlAddy, ctrlPort)

	cmdComplete := make(chan error)
	go waitForController(ctrlUrl, cmdComplete)
	timeout, _ := time.ParseDuration("45s")
	select {
	case e := <-cmdComplete:
		//completed, check for error
		if e != nil {
			t.Fatal(e)
		}
		expectedTestDuration, _ := time.ParseDuration("60s")
		log.Info("controller online")
		go func() {
			cmdComplete <- performQuickstartTest(t)
		}()
		select {
		case e := <-cmdComplete:
			cancel()
			time.Sleep(5 * time.Second)
			if e != nil {
				t.Fatal(e)
			}
		case <-time.After(expectedTestDuration):
			cancel()
			time.Sleep(5 * time.Second)
			t.Fatal("running the test has taken too long")
		}
	case <-time.After(timeout):
		cancel()
		time.Sleep(5 * time.Second)
		t.Fatal("timed out waiting for controller to start")
	}
	log.Info("TestEdgeQuickstartAutomated completed")
}
