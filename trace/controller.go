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

package trace

import (
	"fmt"
	"github.com/openziti/channel/trace/pb"
	"regexp"
	"strings"
)

type SourceType int

const (
	SourceTypePipe SourceType = iota
)

type ToggleVerbosity int

const (
	ToggleVerbosityNone ToggleVerbosity = iota
	ToggleVerbosityMatches
	ToggleVerbosityMisses
	ToggleVerbosityAll
)

func (val ToggleVerbosity) Show(matched bool) bool {
	if matched {
		return val == ToggleVerbosityMatches || val == ToggleVerbosityAll
	}
	return val == ToggleVerbosityMisses || val == ToggleVerbosityAll
}

func GetVerbosity(verbosity trace_pb.TraceToggleVerbosity) ToggleVerbosity {
	switch verbosity {
	case trace_pb.TraceToggleVerbosity_ReportNone:
		return ToggleVerbosityNone
	case trace_pb.TraceToggleVerbosity_ReportMatches:
		return ToggleVerbosityMatches
	case trace_pb.TraceToggleVerbosity_ReportMisses:
		return ToggleVerbosityMisses
	case trace_pb.TraceToggleVerbosity_ReportAll:
		return ToggleVerbosityAll
	}
	return ToggleVerbosityNone
}

type SourceMatcher interface {
	Matches(source string) bool
}

type PipeToggleMatchers struct {
	AppMatcher  SourceMatcher
	PipeMatcher SourceMatcher
}

func NewPipeToggleMatchers(request *trace_pb.TogglePipeTracesRequest) (*PipeToggleMatchers, *ToggleResult) {
	result := &ToggleResult{Success: true, Message: &strings.Builder{}}

	appRegex, err := regexp.Compile(request.AppRegex)
	if err != nil {
		result.Success = false
		errMsg := fmt.Sprintf("Failed to parse app id regex '%v' with error: %v\n", request.AppRegex, err)
		result.Message.WriteString(errMsg)
	}

	pipeRegex, err := regexp.Compile(request.PipeRegex)
	if err != nil {
		result.Success = false
		errMsg := fmt.Sprintf("Failed to parse pipe id regex '%v' with error: %v\n", request.PipeRegex, err)
		result.Message.WriteString(errMsg)
	}

	appMatcher := NewSourceMatcher(appRegex)
	pipeMatcher := NewSourceMatcher(pipeRegex)

	return &PipeToggleMatchers{appMatcher, pipeMatcher}, result
}

type ToggleResult struct {
	Success bool
	Message *strings.Builder
}

func (result *ToggleResult) Append(msg string) {
	result.Message.WriteString(msg)
	result.Message.WriteString("\n")
}

type ToggleApplyResult interface {
	IsMatched() bool
	GetMessage() string
	Append(result *ToggleResult, verbosity ToggleVerbosity)
}

type ToggleApplyResultImpl struct {
	Matched bool
	Message string
}

func (applyResult *ToggleApplyResultImpl) IsMatched() bool {
	return applyResult.Matched
}

func (applyResult *ToggleApplyResultImpl) GetMessage() string {
	return applyResult.Message
}

func (applyResult *ToggleApplyResultImpl) Append(result *ToggleResult, verbosity ToggleVerbosity) {
	if verbosity.Show(applyResult.Matched) {
		result.Append(applyResult.Message)
	}
}

type sourceRegexpMatcher struct {
	regex *regexp.Regexp
}

func (matcher *sourceRegexpMatcher) Matches(source string) bool {
	return matcher.regex.Match([]byte(source))
}

func NewSourceMatcher(regex *regexp.Regexp) SourceMatcher {
	return &sourceRegexpMatcher{regex}
}

type Source interface {
	EnableTracing(sourceType SourceType, matcher SourceMatcher, resultChan chan<- ToggleApplyResult)
	DisableTracing(sourceType SourceType, matcher SourceMatcher, resultChan chan<- ToggleApplyResult)
}

type Controller interface {
	EnableTracing(sourceType SourceType, matcher SourceMatcher, resultChan chan<- ToggleApplyResult)
	DisableTracing(sourceType SourceType, matcher SourceMatcher, resultChan chan<- ToggleApplyResult)
	AddSource(source Source)
	RemoveSource(source Source)
}

func NewController(closeNotify <-chan struct{}) Controller {
	controller := &controllerImpl{
		events:      make(chan interface{}, 1),
		sources:     make(map[Source]Source),
		closeNotify: closeNotify,
	}
	go controller.run()
	return controller
}

type enableSourcesEvent struct {
	sourceType SourceType
	matcher    SourceMatcher
	resultChan chan<- ToggleApplyResult
}

type disableSourcesEvent struct {
	sourceType SourceType
	matcher    SourceMatcher
	resultChan chan<- ToggleApplyResult
}

type sourceAddedEvent struct {
	source Source
}

type sourceRemovedEvent struct {
	source Source
}

type controllerImpl struct {
	events      chan interface{}
	sources     map[Source]Source
	closeNotify <-chan struct{}
}

func (controller *controllerImpl) EnableTracing(sourceType SourceType, matcher SourceMatcher, resultChan chan<- ToggleApplyResult) {
	select {
	case controller.events <- &enableSourcesEvent{sourceType, matcher, resultChan}:
	case <-controller.closeNotify:
		return
	}
}

func (controller *controllerImpl) DisableTracing(sourceType SourceType, matcher SourceMatcher, resultChan chan<- ToggleApplyResult) {
	select {
	case controller.events <- &disableSourcesEvent{sourceType, matcher, resultChan}:
	case <-controller.closeNotify:
		return
	}
}

func (controller *controllerImpl) AddSource(source Source) {
	select {
	case controller.events <- &sourceAddedEvent{source}:
	case <-controller.closeNotify:
		return
	}
}

func (controller *controllerImpl) RemoveSource(source Source) {
	select {
	case controller.events <- &sourceRemovedEvent{source}:
	case <-controller.closeNotify:
		return
	}
}

func (controller *controllerImpl) run() {
	for {
		select {
		case <-controller.closeNotify:
			return
		case i := <-controller.events:
			switch event := i.(type) {
			case *enableSourcesEvent:
				for handler := range controller.sources {
					handler.EnableTracing(event.sourceType, event.matcher, event.resultChan)
				}
				close(event.resultChan)
			case *disableSourcesEvent:
				for handler := range controller.sources {
					handler.DisableTracing(event.sourceType, event.matcher, event.resultChan)
				}
				close(event.resultChan)
			case *sourceAddedEvent:
				controller.sources[event.source] = event.source
			case *sourceRemovedEvent:
				delete(controller.sources, event.source)
			}
		}
	}
}
