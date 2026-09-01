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

package verify

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/channel/v5/protobufs"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/ziti/cmd/api"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
)

type verifyStaleLinksAction struct {
	api.Options

	filter          string
	mode            string
	gc              bool
	includeNotStale bool

	out         io.Writer
	resultsChan chan *mgmt_pb.StaleLinkResult
}

// NewVerifyStaleLinksCmd creates the `ziti ops verify stale-links` command.
// Reports per-link staleness across the network. Default mode is "changed"
// (any detail of the link's listener/dialer differs from current config).
// Mode "orphaned" only flags links whose supporting listener/dialer is
// entirely absent. --gc closes links that both endpoints confirm as
// stale; partial-info links (one endpoint offline) are never GC'd.
func NewVerifyStaleLinksCmd(out io.Writer, _ io.Writer) *cobra.Command {
	action := &verifyStaleLinksAction{out: out}

	cmd := &cobra.Command{
		Use:   "stale-links",
		Short: "Verify links against current router configurations; optionally close stale ones",
		Long: `Verify, per-link, whether each link in the network is still re-establishable
under each endpoint's current link configuration. The router on each end
reports its own side; the controller aggregates and emits a per-link verdict.

A link is "stale" if either endpoint reports stale. Default --mode changed
flags any link whose listener/dialer details have changed in the current
configuration (e.g., advertise address renamed). --mode orphaned only
flags links whose supporting listener/dialer is entirely absent.

With --gc, fully-confirmed stale links (both endpoints reported stale)
are closed via the standard LinkFault path. Partial-info links, where
one endpoint is offline or timed out, are never GC'd.`,
		Example: "ziti ops verify stale-links --filter 'name contains \"east\"' --gc",
		RunE:    action.run,
	}

	action.AddCommonFlags(cmd)
	cmd.Flags().StringVar(&action.filter, "filter", "", "router filter (default all)")
	cmd.Flags().StringVar(&action.mode, "mode", "changed", "matching mode: changed|orphaned")
	cmd.Flags().BoolVar(&action.gc, "gc", false, "close confirmed-stale links (sends LinkFault to both endpoints)")
	cmd.Flags().BoolVar(&action.includeNotStale, "include-not-stale", false, "also print healthy (non-stale) links")
	return cmd
}

func (self *verifyStaleLinksAction) run(_ *cobra.Command, _ []string) error {
	mode, err := parseMatchMode(self.mode)
	if err != nil {
		return err
	}

	closeNotify := make(chan struct{})
	self.resultsChan = make(chan *mgmt_pb.StaleLinkResult, 16)

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandler(int32(mgmt_pb.ContentType_ValidateStaleLinksResultType), self)
		binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
			close(closeNotify)
		}))
		return nil
	}

	ch, err := api.NewWsMgmtChannel(channel.BindHandlerF(bindHandler))
	if err != nil {
		return err
	}

	request := &mgmt_pb.ValidateStaleLinksRequest{
		Filter: self.filter,
		Mode:   mode,
		Gc:     self.gc,
	}

	respMsg, err := protobufs.MarshalTyped(request).
		WithTimeout(time.Duration(self.Timeout) * time.Second).
		SendForReply(ch)

	response := &mgmt_pb.ValidateStaleLinksResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(respMsg, err); err != nil {
		return err
	}
	if !response.Success {
		return fmt.Errorf("failed to start stale-link validation: %s", response.Message)
	}

	_, _ = fmt.Fprintf(self.out, "validating %d link(s) (mode=%s, gc=%v)\n", response.ExpectedLinkCount, self.mode, self.gc)

	staleCount, partialCount, missing := self.consumeResults(int(response.ExpectedLinkCount), closeNotify)

	_, _ = fmt.Fprintf(self.out, "%d link(s) total, %d stale, %d partial\n",
		response.ExpectedLinkCount, staleCount, partialCount)

	// Report an incomplete run as a failure. Callers can't tell a clean sweep
	// from a truncated one by exit code otherwise, and the links whose results
	// never arrived are exactly the ones left unverified.
	if missing > 0 {
		return fmt.Errorf("verification incomplete: %d of %d link result(s) never arrived; %d stale link(s) among those received",
			missing, response.ExpectedLinkCount, staleCount)
	}
	if staleCount > 0 {
		return fmt.Errorf("%d stale link(s) found", staleCount)
	}
	return nil
}

// consumeResults reads up to expected results, returning the stale and partial
// counts and how many never arrived. A non-zero missing count means the stream
// ended early, so the counts describe only the subset that was received.
func (self *verifyStaleLinksAction) consumeResults(expected int, closeNotify <-chan struct{}) (staleCount, partialCount, missing int) {
	accept := func(result *mgmt_pb.StaleLinkResult) {
		if result.Stale {
			staleCount++
		}
		if result.Partial {
			partialCount++
		}
		if result.Stale || self.includeNotStale {
			self.printResult(result)
		}
	}

	for expected > 0 {
		select {
		case <-closeNotify:
			// A select with both cases ready picks at random, so the channel
			// closing says nothing about whether results are still buffered.
			// Drain what the receive handler already accepted before calling
			// any of it missing.
			for expected > 0 {
				select {
				case result := <-self.resultsChan:
					expected--
					accept(result)
				default:
					_, _ = fmt.Fprintln(self.out, "channel closed before all results received")
					return staleCount, partialCount, expected
				}
			}
			return staleCount, partialCount, 0
		case result := <-self.resultsChan:
			expected--
			accept(result)
		}
	}
	return staleCount, partialCount, 0
}

func (self *verifyStaleLinksAction) printResult(r *mgmt_pb.StaleLinkResult) {
	status := "OK"
	if r.Stale {
		status = "STALE"
	}
	if r.Partial {
		status += " (partial)"
	}
	if r.GcApplied {
		status += " gc=applied"
	}
	_, _ = fmt.Fprintf(self.out, "%-25s link=%s src=%s dst=%s dialer=%s listener=%s\n",
		status, r.LinkId, r.SrcRouterId, r.DstRouterId,
		verdictName(r.DialerVerdict), verdictName(r.ListenerVerdict))
	for _, reason := range r.Reasons {
		_, _ = fmt.Fprintf(self.out, "    %s\n", reason)
	}
}

func verdictName(v mgmt_pb.StaleVerdict) string {
	switch v {
	case mgmt_pb.StaleVerdict_StaleVerdictStale:
		return "stale"
	case mgmt_pb.StaleVerdict_StaleVerdictNotStale:
		return "ok"
	default:
		return "unknown"
	}
}

func parseMatchMode(s string) (mgmt_pb.StaleLinkMatchMode, error) {
	switch strings.ToLower(s) {
	case "changed":
		return mgmt_pb.StaleLinkMatchMode_StaleLinkMatchChanged, nil
	case "orphaned":
		return mgmt_pb.StaleLinkMatchMode_StaleLinkMatchOrphaned, nil
	default:
		return 0, fmt.Errorf("unknown --mode %q (expected changed|orphaned)", s)
	}
}

// HandleReceive implements channel.ReceiveHandler — pushes each result
// onto the channel that the consumer loop reads.
func (self *verifyStaleLinksAction) HandleReceive(msg *channel.Message, _ channel.Channel) {
	result := &mgmt_pb.StaleLinkResult{}
	if err := proto.Unmarshal(msg.Body, result); err != nil {
		pfxlog.Logger().WithError(err).Error("unable to unmarshal StaleLinkResult")
		return
	}
	// Deliberately blocking: this is a one-shot CLI whose only job is to drain
	// these results, so backpressure onto the controller is the wanted behavior
	// when it outruns the consumer. If consumeResults gives up first the rxer
	// parks here until the process exits, which is the end of the run anyway.
	self.resultsChan <- result
}
