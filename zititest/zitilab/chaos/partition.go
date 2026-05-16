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

package chaos

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/model"
)

const partitionChain = "ZITI_PARTITION"

// PartitionFromControllers blocks traffic from the selected hosts to all controller
// IPs on ports 1280 (edge API) and 6262 (ctrl channel). Uses a dedicated iptables
// chain so SSH access is never affected. Existing circuits through routers remain
// unaffected since only controller-bound traffic is blocked.
func PartitionFromControllers(run model.Run, hostSelector string, concurrency int) error {
	ctrlHosts := run.GetModel().SelectHosts("component.ctrl")
	if len(ctrlHosts) == 0 {
		return fmt.Errorf("no controller hosts found")
	}

	var ctrlIPs []string
	for _, h := range ctrlHosts {
		ctrlIPs = append(ctrlIPs, h.PublicIp)
		if h.PrivateIp != "" && h.PrivateIp != h.PublicIp {
			ctrlIPs = append(ctrlIPs, h.PrivateIp)
		}
	}

	return applyPartition(run, hostSelector, concurrency, ctrlIPs, []int{1280, 6262})
}

// PartitionFromControllersByComponent blocks traffic from hosts matching hostSelector
// to the hosts of components matching ctrlSelector. This is a more flexible variant
// of PartitionFromControllers that allows targeting specific controllers.
func PartitionFromControllersByComponent(run model.Run, hostSelector, ctrlSelector string, concurrency int) error {
	ctrlComponents := run.GetModel().SelectComponents(ctrlSelector)
	if len(ctrlComponents) == 0 {
		return fmt.Errorf("no components found for selector %q", ctrlSelector)
	}

	seen := map[string]bool{}
	var ctrlIPs []string
	for _, c := range ctrlComponents {
		for _, ip := range []string{c.Host.PublicIp, c.Host.PrivateIp} {
			if ip != "" && !seen[ip] {
				seen[ip] = true
				ctrlIPs = append(ctrlIPs, ip)
			}
		}
	}

	return applyPartition(run, hostSelector, concurrency, ctrlIPs, []int{1280, 6262})
}

// HealPartition restores connectivity by flushing the ZITI_PARTITION chain and
// removing it from OUTPUT. Safe to call even if no partition is active.
func HealPartition(run model.Run, hostSelector string, concurrency int) error {
	log := pfxlog.Logger()

	hosts := run.GetModel().SelectHosts(hostSelector)
	log.Infof("healing partition on %d hosts matching %q", len(hosts), hostSelector)

	return run.GetModel().ForEachHost(hostSelector, concurrency, func(h *model.Host) error {
		// Flush the chain, remove the jump rule, delete the chain. Ignore errors from
		// rules that don't exist (idempotent).
		cmds := fmt.Sprintf(
			"sudo iptables -F %s 2>/dev/null; "+
				"sudo iptables -D OUTPUT -j %s 2>/dev/null; "+
				"sudo iptables -X %s 2>/dev/null; "+
				"true",
			partitionChain, partitionChain, partitionChain,
		)
		return h.ExecLogOnlyOnError(cmds)
	})
}

func applyPartition(run model.Run, hostSelector string, concurrency int, targetIPs []string, ports []int) error {
	log := pfxlog.Logger()

	hosts := run.GetModel().SelectHosts(hostSelector)
	log.Infof("applying partition on %d hosts matching %q, blocking %d IPs on ports %v",
		len(hosts), hostSelector, len(targetIPs), ports)

	return run.GetModel().ForEachHost(hostSelector, concurrency, func(h *model.Host) error {
		// Idempotent setup: flush existing chain if present, or create new one.
		setup := fmt.Sprintf(
			"sudo iptables -N %s 2>/dev/null || sudo iptables -F %s; "+
				"sudo iptables -C OUTPUT -j %s 2>/dev/null || sudo iptables -I OUTPUT -j %s",
			partitionChain, partitionChain,
			partitionChain, partitionChain,
		)
		if err := h.ExecLogOnlyOnError(setup); err != nil {
			return fmt.Errorf("failed to setup partition chain on %s: %w", h.PublicIp, err)
		}

		// Add DROP rules for each IP/port combination.
		for _, ip := range targetIPs {
			for _, port := range ports {
				rule := fmt.Sprintf("sudo iptables -A %s -d %s -p tcp --dport %d -j DROP",
					partitionChain, ip, port)
				if err := h.ExecLogOnlyOnError(rule); err != nil {
					return fmt.Errorf("failed to add partition rule on %s for %s:%d: %w",
						h.PublicIp, ip, port, err)
				}
			}
		}

		log.Infof("partition applied on host %s (%s)", h.Id, h.PublicIp)
		return nil
	})
}

// SelectRandomHosts picks a random percentage of hosts matching the given selector.
func SelectRandomHosts(run model.Run, hostSelector string, pct uint8) []*model.Host {
	hosts := run.GetModel().SelectHosts(hostSelector)
	count := int(float64(len(hosts)) * float64(pct) / 100)
	if count < 1 {
		count = 1
	}
	if count > len(hosts) {
		count = len(hosts)
	}

	rand.Shuffle(len(hosts), func(i, j int) {
		hosts[i], hosts[j] = hosts[j], hosts[i]
	})
	return hosts[:count]
}

// PartitionHostsFromControllers blocks traffic from the given hosts to all
// controller IPs. Use HealPartition with a broad selector afterward since it's
// idempotent.
func PartitionHostsFromControllers(run model.Run, hosts []*model.Host, concurrency int) error {
	ctrlHosts := run.GetModel().SelectHosts("component.ctrl")
	if len(ctrlHosts) == 0 {
		return fmt.Errorf("no controller hosts found")
	}

	var ctrlIPs []string
	for _, h := range ctrlHosts {
		ctrlIPs = append(ctrlIPs, h.PublicIp)
		if h.PrivateIp != "" && h.PrivateIp != h.PublicIp {
			ctrlIPs = append(ctrlIPs, h.PrivateIp)
		}
	}

	selector := hostListToSelector(hosts)
	return applyPartition(run, selector, concurrency, ctrlIPs, []int{1280, 6262})
}

// hostListToSelector builds a comma-separated #id selector from a host list.
func hostListToSelector(hosts []*model.Host) string {
	ids := make([]string, len(hosts))
	for i, h := range hosts {
		ids[i] = "#" + h.Id
	}
	return strings.Join(ids, ",")
}
