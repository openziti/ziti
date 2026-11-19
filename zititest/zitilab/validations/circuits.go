package validations

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/channel/v5/protobufs"
	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/controller/rest_client/circuit"
	"github.com/openziti/ziti/v2/controller/rest_model"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/openziti/ziti/zititest/zitirest"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

func ValidateCircuits(run model.Run, deadline time.Duration, routerFilter string) error {
	ctrls := run.GetModel().SelectComponents(".ctrl")
	errC := make(chan error, len(ctrls))
	dl := time.Now().Add(deadline)
	for _, ctrl := range ctrls {
		ctrlComponent := ctrl
		go func() {
			errC <- ValidateCircuitsForCtrl(run, ctrlComponent, dl, routerFilter)
		}()
	}

	for range len(ctrls) {
		err := <-errC
		if err != nil {
			return err
		}
	}

	return nil
}

func ValidateCircuitsForCtrl(run model.Run, c *model.Component, deadline time.Time, routerFilter string) error {
	clients, err := chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
	if err != nil {
		return err
	}

	start := time.Now()
	logger := tui.ValidationLogger().WithField("ctrl", c.Id)

	first := true
	for {
		count, err := validateCircuitsOnce(c.Id, clients, routerFilter, first)
		if err == nil {
			return nil
		}

		if time.Now().After(deadline) {
			return err
		}

		logger.Infof("current count of circuit errors: %v, elapsed time: %v, current err: %v", count, time.Since(start), err)
		time.Sleep(15 * time.Second)

		clients, err = chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
		if err != nil {
			return err
		}
		first = false
	}
}

func validateCircuitsOnce(id string, clients *zitirest.Clients, routerFilter string, first bool) (int, error) {
	logger := tui.ValidationLogger().WithField("ctrl", id)

	closeNotify := make(chan struct{})
	eventNotify := make(chan *mgmt_pb.RouterCircuitDetails, 1)

	handleResults := func(msg *channel.Message, _ channel.Channel) {
		detail := &mgmt_pb.RouterCircuitDetails{}
		if err := proto.Unmarshal(msg.Body, detail); err != nil {
			pfxlog.Logger().WithError(err).Error("unable to unmarshal circuit validation details")
			return
		}
		eventNotify <- detail
	}

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_ValidateCircuitsResultType), handleResults)
		binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
			close(closeNotify)
		}))
		return nil
	}

	ch, err := clients.NewWsMgmtChannel(channel.BindHandlerF(bindHandler))
	if err != nil {
		return 0, err
	}

	defer func() {
		_ = ch.Close()
	}()

	request := &mgmt_pb.ValidateCircuitsRequest{
		RouterFilter: routerFilter,
	}
	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(10 * time.Second).SendForReply(ch)

	response := &mgmt_pb.ValidateCircuitsResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(responseMsg, err); err != nil {
		return 0, err
	}

	if !response.Success {
		return 0, fmt.Errorf("failed to start circuit validation: %s", response.Message)
	}

	logger.Infof("started validation of %v components", response.RouterCount)

	expected := response.RouterCount

	invalid := 0
	for expected > 0 {
		select {
		case <-closeNotify:
			logger.Info("channel closed, exiting")
			return 0, errors.New("unexpected close of mgmt channel")
		case detail := <-eventNotify:
			if !detail.ValidateSuccess {
				invalid++
				logger.Infof("error validating router %s using ctrl %s: %s", detail.RouterId, id, detail.Message)
			}
			for _, details := range detail.Details {
				if details.IsInErrorState() {
					if !first {
						logger.Infof("\tcircuit: %s ctrl: %v, fwd: %v, edge: %v, sdk: %v, dest: %+v",
							details.CircuitId, details.MissingInCtrl, details.MissingInForwarder,
							details.MissingInEdge, details.MissingInSdk, details.Destinations)
					}
					invalid++
				}
			}
			expected--
		}
	}
	if invalid == 0 {
		logger.Infof("circuit validation of %v routers successful", response.RouterCount)
		return invalid, nil
	}
	return invalid, errors.New("errors found")
}

// circuitSummary is the subset of a circuit's state worth reporting when it fails to drain.
type circuitSummary struct {
	id      string
	service string
	age     time.Duration
}

const drainPollInterval = 5 * time.Second

// ValidateCircuitsDrained polls every controller until none reports a circuit on a service
// selected by includeService, or timeout passes. A nil includeService selects every service.
// Call it once traffic has stopped: circuits still in use count as undrained. No poll runs past
// timeout. On failure it logs the remaining circuits from the last successful poll, grouped by
// service and oldest first; if the final poll failed, the error says drainage could not be verified.
// It does not check that controllers and routers agree on circuits; see ValidateCircuits.
func ValidateCircuitsDrained(run model.Run, timeout time.Duration, includeService func(serviceName string) bool) error {
	ctrls := run.GetModel().SelectComponents(".ctrl")
	logger := tui.ValidationLogger()

	start := time.Now()
	deadline := start.Add(timeout)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	var lastRemaining []*circuitSummary
	var lastListedAt time.Duration
	for {
		remaining, err := listUndrainedCircuits(ctx, run, ctrls, includeService)
		if err == nil {
			if len(remaining) == 0 {
				logger.Infof("all circuits drained after %v", time.Since(start).Round(time.Second))
				return nil
			}
			lastRemaining = remaining
			lastListedAt = time.Since(start).Round(time.Second)
		}

		if time.Until(deadline) < drainPollInterval {
			if err != nil {
				if lastRemaining == nil {
					return fmt.Errorf("unable to verify circuits drained within %v (%w)", timeout, err)
				}
				reportUndrainedCircuits(logger, fmt.Sprintf("last successful poll, %v after traffic stopped", lastListedAt), lastRemaining)
				return fmt.Errorf("unable to verify circuits drained within %v; %v circuits were active at the last successful poll, %v after traffic stopped (%w)",
					timeout, len(lastRemaining), lastListedAt, err)
			}
			reportUndrainedCircuits(logger, fmt.Sprintf("%v after traffic stopped", lastListedAt), lastRemaining)
			return fmt.Errorf("%v circuits still active %v after traffic stopped", len(lastRemaining), lastListedAt)
		}

		if err != nil {
			logger.Infof("unable to list circuits, retrying: %v", err)
		} else {
			logger.Infof("%v circuits still active, elapsed time: %v", len(remaining), lastListedAt)
		}

		time.Sleep(drainPollInterval)
	}
}

// reportUndrainedCircuits logs per-service counts and the oldest of the remaining circuits, with
// when describing the poll they come from.
func reportUndrainedCircuits(logger *logrus.Entry, when string, remaining []*circuitSummary) {
	logger.Infof("circuits not drained as of %s:", when)
	sort.Slice(remaining, func(i, j int) bool {
		return remaining[i].age > remaining[j].age
	})

	byService := map[string]int{}
	for _, c := range remaining {
		byService[c.service]++
	}
	for _, service := range slices.Sorted(maps.Keys(byService)) {
		logger.Infof("\tservice %s: %v circuits", service, byService[service])
	}

	const maxReported = 10
	for idx, c := range remaining {
		if idx >= maxReported {
			logger.Infof("\t... and %v more", len(remaining)-maxReported)
			break
		}
		logger.Infof("\tcircuit %s: service=%s age=%v", c.id, c.service, c.age.Round(time.Second))
	}
}

// listUndrainedCircuits queries all ctrls concurrently and returns the circuits on services
// selected by includeService. Any controller failing fails the whole call, since a partial
// list would understate what remains. No query outlasts ctx.
func listUndrainedCircuits(ctx context.Context, run model.Run, ctrls []*model.Component, includeService func(string) bool) ([]*circuitSummary, error) {
	results := make([][]*circuitSummary, len(ctrls))
	errs := make([]error, len(ctrls))

	var wg sync.WaitGroup
	for idx, ctrl := range ctrls {
		wg.Go(func() {
			results[idx], errs[idx] = listUndrainedCircuitsForCtrl(ctx, run, ctrl, includeService)
		})
	}
	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return slices.Concat(results...), nil
}

func listUndrainedCircuitsForCtrl(ctx context.Context, run model.Run, ctrl *model.Component, includeService func(string) bool) ([]*circuitSummary, error) {
	loginTimeout := time.Minute
	if deadline, ok := ctx.Deadline(); ok {
		loginTimeout = min(loginTimeout, time.Until(deadline))
	}
	clients, err := chaos.EnsureLoggedIntoCtrl(run, ctrl, loginTimeout)
	if err != nil {
		return nil, fmt.Errorf("unable to log into %s (%w)", ctrl.Id, err)
	}

	circuits, err := listCircuits(ctx, clients)
	if err != nil {
		return nil, fmt.Errorf("unable to list circuits on %s (%w)", ctrl.Id, err)
	}

	var result []*circuitSummary
	for _, detail := range circuits {
		serviceName := ""
		if detail.Service != nil {
			serviceName = detail.Service.Name
		}

		if includeService != nil && !includeService(serviceName) {
			continue
		}

		summary := &circuitSummary{service: serviceName}
		if detail.ID != nil {
			summary.id = *detail.ID
		}
		if detail.CreatedAt != nil {
			summary.age = time.Since(time.Time(*detail.CreatedAt))
		}
		result = append(result, summary)
	}

	return result, nil
}

// listCircuits returns every circuit known to the controller, paging through the full list.
// Each page request has its own timeout, and none runs past ctx.
func listCircuits(ctx context.Context, clients *zitirest.Clients) (rest_model.CircuitList, error) {
	limit := int64(500)
	offset := int64(0)

	var result rest_model.CircuitList
	for {
		page, err := listCircuitsPage(ctx, clients, limit, offset)
		if err != nil {
			return nil, err
		}

		result = append(result, page...)

		if len(page) < int(limit) {
			return result, nil
		}
		offset += limit
	}
}

func listCircuitsPage(ctx context.Context, clients *zitirest.Clients, limit, offset int64) (rest_model.CircuitList, error) {
	ctx, cancelF := context.WithTimeout(ctx, 30*time.Second)
	defer cancelF()

	listOk, err := clients.Fabric.Circuit.ListCircuits(&circuit.ListCircuitsParams{
		Limit:   &limit,
		Offset:  &offset,
		Context: ctx,
	}, nil)
	if err != nil {
		return nil, err
	}
	return listOk.Payload.Data, nil
}
