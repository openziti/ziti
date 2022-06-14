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

package xt

import (
	"fmt"
	"time"
)

type Registry interface {
	RegisterFactory(factory Factory)
	GetStrategy(name string) (Strategy, error)
}

type Factory interface {
	GetStrategyName() string
	NewStrategy() Strategy
}

type Terminator interface {
	GetId() string
	GetPrecedence() Precedence
	GetCost() uint16
	GetServiceId() string
	GetInstanceId() string
	GetRouterId() string
	GetBinding() string
	GetAddress() string
	GetPeerData() PeerData
	GetCreatedAt() time.Time
}

type PeerData map[uint32][]byte

type CostedTerminator interface {
	Terminator
	GetRouteCost() uint32
}

type StrategyChangeEvent interface {
	GetServiceId() string
	GetCurrent() []Terminator
	GetAdded() []Terminator
	GetChanged() []Terminator
	GetRemoved() []Terminator
}

type Strategy interface {
	Select(terminators []CostedTerminator) (CostedTerminator, error)
	HandleTerminatorChange(event StrategyChangeEvent) error
	NotifyEvent(event TerminatorEvent)
}

type Precedence interface {
	fmt.Stringer
	getMinCost() uint32
	getMaxCost() uint32
	IsFailed() bool
	IsDefault() bool
	IsRequired() bool
	Unbias(cost uint32) uint32
	GetBiasedCost(cost uint32) uint32
}

type TerminatorEvent interface {
	GetTerminator() Terminator
	Accept(visitor EventVisitor)
}

type EventVisitor interface {
	VisitDialFailed(event TerminatorEvent)
	VisitDialSucceeded(event TerminatorEvent)
	VisitCircuitRemoved(event TerminatorEvent)
}

type Stats interface {
	GetCost() uint32
	GetPrecedence() Precedence
}

type Costs interface {
	SetPrecedenceChangeHandler(f func(terminatorId string, precedence Precedence))
	ClearCost(terminatorId string)
	SetPrecedence(terminatorId string, precedence Precedence)
	SetDynamicCost(terminatorId string, weight uint16)
	UpdateDynamicCost(terminatorId string, updateF func(uint16) uint16)
	GetDynamicCost(terminatorId string) uint16
}

type FailureCosts interface {
	Failure(terminatorId string) uint16
	Success(terminatorId string) uint16
	Clear(terminatorId string)
	CreditOverTime(credit uint8, period time.Duration) *time.Ticker
}
