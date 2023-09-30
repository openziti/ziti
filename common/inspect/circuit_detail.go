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

package inspect

type CircuitInspectDetail struct {
	CircuitId         string                        `json:"circuitId"`
	Forwards          map[string]string             `json:"forwards"`
	XgressDetails     map[string]*XgressDetail      `json:"xgressDetails"`
	LinkDetails       map[string]*LinkInspectDetail `json:"linkDetails"`
	includeGoroutines bool
}

func (self *CircuitInspectDetail) SetIncludeGoroutines(includeGoroutines bool) {
	self.includeGoroutines = includeGoroutines
}

func (self *CircuitInspectDetail) IncludeGoroutines() bool {
	return self.includeGoroutines
}

type XgressDetail struct {
	Address               string                  `json:"address"`
	Originator            string                  `json:"originator"`
	TimeSinceLastLinkRx   string                  `json:"timeSinceLastLinkRx"`
	SendBufferDetail      *XgressSendBufferDetail `json:"sendBufferDetail"`
	RecvBufferDetail      *XgressRecvBufferDetail `json:"recvBufferDetail"`
	XgressPointer         string                  `json:"xgressPointer"`
	LinkSendBufferPointer string                  `json:"linkSendBufferPointer"`
	Goroutines            []string                `json:"goroutines"`
	Sequence              int32                   `json:"sequence"`
	Flags                 string                  `json:"flags"`
}

type XgressSendBufferDetail struct {
	WindowSize            uint32  `json:"windowSize"`
	LinkSendBufferSize    uint32  `json:"linkSendBufferSize"`
	LinkRecvBufferSize    uint32  `json:"linkRecvBufferSize"`
	Accumulator           uint32  `json:"accumulator"`
	SuccessfulAcks        uint32  `json:"successfulAcks"`
	DuplicateAcks         uint32  `json:"duplicateAcks"`
	Retransmits           uint32  `json:"retransmits"`
	Closed                bool    `json:"closed"`
	BlockedByLocalWindow  bool    `json:"blockedByLocalWindow"`
	BlockedByRemoteWindow bool    `json:"blockedByRemoteWindow"`
	RetxScale             float64 `json:"retxScale"`
	RetxThreshold         uint32  `json:"retxThreshold"`
	TimeSinceLastRetx     string  `json:"timeSinceLastRetx"`
	CloseWhenEmpty        bool    `json:"closeWhenEmpty"`
	AcquiredSafely        bool    `json:"acquiredSafely"`
}

type XgressRecvBufferDetail struct {
	Size           uint32 `json:"size"`
	PayloadCount   uint32 `json:"payloadCount"`
	LastSizeSent   uint32 `json:"lastSizeSent"`
	Sequence       int32  `json:"sequence"`
	MaxSequence    int32  `json:"maxSequence"`
	NextPayload    string `json:"nextPayload"`
	AcquiredSafely bool   `json:"acquiredSafely"`
}
