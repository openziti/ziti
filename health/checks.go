package health

import (
	"bytes"
	"fmt"
	health "github.com/AppsFlyer/go-sundheit"
	"github.com/AppsFlyer/go-sundheit/checks"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/pkg/errors"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

type CheckDefinition interface {
	GetType() string
	GetInterval() time.Duration
	GetTimeout() time.Duration
	CreateCheck(name string) (Check, error)
	CreateActions() ([]Action, error)
	fmt.Stringer
}

type ActionDefinition struct {
	Trigger           string
	ConsecutiveEvents *uint16
	Duration          *time.Duration
	Action            string
}

func (self *ActionDefinition) Description() string {
	return fmt.Sprintf("action trigger=%v consectiveEvents=%v duration=%v action=%v",
		self.Trigger, self.ConsecutiveEvents, self.Duration, self.Action)
}

func (self *ActionDefinition) CreateAction() (Action, error) {
	result := &actionImpl{
		ActionDefinition: *self,
	}

	if self.Action == "mark healthy" {
		result.actionImpl = func(state *ServiceState) {
			state.nextPrecedence = state.BaselinePrecedence
		}
	} else if self.Action == "mark unhealthy" {
		result.actionImpl = func(state *ServiceState) {
			state.nextPrecedence = edge.PrecedenceFailed
		}
	} else if self.Action == "send event" {
		result.actionImpl = func(state *ServiceState) {
			state.sendEvent = true
		}
	} else {
		increase := true
		var costStr string
		if strings.HasPrefix(self.Action, "increase cost ") {
			costStr = strings.TrimPrefix(self.Action, "increase cost ")
		} else if strings.HasPrefix(self.Action, "decrease cost ") {
			increase = false
			costStr = strings.TrimPrefix(self.Action, "decrease cost ")
		} else {
			return nil, errors.Errorf("invalid health check action %v", self.Action)
		}

		val, err := strconv.Atoi(costStr)
		if err != nil {
			return nil, errors.Errorf("invalid health check action %v", self.Action)
		}
		if val < 1 {
			return nil, errors.Errorf("cost change must be greater than 0. invalid action: %v", self.Action)
		}
		if val > math.MaxUint16 {
			return nil, errors.Errorf("cost change must be greater than less or equal to %v. invalid action: %v", math.MaxUint16, self.Action)
		}

		if increase {
			result.actionImpl = func(state *ServiceState) {
				// don't continue to increase costs after we've marked the terminator failed
				if state.currentPrecedence != edge.PrecedenceFailed {
					nextCost := uint32(state.nextCost) + uint32(val)
					if nextCost > math.MaxUint16 {
						nextCost = math.MaxUint16
					}
					state.nextCost = uint16(nextCost)
				}
			}
		} else {
			result.actionImpl = func(state *ServiceState) {
				nextCost := int32(state.nextCost) - int32(val)
				if nextCost < int32(state.BaselineCost) {
					nextCost = int32(state.BaselineCost)
				}
				state.nextCost = uint16(nextCost)
			}
		}
	}

	return result, nil
}

type actionImpl struct {
	ActionDefinition
	consectivePasses int64
	passingSince     time.Time
	actionImpl       func(state *ServiceState)
}

func (self *actionImpl) Matches(result *health.Result) bool {
	logger := pfxlog.Logger()
	logger.WithField("action", self.Description()).Debug("evaluating")

	now := roundToClosest(time.Now(), timeClamp)
	if result.IsHealthy() {
		if self.consectivePasses == 0 {
			self.passingSince = now
		}
		self.consectivePasses++
	} else {
		self.consectivePasses = 0
	}

	if self.Trigger == "fail" && result.IsHealthy() {
		return false
	}
	if self.Trigger == "pass" && !result.IsHealthy() {
		return false
	}
	if self.Trigger == "change" && self.consectivePasses != 1 && result.ContiguousFailures != 1 {
		return false
	}

	if self.ConsecutiveEvents != nil {
		if result.IsHealthy() && self.consectivePasses < int64(*self.ConsecutiveEvents) {
			return false
		}
		if !result.IsHealthy() && result.ContiguousFailures < int64(*self.ConsecutiveEvents) {
			return false
		}
	}

	if self.Duration != nil {
		if result.IsHealthy() && now.Sub(self.passingSince) < *self.Duration {
			return false
		}
		if result.TimeOfFirstFailure != nil {
			fmt.Printf("current fail duration: %v, required duration: %v\n", now.Sub(*result.TimeOfFirstFailure), *self.Duration)
		}
		if !result.IsHealthy() && now.Sub(*result.TimeOfFirstFailure) < *self.Duration {
			return false
		}
	}

	return true
}

func (self *actionImpl) Invoke(state *ServiceState) {
	self.actionImpl(state)
}

type BaseCheckDefinition struct {
	Interval time.Duration
	Timeout  time.Duration
	Actions  []*ActionDefinition
}

func (self *BaseCheckDefinition) GetInterval() time.Duration {
	return self.Interval
}

func (self *BaseCheckDefinition) GetTimeout() time.Duration {
	return self.Timeout
}

func (self *BaseCheckDefinition) CreateActions() ([]Action, error) {
	var result []Action
	for _, actionDefinition := range self.Actions {
		action, err := actionDefinition.CreateAction()
		if err != nil {
			return nil, err
		}
		result = append(result, action)
	}
	return result, nil
}

type PortCheckDefinition struct {
	BaseCheckDefinition `mapstructure:",squash"`
	Address             string
}

func (self *PortCheckDefinition) String() string {
	return fmt.Sprintf("port-check address=%v, interval=%v, timeout=%v", self.Address, self.Interval, self.Timeout)
}

func (self *PortCheckDefinition) GetType() string {
	return "port"
}

func (self *PortCheckDefinition) CreateCheck(name string) (Check, error) {
	return checks.NewPingCheck(name, checks.NewDialPinger("tcp", self.Address))
}

type HttpCheckDefinition struct {
	BaseCheckDefinition `mapstructure:",squash"`
	Url                 string
	Method              string
	Body                string
	ExpectStatus        int
	ExpectBody          string
}

func (self *HttpCheckDefinition) String() string {
	return fmt.Sprintf("http-check url=%v, interval=%v, timeout=%v, method=%v, status=%v", self.Url, self.Interval, self.Timeout, self.Method, self.ExpectStatus)
}

func (self *HttpCheckDefinition) GetType() string {
	return "http"
}

func (self *HttpCheckDefinition) CreateCheck(name string) (Check, error) {
	if self.Method == "" {
		self.Method = "GET"
	}

	if self.ExpectStatus == 0 {
		self.ExpectStatus = 200
	}

	var bodyProvider checks.BodyProvider

	if self.Body != "" {
		bodyProvider = func() io.Reader {
			return bytes.NewBufferString(self.Body)
		}
	}

	httpCheckConfig := checks.HTTPCheckConfig{
		CheckName:      name,
		URL:            self.Url,
		Method:         self.Method,
		Body:           bodyProvider,
		ExpectedStatus: self.ExpectStatus,
		ExpectedBody:   self.ExpectBody,
		Timeout:        self.Timeout,
	}

	return checks.NewHTTPCheck(httpCheckConfig)
}
