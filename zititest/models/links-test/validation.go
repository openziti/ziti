/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math/rand"
	"slices"
	"sort"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/channel/v5/protobufs"
	"github.com/openziti/fablab/kernel/lib/parallel"
	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/controller/rest_client/link"
	"github.com/openziti/ziti/v2/controller/rest_model"
	"github.com/openziti/ziti/v2/zitirest"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

type disruptionKey struct {
	sourceHostId string
	destIP       string
	destPort     uint16
}

// makeOutgoingDisruptionTask blocks host -> dest:port for a random 10-30s, then
// unblocks it. Simulates a one-directional network partition between two hosts.
func makeOutgoingDisruptionTask(host *model.Host, dest *model.Host, port uint16) parallel.Task {
	return func() error {
		logger := tui.ActionsLogger()
		logger.Infof("blocking %s -> %s on port %d", host.Id, dest.Id, port)
		if err := host.DisruptOutgoing(dest.PublicIp, port); err != nil {
			return err
		}
		time.Sleep(time.Duration(10+rand.Intn(21)) * time.Second)
		logger.Infof("unblocking %s -> %s on port %d", host.Id, dest.Id, port)
		return host.UnblockOutgoing(dest.PublicIp, port)
	}
}

// makeIncomingDisruptionTask blocks all inbound traffic to host:port for a
// random 10-30s, then unblocks it.
func makeIncomingDisruptionTask(host *model.Host, port uint16) parallel.Task {
	return func() error {
		logger := tui.ActionsLogger()
		logger.Infof("blocking %s incoming on port %d", host.Id, port)
		if err := host.DisruptIncoming(port); err != nil {
			return err
		}
		time.Sleep(time.Duration(10+rand.Intn(21)) * time.Second)
		logger.Infof("unblocking %s incoming on port %d", host.Id, port)
		return host.UnblockIncoming(port)
	}
}

// netemApplyCmd / netemClearCmd build the tc(8) netem commands, detecting the
// default-route interface in-shell so we don't have to know the instance's NIC
// name. `replace`/`del ... || true` keep them idempotent.
func netemApplyCmd(latencyMs, jitterMs, lossPct int) string {
	return fmt.Sprintf("IFACE=$(ip -o route get 1.1.1.1 | awk '{print $5; exit}') && "+
		"sudo tc qdisc replace dev $IFACE root netem delay %dms %dms loss %d%%", latencyMs, jitterMs, lossPct)
}

func netemClearCmd() string {
	return "IFACE=$(ip -o route get 1.1.1.1 | awk '{print $5; exit}') && " +
		"sudo tc qdisc del dev $IFACE root 2>/dev/null || true"
}

// makeNetemTask degrades a host's egress (added latency, jitter, and packet loss)
// for a random 10-30s, then clears it. Unlike a clean iptables DROP, this is the
// degraded-but-not-blocked regime where links flap at the margin, driving the
// rapid re-dial / iteration churn that pure on/off partitions miss. netem applies
// to the whole interface, so every component on the host is degraded together.
func makeNetemTask(host *model.Host, latencyMs, jitterMs, lossPct int) parallel.Task {
	return func() error {
		logger := tui.ActionsLogger()
		logger.Infof("degrading %s egress: %dms+/-%dms latency, %d%% loss", host.Id, latencyMs, jitterMs, lossPct)
		if output, err := host.ExecLogged(netemApplyCmd(latencyMs, jitterMs, lossPct)); err != nil {
			return fmt.Errorf("failed to apply netem on %s: %w (output: %s)", host.Id, err, output)
		}
		time.Sleep(time.Duration(10+rand.Intn(21)) * time.Second)
		logger.Infof("clearing netem on %s", host.Id)
		if output, err := host.ExecLogged(netemClearCmd()); err != nil {
			logger.Warnf("failed to clear netem on %s: %v (output: %s)", host.Id, err, output)
		}
		return nil
	}
}

// randomPairs returns up to maxCount distinct (i, j) index pairs drawn from
// [0,fromLen) x [0,toLen). When excludeSelf is set, pairs with i == j are
// skipped (used when both indices refer to the same component set).
func randomPairs(fromLen, toLen, maxCount int, excludeSelf bool) [][2]int {
	if maxCount <= 0 {
		return nil
	}
	maxPossible := fromLen * toLen
	if excludeSelf {
		maxPossible -= min(fromLen, toLen)
	}
	if maxCount > maxPossible {
		maxCount = maxPossible
	}
	pairs := map[[2]int]struct{}{}
	for range maxCount {
		i := rand.Intn(fromLen)
		j := rand.Intn(toLen)
		if excludeSelf && i == j {
			continue
		}
		pair := [2]int{i, j}
		pairs[pair] = struct{}{}
	}
	return slices.Collect(maps.Keys(pairs))
}

type chaosMode int

const (
	chaosModeRestarts    chaosMode = iota // restarts only
	chaosModeDisruptions                  // connection disruptions only
	chaosModeBoth                         // restarts + disruptions
	chaosModeCOUNT                        // sentinel for cycling
)

func (m chaosMode) String() string {
	switch m {
	case chaosModeRestarts:
		return "restarts-only"
	case chaosModeDisruptions:
		return "disruptions-only"
	case chaosModeBoth:
		return "restarts+disruptions"
	default:
		return "unknown"
	}
}

var currentChaosMode chaosMode

func sowChaos(run model.Run) error {
	mode := currentChaosMode
	currentChaosMode = (currentChaosMode + 1) % chaosModeCOUNT

	var tasks []parallel.Task
	var controllerCount, routerCount, hardKillCount, frozenCount, netemCount int

	// Generate restart tasks for restart and both modes
	if mode == chaosModeRestarts || mode == chaosModeBoth {
		controllers, err := chaos.SelectRandom(run, ".ctrl", chaos.RandomOfTotal())
		if err != nil {
			return err
		}
		routers, err := chaos.SelectRandom(run, ".router", chaos.PercentageRange(0, 50))
		if err != nil {
			return err
		}
		controllerCount = len(controllers)
		routerCount = len(routers)

		// Hard-kill a fraction (~1/3) of the selected routers instead of a
		// graceful restart: SIGKILL gives no chance to close links or send
		// faults, exercising orphan creation that graceful shutdown hides. The
		// gossip reconcile/digest paths must converge these away. Controllers
		// stay graceful (raft sensitivity).
		var gracefulRouters, hardKillRouters []*model.Component
		for _, r := range routers {
			if rand.Intn(3) == 0 {
				hardKillRouters = append(hardKillRouters, r)
			} else {
				gracefulRouters = append(gracefulRouters, r)
			}
		}
		hardKillCount = len(hardKillRouters)
		gracefulList := append(gracefulRouters, controllers...)
		tasks = append(tasks, chaos.RestartTasks(run, gracefulList...)...)
		tasks = append(tasks, chaos.HardKillRestartTasks(run, hardKillRouters...)...)
	}

	// Generate disruption tasks for disruption and both modes
	var disruptionCount int
	if mode == chaosModeDisruptions || mode == chaosModeBoth {
		disruptionTasks := generateDisruptionTasks(run)
		disruptionCount = len(disruptionTasks)
		tasks = append(tasks, disruptionTasks...)

		// Freeze a few routers (SIGSTOP) for longer than the ~60s peer link-close
		// timeout, so peers close their links while the node is paused and it
		// resumes (SIGCONT) holding stale link state the gossip reconcile must
		// converge away — a node alive-but-not-processing, which clean DROPs and
		// restarts don't reproduce.
		frozenRouters, err := chaos.SelectRandom(run, ".router", chaos.RandomInRange(0, 4))
		if err != nil {
			return err
		}
		frozenCount = len(frozenRouters)
		tasks = append(tasks, chaos.FreezeResumeTasks(run, 70*time.Second, 90*time.Second, frozenRouters...)...)

		// Degrade a few router hosts' egress with netem (latency + jitter + loss)
		// for a window. netem applies to the whole interface, so this degrades
		// every router on the host together — the degraded-but-not-blocked regime
		// that clean iptables DROPs miss. Dedup by host since hosts run many
		// routers.
		netemRouters, err := chaos.SelectRandom(run, ".router", chaos.RandomInRange(1, 4))
		if err != nil {
			return err
		}
		netemHosts := map[string]bool{}
		for _, r := range netemRouters {
			if netemHosts[r.Host.Id] {
				continue
			}
			netemHosts[r.Host.Id] = true
			tasks = append(tasks, makeNetemTask(r.Host, 50+rand.Intn(150), 10+rand.Intn(30), 1+rand.Intn(10)))
		}
		netemCount = len(netemHosts)
	}

	tui.ValidationLogger().Infof("chaos mode: %v — restarting %v controllers and %v routers (%v hard-killed), %v frozen, with %v disruption tasks and %v netem-degraded hosts",
		mode, controllerCount, routerCount, hardKillCount, frozenCount, disruptionCount, netemCount)

	return parallel.Execute(tasks, 250)
}

// generateDisruptionTasks builds a randomized mix of network partitions across
// the cluster: controller inbound blocks, controller mesh partitions,
// router->controller partitions, and router->router link partitions. Each task
// blocks for a short window then unblocks. Ports: 6262 = controller ctrl
// channel, 6000+ScaleIndex = router link listener.
func generateDisruptionTasks(run model.Run) []parallel.Task {
	var tasks []parallel.Task

	allCtrls := run.GetModel().SelectComponents(".ctrl")
	allRouters := run.GetModel().SelectComponents(".router")

	outgoingDisruptions := map[disruptionKey]bool{}

	// 0-1 ctrl incoming disruption: block ALL inbound to a random ctrl on port 6262
	if rand.Intn(2) > 0 {
		ctrl := allCtrls[rand.Intn(len(allCtrls))]
		tasks = append(tasks, makeIncomingDisruptionTask(ctrl.Host, 6262))
	}

	// 0-3 ctrl mesh disruptions: ctrl_A → ctrl_B on port 6262
	for _, pair := range randomPairs(len(allCtrls), len(allCtrls), rand.Intn(4), true) {
		srcCtrl := allCtrls[pair[0]]
		dstCtrl := allCtrls[pair[1]]
		key := disruptionKey{sourceHostId: srcCtrl.Host.Id, destIP: dstCtrl.Host.PublicIp, destPort: 6262}
		if !outgoingDisruptions[key] {
			outgoingDisruptions[key] = true
			tasks = append(tasks, makeOutgoingDisruptionTask(srcCtrl.Host, dstCtrl.Host, 6262))
		}
	}

	// 0-100 router→ctrl disruptions
	for _, pair := range randomPairs(len(allRouters), len(allCtrls), rand.Intn(101), false) {
		router := allRouters[pair[0]]
		ctrl := allCtrls[pair[1]]
		key := disruptionKey{sourceHostId: router.Host.Id, destIP: ctrl.Host.PublicIp, destPort: 6262}
		if !outgoingDisruptions[key] {
			outgoingDisruptions[key] = true
			tasks = append(tasks, makeOutgoingDisruptionTask(router.Host, ctrl.Host, 6262))
		}
	}

	// 0-800 link disruptions: routerA → routerB link port
	for _, pair := range randomPairs(len(allRouters), len(allRouters), rand.Intn(801), true) {
		srcRouter := allRouters[pair[0]]
		dstRouter := allRouters[pair[1]]
		port := uint16(6000 + dstRouter.ScaleIndex)
		key := disruptionKey{sourceHostId: srcRouter.Host.Id, destIP: dstRouter.Host.PublicIp, destPort: port}
		if !outgoingDisruptions[key] {
			outgoingDisruptions[key] = true
			tasks = append(tasks, makeOutgoingDisruptionTask(srcRouter.Host, dstRouter.Host, port))
		}
	}

	return tasks
}

// unblockAllHosts clears any leftover network disruptions across all hosts.
// Run before validation so it never observes a cluster mid-partition.
func unblockAllHosts(run model.Run) error {
	return run.GetModel().ForEachHost("*", 500, func(h *model.Host) error {
		return h.UnblockAll()
	})
}

func validateLinks(run model.Run) error {
	ctrls := run.GetModel().SelectComponents(".ctrl")
	errC := make(chan error, len(ctrls))
	for _, ctrl := range ctrls {
		ctrlComponent := ctrl
		go validateLinksForCtrlWithChan(run, ctrlComponent, errC)
	}

	for i := 0; i < len(ctrls); i++ {
		err := <-errC
		if err != nil {
			return err
		}
	}

	return nil
}

func validateLinksForCtrlWithChan(run model.Run, c *model.Component, errC chan<- error) {
	errC <- validateLinksForCtrl(run, c)
}

// expectedLinkCount is the number of links in a fully-converged mesh: exactly
// one link per unordered router pair (links are single-dialer), C(400,2) =
// 400*399/2.
const expectedLinkCount = 79800

// validationTimeout is the per-controller budget to reach a fully-converged,
// validated state. maxHostUnreachableExtension is how much of that budget can be
// reclaimed when the controller's host is unreachable, so an infra/network blip
// (as opposed to slow convergence) does not fail the run.
const (
	validationTimeout           = 15 * time.Minute
	maxHostUnreachableExtension = 10 * time.Minute
)

func validateLinksForCtrl(run model.Run, c *model.Component) error {
	clients, err := chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
	if err != nil {
		return err
	}

	deadline := chaos.NewConvergenceDeadline(validationTimeout, maxHostUnreachableExtension)
	allLinksPresent := false
	start := time.Now()

	logger := tui.ValidationLogger().WithField("ctrl", c.Id)
	var lastLog time.Time
	for !deadline.Expired() && !allLinksPresent {
		linkCount, err := getLinkCount(clients)
		if err != nil {
			// A failure to reach the controller is only a convergence problem if
			// the host is up; if the host itself is unreachable it is infra, so
			// extend the deadline rather than charge it to the budget. Either way
			// keep retrying (re-login best-effort) until the deadline; a login
			// error no longer fails the run outright, since a transient blip
			// would otherwise look like non-convergence.
			if deadline.ExtendForUnreachableHost(run, c) {
				logger.Warn("controller host unreachable; extending deadline (infra, not convergence)")
			}
			logger.WithError(err).Warn("failed to get link count, retrying")
			time.Sleep(5 * time.Second)
			if newClients, loginErr := chaos.EnsureLoggedIntoCtrl(run, c, time.Minute); loginErr == nil {
				clients = newClients
			} else {
				logger.WithError(loginErr).Warn("failed to log in to controller, will retry")
			}
			continue
		}
		if linkCount == expectedLinkCount {
			allLinksPresent = true
		} else {
			time.Sleep(5 * time.Second)
		}
		if time.Since(lastLog) > time.Minute {
			logger.Infof("current link count: %v, elapsed time: %v", linkCount, time.Since(start))
			lastLog = time.Now()
		}
	}

	if allLinksPresent {
		logger.Infof("all links present, elapsed time: %v", time.Since(start))
	} else {
		linkCount, _ := getLinkCount(clients)
		logLinkDiagnostics(logger, clients, linkCount)
		return fmt.Errorf("fail to reach expected link count of %d on controller %v (got %v)", expectedLinkCount, c.Id, linkCount)
	}

	for {
		linkErrs, err := validateRouterLinks(c.Id, clients)
		var gossipErrs int
		if err == nil {
			gossipErrs, err = validateGossip(c.Id, clients)
		}
		if err == nil {
			return nil
		}

		// As above: don't let an unreachable-host (infra) outage count against the
		// deadline; a real validation error (host reachable, state wrong) still
		// fails once the deadline passes.
		deadline.ExtendForUnreachableHost(run, c)
		if deadline.Expired() {
			return err
		}

		logger.Infof("current link errors: %v, gossip errors: %v, elapsed time: %v", linkErrs, gossipErrs, time.Since(start))
		time.Sleep(15 * time.Second)
	}
}

func getLinkCount(clients *zitirest.Clients) (int64, error) {
	ctx, cancelF := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelF()

	filter := "limit 1"
	result, err := clients.Fabric.Link.ListLinks(&link.ListLinksParams{
		Filter:  &filter,
		Context: ctx,
	}, nil)

	if err != nil {
		return 0, err
	}
	linkCount := *result.Payload.Meta.Pagination.TotalCount
	return linkCount, nil
}

func validateRouterLinks(id string, clients *zitirest.Clients) (int, error) {
	logger := tui.ValidationLogger().WithField("ctrl", id)

	closeNotify := make(chan struct{})
	eventNotify := make(chan *mgmt_pb.RouterLinkDetails, 1)

	handleLinkResults := func(msg *channel.Message, _ channel.Channel) {
		detail := &mgmt_pb.RouterLinkDetails{}
		if err := proto.Unmarshal(msg.Body, detail); err != nil {
			pfxlog.Logger().WithError(err).Error("unable to unmarshal router link details")
			return
		}
		eventNotify <- detail
	}

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_ValidateRouterLinksResultType), handleLinkResults)
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

	request := &mgmt_pb.ValidateRouterLinksRequest{
		Filter: "limit none",
	}
	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(10 * time.Second).SendForReply(ch)

	response := &mgmt_pb.ValidateRouterLinksResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(responseMsg, err); err != nil {
		return 0, err
	}

	if !response.Success {
		return 0, fmt.Errorf("failed to start link validation: %s", response.Message)
	}

	logger.Infof("started validation of %v routers", response.RouterCount)

	expected := response.RouterCount

	invalid := 0
	for expected > 0 {
		select {
		case <-closeNotify:
			return 0, errors.New("unexpected close of mgmt channel")
		case routerDetail := <-eventNotify:
			if !routerDetail.ValidateSuccess {
				return invalid, fmt.Errorf("error: unable to validate router %s (%s) on controller %s (%s)",
					routerDetail.RouterId, routerDetail.RouterName, id, routerDetail.Message)
			}
			for _, linkDetail := range routerDetail.LinkDetails {
				if !linkDetail.IsValid {
					invalid++
					logger.Infof("INVALID link %v on router %v (%v): ctrlState=%v routerState=%v destRouter=%v destConnected=%v dialed=%v messages=%v",
						linkDetail.LinkId, routerDetail.RouterId, routerDetail.RouterName,
						linkDetail.CtrlState, linkDetail.RouterState,
						linkDetail.DestRouterId, linkDetail.DestConnected,
						linkDetail.Dialed, linkDetail.Messages)
				}
			}
			expected--
		}
	}
	if invalid == 0 {
		logger.Infof("link validation of %v routers successful", response.RouterCount)
		return invalid, nil
	}
	return invalid, fmt.Errorf("invalid links found")
}

// validateGossip checks link-gossip consistency across the registry, the router
// gossip stores, and this controller's gossip store and link manager. It
// complements validateRouterLinks, which only compares the registry ends against
// the controller; this surfaces divergence in the gossip layers in between.
func validateGossip(id string, clients *zitirest.Clients) (int, error) {
	logger := tui.ValidationLogger().WithField("ctrl", id)

	closeNotify := make(chan struct{})
	eventNotify := make(chan *mgmt_pb.GossipValidationDetails, 1)

	handleResults := func(msg *channel.Message, _ channel.Channel) {
		detail := &mgmt_pb.GossipValidationDetails{}
		if err := proto.Unmarshal(msg.Body, detail); err != nil {
			pfxlog.Logger().WithError(err).Error("unable to unmarshal gossip validation details")
			return
		}
		eventNotify <- detail
	}

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_ValidateGossipResultType), handleResults)
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

	request := &mgmt_pb.ValidateGossipRequest{
		Filter:       "limit none",
		ValidateCtrl: true,
	}
	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(10 * time.Second).SendForReply(ch)

	response := &mgmt_pb.ValidateGossipResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(responseMsg, err); err != nil {
		return 0, err
	}

	if !response.Success {
		return 0, fmt.Errorf("failed to start gossip validation: %s", response.Message)
	}

	logger.Infof("started gossip validation of %v components", response.ComponentCount)

	expected := response.ComponentCount

	invalid := 0
	for expected > 0 {
		select {
		case <-closeNotify:
			return 0, errors.New("unexpected close of mgmt channel")
		case detail := <-eventNotify:
			if !detail.ValidateSuccess && detail.Message != "" {
				return invalid, fmt.Errorf("error: unable to validate gossip for %s %s (%s) on controller %s (%s)",
					detail.ComponentType, detail.ComponentId, detail.ComponentName, id, detail.Message)
			}
			for _, linkDetail := range detail.LinkDetails {
				if !linkDetail.IsValid {
					invalid++
					logger.Infof("INVALID gossip entry %v (iter %v) on %s %v: inSource=%v inLocalGossip=%v inCtrlGossip=%v dest=%v dialed=%v messages=%v",
						linkDetail.LinkId, linkDetail.Iteration, detail.ComponentType, detail.ComponentId,
						linkDetail.InSource, linkDetail.InLocalGossip, linkDetail.InCtrlGossip,
						linkDetail.DestRouterId, linkDetail.Dialed, linkDetail.Messages)
				}
			}
			expected--
		}
	}
	if invalid == 0 {
		logger.Infof("gossip validation of %v components successful", response.ComponentCount)
		return invalid, nil
	}
	return invalid, fmt.Errorf("invalid gossip entries found")
}

func logLinkDiagnostics(logger *logrus.Entry, clients *zitirest.Clients, linkCount int64) {
	logger.Infof("link count mismatch: expected %d, got %v, fetching diagnostics", expectedLinkCount, linkCount)

	ctx, cancelF := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelF()

	filter := "limit none"
	result, err := clients.Fabric.Link.ListLinks(&link.ListLinksParams{
		Filter:  &filter,
		Context: ctx,
	}, nil)
	if err != nil {
		logger.WithError(err).Error("failed to fetch links for diagnostics")
		return
	}

	// Links are single-dialer: each unordered router pair should have exactly one
	// link, in whichever direction the dialer chose. Normalize each link to an
	// unordered pair and look for pairs with the wrong number of links. A directed
	// view (expecting src->dst for every ordered pair) is wrong here — it flags
	// every normal one-directional link as "missing" and every router as off-count.
	type pairKey struct{ a, b string } // a <= b
	linksByPair := map[pairKey][]*rest_model.LinkDetail{}
	routerIds := map[string]struct{}{}

	for _, l := range result.Payload.Data {
		src := l.SourceRouter.ID
		dst := l.DestRouter.ID
		routerIds[src] = struct{}{}
		routerIds[dst] = struct{}{}
		k := pairKey{a: src, b: dst}
		if k.a > k.b {
			k.a, k.b = k.b, k.a
		}
		linksByPair[k] = append(linksByPair[k], l)
	}

	// Duplicate pairs: more than one link for the same unordered pair. This is the
	// real anomaly when the count is over expected (e.g., both directions present
	// because dedup didn't converge).
	dupPairs := 0
	for k, links := range linksByPair {
		if len(links) > 1 {
			dupPairs++
			logger.Infof("DUPLICATE pair %v <-> %v has %d links:", k.a, k.b, len(links))
			for _, l := range links {
				logger.Infof("  link %v: %v -> %v state=%v iteration=%v",
					*l.ID, l.SourceRouter.ID, l.DestRouter.ID, *l.State, *l.Iteration)
			}
		}
	}

	// Missing pairs: unordered router pairs with no link in either direction. This
	// is the real anomaly when the count is under expected.
	ids := make([]string, 0, len(routerIds))
	for id := range routerIds {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	missingPairs := 0
	const missingLogCap = 50
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			k := pairKey{a: ids[i], b: ids[j]}
			if _, ok := linksByPair[k]; !ok {
				missingPairs++
				if missingPairs <= missingLogCap {
					logger.Infof("MISSING pair: %v <-> %v (no link in either direction)", ids[i], ids[j])
				}
			}
		}
	}
	if missingPairs > missingLogCap {
		logger.Infof("... and %d more missing pairs (log output capped at %d)", missingPairs-missingLogCap, missingLogCap)
	}

	logger.Infof("total links: %v, routers: %v, unique pairs: %v, duplicate pairs: %v, missing pairs: %v",
		len(result.Payload.Data), len(routerIds), len(linksByPair), dupPairs, missingPairs)
}
