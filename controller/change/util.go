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

package change

import (
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/ctrlchan"
)

func NewCtrlChannelChange(routerId, routerName, method string, ctrlCh ctrlchan.CtrlChannel) *Context {
	var ch channel.Channel
	if ctrlCh != nil {
		ch = ctrlCh.GetChannel()
	}
	return NewControlChannelChange(routerId, routerName, method, ch)
}

func NewControlChannelChange(routerId, routerName, method string, ch channel.Channel) *Context {
	result := New().
		SetChangeAuthorId(routerId).
		SetChangeAuthorName(routerName).
		SetChangeAuthorType(AuthorTypeRouter).
		SetSourceType(SourceTypeControlChannel).
		SetSourceMethod(method)

	if ch != nil {
		if underlay := ch.Underlay(); underlay != nil {
			result.SetSourceLocal(underlay.GetLocalAddr().String()).
				SetSourceRemote(underlay.GetRemoteAddr().String())
		}
	}

	return result
}
