//go:build quickstart && automated

package run

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/ziti/cmd/helpers"
	log "github.com/sirupsen/logrus"
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
	go waitForController(ctx, ctrlUrl, cmdComplete)
	timeout, _ := time.ParseDuration("90s")
	select {
	case e := <-cmdComplete:
		//completed, check for error
		if e != nil {
			t.Fatal(e)
		}
		expectedTestDuration, _ := time.ParseDuration("120s")
		log.Info("controller online")
		go func() {
			routerAddy := helpers.GetCtrlEdgeAdvertisedAddress()
			routerPort := helpers.GetZitiEdgeRouterPort()
			routerAddr := net.JoinHostPort(routerAddy, routerPort)
			log.Infof("waiting for router at %s", routerAddr)
			for {
				conn, err := net.DialTimeout("tcp", routerAddr, 2*time.Second)
				if err == nil {
					_ = conn.Close()
					log.Infof("router online at %s", routerAddr)
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
			performQuickstartTest(t)
			log.Info("Operation completed")
			cmdComplete <- nil
		}()
		select {
		case e := <-cmdComplete:
			cancel()
			if e != nil {
				time.Sleep(5 * time.Second)
				t.Fatal(e)
			} else {
				time.Sleep(5 * time.Second)
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
}
