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

package runner

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"time"
)

type ErrorHandler func(error, Operation)

type Runner interface {
	AddOperation(Operation) error
	RemoveOperation(Operation) error
	RemovePolicyById(uuid.UUID) (Operation, error)
	Start(closeNotify <-chan struct{}) error
	Stop() error
	IsRunning() bool
}

type LimitedRunner struct {
	minFrequency    time.Duration
	maxFrequency    time.Duration
	tickerEnforcers map[uuid.UUID]*tickerEnforcer
	isRunning       bool
	errHandler      func(error, Operation)
}

func (r *LimitedRunner) IsRunning() bool {
	return r.isRunning
}

type tickerEnforcer struct {
	Enforcer Operation
	Ticker   *time.Ticker
}

func (r *LimitedRunner) AddOperation(e Operation) error {
	d := e.GetFrequency()

	if d < r.minFrequency {
		return fmt.Errorf("error frequency too small, must be larger than %s", r.minFrequency)
	}

	if d > r.maxFrequency {
		return fmt.Errorf("error frequency too large, must be smaller than %s", r.maxFrequency)
	}

	if e.GetId() == uuid.Nil {
		return fmt.Errorf("uuid must not be default/nil")
	}

	r.tickerEnforcers[e.GetId()] = &tickerEnforcer{
		Enforcer: e,
		Ticker:   nil,
	}

	return nil
}

func (r *LimitedRunner) RemoveOperation(e Operation) error {
	_, err := r.RemovePolicyById(e.GetId())

	return err
}

func (r *LimitedRunner) RemovePolicyById(id uuid.UUID) (Operation, error) {
	te, ok := r.tickerEnforcers[id]

	if !ok {
		return nil, fmt.Errorf("not found by id %s", id.String())
	}

	if te.Ticker != nil {
		te.Ticker.Stop()
	}

	delete(r.tickerEnforcers, id)

	return te.Enforcer, nil
}

func (r *LimitedRunner) Start(closeNotify <-chan struct{}) error {
	if r.isRunning {
		return errors.New("already running")
	}

	r.isRunning = true

	for _, te := range r.tickerEnforcers {
		if te.Ticker != nil {
			return errors.New("dirty ticker encountered")
		}

		te.Ticker = time.NewTicker(te.Enforcer.GetFrequency())

		go func(ite *tickerEnforcer) {
			for {
				select {
				case v := <-ite.Ticker.C:
					if v.IsZero() {
						return
					}
					err := ite.Enforcer.Run()

					if err != nil {
						if r.errHandler == nil {
							panic(err)
						}

						r.errHandler(err, ite.Enforcer)
					}
				case <-closeNotify:
					te.Ticker.Stop()
					return
				}
			}
		}(te)
	}
	return nil
}

func (r *LimitedRunner) Stop() error {
	if !r.isRunning {
		return errors.New("not running")
	}

	for eId := range r.tickerEnforcers {
		_, err := r.RemovePolicyById(eId)

		if err != nil {
			return err
		}
	}

	return nil
}

func NewRunner(minF, maxF time.Duration, eh ErrorHandler) (Runner, error) {
	if minF > maxF {
		return nil, fmt.Errorf("min frequency may not be larger than max frequency")
	}

	return &LimitedRunner{
		minFrequency:    minF,
		maxFrequency:    maxF,
		tickerEnforcers: map[uuid.UUID]*tickerEnforcer{},
		isRunning:       false,
		errHandler:      eh,
	}, nil
}
