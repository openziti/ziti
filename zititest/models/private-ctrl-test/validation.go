package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/controller/rest_client/terminator"
	"github.com/openziti/ziti/v2/zitirest"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	zitiLibOps "github.com/openziti/ziti/zititest/zitilab/runlevel/5_operation"
	"google.golang.org/protobuf/proto"
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

	// bit 1: restart random routers
	var routers []*model.Component
	if scenario&0b010 > 0 {
		routers, err = chaos.SelectRandom(run, ".router.test", chaos.PercentageRange(10, 100))
		if err != nil {
			return err
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
	expectedRouterNames := map[string]bool{}
	for _, c := range run.GetModel().SelectComponents(".router") {
		expectedRouterNames[c.Id] = true
	}

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
	isLeader := false
	hasLeader := false

	connectedCount := 0
	for _, peer := range peers {
		if peer.Id == ctrlId {
			if !peer.IsConnected {
				return fmt.Errorf("controller %s is reporting as not connected", peer.Id)
			}
			hasSelf = true
			if peer.IsLeader {
				isLeader = true
			}
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

	if isLeader && connectedCount != expectedCount {
		return fmt.Errorf("leader connected count of %d is not equal to expected count of %d ", connectedCount, expectedCount)
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

	for name := range expectedRouterNames {
		if !connectedRouters[name] {
			return fmt.Errorf("router %s not connected to controller %s", name, ctrlId)
		}
	}

	logger.Infof("connected-routers check passed: %d/%d routers connected", len(routers), len(expectedRouterNames))
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

func validateTerminators(run model.Run) error {
	ctrls := run.GetModel().SelectComponents(".ctrl")
	errC := make(chan error, len(ctrls))
	deadline := time.Now().Add(5 * time.Minute)
	for _, ctrl := range ctrls {
		ctrlComponent := ctrl
		go func() {
			errC <- validateTerminatorsForCtrl(run, ctrlComponent, deadline)
		}()
	}

	for i := 0; i < len(ctrls); i++ {
		err := <-errC
		if err != nil {
			return err
		}
	}

	return nil
}

func validateTerminatorsForCtrl(run model.Run, c *model.Component, deadline time.Time) error {
	clients, err := chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
	if err != nil {
		return err
	}

	start := time.Now()
	logger := tui.ValidationLogger().WithField("ctrl", c.Id)
	var lastLog time.Time

	// Wait for terminators to be present
	for time.Now().Before(deadline) {
		terminatorCount, err := getTerminatorCount(clients)
		if err != nil {
			logger.WithError(err).Warn("error getting terminator count")
			time.Sleep(5 * time.Second)
			clients, err = chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
			if err != nil {
				return err
			}
			continue
		}
		if terminatorCount > 0 {
			logger.Infof("terminators present: %d, elapsed: %v", terminatorCount, time.Since(start))
			break
		}
		if time.Since(lastLog) > 30*time.Second {
			logger.Infof("waiting for terminators, current count: %d, elapsed: %v", terminatorCount, time.Since(start))
			lastLog = time.Now()
		}
		time.Sleep(5 * time.Second)
	}

	// Validate SDK terminators
	for {
		count, err := validateRouterSdkTerminators(c.Id, clients)
		if err == nil {
			return nil
		}

		if time.Now().After(deadline) {
			return err
		}

		logger.Infof("current count of invalid sdk terminators: %v, elapsed: %v", count, time.Since(start))
		time.Sleep(15 * time.Second)

		clients, err = chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
		if err != nil {
			return err
		}
	}
}

func getTerminatorCount(clients *zitirest.Clients) (int64, error) {
	ctx, cancelF := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelF()

	filter := "limit 1"
	result, err := clients.Fabric.Terminator.ListTerminators(&terminator.ListTerminatorsParams{
		Filter:  &filter,
		Context: ctx,
	}, nil)

	if err != nil {
		return 0, err
	}
	count := *result.Payload.Meta.Pagination.TotalCount
	return count, nil
}

func validateRouterSdkTerminators(id string, clients *zitirest.Clients) (int, error) {
	logger := tui.ValidationLogger().WithField("ctrl", id)

	closeNotify := make(chan struct{})
	eventNotify := make(chan *mgmt_pb.RouterSdkTerminatorsDetails, 1)

	handleSdkTerminatorResults := func(msg *channel.Message, _ channel.Channel) {
		detail := &mgmt_pb.RouterSdkTerminatorsDetails{}
		if err := proto.Unmarshal(msg.Body, detail); err != nil {
			pfxlog.Logger().WithError(err).Error("unable to unmarshal router sdk terminator details")
			return
		}
		eventNotify <- detail
	}

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_ValidateRouterSdkTerminatorsResultType), handleSdkTerminatorResults)
		binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
			close(closeNotify)
		}))
		return nil
	}

	ch, err := clients.NewWsMgmtChannel(channel.BindHandlerF(bindHandler))
	if err != nil {
		return 0, err
	}

	defer func() {
		_ = ch.Close()
	}()

	request := &mgmt_pb.ValidateRouterSdkTerminatorsRequest{
		Filter: "limit none",
	}
	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(10 * time.Second).SendForReply(ch)

	response := &mgmt_pb.ValidateRouterSdkTerminatorsResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(responseMsg, err); err != nil {
		return 0, err
	}

	if !response.Success {
		return 0, fmt.Errorf("failed to start sdk terminator validation: %s", response.Message)
	}

	logger.Infof("started validation of %v routers", response.RouterCount)

	expected := response.RouterCount

	invalid := 0
	for expected > 0 {
		select {
		case <-closeNotify:
			fmt.Printf("channel closed, exiting")
			return 0, errors.New("unexpected close of mgmt channel")
		case routerDetail := <-eventNotify:
			if !routerDetail.ValidateSuccess {
				return invalid, fmt.Errorf("error: unable to validate router %s (%s) on controller %s (%s)",
					routerDetail.RouterId, routerDetail.RouterName, id, routerDetail.Message)
			}
			for _, linkDetail := range routerDetail.Details {
				if !linkDetail.IsValid {
					invalid++
				}
			}
			expected--
		}
	}
	if invalid == 0 {
		logger.Infof("sdk terminator validation of %v routers successful", response.RouterCount)
		return invalid, nil
	}
	return invalid, fmt.Errorf("invalid sdk terminators found")
}

func validateCircuits(run model.Run) error {
	ctrls := run.GetModel().SelectComponents(".ctrl")
	errC := make(chan error, len(ctrls))
	deadline := time.Now().Add(time.Minute)
	for _, ctrl := range ctrls {
		ctrlComponent := ctrl
		go func() {
			errC <- validateCircuitsForCtrl(run, ctrlComponent, deadline)
		}()
	}

	for i := 0; i < len(ctrls); i++ {
		err := <-errC
		if err != nil {
			return err
		}
	}

	return nil
}

func validateCircuitsForCtrl(run model.Run, c *model.Component, deadline time.Time) error {
	clients, err := chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
	if err != nil {
		return err
	}

	start := time.Now()

	logger := tui.ValidationLogger().WithField("ctrl", c.Id)

	first := true
	for {
		count, err := validateCircuitsForCtrlOnce(c.Id, clients, first)
		if err == nil {
			return nil
		}

		if time.Now().After(deadline) {
			return err
		}

		logger.Infof("current count of circuit errors: %v, elapsed time: %v, current err: %v", count, time.Since(start), err)
		time.Sleep(15 * time.Second)

		clients, err = chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
		if err != nil {
			return err
		}
		first = false
	}
}

func validateCircuitsForCtrlOnce(id string, clients *zitirest.Clients, first bool) (int, error) {
	logger := tui.ValidationLogger().WithField("ctrl", id)

	closeNotify := make(chan struct{})
	eventNotify := make(chan *mgmt_pb.RouterCircuitDetails, 1)

	handleResults := func(msg *channel.Message, _ channel.Channel) {
		detail := &mgmt_pb.RouterCircuitDetails{}
		if err := proto.Unmarshal(msg.Body, detail); err != nil {
			logger.WithError(err).Error("unable to unmarshal circuit validation details")
			return
		}
		eventNotify <- detail
	}

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_ValidateCircuitsResultType), handleResults)
		binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
			close(closeNotify)
		}))
		return nil
	}

	ch, err := clients.NewWsMgmtChannel(channel.BindHandlerF(bindHandler))
	if err != nil {
		return 0, err
	}

	defer func() {
		_ = ch.Close()
	}()

	request := &mgmt_pb.ValidateCircuitsRequest{
		RouterFilter: `name !="router-metrics" limit none`,
	}
	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(10 * time.Second).SendForReply(ch)

	response := &mgmt_pb.ValidateCircuitsResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(responseMsg, err); err != nil {
		return 0, err
	}

	if !response.Success {
		return 0, fmt.Errorf("failed to start circuit validation: %s", response.Message)
	}

	logger.Infof("started validation of %v components", response.RouterCount)

	expected := response.RouterCount

	invalid := 0
	for expected > 0 {
		select {
		case <-closeNotify:
			logger.Info("channel closed, exiting")
			return 0, errors.New("unexpected close of mgmt channel")
		case detail := <-eventNotify:
			if !detail.ValidateSuccess {
				invalid++
				logger.Infof("error validating router %s using ctrl %s: %s", detail.RouterId, id, detail.Message)
			}
			for _, details := range detail.Details {
				if details.IsInErrorState() {
					if !first {
						logger.Infof("\tcircuit: %s ctrl: %v, fwd: %v, edge: %v, sdk: %v, dest: %+v",
							details.CircuitId, details.MissingInCtrl, details.MissingInForwarder,
							details.MissingInEdge, details.MissingInSdk, details.Destinations)
					}
					invalid++
				}
			}
			expected--
		}
	}
	if invalid == 0 {
		logger.Infof("circuit validation of %v routers successful", response.RouterCount)
		return invalid, nil
	}
	return invalid, errors.New("errors found")
}
