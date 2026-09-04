package posture

import (
	"bytes"
	"slices"
	"sync"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/common/posture"
	cmap "github.com/orcaman/concurrent-map/v2"
	"google.golang.org/protobuf/proto"
)

type Cache struct {
	apiSessionInstances cmap.ConcurrentMap[string, *Instance]
	updateListeners     []func(data *InstanceData)
	totpParser          TotpTokenParser
}

// NewCache creates a new posture data cache holding the current posture data reported by each
// API session, for evaluating posture checks. Data is retained until the session's posture
// instance is evicted; see EvictInactive.
//
// Parameters:
//   - parser: A TOTP token parser implementation, used to verify ToTP tokens on posture response
//
// Returns:
//   - *Cache: A new cache instance ready for storing posture responses
func NewCache(parser TotpTokenParser) *Cache {
	return &Cache{
		apiSessionInstances: cmap.New[*Instance](),
		totpParser:          parser,
	}
}

func (cache *Cache) onUpdate(data *InstanceData) {
	cache.emitUpdate(data)
}

// AddResponses processes a posture responses from an SDK client and updates the cache.
// This function either creates a new posture instance or updates an existing one
// with the new device state information. When posture data changes, registered
// listeners are automatically notified to trigger policy re-evaluation.
//
// The response may contain partial updates (e.g., only OS information) and will
// be merged with existing posture data for the session.
//
// Parameters:
//   - identityId: The identity associated with this posture data
//   - apiSessionId: The API session ID for this posture instance
//   - response: The posture response containing device state information
func (cache *Cache) AddResponses(identityId, apiSessionId string, responses *edge_client_pb.PostureResponses) {
	instance := cache.apiSessionInstances.Upsert(apiSessionId, nil, func(exist bool, valueInMap *Instance, newValue *Instance) *Instance {
		if !exist {
			valueInMap = newInstance()
			valueInMap.ApiSessionId = apiSessionId
			valueInMap.IdentityId = identityId
			valueInMap.updatedListeners = []func(data *InstanceData){cache.onUpdate}
		} else {
			// fresh posture activity for the session: it is not a candidate for eviction
			valueInMap.clearDisconnected()
		}

		return valueInMap
	})

	updated := false
	for _, response := range responses.Responses {
		next := instance.Apply(response, cache.totpParser)
		updated = updated || next
	}

	if updated {
		instance.emitUpdated()
	}
}

func (cache *Cache) emitUpdate(data *InstanceData) {
	for _, listener := range cache.updateListeners {
		listener(data)
	}
}

// AddUpdateListener registers a callback function to be notified when posture data changes.
// These listeners are typically used by policy enforcement systems to react to posture
// updates and re-evaluate access decisions for affected connections.
//
// Parameters:
//   - listener: Function to call when posture data is updated
func (cache *Cache) AddUpdateListener(listener func(data *InstanceData)) {
	cache.updateListeners = append(cache.updateListeners, listener)
}

func (cache *Cache) GetInstance(apiSessionId string) *Instance {
	result, _ := cache.apiSessionInstances.Get(apiSessionId)
	return result
}

// MarkConnected records that an api session has a connection again, so its posture data is no
// longer a candidate for eviction. A session with no posture instance is a no-op: there is
// nothing to retain.
//
// Safe to call concurrently with EvictInactive: the two are mutually exclusive, so a session is
// either retained or already evicted, never left half-retained.
func (cache *Cache) MarkConnected(apiSessionId string) {
	// Updating under the map's lock is what makes this exclusive with EvictInactive's removal.
	// Looking the instance up and then updating it leaves a window where an eviction confirms the
	// disconnect time and deletes the entry, and the update lands on a detached instance.
	cache.apiSessionInstances.RemoveCb(apiSessionId, func(_ string, instance *Instance, exists bool) bool {
		if exists && instance != nil {
			instance.clearDisconnected()
		}
		return false // borrow the lock without removing
	})
}

// MarkDisconnected records that an api session's last connection has gone away, starting its
// retention window. A session already recorded as disconnected keeps its earlier time, and a
// session with no posture instance is a no-op.
//
// Safe to call concurrently with EvictInactive, as for MarkConnected.
func (cache *Cache) MarkDisconnected(apiSessionId string) {
	cache.markDisconnectedAt(apiSessionId, time.Now())
}

func (cache *Cache) markDisconnectedAt(apiSessionId string, at time.Time) {
	cache.apiSessionInstances.RemoveCb(apiSessionId, func(_ string, instance *Instance, exists bool) bool {
		if exists && instance != nil {
			instance.setDisconnected(at)
		}
		return false // borrow the lock without removing
	})
}

// ReconcileDisconnected brings cached sessions back in line with isConnected: sessions it reports
// connected have any recorded disconnect cleared, and sessions it reports disconnected have one
// recorded if they carry none. It corrects drift if a MarkConnected or MarkDisconnected was
// missed, and never removes anything, so a missed notification delays eviction rather than
// preventing it. Eviction decisions are made from the recorded times alone, by EvictInactive.
func (cache *Cache) ReconcileDisconnected(isConnected func(apiSessionId string) bool) {
	now := time.Now()

	for entry := range cache.apiSessionInstances.IterBuffered() {
		if isConnected(entry.Key) {
			cache.MarkConnected(entry.Key)
		} else {
			cache.markDisconnectedAt(entry.Key, now)
		}
	}
}

// EvictInactive removes the posture data of api sessions recorded as disconnected for at least
// retention, returning the number removed. A session with no recorded disconnect is connected, or
// still being connected, and is never evicted.
func (cache *Cache) EvictInactive(retention time.Duration) int {
	evictBefore := time.Now().Add(-retention)
	evicted := 0

	for entry := range cache.apiSessionInstances.IterBuffered() {
		judgedDisconnectedAt, disconnected := entry.Val.getDisconnected()
		if !disconnected || !judgedDisconnectedAt.Before(evictBefore) {
			continue
		}

		// The verdict was formed against one disconnect time, so the removal confirms that exact
		// time rather than re-reading the state: a reconnect clears it and a later disconnect
		// replaces it, either of which makes this verdict stale and the removal a no-op.
		removed := cache.apiSessionInstances.RemoveCb(entry.Key, func(_ string, instance *Instance, exists bool) bool {
			return exists && instance != nil && instance.disconnectedAtIs(judgedDisconnectedAt)
		})
		if removed {
			evicted++
		}
	}

	return evicted
}

// Instance represents a managed posture data container for a specific API session,
// providing thread-safe access to posture information and change notification
// capabilities for real-time posture policy evaluation.
type Instance struct {
	lock             sync.Mutex
	updatedListeners []func(data *InstanceData)
	// disconnectedAt is when this instance's api session lost its last connection, or the zero
	// time while it is connected. It starts the eviction retention window. Guarded by lock.
	disconnectedAt time.Time
	InstanceData
}

// setDisconnected records at as when the api session lost its last connection, if no disconnect
// is recorded yet, and returns the effective time. An existing time is never moved, so the
// recorded time is always the earliest known disconnect.
func (instance *Instance) setDisconnected(at time.Time) time.Time {
	instance.lock.Lock()
	defer instance.lock.Unlock()

	if instance.disconnectedAt.IsZero() {
		instance.disconnectedAt = at
	}
	return instance.disconnectedAt
}

// clearDisconnected drops any recorded disconnect, so the instance is not a candidate for
// eviction.
func (instance *Instance) clearDisconnected() {
	instance.lock.Lock()
	defer instance.lock.Unlock()
	instance.disconnectedAt = time.Time{}
}

// getDisconnected returns when the api session lost its last connection, and whether any
// disconnect is recorded at all.
func (instance *Instance) getDisconnected() (time.Time, bool) {
	instance.lock.Lock()
	defer instance.lock.Unlock()
	return instance.disconnectedAt, !instance.disconnectedAt.IsZero()
}

// disconnectedAtIs reports whether the instance still carries exactly the disconnect time a
// caller judged, identifying the incarnation that verdict was formed against. A cleared time (the
// session reconnected) or a different one means the caller's verdict has been overtaken.
func (instance *Instance) disconnectedAtIs(judged time.Time) bool {
	instance.lock.Lock()
	defer instance.lock.Unlock()
	return !instance.disconnectedAt.IsZero() && instance.disconnectedAt.Equal(judged)
}

// InstanceData is separated from Instance in order to make creating copies without copying locks or other sensitive
// instance specific fields.
type InstanceData struct {
	IdentityId   string
	ApiSessionId string
	Time         time.Time
	Os           *edge_client_pb.PostureResponse_Os
	Domain       *edge_client_pb.PostureResponse_Domain
	Macs         *edge_client_pb.PostureResponse_Macs
	Unlocked     *edge_client_pb.PostureResponse_Unlocked
	Woken        *edge_client_pb.PostureResponse_Woken
	ProcessList  *edge_client_pb.PostureResponse_ProcessList
	PassedMfaAt  *time.Time
}

func newInstance() *Instance {
	return &Instance{
		InstanceData: InstanceData{
			Time: time.Now(),
		},
	}
}

type TotpTokenParser interface {
	ParseTotpToken(string) (*common.TotpClaims, error)
}

// Apply updates the posture instance with new device state information from a posture response.
// This function merges the incoming posture data with existing data, only updating fields
// that are present in the response. Changes are detected by comparing new values with
// existing ones, and update listeners are notified only when actual changes occur.
//
// The function handles various types of posture data including OS information, domain
// membership, MAC addresses, process lists, device lock status, and wake events.
//
// Parameters:
//   - response: The posture response containing updated device state information
//   - parser: A TOTP token parser implementation, used to verify ToTP tokens on posture response
//
// Returns:
//   - bool: True if the posture instance was updated, false if no changes were detected
func (instance *Instance) Apply(response *edge_client_pb.PostureResponse, parser TotpTokenParser) bool {
	instance.lock.Lock()
	defer instance.lock.Unlock()

	updated := false

	if os := response.GetOs(); os != nil {
		if isOsDifferent(instance.Os, os) {
			// replace rather than mutate: snapshots share the previous pointer with concurrent readers
			instance.Os = &edge_client_pb.PostureResponse_Os{Os: os}
			updated = true
		}
	} else if domain := response.GetDomain(); domain != nil {
		if instance.Domain == nil || instance.Domain.Name != domain.Name {
			instance.Domain = domain
			updated = true
		}
	} else if macs := response.GetMacs(); macs != nil {
		addresses := make([]string, 0, len(macs.Addresses))
		for _, address := range macs.Addresses {
			addresses = append(addresses, posture.CleanMacAddress(address))
		}

		if instance.Macs == nil || !slices.Equal(addresses, instance.Macs.Addresses) {
			instance.Macs = &edge_client_pb.PostureResponse_Macs{Addresses: addresses}
			updated = true
		}
	} else if unlocked := response.GetUnlocked(); unlocked != nil {
		if instance.Unlocked == nil || instance.Unlocked.Time.AsTime().Before(unlocked.GetTime().AsTime()) {
			instance.Unlocked = unlocked
			updated = true
		}
	} else if woken := response.GetWoken(); woken != nil {
		if instance.Woken == nil || instance.Woken.Time.AsTime().Before(woken.GetTime().AsTime()) {
			instance.Woken = woken
			updated = true
		}
	} else if processList := response.GetProcessList(); processList != nil {
		if instance.mergeProcessList(processList) {
			updated = true
		}
	} else if totpToken := response.GetTotpToken(); totpToken != nil {
		if totpToken.Token == "" {
			pfxlog.Logger().Error("received empty totp token for posture response")
		}

		totpClaims, err := parser.ParseTotpToken(totpToken.Token)

		if err != nil {
			pfxlog.Logger().WithError(err).Error("error parsing totp token")
		} else if totpClaims.IssuedAt == nil {
			pfxlog.Logger().Error("received totp token with no issued at time")
		} else {

			if totpClaims.ApiSessionId == instance.ApiSessionId {
				passedAt := totpClaims.IssuedAt.Time
				if instance.PassedMfaAt == nil || instance.PassedMfaAt.Before(passedAt) {
					instance.PassedMfaAt = &passedAt
					updated = true
				}
			} else {
				pfxlog.Logger().Errorf("received totp token for api session %s, but instance is for %s", totpClaims.ApiSessionId, instance.ApiSessionId)
			}
		}
	} else {
		pfxlog.Logger().Warnf("received unknown posture response type: no fields updated, type: %T", response.GetType())
	}

	return updated
}

// mergeProcessList folds an incoming process-list delta into the stored process state, keyed by
// process path, reporting whether anything changed. SDKs report processes as per-path deltas (a
// message may carry any subset of the watched paths), so entries for paths absent from the
// incoming list are preserved. The stored list is replaced rather than mutated: snapshots share
// the previous pointer with concurrent readers. Caller must hold the instance lock.
func (instance *Instance) mergeProcessList(incoming *edge_client_pb.PostureResponse_ProcessList) bool {
	if len(incoming.Processes) == 0 {
		return false
	}

	existing := instance.ProcessList.GetProcesses()

	merged := make([]*edge_client_pb.PostureResponse_Process, 0, len(existing)+len(incoming.Processes))
	indexByPath := make(map[string]int, len(existing)+len(incoming.Processes))
	for _, proc := range existing {
		indexByPath[proc.Path] = len(merged)
		merged = append(merged, proc)
	}

	changed := false
	for _, proc := range incoming.Processes {
		if idx, ok := indexByPath[proc.Path]; ok {
			if compareProc(merged[idx], proc) != 0 {
				merged[idx] = proc
				changed = true
			}
		} else {
			indexByPath[proc.Path] = len(merged)
			merged = append(merged, proc)
			changed = true
		}
	}

	if changed {
		instance.ProcessList = &edge_client_pb.PostureResponse_ProcessList{Processes: merged}
	}
	return changed
}

func isOsDifferent(old *edge_client_pb.PostureResponse_Os, new *edge_client_pb.PostureResponse_OperatingSystem) bool {
	if old == nil || old.Os == nil {
		return true
	}

	if old.Os.Type != new.Type {
		return true
	}

	if old.Os.Version != new.Version {
		return true
	}

	if old.Os.Build != new.Build {
		return true
	}

	return false
}

func (instance *Instance) emitUpdated() {
	instance.Time = time.Now()

	instanceFieldCopy := instance.InstanceData

	for _, listener := range instance.updatedListeners {
		listener(&instanceFieldCopy)
	}
}

type Checker interface {
	Evaluate(*InstanceData) *CheckError
}

func CtrlCheckToLogic(postureCheck *edge_ctrl_pb.DataState_PostureCheck) Checker {
	switch subCheck := postureCheck.Subtype.(type) {
	case *edge_ctrl_pb.DataState_PostureCheck_Mac_:
		return &MacCheck{
			DataState_PostureCheck:     postureCheck,
			DataState_PostureCheck_Mac: subCheck.Mac,
		}
	case *edge_ctrl_pb.DataState_PostureCheck_OsList_:
		return &OsCheck{
			DataState_PostureCheck:        postureCheck,
			DataState_PostureCheck_OsList: subCheck.OsList,
		}
	case *edge_ctrl_pb.DataState_PostureCheck_Process_:
		return &ProcessCheck{
			DataState_PostureCheck: postureCheck,
			DataState_PostureCheck_ProcessMulti: &edge_ctrl_pb.DataState_PostureCheck_ProcessMulti{
				Semantic: "AllOf",
				Processes: []*edge_ctrl_pb.DataState_PostureCheck_Process{
					{
						OsType:       subCheck.Process.OsType,
						Path:         subCheck.Process.Path,
						Hashes:       subCheck.Process.Hashes,
						Fingerprints: subCheck.Process.Fingerprints,
					},
				},
			},
		}
	case *edge_ctrl_pb.DataState_PostureCheck_ProcessMulti_:
		return &ProcessCheck{
			DataState_PostureCheck:              postureCheck,
			DataState_PostureCheck_ProcessMulti: subCheck.ProcessMulti,
		}
	case *edge_ctrl_pb.DataState_PostureCheck_Domains_:
		return &DomainCheck{
			DataState_PostureCheck:         postureCheck,
			DataState_PostureCheck_Domains: subCheck.Domains,
		}
	case *edge_ctrl_pb.DataState_PostureCheck_Mfa_:
		return &MfaCheck{
			DataState_PostureCheck:     postureCheck,
			DataState_PostureCheck_Mfa: subCheck.Mfa,
		}
	}

	return nil
}

func compareProc(procA, procB *edge_client_pb.PostureResponse_Process) int {
	aBytes, err := proto.Marshal(procA)

	if err != nil {
		return -1
	}

	bBytes, err := proto.Marshal(procB)

	if err != nil {
		return -1
	}

	return bytes.Compare(aBytes, bBytes)
}
