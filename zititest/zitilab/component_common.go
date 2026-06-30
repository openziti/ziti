/*
	Copyright 2019 NetFoundry Inc.

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

package zitilab

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/openziti/fablab/kernel/model"
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
	"github.com/sirupsen/logrus"
)

// LogStrategy controls how a ziti component's stdout/stderr is written to its
// log file. Set the "logStrategy" variable (per-component or model-wide) to one
// of these values; the default is truncate.
type LogStrategy string

const (
	// LogStrategyTruncate overwrites the log file on each (re)start (the default).
	// History is lost on restart.
	LogStrategyTruncate LogStrategy = "truncate"
	// LogStrategyAppend appends to the log file, preserving history across
	// restarts. The file is not rotated and can grow unbounded over a long run.
	LogStrategyAppend LogStrategy = "append"
	// LogStrategyRotate pipes the log through "ziti ops log-pipe", size-rotating
	// it (same lumberjack rotation as the controller's event/metrics logs). This
	// preserves recent history across restarts without growing unbounded. The
	// log-pipe side runs the component's own binary by default, so a component
	// pinned to a ziti version that predates "ops log-pipe" must select a newer
	// binary via the "logPipeBinaryVersion" variable (see resolveLogPipeBinary);
	// otherwise the pipe breaks and takes the component's stdout down with it.
	LogStrategyRotate LogStrategy = "rotate"
)

const (
	// defaultLogRotateMaxSizeMb and defaultLogRotateMaxBackups are the rotate
	// strategy defaults. The per-component log footprint is
	// maxSizeMb * (maxBackups + 1); on hosts that pack many components onto one
	// disk (e.g. links-test runs 10 routers per host), set the "logRotateMaxSizeMb"
	// and "logRotateMaxBackups" variables lower so the total stays well under disk.
	defaultLogRotateMaxSizeMb  = 50
	defaultLogRotateMaxBackups = 10
)

// resolveLogStrategy returns the component's configured log strategy, defaulting
// to truncate.
func resolveLogStrategy(c *model.Component) LogStrategy {
	switch LogStrategy(strings.ToLower(c.GetStringVariableOr("logStrategy", string(LogStrategyTruncate)))) {
	case LogStrategyAppend:
		return LogStrategyAppend
	case LogStrategyRotate:
		return LogStrategyRotate
	default:
		return LogStrategyTruncate
	}
}

// logPipeBinaryVersion is the model variable naming the ziti version whose binary
// runs "ops log-pipe" for the rotate log strategy. When unset, a component uses
// its own binary; when set it selects the binary for that version, with the empty
// string meaning the current local build. The override exists because a component
// can be pinned to a ziti version that predates "ops log-pipe": piping its output
// through that binary's (nonexistent) log-pipe exits immediately and breaks the
// component's stdout, so such a model points this at a version that has the
// command. StageFiles stages the selected binary so switching to rotate never
// needs the environment rebuilt.
const logPipeBinaryVersion = "logPipeBinaryVersion"

// resolveLogPipeBinaryVersion returns the configured log-pipe binary version and
// whether it was set. An unset variable means the component should use its own
// binary; a set-but-empty value means the current local build. The version is
// canonicalized the same way component versions are, so "1.2.3" and "v1.2.3"
// resolve to the same binary and staging op rather than duplicating either.
func resolveLogPipeBinaryVersion(c *model.Component) (version string, set bool) {
	v, found := c.GetVariable(logPipeBinaryVersion)
	if !found {
		return "", false
	}
	s, _ := v.(string)
	canonicalizeGoAppVersion(&s)
	return s, true
}

// resolveLogPipeBinary returns the ziti binary path used to run "ops log-pipe" for
// the rotate log strategy. It defaults to the component's own binary (binaryPath),
// but resolves to the logPipeBinaryVersion binary when that variable is set.
func resolveLogPipeBinary(c *model.Component, binaryPath string) string {
	if version, set := resolveLogPipeBinaryVersion(c); set {
		return GetZitiBinaryPath(c, version)
	}
	return binaryPath
}

// stageLogPipeBinary stages the binary selected by logPipeBinaryVersion when that
// variable is set to a version other than the component's own (ownVersion). This
// ensures the rotate log strategy has a log-pipe-capable binary on the host even
// when the component itself is pinned to an older version. It stages regardless of
// the current log strategy, so switching a component to rotate later doesn't
// require re-staging the environment.
func stageLogPipeBinary(r model.Run, c *model.Component, ownVersion string) error {
	version, set := resolveLogPipeBinaryVersion(c)
	if !set || version == ownVersion {
		return nil
	}
	return stageziti.StageZitiOnce(r, c, version, "")
}

// resolveLogRotate returns the rotate strategy's max file size (MB) and max
// number of retained backups for the component, from the "logRotateMaxSizeMb"
// and "logRotateMaxBackups" variables, defaulting when unset.
func resolveLogRotate(c *model.Component) (maxSizeMb int, maxBackups int) {
	maxSizeMb = logRotateIntVar(c, "logRotateMaxSizeMb", defaultLogRotateMaxSizeMb)
	maxBackups = logRotateIntVar(c, "logRotateMaxBackups", defaultLogRotateMaxBackups)
	return maxSizeMb, maxBackups
}

// logRotateIntVar reads an int-valued model variable, tolerating the int and
// string forms a variable may take, and falls back to def when unset or invalid.
func logRotateIntVar(c *model.Component, name string, def int) int {
	v, found := c.GetVariable(name)
	if !found {
		return def
	}
	switch tv := v.(type) {
	case int:
		return tv
	case int64:
		return int(tv)
	case float64:
		return int(tv)
	case string:
		if n, err := strconv.Atoi(tv); err == nil {
			return n
		}
	}
	return def
}

func getZitiProcessFilter(c *model.Component, zitiType string) func(string) bool {
	return func(s string) bool {
		matches := strings.Contains(s, "ziti") &&
			strings.Contains(s, zitiType) &&
			strings.Contains(s, fmt.Sprintf("--cli-agent-alias %s ", c.Id)) &&
			!strings.Contains(s, "sudo ")
		return matches
	}
}

func startZitiComponent(c *model.Component, zitiType string, version string, configName string, extraArgs string) error {
	user := c.GetHost().GetSshUser()

	binaryPath := GetZitiBinaryPath(c, version)
	configPath := fmt.Sprintf("/home/%s/fablab/cfg/%s", user, configName)
	logsPath := fmt.Sprintf("/home/%s/logs/%s.log", user, c.Id)

	useSudo := ""
	if zitiType == "tunnel" || c.HasTag("tunneler") {
		useSudo = "sudo"
	}

	// Default to truncate (the long-standing behavior) so existing models are
	// unaffected; models opt into append or rotate per-component as appropriate.
	// Truncating on each start discards pre-restart history, which is often
	// exactly what's needed to debug a failure, so chaos-heavy models will want
	// append (survives restarts) or rotate (survives restarts, size-bounded).
	var logRedirect string
	switch resolveLogStrategy(c) {
	case LogStrategyTruncate:
		logRedirect = fmt.Sprintf("> %s 2>&1", logsPath)
	case LogStrategyRotate:
		// Pipe the run process's output into log-pipe for rotation. log-pipe's
		// OWN stdout/stderr must go to /dev/null (and it must be nohup'd) so it
		// doesn't hold the start command's ssh channel open (which made the
		// backgrounded start never return) or die on SIGHUP when the channel
		// closes. log-pipe runs the component's own binary unless the model
		// overrides it (needed when the component predates "ops log-pipe").
		maxSizeMb, maxBackups := resolveLogRotate(c)
		logPipeBinary := resolveLogPipeBinary(c, binaryPath)
		logRedirect = fmt.Sprintf("2>&1 | nohup %s ops log-pipe %s --max-size-mb %d --max-backups %d >/dev/null 2>&1",
			logPipeBinary, logsPath, maxSizeMb, maxBackups)
	default: // append
		logRedirect = fmt.Sprintf(">> %s 2>&1", logsPath)
	}

	serviceCmd := fmt.Sprintf("nohup %s %s %s run %s --cli-agent-alias %s --log-formatter json %s %s &",
		useSudo, binaryPath, zitiType, extraArgs, c.Id, configPath, logRedirect)

	if quiet, _ := c.GetBoolVariable("quiet_startup"); !quiet {
		logrus.Info(serviceCmd)
	}

	value, err := c.GetHost().ExecLogged(serviceCmd)
	if err != nil {
		return err
	}

	if len(value) > 0 {
		logrus.Infof("output [%s]", strings.Trim(value, " \t\r\n"))
	}

	return nil
}

func canonicalizeGoAppVersion(version *string) {
	if version != nil {
		if *version != "" && *version != "latest" && !strings.HasPrefix(*version, "v") {
			*version = "v" + *version
		}
	}
}

func GetZitiBinaryPath(c *model.Component, version string) string {
	return getBinaryPath(c, "ziti", version)
}

func getBinaryPath(c *model.Component, binaryName string, version string) string {
	if version != "" {
		binaryName += "-" + version
	}
	user := c.GetHost().GetSshUser()
	return fmt.Sprintf("/home/%s/fablab/bin/%s", user, binaryName)
}

func reEnrollIdentity(run model.Run, c *model.Component, zitiBinaryPath string, configPath string) error {
	if err := zitilib_actions.EdgeExec(run.GetModel(), "delete", "authenticator", "where", fmt.Sprintf("identity=\"%v\"", c.Id)); err != nil {
		return err
	}

	if err := zitilib_actions.EdgeExec(run.GetModel(), "delete", "enrollment", "where", fmt.Sprintf("identity=\"%v\"", c.Id)); err != nil {
		return err
	}

	jwtFileName := filepath.Join(model.ConfigBuild(), c.Id+".jwt")

	args := []string{"create", "enrollment", "ott", "--jwt-output-file", jwtFileName, "--", c.Id}

	if err := zitilib_actions.EdgeExec(c.GetModel(), args...); err != nil {
		return err
	}

	configDir := filepath.Dir(configPath)
	remoteJwt := configDir + c.Id + ".jwt"
	if err := c.GetHost().SendFile(jwtFileName, remoteJwt); err != nil {
		return err
	}

	tmpl := "set -o pipefail; mkdir -p %s; %s edge enroll %s -o %s 2>&1 | tee /home/ubuntu/logs/%s.identity.enroll.log "
	cmd := fmt.Sprintf(tmpl, configDir, zitiBinaryPath, remoteJwt, configPath, c.Id)

	return c.GetHost().ExecLogOnlyOnError(cmd)
}

func setupDnsForTunneler(c *model.Component) error {
	key := "ziti_tunnel.resolve_setup_done"
	if _, found := c.Host.Data[key]; !found {
		cmds := []string{
			"sudo sed -i 's/#DNS=/DNS=127.0.0.1/g' /etc/systemd/resolved.conf",
			"sudo systemctl restart systemd-resolved",
		}
		if err := c.Host.ExecLogOnlyOnError(cmds...); err != nil {
			return err
		}
		c.Host.Data[key] = true
		return nil
	}
	return nil
}
