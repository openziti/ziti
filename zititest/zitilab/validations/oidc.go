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

package validations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	inspectCommon "github.com/openziti/ziti/v2/common/inspect"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	fabricInspect "github.com/openziti/ziti/v2/controller/rest_client/inspect"
	"github.com/openziti/ziti/v2/controller/rest_model"
	"github.com/openziti/ziti/v2/zitirest"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	zitilibOps "github.com/openziti/ziti/zititest/zitilab/runlevel/5_operation"
	"google.golang.org/protobuf/proto"
)

// ValidateOidcAuthenticated waits until every identity in expectedIds has at
// least one JWT "created" or "refreshed" event in the collector. This accepts
// either event type so it works even when the collector was restarted and
// missed the original "created" events from bootstrap.
func ValidateOidcAuthenticated(collector *zitilibOps.OidcEventCollector, expectedIds map[string]bool, timeout time.Duration) error {
	log := tui.ValidationLogger()
	deadline := time.Now().Add(timeout)
	expectedCount := len(expectedIds)
	var lastLog time.Time

	for time.Now().Before(deadline) {
		authenticated := collector.AllAuthenticatedIdentityIds()
		missing := missingIds(expectedIds, authenticated)

		if len(missing) == 0 {
			log.Infof("all %d expected identities authenticated via OIDC", expectedCount)
			return nil
		}

		if time.Since(lastLog) > 15*time.Second {
			log.Infof("OIDC-authenticated identities: %d / %d (total events: %d), waiting...",
				expectedCount-len(missing), expectedCount, collector.TotalEventCount())
			lastLog = time.Now()
		}
		time.Sleep(5 * time.Second)
	}

	authenticated := collector.AllAuthenticatedIdentityIds()
	missing := missingIds(expectedIds, authenticated)
	sample := missing
	if len(sample) > 10 {
		sample = sample[:10]
	}
	return fmt.Errorf("%d of %d identities never authenticated. sample: %v (total events: %d)",
		len(missing), expectedCount, sample, collector.TotalEventCount())
}

// ValidateOidcNewSessions waits until every identity in expectedIds has a JWT
// "created" event since the given timestamp. Use this after restarting clients
// to confirm they performed a full OIDC authentication.
func ValidateOidcNewSessions(collector *zitilibOps.OidcEventCollector, expectedIds map[string]bool, since time.Time, timeout time.Duration) error {
	log := tui.ValidationLogger()
	deadline := time.Now().Add(timeout)
	expectedCount := len(expectedIds)
	var lastLog time.Time

	for time.Now().Before(deadline) {
		created := collector.CreatedIdentitiesSince(since)
		missing := missingIds(expectedIds, created)

		if len(missing) == 0 {
			log.Infof("all %d expected identities created new OIDC sessions since %s",
				expectedCount, since.UTC().Format(time.RFC3339))
			return nil
		}

		if time.Since(lastLog) > 15*time.Second {
			log.Infof("new OIDC sessions: %d / %d since %s (total events: %d), waiting...",
				expectedCount-len(missing), expectedCount,
				since.UTC().Format(time.RFC3339), collector.TotalEventCount())
			lastLog = time.Now()
		}
		time.Sleep(5 * time.Second)
	}

	created := collector.CreatedIdentitiesSince(since)
	missing := missingIds(expectedIds, created)
	sample := missing
	if len(sample) > 10 {
		sample = sample[:10]
	}
	return fmt.Errorf("%d of %d identities did not create new sessions since %s. sample: %v (total events: %d)",
		len(missing), expectedCount, since.UTC().Format(time.RFC3339), sample, collector.TotalEventCount())
}

// ValidateAllIdentitiesRefreshed checks that every identity in expectedIds has
// at least one JWT "refreshed" event since the given timestamp.
func ValidateAllIdentitiesRefreshed(collector *zitilibOps.OidcEventCollector, expectedIds map[string]bool, since time.Time) error {
	refreshed := collector.RefreshedIdentitiesSince(since)
	missing := missingIds(expectedIds, refreshed)

	if len(missing) > 0 {
		sample := missing
		if len(sample) > 10 {
			sample = sample[:10]
		}
		return fmt.Errorf("%d of %d identities did not refresh since %s. sample: %v",
			len(missing), len(expectedIds), since.UTC().Format(time.RFC3339), sample)
	}

	tui.ValidationLogger().Infof("all %d identities refreshed since %s (%d refresh events)",
		len(expectedIds), since.UTC().Format(time.RFC3339), collector.RefreshEventsSince(since))
	return nil
}

// missingIds returns the IDs in expected that are not in actual.
func missingIds(expected, actual map[string]bool) []string {
	var missing []string
	for id := range expected {
		if !actual[id] {
			missing = append(missing, id)
		}
	}
	return missing
}

// ValidateRevocationHealth checks controller logs for revocation queue overflow
// warnings since the given timestamp.
func ValidateRevocationHealth(run model.Run, since time.Time) error {
	log := tui.ValidationLogger()
	ctrls := run.GetModel().SelectComponents(".ctrl")
	sinceStr := since.UTC().Format(time.RFC3339)

	for _, ctrl := range ctrls {
		user := ctrl.Host.GetSshUser()
		logFile := fmt.Sprintf("/home/%s/logs/%s.log", user, ctrl.Id)

		cmd := fmt.Sprintf(
			`grep "revocation.*queue.*full\|revocation.*dropped\|revocation.*overflow" %s 2>/dev/null `+
				`| jq -r 'select(.time >= "%s")' | wc -l`,
			logFile, sinceStr,
		)
		output, err := ctrl.Host.ExecLogged(cmd)
		if err != nil {
			log.WithError(err).WithField("ctrl", ctrl.Id).Warn("failed to check revocation health")
			continue
		}

		output = strings.TrimSpace(output)
		if output != "" && output != "0" {
			return fmt.Errorf("controller %s has %s revocation queue overflow warnings since %s",
				ctrl.Id, output, sinceStr)
		}
	}

	log.Info("revocation health check passed on all controllers")
	return nil
}

// ValidateIdentityConnectionStatuses checks that all identities with active
// sessions are connected to edge routers. It queries each controller via the
// management WebSocket channel, which fans out to all routers.
func ValidateIdentityConnectionStatuses(run model.Run, timeout time.Duration) error {
	ctrls := run.GetModel().SelectComponents(".ctrl")
	errC := make(chan error, len(ctrls))
	deadline := time.Now().Add(timeout)

	for _, ctrl := range ctrls {
		ctrlComponent := ctrl
		go func() {
			errC <- validateConnectionStatusesForCtrl(run, ctrlComponent, deadline)
		}()
	}

	for range len(ctrls) {
		if err := <-errC; err != nil {
			return err
		}
	}

	return nil
}

func validateConnectionStatusesForCtrl(run model.Run, c *model.Component, deadline time.Time) error {
	logger := tui.ValidationLogger().WithField("ctrl", c.Id)
	start := time.Now()

	var clients *zitirest.Clients
	for {
		if clients == nil {
			var err error
			clients, err = chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
			if err != nil {
				logger.WithError(err).Info("error logging into ctrl, will retry")
				if time.Now().After(deadline) {
					return err
				}
				time.Sleep(5 * time.Second)
				continue
			}
		}

		count, err := checkConnectionStatuses(c.Id, clients)
		if err == nil {
			return nil
		}

		clients = nil

		if time.Now().After(deadline) {
			return err
		}

		logger.Infof("invalid identity connection statuses: %d, elapsed: %v", count, time.Since(start))
		time.Sleep(5 * time.Second)
	}
}

func checkConnectionStatuses(ctrlId string, clients *zitirest.Clients) (int, error) {
	logger := tui.ValidationLogger().WithField("ctrl", ctrlId)

	closeNotify := make(chan struct{})
	eventNotify := make(chan *mgmt_pb.RouterIdentityConnectionStatusesDetails, 1)

	handleResults := func(msg *channel.Message, _ channel.Channel) {
		detail := &mgmt_pb.RouterIdentityConnectionStatusesDetails{}
		if err := proto.Unmarshal(msg.Body, detail); err != nil {
			pfxlog.Logger().WithError(err).Error("unable to unmarshal identity connection status details")
			return
		}
		eventNotify <- detail
	}

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_ValidateIdentityConnectionStatusesResultType), handleResults)
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

	request := &mgmt_pb.ValidateIdentityConnectionStatusesRequest{
		RouterFilter: "limit none",
	}
	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(10 * time.Second).SendForReply(ch)

	response := &mgmt_pb.ValidateIdentityConnectionStatusesResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(responseMsg, err); err != nil {
		return 0, err
	}

	if !response.Success {
		return 0, fmt.Errorf("failed to start identity connection status validation: %s", response.Message)
	}

	logger.Infof("started identity connection status validation of %d routers", response.ComponentCount)

	expected := response.ComponentCount
	invalid := 0
	for expected > 0 {
		select {
		case <-closeNotify:
			return 0, errors.New("unexpected close of mgmt channel")
		case routerDetail := <-eventNotify:
			for _, errMsg := range routerDetail.Errors {
				logger.Infof("router %s (%s): %s", routerDetail.ComponentId, routerDetail.ComponentName, errMsg)
				invalid++
			}
			expected--
		}
	}

	if invalid == 0 {
		logger.Infof("identity connection status validation of %d routers successful", response.ComponentCount)
		return 0, nil
	}
	return invalid, fmt.Errorf("%d invalid identity connection statuses found", invalid)
}

// ValidateIdentitiesConnected waits until every identity in identityIds has at
// least one active (non-closed) connection to an edge router. It uses the REST
// inspect API to query each router's identity connection state.
//
// This is a stronger readiness check than ValidateOidcAuthenticated: OIDC auth
// only confirms the SDK received a JWT, not that it successfully fetched the
// api-session, loaded services, and connected to routers. Under load, those
// post-auth steps can lag behind JWT issuance by minutes.
func ValidateIdentitiesConnected(run model.Run, identityIds map[string]bool, timeout time.Duration) error {
	log := tui.ValidationLogger()
	deadline := time.Now().Add(timeout)
	expectedCount := len(identityIds)
	var lastLog time.Time

	for time.Now().Before(deadline) {
		connected, err := getConnectedIdentities(run)
		if err != nil {
			log.WithError(err).Warn("failed to get connected identities, will retry")
			time.Sleep(5 * time.Second)
			continue
		}

		var missing []string
		for id := range identityIds {
			if !connected[id] {
				missing = append(missing, id)
			}
		}

		if len(missing) == 0 {
			log.Infof("confirmed %d identities are connected to edge routers", expectedCount)
			return nil
		}

		if time.Since(lastLog) > 15*time.Second {
			log.Infof("identities connected to routers: %d / %d, waiting...",
				expectedCount-len(missing), expectedCount)
			lastLog = time.Now()
		}
		time.Sleep(5 * time.Second)
	}

	connected, err := getConnectedIdentities(run)
	if err != nil {
		return fmt.Errorf("failed to get connected identities: %w", err)
	}

	var missing []string
	for id := range identityIds {
		if !connected[id] {
			missing = append(missing, id)
		}
	}

	sample := missing
	if len(sample) > 10 {
		sample = sample[:10]
	}
	return fmt.Errorf("%d of %d identities never connected to any edge router after %s. sample: %v",
		len(missing), expectedCount, timeout, sample)
}

// ValidateIdentitiesDisconnected checks that none of the specified identities
// have active connections to any edge router. It uses the REST inspect API to
// query each router's identity connection state.
func ValidateIdentitiesDisconnected(run model.Run, identityIds map[string]bool, timeout time.Duration) error {
	log := tui.ValidationLogger()
	deadline := time.Now().Add(timeout)
	var lastLog time.Time

	for time.Now().Before(deadline) {
		connected, err := getConnectedIdentities(run)
		if err != nil {
			log.WithError(err).Warn("failed to get connected identities, will retry")
			time.Sleep(5 * time.Second)
			continue
		}

		var stillConnected []string
		for id := range identityIds {
			if connected[id] {
				stillConnected = append(stillConnected, id)
			}
		}

		if len(stillConnected) == 0 {
			log.Infof("confirmed %d identities are disconnected from all routers", len(identityIds))
			return nil
		}

		if time.Since(lastLog) > 15*time.Second {
			log.Infof("%d of %d identities still connected, waiting...", len(stillConnected), len(identityIds))
			lastLog = time.Now()
		}
		time.Sleep(10 * time.Second)
	}

	// Final check for error reporting.
	connected, err := getConnectedIdentities(run)
	if err != nil {
		return fmt.Errorf("failed to get connected identities: %w", err)
	}

	var stillConnected []string
	for id := range identityIds {
		if connected[id] {
			stillConnected = append(stillConnected, id)
		}
	}

	sample := stillConnected
	if len(sample) > 10 {
		sample = sample[:10]
	}
	return fmt.Errorf("%d of %d identities still connected after %s. sample: %v",
		len(stillConnected), len(identityIds), timeout, sample)
}

// getConnectedIdentities queries the controller inspect API for identity
// connection statuses across all routers and returns the set of identity IDs
// that have at least one active (non-closed) connection.
func getConnectedIdentities(run model.Run) (map[string]bool, error) {
	ctrls := run.GetModel().SelectComponents(".ctrl")
	if len(ctrls) == 0 {
		return nil, fmt.Errorf("no controllers found")
	}

	// Use the first available controller.
	var clients *zitirest.Clients
	var lastErr error
	for _, ctrl := range ctrls {
		var err error
		clients, err = chaos.EnsureLoggedIntoCtrl(run, ctrl, time.Minute)
		if err != nil {
			lastErr = err
			continue
		}
		break
	}
	if clients == nil {
		return nil, fmt.Errorf("failed to log into any controller: %w", lastErr)
	}

	appRegex := ".*"
	inspectResult, err := clients.Fabric.Inspect.Inspect(&fabricInspect.InspectParams{
		Request: &rest_model.InspectRequest{
			AppRegex:        &appRegex,
			RequestedValues: []string{inspectCommon.RouterIdentityConnectionStatusesKey},
		},
		Context: context.Background(),
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("inspect request failed: %w", err)
	}

	resp := inspectResult.Payload
	if !*resp.Success {
		return nil, fmt.Errorf("inspect request unsuccessful: %v", resp.Errors)
	}

	connected := make(map[string]bool)
	for _, value := range resp.Values {
		if *value.Name != inspectCommon.RouterIdentityConnectionStatusesKey {
			continue
		}

		jsonBytes, err := json.Marshal(value.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal inspect value for router %s: %w", *value.AppID, err)
		}

		var routerConns inspectCommon.RouterIdentityConnections
		if err := json.Unmarshal(jsonBytes, &routerConns); err != nil {
			return nil, fmt.Errorf("failed to unmarshal identity connections for router %s: %w", *value.AppID, err)
		}

		for identityId, detail := range routerConns.IdentityConnections {
			for _, conn := range detail.Connections {
				if !conn.Closed {
					connected[identityId] = true
					break
				}
			}
		}
	}

	return connected, nil
}
