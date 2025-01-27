package edge

import (
	"context"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/ziti/cmd/helpers"
	"os"
	"time"
)

type QuickstartTestEnv struct {
	controllerStarted chan struct{}
	routerStarted     chan struct{}
	complete          chan bool
}

func (q QuickstartTestEnv) Start(ctx context.Context, onCancel context.CancelFunc) {
	_ = os.Setenv("ZITI_CTRL_EDGE_ADVERTISED_ADDRESS", "localhost") //force localhost
	_ = os.Setenv("ZITI_ROUTER_NAME", "quickstart-router")

	qs := NewQuickStartCmd(os.Stdout, os.Stderr, ctx)
	go func() {
		err := qs.Execute()
		if err != nil {
			pfxlog.Logger().Fatal(err)
		}
		q.complete <- true
	}()

	ctrlAddy := helpers.GetCtrlEdgeAdvertisedAddress()
	ctrlPort := helpers.GetCtrlEdgeAdvertisedPort()
	ctrlUrl := fmt.Sprintf("https://%s:%s", ctrlAddy, ctrlPort)

	go waitForController(ctrlUrl, q.controllerStarted)
	timeout, _ := time.ParseDuration("60s")
	select {
	case <-q.controllerStarted:
		//completed normally
		pfxlog.Logger().Info("controller online")
	case <-time.After(timeout):
		onCancel()
		panic("timed out waiting for controller")
	}
}
