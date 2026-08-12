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
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab"
)

// memSamplerScript records one "<epoch>,<rss_kb>" line per second for a controller, and captures a
// heap profile the first time RSS crosses the soft threshold. The heap path is unique per sampler
// start, so the guard means one profile per iteration rather than one per host for the whole run.
//
// The controller is located by the agent alias fablab passes at start, which is the same
// discriminator its process filter uses and is independent of the binary version, so the sampler
// keeps following the process across an upgrade. A stopped or restarting controller records a zero
// rather than a gap, which is what makes a restart loop visible in the samples.
//
// The alias only ever appears inside this file, never on the sampler's own command line, so pgrep
// cannot match the sampler instead of the controller. The pid is recorded so the sampler can be
// stopped without a pattern match, for the same reason.
const memSamplerScript = `#!/bin/sh
samples=%s
heap=%s
ziti=%s
alias=%s
soft_kb=%d
echo $$ > %s
while true; do
	pids=$(pgrep -f "cli-agent-alias $alias " 2>/dev/null | tr '\n' ',' | sed 's/,$//')
	rss=0
	if [ -n "$pids" ]; then
		rss=$(ps -o rss= -p "$pids" 2>/dev/null | awk '{s+=$1} END {print s+0}')
	fi
	echo "$(date +%%s),$rss" >> "$samples"
	if [ "$soft_kb" -gt 0 ] && [ "$rss" -gt "$soft_kb" ] && [ ! -f "$heap" ]; then
		"$ziti" agent pprof-heap -a "$alias" -o "$heap" >/dev/null 2>&1
	fi
	sleep 1
done
`

// samplerStartMarker prefixes the line written when a sampler starts. It divides the accumulated
// samples into one window per iteration, so a report can scope itself without being told when the
// iteration began.
const samplerStartMarker = "# start,"

// ctrlMemBaselineWindow is how far back an upgrade report looks for the steady state to compare
// against. The steady-state gate runs immediately before the cluster upgrade, so this lands on
// settled traffic rather than on the tail of an earlier disruption.
const ctrlMemBaselineWindow = 2 * time.Minute

// ctrlMemPath builds a per-controller path under the host's log directory, which is outside the kit
// and so survives the --delete rsync that resetToBaseline runs.
func ctrlMemPath(c *model.Component, suffix string) string {
	return fmt.Sprintf("/home/%s/logs/%s-%s", c.GetHost().GetSshUser(), c.Id, suffix)
}

// ctrlHeapPattern is where c's heap profiles land, one per sampler start.
func ctrlHeapPattern(c *model.Component) string {
	return ctrlMemPath(c, "mem-*.pprof")
}

// ctrlComponents returns the model's controllers sorted by id, so every phase that walks them uses
// the same order.
func ctrlComponents(m *model.Model) []*model.Component {
	ctrls := m.SelectComponents(".ctrl")
	sort.Slice(ctrls, func(i, j int) bool { return ctrls[i].Id < ctrls[j].Id })
	return ctrls
}

// ctrlMemThresholdKb reads a MiB-valued model variable as KiB, the unit ps reports RSS in. A missing
// or unparsable value disables the threshold.
func ctrlMemThresholdKb(m *model.Model, name string) int64 {
	mb, err := strconv.ParseInt(m.GetStringVariableOr(name, ""), 10, 64)
	if err != nil {
		return 0
	}
	return mb * 1024
}

// ctrlMemRatio reads a multiplier-valued model variable. A missing or unparsable value, or anything
// at or below one, disables the check.
func ctrlMemRatio(m *model.Model, name string) float64 {
	ratio, err := strconv.ParseFloat(m.GetStringVariableOr(name, ""), 64)
	if err != nil || ratio <= 1 {
		return 0
	}
	return ratio
}

// startCtrlMemorySamplers installs and starts the RSS sampler on every controller host, replacing any
// sampler left over from a previous pass. Sampling covers the whole iteration, not just the cluster
// upgrade, so a node's upgrade behavior can be compared against its own earlier steady state as well
// as against its peers.
//
// Nothing already recorded is discarded. The samples accumulate across iterations behind a start
// marker that scopes reports to the current one, and each start gets its own heap profile path, so an
// iteration cannot destroy the evidence an earlier one captured.
func startCtrlMemorySamplers(run model.Run) error {
	m := run.GetModel()
	softKb := ctrlMemThresholdKb(m, "ctrlMemory.heapDumpAtMb")
	startedAt := time.Now().Unix()
	log := tui.ValidationLogger()

	for _, c := range ctrlComponents(m) {
		samples := ctrlMemPath(c, "mem.csv")
		heap := ctrlMemPath(c, fmt.Sprintf("mem-%d.pprof", startedAt))
		scriptPath := ctrlMemSamplerScriptPath(c)
		script := fmt.Sprintf(memSamplerScript, samples, heap, zitilab.GetZitiBinaryPath(c, ""), c.Id, softKb,
			ctrlMemSamplerPidPath(c))

		if err := stopCtrlMemorySampler(c); err != nil {
			return err
		}
		if err := c.GetHost().SendData([]byte(script), scriptPath); err != nil {
			return fmt.Errorf("failed to install the memory sampler on %s: %w", c.Id, err)
		}
		// Two commands, not one backgrounded list. Backgrounding a list makes bash fork a subshell
		// that holds the ssh session's stdout while it waits for the sampler, and the sampler never
		// exits, so the session never closes. A simple command with its own redirects does not.
		// The marker is written first, so no sample of this iteration precedes it.
		marker := fmt.Sprintf("echo '%s%d' >> %s", samplerStartMarker, startedAt, samples)
		start := fmt.Sprintf("nohup sh %s >/dev/null 2>&1 &", scriptPath)
		if err := c.GetHost().ExecLogOnlyOnError(marker, start); err != nil {
			return fmt.Errorf("failed to start the memory sampler on %s: %w", c.Id, err)
		}
		log.Infof("controller memory sampler running on %s, samples in %s", c.Id, samples)
	}
	return nil
}

// ctrlMemSamplerScriptPath is where c's sampler script is installed.
func ctrlMemSamplerScriptPath(c *model.Component) string {
	return ctrlMemPath(c, "mem-sampler.sh")
}

// ctrlMemSamplerPidPath is where c's sampler records its pid so it can be stopped again.
func ctrlMemSamplerPidPath(c *model.Component) string {
	return ctrlMemPath(c, "mem-sampler.pid")
}

// stopCtrlMemorySampler kills c's sampler if one is running, and is a no-op when none is.
//
// The pid comes from the file the sampler writes, never from a pattern match. ssh runs a command
// through a shell whose own command line is the whole command string, so a pkill pattern naming the
// sampler matches that shell too and the sampler stop kills its own session. The recorded pid is
// checked against the running process before signalling, since a pid left behind by a host reboot
// could by then belong to something else.
func stopCtrlMemorySampler(c *model.Component) error {
	pidFile := ctrlMemSamplerPidPath(c)
	cmd := fmt.Sprintf(
		`pid=$(cat %s 2>/dev/null); if [ -n "$pid" ] && grep -qasF %s /proc/$pid/cmdline; then kill "$pid"; fi; rm -f %s; true`,
		pidFile, ctrlMemSamplerScriptPath(c), pidFile)
	return c.GetHost().ExecLogOnlyOnError(cmd)
}

// stopCtrlMemorySamplers stops sampling on every controller host. Nothing calls this during a run;
// it is here so sampling can be paused on a live instance.
func stopCtrlMemorySamplers(run model.Run) error {
	for _, c := range ctrlComponents(run.GetModel()) {
		if err := stopCtrlMemorySampler(c); err != nil {
			return err
		}
	}
	return nil
}

// memSample is one sampler reading.
type memSample struct {
	ts  int64
	rss int64
}

// ctrlMemSeries is everything one controller's sampler has recorded, with the start markers that
// divide it into one window per iteration.
type ctrlMemSeries struct {
	c       *model.Component
	samples []memSample
	starts  []int64
}

// ctrlMemStats summarizes a controller's samples over some window.
type ctrlMemStats struct {
	peakKb  int64
	lastKb  int64
	samples int
}

// readCtrlMemSeries fetches and parses everything c's sampler has recorded. The whole file is read
// once so a caller can summarize several windows of it without going back over the network.
func readCtrlMemSeries(c *model.Component) (*ctrlMemSeries, error) {
	out, err := c.GetHost().ExecLogged("cat " + ctrlMemPath(c, "mem.csv"))
	if err != nil {
		return nil, fmt.Errorf("failed to read memory samples for %s: %w", c.Id, err)
	}
	samples, starts := parseCtrlMemSeries(out)
	return &ctrlMemSeries{c: c, samples: samples, starts: starts}, nil
}

// parseCtrlMemSeries splits sampler output into samples and the timestamps of the start markers
// between them. Unparsable lines are skipped: the file is appended to by a shell loop that can be
// killed mid-write, so a torn final line is normal rather than a fault.
func parseCtrlMemSeries(out string) ([]memSample, []int64) {
	var samples []memSample
	var starts []int64
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if value, isMarker := strings.CutPrefix(line, samplerStartMarker); isMarker {
			if ts, err := strconv.ParseInt(value, 10, 64); err == nil {
				starts = append(starts, ts)
			}
			continue
		}
		if sample, ok := parseMemSample(line); ok {
			samples = append(samples, sample)
		}
	}
	return samples, starts
}

// parseMemSample parses one "<epoch>,<rss_kb>" sampler line, reporting false for anything else.
func parseMemSample(line string) (memSample, bool) {
	tsField, rssField, found := strings.Cut(strings.TrimSpace(line), ",")
	if !found {
		return memSample{}, false
	}
	ts, err := strconv.ParseInt(tsField, 10, 64)
	if err != nil {
		return memSample{}, false
	}
	rss, err := strconv.ParseInt(rssField, 10, 64)
	if err != nil {
		return memSample{}, false
	}
	return memSample{ts: ts, rss: rss}, true
}

// statsIn summarizes the samples in [from, to). A zero from starts at the most recent sampler start,
// which scopes a report to the current iteration; a zero to runs to the newest sample.
func (self *ctrlMemSeries) statsIn(from, to time.Time) ctrlMemStats {
	start := self.currentWindowStart()
	if !from.IsZero() {
		start = from.Unix()
	}
	end := int64(0)
	if !to.IsZero() {
		end = to.Unix()
	}

	var stats ctrlMemStats
	for _, sample := range self.samples {
		if sample.ts < start || (end != 0 && sample.ts >= end) {
			continue
		}
		stats.samples++
		stats.lastKb = sample.rss
		if sample.rss > stats.peakKb {
			stats.peakKb = sample.rss
		}
	}
	return stats
}

// currentWindowStart returns the timestamp of the most recent sampler start. A series with no marker
// yields a start that admits every sample, which is what a sampler predating the marker leaves behind.
func (self *ctrlMemSeries) currentWindowStart() int64 {
	if len(self.starts) == 0 {
		return 0
	}
	return self.starts[len(self.starts)-1]
}

// mib renders a KiB reading the way the reports talk about memory.
func mib(kb int64) int64 {
	return kb / 1024
}

// reportCtrlMemory logs peak and current RSS for every controller over the window opening at since (a
// zero since means the current iteration), and fails when a controller's peak exceeds
// ctrlMemory.failAtMb.
func reportCtrlMemory(run model.Run, since time.Time, phase string) error {
	m := run.GetModel()
	log := tui.ValidationLogger()
	failKb := ctrlMemThresholdKb(m, "ctrlMemory.failAtMb")

	var worstPeak int64
	var worst *model.Component

	for _, c := range ctrlComponents(m) {
		series, err := readCtrlMemSeries(c)
		if err != nil {
			log.WithError(err).Warnf("no memory samples for %s", c.Id)
			continue
		}
		stats := series.statsIn(since, time.Time{})
		if stats.samples == 0 {
			log.Warnf("controller memory [%s] %s: no samples", phase, c.Id)
			continue
		}
		log.Infof("controller memory [%s] %s: peak %d MiB, current %d MiB, %d samples",
			phase, c.Id, mib(stats.peakKb), mib(stats.lastKb), stats.samples)
		if stats.peakKb > worstPeak {
			worstPeak, worst = stats.peakKb, c
		}
	}

	if failKb > 0 && worstPeak > failKb {
		return fmt.Errorf("controller %s peaked at %d MiB during %s, over the %d MiB limit; heap profiles, if any were captured, are at %s",
			worst.Id, mib(worstPeak), phase, mib(failKb), ctrlHeapPattern(worst))
	}
	return nil
}

// reportCtrlMemoryUpgrade reports each controller's memory across an upgrade against its own steady
// state immediately before it, and fails when a node exceeds either ctrlMemory.failAtRatio or the
// absolute ctrlMemory.failAtMb ceiling. The baseline window is [baselineStart, upgradeStart) and the
// upgrade window runs from upgradeStart to now.
//
// The ratio is the check that bites at this model's scale. The absolute ceiling is sized for the
// failure #4219 reports, which needs a far larger data set to reach, whereas a node using several
// times its own steady state is visible whatever the data set is.
func reportCtrlMemoryUpgrade(run model.Run, baselineStart, upgradeStart time.Time, phase string) error {
	m := run.GetModel()
	log := tui.ValidationLogger()
	failKb := ctrlMemThresholdKb(m, "ctrlMemory.failAtMb")
	failRatio := ctrlMemRatio(m, "ctrlMemory.failAtRatio")

	var violations []string
	for _, c := range ctrlComponents(m) {
		series, err := readCtrlMemSeries(c)
		if err != nil {
			log.WithError(err).Warnf("no memory samples for %s", c.Id)
			continue
		}
		base := series.statsIn(baselineStart, upgradeStart)
		upgrade := series.statsIn(upgradeStart, time.Time{})
		if upgrade.samples == 0 {
			log.Warnf("controller memory [%s] %s: no samples", phase, c.Id)
			continue
		}

		growth := "n/a"
		if base.peakKb > 0 {
			growth = fmt.Sprintf("%.1fx", float64(upgrade.peakKb)/float64(base.peakKb))
		}
		log.Infof("controller memory [%s] %s: baseline peak %d MiB, upgrade peak %d MiB (%s), current %d MiB",
			phase, c.Id, mib(base.peakKb), mib(upgrade.peakKb), growth, mib(upgrade.lastKb))

		before := len(violations)
		if failRatio > 0 && base.peakKb > 0 && float64(upgrade.peakKb) > failRatio*float64(base.peakKb) {
			violations = append(violations, fmt.Sprintf(
				"%s grew to %s of its %d MiB baseline (%d MiB), over the %.1fx limit",
				c.Id, growth, mib(base.peakKb), mib(upgrade.peakKb), failRatio))
		}
		if failKb > 0 && upgrade.peakKb > failKb {
			violations = append(violations, fmt.Sprintf("%s peaked at %d MiB, over the %d MiB limit",
				c.Id, mib(upgrade.peakKb), mib(failKb)))
		}
		if len(violations) > before {
			log.Infof("heap profiles for %s, if any were captured, are at %s", c.Id, ctrlHeapPattern(c))
		}
	}

	if len(violations) > 0 {
		return fmt.Errorf("controller memory grew unexpectedly during %s: %s", phase, strings.Join(violations, "; "))
	}
	return nil
}

// summarizeCtrlMemory logs one row per controller per sampler run, which is one row per iteration, so
// a finished multi-iteration run can be read back with a single command instead of scrolled for.
func summarizeCtrlMemory(run model.Run) error {
	log := tui.ValidationLogger()

	var worstPeak int64
	worst := ""
	for _, c := range ctrlComponents(run.GetModel()) {
		series, err := readCtrlMemSeries(c)
		if err != nil {
			log.WithError(err).Warnf("no memory samples for %s", c.Id)
			continue
		}
		if len(series.samples) == 0 {
			log.Warnf("controller memory summary: %s has no samples", c.Id)
			continue
		}

		// a series with no marker predates them, so it is reported as the single window it is
		starts := series.starts
		if len(starts) == 0 {
			starts = []int64{series.samples[0].ts}
		}
		for i, start := range starts {
			from := time.Unix(start, 0)
			to := time.Time{}
			if i+1 < len(starts) {
				to = time.Unix(starts[i+1], 0)
			}
			stats := series.statsIn(from, to)
			log.Infof("controller memory summary: %s iteration %d (%s): peak %d MiB, final %d MiB, %d samples",
				c.Id, i+1, from.Format(time.RFC3339), mib(stats.peakKb), mib(stats.lastKb), stats.samples)
			if stats.peakKb > worstPeak {
				worstPeak = stats.peakKb
				worst = fmt.Sprintf("%s in iteration %d", c.Id, i+1)
			}
		}
	}

	if worst != "" {
		log.Infof("controller memory summary: highest peak was %d MiB, %s", mib(worstPeak), worst)
	}
	return nil
}
