package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math/rand"
	"slices"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	zitiLibOps "github.com/openziti/ziti/zititest/zitilab/runlevel/5_operation"
	"github.com/openziti/ziti/zititest/zitilab/validations"
)

// start with a random scenario then cycle through them
var scenarioCounter = rand.Intn(7)

func sowChaos(run model.Run) error {
	var toRestart []*model.Component
	var err error

	scenarioCounter = (scenarioCounter + 1) % 7
	scenario := scenarioCounter + 1

	// bit 0: restart random controllers
	var controllers []*model.Component
	if scenario&0b001 > 0 {
		controllers, err = chaos.SelectRandom(run, ".ctrl", chaos.RandomOfTotal())
		if err != nil {
			return err
		}
		time.Sleep(5 * time.Second)
	}

	// bit 1: restart random routers (including running lifecycle routers)
	var routers []*model.Component
	if scenario&0b010 > 0 {
		routers, err = chaos.SelectRandom(run, ".router.test", chaos.PercentageRange(10, 100))
		if err != nil {
			return err
		}
		// also include some running lifecycle routers in the restart pool
		runningLifecycle := selectRunningLifecycleComponents(run.GetModel())
		if len(runningLifecycle) > 0 {
			count := rand.Intn(len(runningLifecycle)) + 1
			rand.Shuffle(len(runningLifecycle), func(i, j int) {
				runningLifecycle[i], runningLifecycle[j] = runningLifecycle[j], runningLifecycle[i]
			})
			routers = append(routers, runningLifecycle[:count]...)
		}
	}

	// bit 2: disrupt connections to non-restarted components
	if scenario&0b100 > 0 {
		if err := disruptNonRestarted(run, controllers, routers); err != nil {
			return err
		}
	}

	toRestart = append(toRestart, controllers...)
	toRestart = append(toRestart, routers...)
	tui.ValidationLogger().Infof("restarting %d controllers and %d routers\n", len(controllers), len(routers))
	return chaos.RestartSelected(run, 100, toRestart...)
}

func disruptNonRestarted(run model.Run, restartedCtrls, restartedRouters []*model.Component) error {
	logger := tui.ActionsLogger()

	restartedSet := map[string]bool{}
	for _, c := range restartedCtrls {
		restartedSet[c.Id] = true
	}
	for _, c := range restartedRouters {
		restartedSet[c.Id] = true
	}

	// If router-west is not being restarted, disrupt its ctrl listener port
	if !restartedSet["router-west"] {
		routerWestHost, err := run.GetModel().SelectHost("router-west")
		if err == nil {
			logger.Info("disrupting incoming connections on router-west port 6263")
			if err := routerWestHost.KillIncoming(6263); err != nil {
				logger.WithError(err).Warn("failed to disrupt incoming on router-west")
			}
		}
	}

	// Disrupt a random non-restarted controller's ctrl port
	var nonRestartedCtrls []*model.Component
	for _, c := range run.GetModel().SelectComponents(".ctrl") {
		if !restartedSet[c.Id] {
			nonRestartedCtrls = append(nonRestartedCtrls, c)
		}
	}
	if len(nonRestartedCtrls) > 0 {
		target := nonRestartedCtrls[rand.Intn(len(nonRestartedCtrls))]
		logger.Infof("disrupting incoming connections on %s port 6262", target.Id)
		if err := target.Host.DisruptIncoming(6262); err != nil {
			logger.WithError(err).Warnf("failed to disrupt incoming on %s", target.Id)
		} else {
			time.Sleep(10 * time.Second)
			logger.Infof("unblocking incoming connections on %s port 6262", target.Id)
			_ = target.Host.UnblockIncoming(6262)
		}
	}

	return nil
}

func validateClusterConnectivity(run model.Run) error {
	expectedCtrlCount := len(run.GetModel().SelectComponents(".ctrl"))

	ctrls := run.GetModel().SelectComponents(".ctrl")
	deadline := time.Now().Add(2 * time.Minute)

	logger := tui.ValidationLogger()

	for time.Now().Before(deadline) {
		ctrlClients, err := chaos.NewCtrlClients(run, ".ctrl")
		if err != nil {
			pfxlog.Logger().WithError(err).Warn("failed to create ctrl clients for connectivity check")
			time.Sleep(5 * time.Second)
			continue
		}

		success := true
		for _, ctrl := range ctrls {
			if err := checkConnectedPeers(ctrl.Id, ctrlClients, expectedCtrlCount); err != nil {
				logger.WithField("ctrl", ctrl.Id).WithError(err).Info("connected-peers check failed, will retry")
				success = false
				continue
			}

			expectedRouterNames := expectedRoutersForCtrl(run.GetModel(), ctrl)
			if err := checkConnectedRouters(ctrl.Id, ctrlClients, expectedRouterNames); err != nil {
				logger.WithField("ctrl", ctrl.Id).WithError(err).Info("connected-routers check failed, will retry")
				success = false
				continue
			}
		}

		if success {
			logger.Info("cluster connectivity validation successful")
			return nil
		}
		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("cluster connectivity validation timed out")
}

type peerInfo struct {
	Id          string `json:"id"`
	IsLeader    bool   `json:"isLeader"`
	IsConnected bool   `json:"isConnected"`
}

type routerInfo struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

func checkConnectedPeers(ctrlId string, ctrlClients *chaos.CtrlClients, expectedCount int) error {
	logger := tui.ValidationLogger().WithField("ctrl", ctrlId)

	resp, err := ctrlClients.Inspect(ctrlId, ctrlId, "connected-peers")
	if err != nil {
		return fmt.Errorf("failed to inspect connected-peers: %w", err)
	}

	if !*resp.Success {
		return fmt.Errorf("connected-peers inspection failed: %v", resp.Errors)
	}

	value, ok := ctrlClients.GetInspectValue(resp, ctrlId, "connected-peers")
	if !ok {
		return fmt.Errorf("connected-peers inspection did not return any values")
	}

	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal connected-peers value: %w", err)
	}

	var peers []peerInfo
	if err := json.Unmarshal(jsonBytes, &peers); err != nil {
		return fmt.Errorf("failed to unmarshal connected-peers: %w", err)
	}

	if len(peers) != expectedCount {
		return fmt.Errorf("expected %d peers, got %d", expectedCount, len(peers))
	}

	hasSelf := false
	hasLeader := false

	connectedCount := 0
	for _, peer := range peers {
		if peer.Id == ctrlId {
			if !peer.IsConnected {
				return fmt.Errorf("controller %s is reporting as not connected", peer.Id)
			}
			hasSelf = true
		}
		if peer.IsLeader {
			hasLeader = true
			if !peer.IsConnected {
				return fmt.Errorf("controller %s is reporting as not connected to leader %s", ctrlId, peer.Id)
			}
		}
		if peer.IsConnected {
			connectedCount++
		}
	}

	if !hasLeader {
		return fmt.Errorf("no leader found among peers")
	}

	if !hasSelf {
		return fmt.Errorf("controller %s doesn't have self as peer", ctrlId)
	}

	if connectedCount != expectedCount {
		return fmt.Errorf("controller %s connected count of %d is not equal to expected count of %d", ctrlId, connectedCount, expectedCount)
	}

	logger.Infof("connected-peers check passed: %d peers, leader present, all connected", len(peers))
	return nil
}

func checkConnectedRouters(ctrlId string, ctrlClients *chaos.CtrlClients, expectedRouterNames map[string]bool) error {
	logger := tui.ValidationLogger().WithField("ctrl", ctrlId)

	resp, err := ctrlClients.Inspect(ctrlId, ctrlId, "connected-routers")
	if err != nil {
		return fmt.Errorf("failed to inspect connected-routers: %w", err)
	}

	if !*resp.Success {
		return fmt.Errorf("connected-routers inspection failed: %v", resp.Errors)
	}

	value, ok := ctrlClients.GetInspectValue(resp, ctrlId, "connected-routers")
	if !ok {
		return fmt.Errorf("connected-routers inspection did not return any values")
	}

	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal connected-routers value: %w", err)
	}

	var routers []routerInfo
	if err := json.Unmarshal(jsonBytes, &routers); err != nil {
		return fmt.Errorf("failed to unmarshal connected-routers: %w", err)
	}

	connectedRouters := map[string]bool{}
	for _, r := range routers {
		connectedRouters[r.Name] = true
	}

	var errList []error
	names := slices.Sorted(maps.Keys(expectedRouterNames))
	for _, name := range names {
		if !connectedRouters[name] {
			errList = append(errList, fmt.Errorf("router %s not connected to controller %s", name, ctrlId))
		}
	}

	if len(errList) != 0 {
		return errors.Join(errList...)
	}

	logger.Infof("connected-routers check passed: %d/%d routers connected", len(routers), len(expectedRouterNames))
	return nil
}

func validateCircuitsPerCtrl(run model.Run) error {
	ctrls := run.GetModel().SelectComponents(".ctrl")
	errC := make(chan error, len(ctrls))
	deadline := time.Now().Add(2 * time.Minute)
	for _, ctrl := range ctrls {
		ctrlComponent := ctrl
		go func() {
			expected := expectedRoutersForCtrl(run.GetModel(), ctrlComponent)
			delete(expected, "router-metrics")

			names := slices.Sorted(maps.Keys(expected))
			filter := `name in ["` + strings.Join(names, `","`) + `"] limit none`

			errC <- validations.ValidateCircuitsForCtrl(run, ctrlComponent, deadline, filter)
		}()
	}

	for range len(ctrls) {
		if err := <-errC; err != nil {
			return err
		}
	}
	return nil
}

func RunSimScenarios(run model.Run, services *zitiLibOps.SimServices) error {
	if err := run.GetModel().Exec(run, "startSimMetrics"); err != nil {
		return err
	}

	simControl, err := services.GetSimController(run, "sim-control", nil)
	if err != nil {
		return err
	}

	sims := run.GetModel().FilterComponents(".loop-client", func(c *model.Component) bool {
		t, ok := c.Type.(*zitilab.Loop4SimType)
		return ok && t.Mode == zitilab.Loop4RemoteControlled
	})

	err = simControl.WaitForAllConnected(time.Second*30, sims)
	if err != nil {
		return err
	}

	results, err := simControl.StartSimScenarios()
	if err != nil {
		return err
	}

	return results.GetResults(5 * time.Minute)
}
