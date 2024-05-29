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

package xt

import (
	"fmt"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/common/logcontext"
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
	GetHostId() string
	GetSourceCtrl() string
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

type CreateCircuitParams interface {
	GetServiceId() string
	GetClientId() *identity.TokenId
	GetLogContext() logcontext.Context
}

type Strategy interface {
	Select(param CreateCircuitParams, terminators []CostedTerminator) (CostedTerminator, PeerData, error)
	HandleTerminatorChange(event StrategyChangeEvent) error
	NotifyEvent(event TerminatorEvent)
}

type Precedence interface {
	fmt.Stringer
	GetBaseCost() uint32
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

type Costs interface {
	ClearCost(terminatorId string)
	SetDynamicCost(terminatorId string, c Cost)
	GetDynamicCost(terminatorId string) uint16
	GetCost(terminatorId string) Cost
	IterCosts(func(terminatorId string, cost Cost))
}
