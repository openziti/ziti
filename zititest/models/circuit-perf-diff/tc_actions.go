package main

import (
	"fmt"
	"strings"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/model"
)

// hostIfaceLookup reads the host's default route interface so the netem qdisc
// is attached to whichever NIC carries data-plane traffic. AWS AMIs vary
// (eth0, ens5, enX0) so don't hardcode it. The shell expression is embedded
// directly in each command so we don't pay an extra ssh round-trip per host.
const hostIfaceLookup = `$(ip -o -4 route show default | awk '{print $5}' | head -n1)`

// applyNetem applies (or clears) a netem qdisc on the default interface of
// every component.data-plane host. An empty netemSpec clears the qdisc.
//
// netem requires root, so we use sudo. The replace command is idempotent —
// it adds the qdisc if absent or rewrites the parameters if present.
//
// Safety: tc on the wrong interface can sever ssh. We only ever touch the
// interface carrying the default route — i.e., the one already serving data
// traffic to the test peers — never loopback or a dedicated mgmt NIC.
func applyNetem(run model.Run, netemSpec string) error {
	netemSpec = strings.TrimSpace(netemSpec)
	log := pfxlog.Logger()

	var cmd string
	if netemSpec == "" {
		// Best-effort clear; ignore "no such file" when there's nothing to
		// remove so the action stays idempotent.
		cmd = fmt.Sprintf("sudo tc qdisc del dev %s root 2>/dev/null || true", hostIfaceLookup)
		log.Info("clearing netem on data-plane hosts")
	} else {
		cmd = fmt.Sprintf("sudo tc qdisc replace dev %s root netem %s", hostIfaceLookup, netemSpec)
		log.Infof("applying netem on data-plane hosts: %s", netemSpec)
	}

	return run.GetModel().ForEachHost("component.data-plane", 10, func(host *model.Host) error {
		if err := host.ExecLogOnlyOnError(cmd); err != nil {
			return fmt.Errorf("netem apply failed on %s: %w", host.PublicIp, err)
		}
		return nil
	})
}

// showNetem prints the current qdisc on each data-plane host. Diagnostic
// only; runs in series so the output is grouped per-host.
func showNetem(run model.Run) error {
	log := pfxlog.Logger()
	cmd := fmt.Sprintf("tc qdisc show dev %s", hostIfaceLookup)

	return run.GetModel().ForEachHost("component.data-plane", 1, func(host *model.Host) error {
		out, err := host.ExecLogged(cmd)
		if err != nil {
			log.WithError(err).Warnf("tc qdisc show failed on %s", host.PublicIp)
			return nil
		}
		log.Infof("[%s] %s", host.PublicIp, strings.TrimSpace(out))
		return nil
	})
}

// registerNetemActions adds the tc-based scenario actions to the model.
// Each scenario applies one netem configuration to every data-plane host's
// default interface, shaping both ingress (to that host) and egress (from
// that host) symmetrically across the whole circuit fabric.
//
// Calling order is up to the operator: scenarios fully replace the previous
// qdisc, so a sequence of calls walks through configurations cleanly
// without needing tcClear between them. tcClear is provided so the test
// fabric can be returned to a baseline (no shaping) on demand.
func registerNetemActions(m *model.Model) {
	scenarios := []struct {
		name string
		spec string
		desc string
	}{
		// tcClear removes any tc qdisc, returning the host to default
		// kernel behavior. Idempotent.
		{"tcClear", "", "clear netem"},

		// tcJitter: mild RTT variability around a low base. Exercises
		// the RTTVAR estimator without large absolute delays.
		{"tcJitter", "delay 10ms 5ms", "mild jitter"},

		// tcHighRtt: simulates a long-haul link (e.g. us↔eu). High BDP
		// stresses the window-growth path. Note: 150ms compounded across
		// the four data-plane hops blew past the 7-min scenario timeout;
		// 60ms keeps the per-hop window-bound throughput high enough for
		// the throughput workload to complete in budget while still
		// roughly doubling natural cross-region RTT.
		{"tcHighRtt", "delay 60ms 10ms", "long-haul RTT"},

		// tcLoss1pct: mild random loss on a moderate-RTT link. Primary
		// stress test for the retx-scale feedback loop.
		{"tcLoss1pct", "delay 50ms 10ms loss 1%", "1% random loss"},

		// tcLoss5pct: heavy random loss. If the dup-ack/floor-boost
		// safeguards regress, this is where the window-deadlock spiral
		// would re-emerge.
		{"tcLoss5pct", "delay 50ms 10ms loss 5%", "5% random loss"},

		// tcBurstyLoss: 1% loss with 50% correlation; losses cluster
		// rather than IID. More realistic for buffer overflow events.
		{"tcBurstyLoss", "delay 50ms 15ms loss 1% 50%", "bursty loss"},

		// tcRateLimit: 50 Mbit/s ceiling with moderate jitter. Forces
		// blocked-by-window conditions when the bottleneck is bandwidth
		// rather than loss.
		{"tcRateLimit", "rate 50mbit delay 50ms 10ms", "50mbit cap"},

		// tcBadLink: combined "real-world bad" — moderate bandwidth
		// cap, long RTT with high jitter, low background loss.
		{"tcBadLink", "rate 30mbit delay 80ms 30ms loss 0.5%", "degraded WAN"},
	}

	for _, s := range scenarios {
		spec := s.spec
		m.AddActionF(s.name, func(run model.Run) error {
			return applyNetem(run, spec)
		})
	}

	// tcShow prints the current qdisc on every data-plane host without
	// changing it. Diagnostic; useful after manual changes or to confirm
	// a scenario is still in effect.
	m.AddActionF("tcShow", showNetem)
}
