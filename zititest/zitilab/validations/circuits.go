package validations

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/channel/v5/protobufs"
	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/controller/rest_client/circuit"
	"github.com/openziti/ziti/v2/controller/rest_model"
	"github.com/openziti/ziti/v2/zitirest"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
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

// ValidateCircuitsDrained waits for circuits to be torn down once traffic has stopped, and
// reports what is left if they aren't. It complements ValidateCircuits, which checks that the
// controllers, forwarders and SDKs agree about the circuits that exist: a circuit that never
// completes its close handshake is consistent everywhere and so passes that check, while still
// holding its xgress instances, buffers and goroutines for as long as it survives.
//
// Teardown is asynchronous. An endpoint closing faults to its controller, which removes the
// circuit and unroutes the remaining endpoints, so this polls until the deadline rather than
// expecting a drained network the instant the last request completes.
//
// includeService selects which circuits are expected to drain, by service name. Models that run
// their own control or telemetry traffic over the overlay should exclude those services, since
// they stay up for the life of the run. A nil predicate checks every circuit.
//
// Every controller is queried, since in an HA cluster each one owns the circuits it created.
func ValidateCircuitsDrained(run model.Run, deadline time.Duration, includeService func(serviceName string) bool) error {
	ctrls := run.GetModel().SelectComponents(".ctrl")
	logger := tui.ValidationLogger()

	start := time.Now()
	dl := start.Add(deadline)

	for {
		remaining, err := listUndrainedCircuits(run, ctrls, includeService)

		if err == nil && len(remaining) == 0 {
			logger.Infof("all circuits drained after %v", time.Since(start).Round(time.Second))
			return nil
		}

		if time.Now().After(dl) {
			if err != nil {
				return err
			}
			reportUndrainedCircuits(logger, remaining)
			return fmt.Errorf("%v circuits still active %v after traffic stopped", len(remaining), deadline)
		}

		if err != nil {
			logger.Infof("unable to list circuits, retrying: %v", err)
		} else {
			logger.Infof("%v circuits still active, elapsed time: %v", len(remaining), time.Since(start).Round(time.Second))
		}

		time.Sleep(5 * time.Second)
	}
}

// reportUndrainedCircuits logs the oldest circuits still standing. Age is the useful signal:
// circuits that outlive the traffic that created them by minutes are stuck, not in flight.
func reportUndrainedCircuits(logger *logrus.Entry, remaining []*circuitSummary) {
	sort.Slice(remaining, func(i, j int) bool {
		return remaining[i].age > remaining[j].age
	})

	byService := map[string]int{}
	for _, c := range remaining {
		byService[c.service]++
	}
	for service, count := range byService {
		logger.Infof("\tservice %s: %v circuits not drained", service, count)
	}

	const maxReported = 10
	for idx, c := range remaining {
		if idx >= maxReported {
			logger.Infof("\t... and %v more", len(remaining)-maxReported)
			break
		}
		logger.Infof("\tcircuit %s not drained: service=%s age=%v", c.id, c.service, c.age.Round(time.Second))
	}
}

func listUndrainedCircuits(run model.Run, ctrls []*model.Component, includeService func(string) bool) ([]*circuitSummary, error) {
	var result []*circuitSummary

	for _, ctrl := range ctrls {
		clients, err := chaos.EnsureLoggedIntoCtrl(run, ctrl, time.Minute)
		if err != nil {
			return nil, err
		}

		circuits, err := listCircuits(clients)
		if err != nil {
			return nil, fmt.Errorf("unable to list circuits on %s (%w)", ctrl.Id, err)
		}

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
	}

	return result, nil
}

// listCircuits pages through all circuits known to a controller. The page size is well above
// what a drained network should return, but a leak can produce thousands, and reporting only
// the first page would understate it.
func listCircuits(clients *zitirest.Clients) (rest_model.CircuitList, error) {
	ctx, cancelF := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelF()

	limit := int64(500)
	offset := int64(0)

	var result rest_model.CircuitList
	for {
		listOk, err := clients.Fabric.Circuit.ListCircuits(&circuit.ListCircuitsParams{
			Limit:   &limit,
			Offset:  &offset,
			Context: ctx,
		}, nil)
		if err != nil {
			return nil, err
		}

		result = append(result, listOk.Payload.Data...)

		if len(listOk.Payload.Data) < int(limit) {
			return result, nil
		}
		offset += limit
	}
}
