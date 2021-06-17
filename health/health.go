package health

import (
	"context"
	"fmt"
	health "github.com/AppsFlyer/go-sundheit"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/sirupsen/logrus"
	"reflect"
	"sync"
	"time"
)

const (
	timeClamp = 100 * time.Millisecond
)

type ServiceUpdater interface {
	UpdateCostAndPrecedence(cost uint16, precedence edge.Precedence) error
	SendHealthEvent(pass bool) error
}

type Check interface {
	Name() string
	Execute(context context.Context) (details interface{}, err error)
}

type Action interface {
	Matches(result *health.Result) bool
	Invoke(state *ServiceState)
	Description() string
}

func NewServiceState(service string, precedence ziti.Precedence, cost uint16, updater ServiceUpdater) *ServiceState {
	return NewServiceStateWithContext("0", service, precedence, cost, updater)
}

func NewServiceStateWithContext(service, hostContext string, precedence ziti.Precedence, cost uint16, updater ServiceUpdater) *ServiceState {
	return &ServiceState{
		Service:            service,
		HostContext:        hostContext,
		BaselinePrecedence: edge.Precedence(precedence),
		currentPrecedence:  edge.Precedence(precedence),
		nextPrecedence:     edge.Precedence(precedence),
		BaselineCost:       cost,
		Updater:            updater,
		currentCost:        cost,
		nextCost:           cost,
	}
}

type ServiceState struct {
	Service     string
	HostContext string

	BaselinePrecedence edge.Precedence
	currentPrecedence  edge.Precedence
	nextPrecedence     edge.Precedence
	sendEvent          bool

	BaselineCost uint16
	Updater      ServiceUpdater
	currentCost  uint16
	nextCost     uint16
}

func (self *ServiceState) IsChanged() bool {
	return self.nextPrecedence != self.currentPrecedence || self.nextCost != self.currentCost
}

func (self *ServiceState) HandleActionResults(healthy bool) {
	if self.sendEvent {
		if err := self.Updater.SendHealthEvent(healthy); err != nil {
			logrus.WithError(err).
				WithField("service", self.Service).
				WithField("hostContext", self.HostContext).
				WithField("isHealth", healthy).
				Error("error sending health event")
		}
		self.sendEvent = false
	}

	if !self.IsChanged() {
		return
	}

	if err := self.Updater.UpdateCostAndPrecedence(self.nextCost, self.nextPrecedence); err != nil {
		logrus.WithError(err).
			WithField("service", self.Service).
			WithField("hostContext", self.HostContext).
			WithField("nextCost", self.nextCost).
			WithField("nextPrecedence", self.nextPrecedence).
			Error("error updating cost/precedence on service")
	} else {
		self.currentCost = self.nextCost
		self.currentPrecedence = self.nextPrecedence
	}
}

type checkContext struct {
	id           string
	checkType    string
	serviceState *ServiceState
	actions      []Action
}

func NewManager() Manager {
	result := &manager{
		results: make(chan *result, 16),
	}
	result.health = health.New(health.WithCheckListeners(result))

	go result.handleResults()

	return result
}

type Manager interface {
	RegisterServiceChecks(service *ServiceState, checkDefinitions []CheckDefinition) error
	UnregisterServiceChecks(service string)
	UnregisterServiceContextChecks(service, context string)
	Shutdown()
}

type manager struct {
	checks  sync.Map
	health  health.Health
	results chan *result
	closed  concurrenz.AtomicBoolean
}

func (self *manager) Shutdown() {
	if self.closed.CompareAndSwap(false, true) {
		self.health.DeregisterAll()
		close(self.results)
	}
}

func (self *manager) handleResults() {
	for result := range self.results {
		self.handleResult(result)
	}
}

func (self *manager) handleResult(result *result) {
	if result.TimeOfFirstFailure != nil {
		rounded := roundToClosest(*result.TimeOfFirstFailure, timeClamp)
		result.TimeOfFirstFailure = &rounded
	}
	if val, ok := self.checks.Load(result.name); ok {
		if check, ok := val.(*checkContext); ok {
			for _, action := range check.actions {
				if action.Matches(&result.Result) {
					action.Invoke(check.serviceState)
				}
			}
			check.serviceState.HandleActionResults(result.IsHealthy())
		} else {
			logrus.Errorf("coding error, check was of incorrect type: %v", reflect.TypeOf(val))
		}
	} else {
		logrus.Warnf("no health check context found for %v", result.name)
	}
}

func (self *manager) UnregisterServiceChecks(service string) {
	self.checks.Range(func(key, val interface{}) bool {
		check := val.(*checkContext)
		if check.serviceState.Service == service {
			self.checks.Delete(check.id)
			self.health.Deregister(check.id)
		}
		return true
	})
}

func (self *manager) UnregisterServiceContextChecks(service, hostContext string) {
	log := logrus.WithField("service", service).WithField("hostContext", hostContext)
	log.Debug("removing health checks")

	self.checks.Range(func(key, val interface{}) bool {
		check := val.(*checkContext)
		if check.serviceState.Service == service && check.serviceState.HostContext == hostContext {
			log.WithField("check", check.id).Debug("removing check")
			self.checks.Delete(check.id)
			self.health.Deregister(check.id)
		}
		return true
	})
}

func (self *manager) RegisterServiceChecks(service *ServiceState, checkDefinitions []CheckDefinition) error {
	logger := pfxlog.Logger()
	for idx, checkDefinition := range checkDefinitions {
		id := fmt.Sprintf("%v_%v_%v", service.Service, service.HostContext, idx)
		_, found := self.checks.Load(id)
		counter := 0
		for found {
			counter++
			id := fmt.Sprintf("%v_%v_%v_%v", service.Service, service.HostContext, idx, counter)
			_, found = self.checks.Load(id)
		}

		actions, err := checkDefinition.CreateActions()
		if err != nil {
			return err
		}

		context := &checkContext{
			id:           id,
			checkType:    checkDefinition.GetType(),
			serviceState: service,
			actions:      actions,
		}

		check, err := checkDefinition.CreateCheck(id)
		if err != nil {
			return err
		}

		self.checks.Store(id, context)

		options := []health.CheckOption{
			health.ExecutionPeriod(checkDefinition.GetInterval()),
			health.InitiallyPassing(true),
		}

		if checkDefinition.GetTimeout() > 0 {
			options = append(options, health.ExecutionTimeout(checkDefinition.GetTimeout()))
		}

		logger.WithField("service", service.Service).
			WithField("hostContext", service.HostContext).
			Debugf("adding check: %v", checkDefinition.String())
		if err = self.health.RegisterCheck(check, options...); err != nil {
			return err
		}
	}
	return nil
}

func (self *manager) OnCheckStarted(string) {
	// does nothing
}

func (self *manager) OnCheckRegistered(string, health.Result) {
	// does nothing
}

func (self *manager) OnCheckCompleted(name string, r health.Result) {
	if self.closed.Get() {
		return
	}

	logger := pfxlog.Logger().WithField("name", name).WithField("result", r.IsHealthy())
	if !r.IsHealthy() {
		logger = logger.WithError(r.Error)
		logger.Warn("health check failed")
	} else {
		logger.Debug("health check passed")
	}
	wrapped := &result{
		name:   name,
		Result: r,
	}

	// If result processing is slow, let it drop on the floor. In general results should be coming in
	// slowly enough that we shouln't have any trouble keeping up
	select {
	case self.results <- wrapped:
	default:
		logger.Warn("health check result dropped!")
	}
}

type result struct {
	name string
	health.Result
}

func roundToClosest(t time.Time, interval time.Duration) time.Time {
	roundDown := t.Truncate(interval)
	if t.Sub(roundDown) >= interval/2 {
		return roundDown.Add(interval)
	}
	return roundDown
}
