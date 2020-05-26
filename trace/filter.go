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

package trace

import tracepb "github.com/openziti/foundation/trace/pb"

type Filter interface {
	Accept(event *tracepb.ChannelMessage) bool
}

func NewAllowAllFilter() Filter {
	return &allowAllFilter{}
}

func NewIncludeFilter(includedContentTypes []int32) Filter {
	return &includeFilter{contentTypes: includedContentTypes}
}

func NewExcludeFilter(excludedContentTypes []int32) Filter {
	return &excludeFilter{contentTypes: excludedContentTypes}
}

type allowAllFilter struct{}

func (*allowAllFilter) Accept(event *tracepb.ChannelMessage) bool {
	return true
}

type includeFilter struct {
	contentTypes []int32
}

func (filter *includeFilter) Accept(event *tracepb.ChannelMessage) bool {
	for _, contentType := range filter.contentTypes {
		if event.ContentType == contentType {
			return true
		}
	}
	return false
}

type excludeFilter struct {
	contentTypes []int32
}

func (filter *excludeFilter) Accept(event *tracepb.ChannelMessage) bool {
	for _, contentType := range filter.contentTypes {
		if event.ContentType == contentType {
			return false
		}
	}
	return true
}
