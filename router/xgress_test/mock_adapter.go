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

package xgress_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/sdk-golang/xgress"
)

// MockDataPlaneAdapter is a shared test implementation of DataPlaneAdapter that routes messages between xgress instances
type MockDataPlaneAdapter struct {
	// closed tracks whether the adapter is closed
	closed atomic.Bool

	// env provides required environment components
	env xgress.Env

	// mutex protects concurrent access to the adapter
	mutex sync.RWMutex

	// xgressInstances maps addresses to xgress instances
	xgressInstances map[xgress.Address]*xgress.Xgress

	// circuits maps circuit IDs to a pair of xgress addresses for efficient routing
	circuits map[string]map[xgress.Address]xgress.Address
}

// NewMockDataPlaneAdapter creates a new shared mock adapter
func NewMockDataPlaneAdapter() *MockDataPlaneAdapter {
	return &MockDataPlaneAdapter{
		env:             NewMockEnv(),
		xgressInstances: make(map[xgress.Address]*xgress.Xgress),
		circuits:        make(map[string]map[xgress.Address]xgress.Address),
	}
}

func (self *MockDataPlaneAdapter) CloseCircuit(circuitId string) {
	var instances []*xgress.Xgress
	self.mutex.Lock()
	for _, instance := range self.xgressInstances {
		if instance.CircuitId() == circuitId {
			instances = append(instances, instance)
		}
	}
	self.mutex.Unlock()

	for _, instance := range instances {
		self.UnregisterXgress(instance.Address())
		instance.Close()
	}

	self.mutex.Lock()
	delete(self.circuits, circuitId)
	self.mutex.Unlock()
}

// RegisterXgress registers an xgress instance with this shared adapter
func (self *MockDataPlaneAdapter) RegisterXgress(x *xgress.Xgress) {
	if self.closed.Load() {
		return
	}
	self.mutex.Lock()
	self.xgressInstances[x.Address()] = x
	self.mutex.Unlock()
}

// UnregisterXgress removes an xgress instance from this shared adapter
func (self *MockDataPlaneAdapter) UnregisterXgress(address xgress.Address) {
	if self.closed.Load() {
		return
	}

	self.mutex.Lock()
	delete(self.xgressInstances, address)
	self.mutex.Unlock()
}

// ConnectCircuit establishes a circuit between two xgress addresses
func (self *MockDataPlaneAdapter) ConnectCircuit(circuitId string, addr1, addr2 xgress.Address) {
	if self.closed.Load() {
		return
	}

	self.mutex.Lock()
	defer self.mutex.Unlock()
	circuitMap, ok := self.circuits[circuitId]
	if !ok {
		circuitMap = map[xgress.Address]xgress.Address{}
		self.circuits[circuitId] = circuitMap
	}
	circuitMap[addr1] = addr2
}

// forwardToDestination forwards payload to the destination xgress
func (self *MockDataPlaneAdapter) withDestination(circuitId string, srcAddr xgress.Address, f func(x *xgress.Xgress) error) error {
	if self.closed.Load() {
		return errors.New("forwarder closed")
	}

	logger := pfxlog.Logger().WithField("circuitId", circuitId).WithField("srcAddr", srcAddr)
	self.mutex.RLock()
	circuitMap, exists := self.circuits[circuitId]
	self.mutex.RUnlock()
	if !exists {
		logger.Error("no forwarding information found for circuit")
		return fmt.Errorf("no forwarding information found for circuit %s, srcAddr: %s", circuitId, srcAddr)
	}

	self.mutex.RLock()
	destAddr, exists := circuitMap[srcAddr]
	self.mutex.RUnlock()
	if !exists {
		logger.Error("no destination address found for source address")
		return fmt.Errorf("no destination address found for src address circuit %s, srcAddr: %s", circuitId, srcAddr)
	}

	logger = logger.WithField("destAddr", destAddr)

	self.mutex.RLock()
	targetXgress, exists := self.xgressInstances[destAddr]
	self.mutex.RUnlock()
	if !exists {
		logger.Error("no destination found for destination address")
		return fmt.Errorf("no destination found for destination address circuit %s, srcAddr: %s, destAddr: %s", circuitId, srcAddr, destAddr)
	}

	return f(targetXgress)
}

// ForwardPayload forwards a payload from one xgress to another based on circuit routing
func (self *MockDataPlaneAdapter) ForwardPayload(payload *xgress.Payload, x *xgress.Xgress, _ context.Context) {
	//	debugz.DumpLocalStack()
	err := self.withDestination(payload.CircuitId, x.Address(), func(x *xgress.Xgress) error {
		return x.SendPayload(payload, time.Second, xgress.PayloadTypeFwd)
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Error("error forwarding payload")
	}
}

// RetransmitPayload retransmits a payload by reusing the forward logic
func (self *MockDataPlaneAdapter) RetransmitPayload(srcAddr xgress.Address, payload *xgress.Payload) error {
	return self.withDestination(payload.CircuitId, srcAddr, func(x *xgress.Xgress) error {
		return x.SendPayload(payload, time.Second, xgress.PayloadTypeRtx)
	})
}

// ForwardControlMessage forwards a control message between xgress instances
func (self *MockDataPlaneAdapter) ForwardControlMessage(control *xgress.Control, x *xgress.Xgress) {
	err := self.withDestination(control.CircuitId, x.Address(), func(x *xgress.Xgress) error {
		return x.SendControl(control)
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Error("error forwarding control message")
	}
}

// ForwardAcknowledgement forwards an acknowledgement to the target address
func (self *MockDataPlaneAdapter) ForwardAcknowledgement(ack *xgress.Acknowledgement, address xgress.Address) {
	err := self.withDestination(ack.CircuitId, address, func(x *xgress.Xgress) error {
		return x.SendAcknowledgement(ack)
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Error("error forwarding acknowledgement")
	}
}

// GetRetransmitter returns the retransmitter from the environment
func (self *MockDataPlaneAdapter) GetRetransmitter() *xgress.Retransmitter {
	return self.env.GetRetransmitter()
}

// GetPayloadIngester returns the payload ingester from the environment
func (self *MockDataPlaneAdapter) GetPayloadIngester() *xgress.PayloadIngester {
	return self.env.GetPayloadIngester()
}

// GetMetrics returns the metrics from the environment
func (self *MockDataPlaneAdapter) GetMetrics() xgress.Metrics {
	return self.env.GetMetrics()
}

func (self *MockDataPlaneAdapter) Close() {
	if !self.closed.CompareAndSwap(false, true) {
		return
	}

	self.mutex.Lock()
	defer self.mutex.Unlock()

	// Close all registered xgress instances
	for _, x := range self.xgressInstances {
		x.CloseSendBuffer()
		x.CloseXgToClient()
	}

	self.xgressInstances = make(map[xgress.Address]*xgress.Xgress)
	self.circuits = make(map[string]map[xgress.Address]xgress.Address)
}
