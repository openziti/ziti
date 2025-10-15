package posture

import (
	"bytes"
	"sync"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	cmap "github.com/orcaman/concurrent-map/v2"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/proto"
)

type Cache struct {
	apiSessionInstances       cmap.ConcurrentMap[string, *Instance]
	apiSessionInstanceHistory cmap.ConcurrentMap[string, []*InstanceData]
	updateListeners           []func(data *InstanceData)
	totpParser                TotpTokenParser
}

// NewCache creates a new posture data cache for managing device state information
// across API sessions. The cache maintains both current posture data and historical
// snapshots to support policy evaluation and audit requirements.
//
// Parameters:
//   - parser: A TOTP token parser implementation, used to verify ToTP tokens on posture response
//
// Returns:
//   - *Cache: A new cache instance ready for storing posture responses
func NewCache(parser TotpTokenParser) *Cache {
	return &Cache{
		apiSessionInstances:       cmap.New[*Instance](),
		apiSessionInstanceHistory: cmap.New[[]*InstanceData](),
		totpParser:                parser,
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
			if instance.Os == nil {
				instance.Os = &edge_client_pb.PostureResponse_Os{}
			}
			instance.Os.Os = os
			updated = true
		}
	} else if domain := response.GetDomain(); domain != nil {
		if instance.Domain == nil || instance.Domain.Name != domain.Name {
			instance.Domain = domain
			updated = true
		}
	} else if macs := response.GetMacs(); macs != nil {
		if instance.Macs == nil || !slices.Equal(macs.Addresses, instance.Macs.Addresses) {
			instance.Macs = macs
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
	} else if processList := response.GetProcessList(); isProcessListDifferent(instance.ProcessList, processList) {
		instance.ProcessList = processList
		updated = true
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

func isProcessListDifferent(listA *edge_client_pb.PostureResponse_ProcessList, listB *edge_client_pb.PostureResponse_ProcessList) bool {

	listAEmpty := listA == nil || len(listA.Processes) == 0
	listBEmpty := listB == nil || len(listB.Processes) == 0

	if listAEmpty && listBEmpty {
		return false
	}

	if listAEmpty != listBEmpty {
		return true
	}

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
