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

// Package managedconfig contains the router-side machinery for receiving
// controller-managed configuration: a handler interface that subsystems
// implement and a registry that routes Config events from the RDM, picks the
// highest-version a handler supports, and drives Apply/Remove with rollback
// semantics. See doc/design/ctrl-managed-router-config.md for the full design.
//
// Config types follow the convention `<baseType>.v<N>` where N is a positive
// integer (e.g. "router.link.v2"). The registry parses incoming names, keys
// state by base type, and selects the highest version that is both available
// from the controller and supported by the registered handler.
package managedconfig

// ConfigHandler is implemented by router subsystems that accept controller-
// managed configuration. The registry routes Config events to the handler
// whose BaseType matches.
//
// Implementations must be safe to call from a single goroutine; the registry
// serializes Apply / Remove for a given handler.
type ConfigHandler interface {
	// BaseType returns the un-versioned config type family this handler owns,
	// e.g. "router.link". Every config type whose name parses to this base
	// will be routed to this handler.
	BaseType() string

	// SupportedVersions returns the integer versions of BaseType this handler
	// can apply. Order is not significant; the registry picks max(supported ∩
	// available) on every reconcile.
	SupportedVersions() []int

	// Apply is called when the registry has selected an active version for
	// this handler. data is the raw JSON payload from the controller,
	// untransformed. Returning an error triggers rollback (or, if no previous
	// config exists, Remove).
	Apply(version int, data string) error

	// Remove is called when no version of this handler's BaseType is
	// currently available, or when both a fresh Apply and rollback Apply have
	// failed and the registry is forcing the subsystem offline.
	Remove() error
}
