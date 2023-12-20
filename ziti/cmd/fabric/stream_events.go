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

package fabric

import (
	"encoding/json"
	"fmt"
	"github.com/openziti/channel/v2"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/controller/event"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"time"
)

type streamEventsAction struct {
	api.Options
	all          bool
	apiSessions  bool
	circuits     bool
	entityChange bool
	entityCounts bool
	links        bool
	metrics      bool
	routers      bool
	services     bool
	sessions     bool
	terminators  bool
	usage        bool

	metricsSourceFilter  string
	metricsFilter        string
	entityCountsInterval time.Duration
	usageVersion         uint8
}

func NewStreamEventsCmd(p common.OptionsProvider) *cobra.Command {
	action := streamEventsAction{
		Options: api.Options{
			CommonOptions: p(),
		},
	}

	streamEventsCmd := &cobra.Command{
		Use:     "events",
		Short:   "Stream events",
		Example: "ziti fabric stream events --circuits --metrics --metrics-filter '.*'",
		Args:    cobra.ExactArgs(0),
		RunE:    action.streamEvents,
	}

	action.AddCommonFlags(streamEventsCmd)
	streamEventsCmd.Flags().BoolVar(&action.all, "all", false, "Include all events")
	streamEventsCmd.Flags().BoolVar(&action.apiSessions, "api-sessions", false, "Include api-session events")
	streamEventsCmd.Flags().BoolVar(&action.circuits, "circuits", false, "Include circuit events")
	streamEventsCmd.Flags().BoolVar(&action.entityChange, "entity-change", false, "Include entity change events")
	streamEventsCmd.Flags().BoolVar(&action.entityCounts, "entity-counts", false, "Include entity count events")
	streamEventsCmd.Flags().BoolVar(&action.links, "links", false, "Include link events")
	streamEventsCmd.Flags().BoolVar(&action.metrics, "metrics", false, "Include metrics events")
	streamEventsCmd.Flags().BoolVar(&action.routers, "routers", false, "Include router events")
	streamEventsCmd.Flags().BoolVar(&action.services, "services", false, "Include service events")
	streamEventsCmd.Flags().BoolVar(&action.sessions, "sessions", false, "Include session events")
	streamEventsCmd.Flags().BoolVar(&action.terminators, "terminators", false, "Include terminators events")
	streamEventsCmd.Flags().BoolVar(&action.usage, "usage", false, "Include usage events")
	streamEventsCmd.Flags().DurationVar(&action.entityCountsInterval, "entity-counts-interval", 5*time.Minute, "Specify the entity count event interval")
	streamEventsCmd.Flags().StringVar(&action.metricsSourceFilter, "metrics-source-filter", "", "Specify which sources to stream metrics from")
	streamEventsCmd.Flags().StringVar(&action.metricsFilter, "metrics-filter", "", "Specify which metrics to stream")
	streamEventsCmd.Flags().Uint8Var(&action.usageVersion, "usage-version", 3, "Specify which version of usage data to stream. Valid versions: [2,3]")
	return streamEventsCmd
}

func (self *streamEventsAction) buildSubscriptions(cmd *cobra.Command) []*event.Subscription {
	var subscriptions []*event.Subscription

	if self.apiSessions || (self.all && !cmd.Flags().Changed("api-sessions")) {
		subscriptions = append(subscriptions, &event.Subscription{
			Type: "edge.apiSessions",
		})
	}

	if self.circuits || (self.all && !cmd.Flags().Changed("circuits")) {
		subscriptions = append(subscriptions, &event.Subscription{
			Type: "fabric.circuits",
		})
	}

	if self.entityChange || (self.all && !cmd.Flags().Changed("entity-change")) {
		subscription := &event.Subscription{
			Type: "entityChange",
		}
		subscriptions = append(subscriptions, subscription)
	}

	if self.entityCounts || (self.all && !cmd.Flags().Changed("entity-counts")) {
		subscription := &event.Subscription{
			Type: "edge.entityCounts",
		}
		if cmd.Flags().Changed("entity-counts-interval") {
			subscription.Options = map[string]interface{}{
				"interval": self.entityCountsInterval.String(),
			}
		}
		subscriptions = append(subscriptions, subscription)
	}

	if self.links || (self.all && !cmd.Flags().Changed("links")) {
		subscriptions = append(subscriptions, &event.Subscription{
			Type: "fabric.links",
		})
	}

	if self.metrics || (self.all && !cmd.Flags().Changed("metrics")) {
		subscription := &event.Subscription{
			Type:    "metrics",
			Options: map[string]interface{}{},
		}

		if cmd.Flags().Changed("metrics-source-filter") {
			subscription.Options["sourceFilter"] = self.metricsSourceFilter
		}

		if cmd.Flags().Changed("metrics-filter") {
			subscription.Options["metricFilter"] = self.metricsFilter
		}

		subscriptions = append(subscriptions, subscription)
	}

	if self.routers || (self.all && !cmd.Flags().Changed("routers")) {
		subscriptions = append(subscriptions, &event.Subscription{
			Type: "fabric.routers",
		})
	}

	if self.sessions || (self.all && !cmd.Flags().Changed("services")) {
		subscriptions = append(subscriptions, &event.Subscription{
			Type: "services",
		})
	}

	if self.sessions || (self.all && !cmd.Flags().Changed("sessions")) {
		subscriptions = append(subscriptions, &event.Subscription{
			Type: "edge.sessions",
		})
	}

	if self.terminators || (self.all && !cmd.Flags().Changed("terminators")) {
		subscriptions = append(subscriptions, &event.Subscription{
			Type: "fabric.terminators",
		})
	}

	if self.usage || (self.all && !cmd.Flags().Changed("usage")) {
		subscription := &event.Subscription{
			Type: "fabric.usage",
			Options: map[string]interface{}{
				"version": self.usageVersion,
			},
		}

		subscriptions = append(subscriptions, subscription)
	}

	return subscriptions
}

func (self *streamEventsAction) streamEvents(cmd *cobra.Command, _ []string) error {
	if self.usageVersion < 2 || self.usageVersion > 3 {
		return errors.New("invalid usage version")
	}
	streamEventsRequest := map[string]interface{}{}
	streamEventsRequest["format"] = "json"

	subscriptions := self.buildSubscriptions(cmd)
	if len(subscriptions) == 0 {
		self.all = true
		subscriptions = self.buildSubscriptions(cmd)
	}

	streamEventsRequest["subscriptions"] = subscriptions

	closeNotify := make(chan struct{})

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandler(int32(mgmt_pb.ContentType_StreamEventsEventType), self)
		binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
			close(closeNotify)
		}))
		return nil
	}

	ch, err := api.NewWsMgmtChannel(channel.BindHandlerF(bindHandler))
	if err != nil {
		return err
	}

	msgBytes, err := json.Marshal(streamEventsRequest)
	if err != nil {
		return err
	}

	if self.Verbose {
		fmt.Printf("Request: %v\n", string(msgBytes))
	}

	requestMsg := channel.NewMessage(int32(mgmt_pb.ContentType_StreamEventsRequestType), msgBytes)
	responseMsg, err := requestMsg.WithTimeout(time.Duration(self.Timeout) * time.Second).SendForReply(ch)
	if err != nil {
		return err
	}

	if responseMsg.ContentType == channel.ContentTypeResultType {
		result := channel.UnmarshalResult(responseMsg)
		if result.Success {
			if self.Verbose {
				fmt.Printf("event streaming started: %v\n", result.Message)
			}
		} else {
			fmt.Printf("error starting event streaming [%s]\n", result.Message)
			os.Exit(1)
		}
	} else {
		return errors.Errorf("unexpected response type %v", responseMsg.ContentType)
	}

	<-closeNotify
	return nil
}

func (self *streamEventsAction) HandleReceive(msg *channel.Message, _ channel.Channel) {
	fmt.Println(string(msg.Body))
}
