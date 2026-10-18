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

// RouterGossipLinkEntry describes a single entry in the router's local gossip store.
type RouterGossipLinkEntry struct {
	Key        string `json:"key"`
	LinkId     string `json:"linkId"`
	Iteration  uint32 `json:"iteration"`
	Owner      string `json:"owner"`
	Version    uint64 `json:"version"`
	DestRouter string `json:"destRouter,omitempty"`
	Tombstone  bool   `json:"tombstone"`
	Epoch      string `json:"epoch,omitempty"`
}

// RouterGossipLinksInspect is the JSON structure returned by the router
// "gossip-links" inspect key.
type RouterGossipLinksInspect struct {
	Entries []RouterGossipLinkEntry `json:"entries"`
	Count   int                     `json:"count"`
}

// RouterGossipStoreInspect is the JSON structure returned by the router
// "gossip-store" inspect key.
type RouterGossipStoreInspect struct {
	RouterId     string            `json:"routerId"`
	Epoch        string            `json:"epoch"`
	LamportClock uint64            `json:"lamportClock"`
	EntryCount   int64             `json:"entryCount"`  // incrementally-tracked count advertised to controllers via the canary
	LiveEntries  int               `json:"liveEntries"` // live entries counted directly from the store
	Tombstones   int               `json:"tombstones"`  // tombstone entries counted directly from the store
	EntryHash    uint64            `json:"entryHash"`
	MaxSent      map[string]uint64 `json:"maxSent,omitempty"`
}
