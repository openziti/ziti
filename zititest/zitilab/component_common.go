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
	"strings"

	"github.com/openziti/fablab/kernel/model"
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
	"github.com/sirupsen/logrus"
)

// LogStrategy controls how a ziti component's stdout/stderr is written to its log
// file. Set it via the component type's LogConfig; the default is truncate.
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
	// binary via LogConfig.PipeBinaryVersion; otherwise the pipe breaks and takes
	// the component's stdout down with it.
	LogStrategyRotate LogStrategy = "rotate"
)

const (
	// DefaultLogRotateMaxSizeMb and DefaultLogRotateMaxBackups are the rotate
	// strategy's size bounds, used when a LogConfig leaves them zero. The
	// per-component log footprint is maxSizeMb * (maxBackups + 1); models that
	// pack many components onto one disk (e.g. links-test runs 10 routers per
	// host) should set lower explicit values so the total stays well under disk.
	DefaultLogRotateMaxSizeMb  = 50
	DefaultLogRotateMaxBackups = 10
)

// LogConfig configures how a ziti component's stdout/stderr is captured to its log
// file. Embed it in a component type (ControllerType, RouterType, ...) to select
// the strategy declaratively; the zero value means truncate.
type LogConfig struct {
	// Strategy selects truncate (default), append, or rotate.
	Strategy LogStrategy

	// RotateMaxSizeMb and RotateMaxBackups bound the rotate strategy's on-disk
	// footprint, which is RotateMaxSizeMb * (RotateMaxBackups + 1). Zero uses
	// DefaultLogRotateMaxSizeMb / DefaultLogRotateMaxBackups.
	RotateMaxSizeMb  int
	RotateMaxBackups int

	// PipeBinaryVersion selects the ziti version whose "ops log-pipe" runs the
	// rotate pipe. Nil uses the component's own binary; a non-nil value selects
	// that version's binary, with the empty string meaning the current local
	// build. It exists because a component pinned to a version predating
	// "ops log-pipe" would otherwise break its stdout piping through a binary
	// that lacks the command.
	PipeBinaryVersion *string
}

// GetLogConfig returns the log configuration, letting a component type expose its
// embedded LogConfig through the logConfigProvider interface.
func (l LogConfig) GetLogConfig() LogConfig { return l }

// logConfigProvider is implemented by component types that carry a LogConfig (via
// embedding), letting the shared start path read a component's log settings off
// its type.
type logConfigProvider interface {
	GetLogConfig() LogConfig
}

// logConfigOf returns the component's LogConfig, or the zero value (truncate) when
// its type doesn't carry one.
func logConfigOf(c *model.Component) LogConfig {
	if p, ok := c.Type.(logConfigProvider); ok {
		return p.GetLogConfig()
	}
	return LogConfig{}
}

// resolveLogStrategy returns the component's configured log strategy, defaulting
// to truncate.
func resolveLogStrategy(c *model.Component) LogStrategy {
	switch logConfigOf(c).Strategy {
	case LogStrategyAppend:
		return LogStrategyAppend
	case LogStrategyRotate:
		return LogStrategyRotate
	default:
		return LogStrategyTruncate
	}
}

// resolveLogPipeBinaryVersion returns the configured log-pipe binary version and
// whether it was set. A nil LogConfig.PipeBinaryVersion means the component uses
// its own binary; a non-nil value selects that version, with the empty string
// meaning the current local build. The version is canonicalized the same way
// component versions are, so "1.2.3" and "v1.2.3" resolve to the same binary and
// staging op rather than duplicating either.
func resolveLogPipeBinaryVersion(c *model.Component) (version string, set bool) {
	v := logConfigOf(c).PipeBinaryVersion
	if v == nil {
		return "", false
	}
	s := *v
	canonicalizeGoAppVersion(&s)
	return s, true
}

// resolveLogPipeBinary returns the ziti binary path used to run "ops log-pipe" for
// the rotate log strategy. It defaults to the component's own binary (binaryPath),
// but resolves to the PipeBinaryVersion binary when that is set.
func resolveLogPipeBinary(c *model.Component, binaryPath string) string {
	if version, set := resolveLogPipeBinaryVersion(c); set {
		return GetZitiBinaryPath(c, version)
	}
	return binaryPath
}

// stageLogPipeBinary stages the binary selected by LogConfig.PipeBinaryVersion when
// it is set to a version other than the component's own (ownVersion). This ensures
// the rotate log strategy has a log-pipe-capable binary on the host even when the
// component itself is pinned to an older version. It stages regardless of the
// current log strategy, so switching a component to rotate later doesn't require
// re-staging the environment.
func stageLogPipeBinary(r model.Run, c *model.Component, ownVersion string) error {
	version, set := resolveLogPipeBinaryVersion(c)
	if !set || version == ownVersion {
		return nil
	}
	return stageziti.StageZitiOnce(r, c, version, "")
}

// resolveLogRotate returns the rotate strategy's max file size (MB) and retained
// backup count for the component, applying the package defaults when the LogConfig
// leaves them zero.
func resolveLogRotate(c *model.Component) (maxSizeMb int, maxBackups int) {
	cfg := logConfigOf(c)
	maxSizeMb, maxBackups = cfg.RotateMaxSizeMb, cfg.RotateMaxBackups
	if maxSizeMb <= 0 {
		maxSizeMb = DefaultLogRotateMaxSizeMb
	}
	if maxBackups <= 0 {
		maxBackups = DefaultLogRotateMaxBackups
	}
	return maxSizeMb, maxBackups
}

// logRedirect returns the shell redirect fragment that captures a component's
// stdout/stderr per its log strategy: truncate ("> file"), append (">> file"), or
// rotate (pipe through "ops log-pipe"). defaultLogPipeBinary is the binary used to
// run "ops log-pipe" for the rotate strategy unless LogConfig.PipeBinaryVersion
// selects another; it must be a ziti binary that has the "ops log-pipe" command
// (the ziti-edge-tunnel binary does not, so ZET passes its staged ziti binary).
//
// For rotate, log-pipe's OWN stdout/stderr must go to /dev/null and it must be
// nohup'd, so it doesn't hold the start command's ssh channel open (which makes a
// backgrounded start never return) or die on SIGHUP when the channel closes.
func logRedirect(c *model.Component, logsPath, defaultLogPipeBinary string) string {
	switch resolveLogStrategy(c) {
	case LogStrategyRotate:
		maxSizeMb, maxBackups := resolveLogRotate(c)
		logPipeBinary := resolveLogPipeBinary(c, defaultLogPipeBinary)
		return fmt.Sprintf("2>&1 | nohup %s ops log-pipe %s --max-size-mb %d --max-backups %d >/dev/null 2>&1",
			logPipeBinary, logsPath, maxSizeMb, maxBackups)
	case LogStrategyAppend:
		return fmt.Sprintf(">> %s 2>&1", logsPath)
	default: // truncate
		return fmt.Sprintf("> %s 2>&1", logsPath)
	}
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
	redirect := logRedirect(c, logsPath, binaryPath)

	serviceCmd := fmt.Sprintf("nohup %s %s %s run %s --cli-agent-alias %s --log-formatter json %s %s &",
		useSudo, binaryPath, zitiType, extraArgs, c.Id, configPath, redirect)

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
