/*
	Copyright NetFoundry, Inc.

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

package xt_ha

import (
	"github.com/openziti/fabric/controller/xt"
	"sync/atomic"
)

func NewFactory() xt.Factory {
	return factory{}
}

/**
The ha stategy manages an active/passive failover setup. It is assumed that one terminator
will have the Required precedence, and the other will be Default. Smart routing will always
pick the required terminator. If the required terminator is marked as Failed, either by an
external monitoring system or by an internal algorithm then smart routing will start picking
the secondary terminator. The system relies on an external system or operator to mark the
primary as Required again when it has recovered.
*/
type factory struct{}

func (f factory) GetStrategyName() string {
	return "ha"
}

func (f factory) NewStrategy() xt.Strategy {
	return strategy{}
}

type strategy struct {
	xt.DefaultEventVisitor
	failCount int32
}

func (s strategy) VisitDialFailed(event xt.TerminatorEvent) {
	failCount := atomic.AddInt32(&s.failCount, 1)
	if failCount >= 3 {
		xt.GlobalCosts().SetPrecedence(event.GetTerminator().GetId(), xt.Precedences.Failed)
	}
}

func (s strategy) VisitDialSucceeded(xt.TerminatorEvent) {
	atomic.StoreInt32(&s.failCount, 0)
}

func (s strategy) Select(terminators []xt.CostedTerminator) (xt.Terminator, error) {
	return terminators[0], nil
}

func (s strategy) NotifyEvent(event xt.TerminatorEvent) {
	event.Accept(s)
}

func (s strategy) HandleTerminatorChange(xt.StrategyChangeEvent) error {
	return nil
}
