package posture

import (
	"bytes"
	"sync"
	"time"

	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	cmap "github.com/orcaman/concurrent-map/v2"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/proto"
)

type Cache struct {
	apiSessionInstances       cmap.ConcurrentMap[string, *Instance]
	apiSessionInstanceHistory cmap.ConcurrentMap[string, []*InstanceData]
	updateListeners           []func(data *InstanceData)
}

// NewCache creates a new posture data cache for managing device state information
// across API sessions. The cache maintains both current posture data and historical
// snapshots to support policy evaluation and audit requirements.
//
// Returns:
//   - *Cache: A new cache instance ready for storing posture responses
func NewCache() *Cache {
	return &Cache{
		apiSessionInstances:       cmap.New[*Instance](),
		apiSessionInstanceHistory: cmap.New[[]*InstanceData](),
	}
}

func (cache *Cache) onUpdate(data *InstanceData) {
	cache.saveHistory(data)
	cache.emitUpdate(data)
}

func (cache *Cache) saveHistory(data *InstanceData) {
	cache.apiSessionInstanceHistory.Upsert(data.ApiSessionId, nil, func(exist bool, valueInMap []*InstanceData, newValue []*InstanceData) []*InstanceData {
		if len(valueInMap) > 50 {
			valueInMap = valueInMap[1:]
		}
		valueInMap = append(valueInMap, data)
		return valueInMap
	})
}

// AddResponse processes a posture response from an SDK client and updates the cache.
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
func (cache *Cache) AddResponse(identityId, apiSessionId string, response *edge_client_pb.PostureResponse) {

	instance := cache.apiSessionInstances.Upsert(apiSessionId, nil, func(exist bool, valueInMap *Instance, newValue *Instance) *Instance {
		if !exist {
			valueInMap = newInstance()
			valueInMap.ApiSessionId = apiSessionId
			valueInMap.IdentityId = identityId
			valueInMap.updatedListeners = []func(data *InstanceData){cache.onUpdate}
		}

		return valueInMap
	})

	instance.Apply(response)
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

// Instance represents a managed posture data container for a specific API session,
// providing thread-safe access to posture information and change notification
// capabilities for real-time posture policy evaluation.
type Instance struct {
	lock             sync.Mutex
	updatedListeners []func(data *InstanceData)
	InstanceData
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
func (instance *Instance) Apply(response *edge_client_pb.PostureResponse) {
	instance.lock.Lock()
	defer instance.lock.Unlock()

	updated := false

	if os := response.GetOs(); os != nil {
		instance.Os.Os = os
		updated = true
	}

	if domain := response.GetDomain(); domain != nil {
		if instance.Domain == nil || instance.Domain.Name != domain.Name {
			instance.Domain = domain
			updated = true
		}
	}

	if macs := response.GetMacs(); macs != nil {
		if instance.Macs == nil || slices.Equal(macs.Addresses, instance.Macs.Addresses) {
			instance.Macs = macs
			updated = true
		}
	}

	if unlocked := response.GetUnlocked(); unlocked != nil {
		if instance.Unlocked == nil || instance.Unlocked.Time.AsTime().Before(unlocked.GetTime().AsTime()) {
			instance.Unlocked = unlocked
			updated = true
		}
	}

	if woken := response.GetWoken(); woken != nil {
		if instance.Woken == nil || instance.Woken.Time.AsTime().Before(woken.GetTime().AsTime()) {
			instance.Woken = woken
			updated = true
		}
	}

	if processList := response.GetProcessList(); isProcessListDifferent(instance.ProcessList, processList) {
		instance.ProcessList = processList
		updated = true
	}

	if updated {
		instance.emitUpdated()
	}
}

func (instance *Instance) ApplyMfa(mfaAt time.Time) {
	instance.lock.Lock()
	defer instance.lock.Unlock()

	if instance.PassedMfaAt == nil || instance.PassedMfaAt.Before(mfaAt) {
		instance.PassedMfaAt = &mfaAt
		instance.emitUpdated()
	}
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

func isProcessListDifferent(listA *edge_client_pb.PostureResponse_ProcessList, listB *edge_client_pb.PostureResponse_ProcessList) bool {
	procAs := map[string]*edge_client_pb.PostureResponse_Process{}
	for _, proc := range listA.Processes {
		procAs[proc.Path] = proc
	}

	procBs := map[string]*edge_client_pb.PostureResponse_Process{}
	for _, proc := range listB.Processes {
		procBs[proc.Path] = proc
	}

	if len(procAs) != len(procBs) {
		return true
	}

	var checkedPaths []string
	for pathA, procA := range procAs {
		procB, ok := procBs[pathA]

		if !ok {
			return true
		}
		if compareProc(procA, procB) != 0 {
			return true
		}

		checkedPaths = append(checkedPaths, pathA)
	}

	for pathB, procB := range procBs {
		if !stringz.Contains(checkedPaths, pathB) {
			procA, ok := procAs[pathB]

			if !ok {
				return true
			}

			if compareProc(procA, procB) != 0 {
				return true
			}
		}
	}

	return false
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
