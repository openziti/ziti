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

package event

import (
	"fmt"
	"time"
)

const (
	AlertEventNS = "alert"

	AlertSourceTypeRouter = "router"
)

// An AlertEvent is emitted when a ziti component generates an alert. Alerts are expected to be something that
// a network operator can fix, such as a problem with a service configuration.
//
// Valid values for alert source type:
//   - router
//
// In the future, other alert sources may be supported, such as controller or SDK.
//
// Valid values for severity:
//   - error
//
// In the future, other severities may be supported, such as info or warning.
//
// Example: An alert generated because a config referenced an interface which was currently unavailable.
//
//	{
//	  "namespace"         : "alert",
//	  "event_src_id"      : "ctrl1",
//	  "timestamp"         : "2021-11-08T14:45:45.785561479-05:00",
//	  "alert_source_type" : "router",
//	  "alert_source_id"   : "DJFljCCoLs",
//	  "severity"          : "error",
//	  "message"           : "error starting proxy listener for service 'test'"
//	  "details"              : [
//	    "unable to bind eth0, no address"
//	  ],
//	  "relatedEntities"   : {
//	    "router"   : "DJFljCCoLs",
//	    "identity" : "DJFljCCoLs",
//	    "service"  : "3DPjxybDvXlo878CB0X2Zs",
//	  }
//	}
type AlertEvent struct {
	Namespace  string    `json:"namespace"`
	EventSrcId string    `json:"event_src_id"`
	Timestamp  time.Time `json:"timestamp"`

	// The type of the component which generated the alert
	AlertSourceType string `json:"alert_source_type"`

	// The id of the component which generated the alert
	AlertSourceId string `json:"alert_source_id"`

	// The severity of the alert
	Severity string `json:"severity"`

	// The alert text
	Message string `json:"message"`

	// Any support information - log messages, extra details, etc
	Details []string `json:"details"`

	// Entities related to the alert. The map is keyed by entity type and the value is the entity
	RelatedEntities map[string]string `json:"related_entities"`
}

func (event *AlertEvent) String() string {
	return fmt.Sprintf("src: %s:%s message: %s", event.AlertSourceType, event.AlertSourceId, event.Message)
}

type AlertEventHandler interface {
	AcceptAlertEvent(event *AlertEvent)
}

type AlertEventHandlerWrapper interface {
	AlertEventHandler
	IsWrapping(value AlertEventHandler) bool
}
