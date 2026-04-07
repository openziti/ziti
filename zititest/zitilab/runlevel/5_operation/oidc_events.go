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

package zitilib_runlevel_5_operation

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/foundation/v2/util"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/enroll"
	"github.com/openziti/ziti/v2/controller/event"
	"github.com/openziti/ziti/v2/zitirest"
	"github.com/openziti/ziti/zititest/zitilab/models"
)

const OidcEventCollectorName = "oidc-event-collector"

// ApiSessionEvent mirrors the controller's event.ApiSessionEvent structure.
type ApiSessionEvent struct {
	Namespace  string    `json:"namespace"`
	EventSrcId string    `json:"event_src_id"`
	Timestamp  time.Time `json:"timestamp"`
	EventType  string    `json:"event_type"`
	Id         string    `json:"id"`
	Type       string    `json:"type"`
	Token      string    `json:"token"`
	IdentityId string    `json:"identity_id"`
	IpAddress  string    `json:"ip_address"`
}

// OidcEventCollector listens on a Ziti service for apiSession events streamed
// from controller hosts. Events are stored in memory and indexed by identity
// for per-identity validation queries.
type OidcEventCollector struct {
	mu          sync.RWMutex
	events      []ApiSessionEvent
	byIdentity  map[string][]ApiSessionEvent
	zitiContext ziti.Context
	listener    net.Listener
	started     atomic.Bool
	connections atomic.Int32
}

// SetupCollectorIdentity creates and enrolls the collector identity using the
// provided management API clients. Called during the model's activation stage.
func (self *OidcEventCollector) SetupCollectorIdentity(run model.Run, clients *zitirest.Clients) error {
	return setupCollectorIdentity(run, clients, OidcEventCollectorName, "oidc-event-collector")
}

// StartCollecting begins listening on the given Ziti service for event
// connections. Called during the model's operating stage.
func (self *OidcEventCollector) StartCollecting(run model.Run, service string) error {
	if !self.started.CompareAndSwap(false, true) {
		return nil
	}

	self.byIdentity = make(map[string][]ApiSessionEvent)

	configPath := run.GetLabel().GetFilePath(OidcEventCollectorName + ".json")
	cfg, err := ziti.NewConfigFromFile(configPath)
	if err != nil {
		return err
	}

	ctx, err := ziti.NewContext(cfg)
	if err != nil {
		return err
	}
	self.zitiContext = ctx

	listener, err := ctx.Listen(service)
	if err != nil {
		return err
	}
	self.listener = listener

	go func() {
		log := pfxlog.Logger()
		log.Infof("oidc event collector listening on service %q", service)
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.WithError(err).Info("oidc event listener closed")
				return
			}
			go self.handleConnection(conn)
		}
	}()

	return nil
}

// StartCollectingStage returns a model.Stage that starts event collection.
func (self *OidcEventCollector) StartCollectingStage(service string) model.Stage {
	return model.StageActionF(func(run model.Run) error {
		return self.StartCollecting(run, service)
	})
}

// WaitForConnections blocks until at least n forwarder connections have been
// received or the timeout expires.
func (self *OidcEventCollector) WaitForConnections(n int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if int(self.connections.Load()) >= n {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("timed out waiting for %d event forwarder connections (have %d)", n, self.connections.Load())
}

func (self *OidcEventCollector) handleConnection(conn net.Conn) {
	defer conn.Close()
	self.connections.Add(1)
	defer self.connections.Add(-1)
	log := pfxlog.Logger()
	log.Infof("new event forwarder connection from %s", conn.RemoteAddr())

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		var evt ApiSessionEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			if len(line) > 0 {
				log.Errorf("unmarshal failed for line (%d bytes): %v", len(line), err)
			}
			continue
		}

		// Only index apiSession events with JWT type.
		if evt.Namespace != event.ApiSessionEventNS || evt.Type != "jwt" {
			continue
		}

		self.mu.Lock()
		self.events = append(self.events, evt)
		self.byIdentity[evt.IdentityId] = append(self.byIdentity[evt.IdentityId], evt)
		self.mu.Unlock()
	}

	if err := scanner.Err(); err != nil {
		log.WithError(err).Info("event forwarder connection read error")
	}
}

// UniqueCreatedIdentities returns the number of unique identities that have
// at least one JWT "created" event.
func (self *OidcEventCollector) UniqueCreatedIdentities() int {
	return len(self.AllCreatedIdentityIds())
}

// AllCreatedIdentityIds returns the set of all identity IDs that have at least
// one JWT "created" event.
func (self *OidcEventCollector) AllCreatedIdentityIds() map[string]bool {
	self.mu.RLock()
	defer self.mu.RUnlock()

	seen := map[string]bool{}
	for _, evt := range self.events {
		if evt.EventType == "created" {
			seen[evt.IdentityId] = true
		}
	}
	return seen
}

// AllAuthenticatedIdentityIds returns the set of all identity IDs that have at
// least one JWT "created" or "refreshed" event. This is a lenient check that
// works even when the collector was restarted and missed the original "created"
// events.
func (self *OidcEventCollector) AllAuthenticatedIdentityIds() map[string]bool {
	self.mu.RLock()
	defer self.mu.RUnlock()

	seen := map[string]bool{}
	for _, evt := range self.events {
		if evt.EventType == "created" || evt.EventType == "refreshed" || evt.EventType == "exchanged" {
			seen[evt.IdentityId] = true
		}
	}
	return seen
}

// RefreshEventsSince returns the count of JWT "refreshed" or "exchanged"
// events after the given timestamp. The Go SDK uses "exchanged" for token
// renewal while the C-SDK uses "refreshed".
func (self *OidcEventCollector) RefreshEventsSince(since time.Time) int {
	self.mu.RLock()
	defer self.mu.RUnlock()

	count := 0
	for _, evt := range self.events {
		if (evt.EventType == "refreshed" || evt.EventType == "exchanged") && evt.Timestamp.After(since) {
			count++
		}
	}
	return count
}

// CreatedIdentitiesSince returns the set of identity IDs that have JWT "created"
// events after the given timestamp.
func (self *OidcEventCollector) CreatedIdentitiesSince(since time.Time) map[string]bool {
	self.mu.RLock()
	defer self.mu.RUnlock()

	result := map[string]bool{}
	for _, evt := range self.events {
		if evt.EventType == "created" && evt.Timestamp.After(since) {
			result[evt.IdentityId] = true
		}
	}
	return result
}

// RefreshedIdentitiesSince returns the set of identity IDs that have JWT
// "refreshed" or "exchanged" events after the given timestamp. The Go SDK
// uses "exchanged" for token renewal while the C-SDK uses "refreshed".
func (self *OidcEventCollector) RefreshedIdentitiesSince(since time.Time) map[string]bool {
	self.mu.RLock()
	defer self.mu.RUnlock()

	result := map[string]bool{}
	for _, evt := range self.events {
		if (evt.EventType == "refreshed" || evt.EventType == "exchanged") && evt.Timestamp.After(since) {
			result[evt.IdentityId] = true
		}
	}
	return result
}

// EventsForIdentity returns all events for a specific identity, ordered by
// insertion time.
func (self *OidcEventCollector) EventsForIdentity(identityId string) []ApiSessionEvent {
	self.mu.RLock()
	defer self.mu.RUnlock()

	return append([]ApiSessionEvent{}, self.byIdentity[identityId]...)
}

// TotalEventCount returns the total number of JWT apiSession events collected.
func (self *OidcEventCollector) TotalEventCount() int {
	self.mu.RLock()
	defer self.mu.RUnlock()
	return len(self.events)
}

// setupCollectorIdentity creates and enrolls a collector identity using the
// management REST API and SDK enrollment. It writes the enrolled config to
// a file named <name>.json in the run label directory.
func setupCollectorIdentity(run model.Run, clients *zitirest.Clients, name, roleAttribute string) error {
	log := pfxlog.Logger().WithField("identity", name)

	// Delete existing identity if present
	existingId, err := models.GetIdentityId(clients, name, 5*time.Second)
	if err == nil {
		if err = models.DeleteIdentity(clients, existingId, 15*time.Second); err != nil {
			log.WithError(err).Warn("failed to delete existing identity, continuing")
		}
	}

	identityType := rest_model.IdentityTypeDefault
	newId, err := models.CreateIdentity(clients, &rest_model.IdentityCreate{
		Enrollment:     &rest_model.IdentityCreateEnrollment{Ott: true},
		IsAdmin:        util.Ptr(false),
		Name:           util.Ptr(name),
		RoleAttributes: util.Ptr(rest_model.Attributes{roleAttribute}),
		Type:           &identityType,
	}, 15*time.Second)
	if err != nil {
		return fmt.Errorf("failed to create identity %s: %w", name, err)
	}

	detail, err := models.DetailIdentity(clients, newId, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get identity detail for %s: %w", name, err)
	}

	if detail.Enrollment == nil || detail.Enrollment.Ott == nil || detail.Enrollment.Ott.JWT == "" {
		return fmt.Errorf("identity %s has no OTT enrollment JWT", name)
	}

	jwtStr := detail.Enrollment.Ott.JWT
	claims, jwtToken, err := enroll.ParseToken(jwtStr)
	if err != nil {
		return fmt.Errorf("failed to parse enrollment JWT for %s: %w", name, err)
	}

	var keyAlg ziti.KeyAlgVar
	_ = keyAlg.Set("RSA")

	conf, err := enroll.Enroll(enroll.EnrollmentFlags{
		Token:     claims,
		JwtToken:  jwtToken,
		JwtString: jwtStr,
		KeyAlg:    keyAlg,
	})
	if err != nil {
		return fmt.Errorf("failed to enroll identity %s: %w", name, err)
	}

	configPath := run.GetLabel().GetFilePath(name + ".json")
	output, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("failed to create config file for %s: %w", name, err)
	}

	enc := json.NewEncoder(output)
	enc.SetEscapeHTML(false)
	encErr := enc.Encode(conf)
	_ = output.Close()

	if encErr != nil {
		return fmt.Errorf("failed to write config for %s: %w", name, encErr)
	}

	log.Info("collector identity initialized successfully")
	return nil
}
