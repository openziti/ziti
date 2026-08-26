/*
	(c) Copyright NetFoundry Inc.

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
	"strconv"
	"strings"
	"time"

	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/openziti/ziti/zititest/zitilab/cli"
)

// apiSessionEvent mirrors the fields we care about from the controller's apiSession event
// (controller/event/api_session.go). Type is "legacy" or "jwt"; jwt is an OIDC/token session.
type apiSessionEvent struct {
	Namespace  string `json:"namespace"`
	EventType  string `json:"event_type"`
	Type       string `json:"type"`
	IdentityId string `json:"identity_id"`
}

// restartAndVerifyOidc restarts the -restart instance of each client/host pair (leaving the -stable
// instance up) and reads the controller's apiSession event log to determine the session type each
// restarted client authenticated with. Go SDK clients/hosts are expected to switch to OIDC (jwt) once
// the controller advertises it; ZET (C SDK) behavior is recorded informationally.
func restartAndVerifyOidc(run model.Run) error {
	m := run.GetModel()
	// all intentional test output routes to the validation pane; channel/library noise falls to actions
	log := tui.ValidationLogger()

	sshUser := m.MustStringVariable("credentials.ssh.username")
	logPath := fmt.Sprintf("/home/%s/logs/api-sessions.log", sshUser)
	ctrl := m.MustSelectHost("component.bootstrap-ctrl")

	restartComps := m.SelectComponents(".restart")
	if len(restartComps) == 0 {
		return fmt.Errorf("no components tagged 'restart' found")
	}

	// refresh the CLI session before querying identities: the login from the controller upgrade can go
	// stale across the preceding validation window, and a stale session would make the identity lookup
	// fail confusingly rather than cleanly.
	if err := edge.Login("#ctrl1").Execute(run); err != nil {
		return err
	}

	nameToId, err := restartIdentityIds(m)
	if err != nil {
		return err
	}

	// snapshot the log so we only consider events produced by the restart
	preCount, err := logLineCount(ctrl, logPath)
	if err != nil {
		return err
	}

	log.Infof("restarting %d -restart instance(s); -stable siblings stay up", len(restartComps))
	if err = chaos.RestartSelected(run, 10, restartComps...); err != nil {
		return err
	}

	// poll for new api-session created events until every restarted identity is seen or we time out
	idToType := map[string]string{}
	deadline := time.Now().Add(90 * time.Second)
	for {
		events, err := newApiSessionEvents(ctrl, logPath, preCount)
		if err != nil {
			return err
		}
		for _, e := range events {
			if e.Namespace == "apiSession" && e.EventType == "created" && e.IdentityId != "" {
				idToType[e.IdentityId] = e.Type
			}
		}
		if allSeen(restartComps, nameToId, idToType) || time.Now().After(deadline) {
			break
		}
		time.Sleep(3 * time.Second)
	}

	var problems []string
	for _, c := range restartComps {
		id, ok := nameToId[c.Id]
		if !ok {
			problems = append(problems, c.Id+": identity id not found")
			continue
		}
		sessType, seen := idToType[id]
		if !seen {
			problems = append(problems, c.Id+": no api-session created after restart")
			continue
		}
		if c.HasTag("zet") {
			// ZET (C SDK) OIDC support is version-dependent; record but do not fail on it.
			log.Infof("OIDC restart check [ZET, informational]: %s -> api-session type=%s", c.Id, sessType)
			continue
		}
		if sessType != apiSessionTypeJwt {
			problems = append(problems, fmt.Sprintf("%s: expected jwt (OIDC) after restart, got %s", c.Id, sessType))
		} else {
			log.Infof("OIDC restart check: %s switched to OIDC (jwt)", c.Id)
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("OIDC restart verification failed: %s", strings.Join(problems, "; "))
	}
	return nil
}

const apiSessionTypeJwt = "jwt"

// allSeen reports whether every restart component's identity has a recorded session type yet.
func allSeen(comps []*model.Component, nameToId, idToType map[string]string) bool {
	for _, c := range comps {
		id, ok := nameToId[c.Id]
		if !ok {
			return false
		}
		if _, seen := idToType[id]; !seen {
			return false
		}
	}
	return true
}

// restartIdentityIds returns a map of component id (== identity name) to identity id for every
// identity whose name ends in "-restart".
func restartIdentityIds(m *model.Model) (map[string]string, error) {
	out, err := cli.Exec(m, "edge", "list", "identities", "limit none", "-j")
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []struct {
			Id   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err = json.Unmarshal([]byte(out), &resp); err != nil {
		return nil, fmt.Errorf("parsing identities list: %w", err)
	}

	// the default admin always exists, so an empty list is never legitimate; treat it as a controller
	// or session failure rather than letting downstream lookups report "identity id not found".
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("edge list identities returned no identities; the default admin should always be present, so this indicates a controller or session problem")
	}

	result := map[string]string{}
	for _, d := range resp.Data {
		if strings.HasSuffix(d.Name, "-restart") {
			result[d.Name] = d.Id
		}
	}
	return result, nil
}

// logLineCount returns the current line count of the given remote log file, or 0 if it is absent.
func logLineCount(host *model.Host, path string) (int, error) {
	out, err := host.ExecLogged(fmt.Sprintf("wc -l < %s 2>/dev/null || echo 0", path))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(lastToken(out)))
}

// newApiSessionEvents reads and parses the api-session log lines appended after afterLine.
func newApiSessionEvents(host *model.Host, path string, afterLine int) ([]apiSessionEvent, error) {
	out, err := host.ExecLogged(fmt.Sprintf("tail -n +%d %s 2>/dev/null || true", afterLine+1, path))
	if err != nil {
		return nil, err
	}
	var events []apiSessionEvent
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var e apiSessionEvent
		if err = json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		events = append(events, e)
	}
	return events, nil
}

// lastToken returns the last whitespace-separated token of s, which lets us tolerate any command
// echo or log prefix that precedes a numeric result.
func lastToken(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return "0"
	}
	return fields[len(fields)-1]
}
