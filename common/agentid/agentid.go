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

// Package agentid defines shared agent application identifier values used in
// the IPC agent protocol to dispatch requests to the correct ziti application.
package agentid

// AppIdAny is the sentinel app id used in agent requests that are not specific
// to any particular ziti application type. Servers accept it alongside their
// own concrete app id.
const AppIdAny byte = 0
