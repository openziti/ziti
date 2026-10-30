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

package common

import (
	"github.com/openziti/metrics"
)

const (
	MetricSubscriberEvent          = "rdm.subscriber.event"
	MetricSubscriberEventQueueTime = "rdm.subscriber.event.queue_time"
	MetricSubscriberEventQueueSize = "rdm.subscriber.event.queue_size"
	MetricSubscriberScan           = "rdm.subscriber.scan"
	MetricSubscriberScanQueueTime  = "rdm.subscriber.scan.queue_time"
	MetricSubscriberScanIdentities = "rdm.subscriber.scan.identities"
	MetricSubscriberFullSync       = "rdm.subscriber.full_sync"
)

// SubscriberMetrics records the cost of keeping identity subscriptions current. Subscriber events,
// overflow scans and full syncs all run on one goroutine, so queue times are what show it falling
// behind; its cpu use is capped at one core.
type SubscriberMetrics struct {
	// Event times processing one queued subscriber event, and EventQueueTime how long it waited.
	Event          metrics.Timer
	EventQueueTime metrics.Timer

	// Scan times a scan of identities marked while the event queue was full, and ScanQueueTime how
	// long the oldest request for it waited. ScanIdentities records how many identities each scan
	// processed.
	Scan           metrics.Timer
	ScanQueueTime  metrics.Timer
	ScanIdentities metrics.Histogram

	// FullSync times a check of every subscription, as done after a data model replacement.
	FullSync metrics.Timer
}

// NewSubscriberMetrics resolves the subscriber metrics from registry. Call it once and share the
// result across data models; it does not register the queue size gauge, which has to read whichever
// data model is current (see RouterDataModel.SubscriberEventQueueSize).
func NewSubscriberMetrics(registry metrics.Registry) *SubscriberMetrics {
	return &SubscriberMetrics{
		Event:          registry.Timer(MetricSubscriberEvent),
		EventQueueTime: registry.Timer(MetricSubscriberEventQueueTime),
		Scan:           registry.Timer(MetricSubscriberScan),
		ScanQueueTime:  registry.Timer(MetricSubscriberScanQueueTime),
		ScanIdentities: registry.Histogram(MetricSubscriberScanIdentities),
		FullSync:       registry.Timer(MetricSubscriberFullSync),
	}
}
