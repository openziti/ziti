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
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/concurrenz"
)

const debugAddr = "127.0.0.1:6060"

var lastRestarted concurrenz.AtomicValue[time.Time]

// startDebugServer starts an HTTP server exposing test state for debugging.
// It runs in the background and never blocks.
func startDebugServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/missing-oidc", handleMissingOidc)
	mux.HandleFunc("/debug/missing-oidc-created", handleMissingOidcCreated)
	mux.HandleFunc("/debug/oidc-stats", handleOidcStats)
	mux.HandleFunc("/debug/traffic-stats", handleTrafficStats)
	mux.HandleFunc("/debug/events", handleEvents)
	mux.HandleFunc("/debug/identity", handleIdentity)

	go func() {
		pfxlog.Logger().Infof("debug server listening on %s", debugAddr)
		if err := http.ListenAndServe(debugAddr, mux); err != nil {
			pfxlog.Logger().WithError(err).Warn("debug server stopped")
		}
	}()
}

// GET /debug/missing-oidc
// Returns the list of expected identity IDs that have no OIDC "created" or "refreshed" event.
func handleMissingOidc(w http.ResponseWriter, _ *http.Request) {
	if clientIdentityIds == nil {
		http.Error(w, "identity registry not loaded", http.StatusServiceUnavailable)
		return
	}

	authenticated := eventCollector.AllAuthenticatedIdentityIds()
	var missing []string
	for id := range clientIdentityIds {
		if !authenticated[id] {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)

	writeJSON(w, map[string]any{
		"expectedCount":      len(clientIdentityIds),
		"authenticatedCount": len(authenticated),
		"missingCount":       len(missing),
		"missing":            missing,
	})
}

// GET /debug/missing-oidc-created
// Returns the list of expected identity IDs that have no OIDC "created" or "refreshed" event.
func handleMissingOidcCreated(w http.ResponseWriter, _ *http.Request) {
	if clientIdentityIds == nil {
		http.Error(w, "identity registry not loaded", http.StatusServiceUnavailable)
		return
	}

	authenticated := eventCollector.CreatedIdentitiesSince(lastRestarted.Load())
	var missing []string
	for id := range clientIdentityIds {
		if !authenticated[id] {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)

	writeJSON(w, map[string]any{
		"expectedCount":      len(clientIdentityIds),
		"authenticatedCount": len(authenticated),
		"missingCount":       len(missing),
		"missing":            missing,
	})
}

// GET /debug/oidc-stats
// Returns summary statistics about collected OIDC events.
func handleOidcStats(w http.ResponseWriter, r *http.Request) {
	sinceStr := r.URL.Query().Get("since")
	since := time.Time{}
	if sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		}
	}

	writeJSON(w, map[string]any{
		"totalEvents":            eventCollector.TotalEventCount(),
		"uniqueCreated":          eventCollector.UniqueCreatedIdentities(),
		"refreshEventsSince":     eventCollector.RefreshEventsSince(since),
		"createdIdentitiesSince": len(eventCollector.CreatedIdentitiesSince(since)),
		"expectedIdentities":     len(clientIdentityIds),
		"proxIdentities":         len(proxIdentityIds),
		"goClientIdentities":     len(goClientIdentityIds),
	})
}

// GET /debug/traffic-stats
// Returns traffic collector summary.
func handleTrafficStats(w http.ResponseWriter, r *http.Request) {
	since := time.Now().Add(-time.Minute)
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		}
	}

	writeJSON(w, map[string]any{
		"totalCount":   trafficCollector.TotalCount(),
		"successCount": trafficCollector.SuccessCount(since),
		"errorCount":   trafficCollector.ErrorCount(since),
		"since":        since.UTC().Format(time.RFC3339),
	})
}

// GET /debug/events?identity=<id>
// Returns all collected events for a specific identity.
func handleEvents(w http.ResponseWriter, r *http.Request) {
	identityId := r.URL.Query().Get("identity")
	if identityId == "" {
		http.Error(w, "identity query parameter required", http.StatusBadRequest)
		return
	}

	events := eventCollector.EventsForIdentity(identityId)
	writeJSON(w, map[string]any{
		"identityId": identityId,
		"eventCount": len(events),
		"events":     events,
	})
}

// GET /debug/identity?id=<id>
// Reports whether an identity is in the expected sets and whether it has authenticated.
func handleIdentity(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id query parameter required", http.StatusBadRequest)
		return
	}

	authenticated := eventCollector.AllAuthenticatedIdentityIds()
	events := eventCollector.EventsForIdentity(id)

	writeJSON(w, map[string]any{
		"identityId":    id,
		"inClientSet":   clientIdentityIds[id],
		"inProxSet":     proxIdentityIds[id],
		"inGoClientSet": goClientIdentityIds[id],
		"authenticated": authenticated[id],
		"eventCount":    len(events),
		"events":        events,
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(w, `{"error": %q}`, err.Error())
	}
}
