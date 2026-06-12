package handler_edge_ctrl

import (
	"testing"
	"time"

	sdkedge_pb "github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/ziti/v2/controller/model"
)

// These tests cover postureResponsesToModel, which turns posture data forwarded from a router (in
// SDK protobuf form) into the controller's internal model objects so the controller can evaluate
// posture and report isPassing. Process checks are the tricky part and the reason most of these
// tests exist. There are two flavors that the controller evaluates differently:
//
//   PROCESS_MULTI checks are matched by the process PATH.
//   legacy single PROCESS checks are matched by the posture-check ID.
//
// The forwarded data only tells us the process path, not which check it belongs to. So the
// conversion stores every process by path (so PROCESS_MULTI works) and, when we know a single
// PROCESS check watches that path, also emits a copy tagged with that check's real ID (so legacy
// PROCESS works). OS, MAC, and Domain are simpler (a plain type-and-field mapping) and are covered
// to catch a field accidentally getting mapped to the wrong place.

func processListResponse(procs ...*sdkedge_pb.PostureResponse_Process) *sdkedge_pb.PostureResponses {
	return &sdkedge_pb.PostureResponses{
		Responses: []*sdkedge_pb.PostureResponse{
			{
				Type: &sdkedge_pb.PostureResponse_ProcessList_{
					ProcessList: &sdkedge_pb.PostureResponse_ProcessList{Processes: procs},
				},
			},
		},
	}
}

// applyAll stores the given responses into a PostureData the same way the controller does at
// runtime, so a test can then inspect what landed where. It only sets up the process fields, so
// pass it process responses. The OS/MAC/Domain test checks the converted responses directly
// instead, because storing those needs a fully built PostureData that this helper does not create.
func applyAll(responses []*model.PostureResponse) *model.PostureData {
	pd := &model.PostureData{
		Processes:      []*model.PostureResponseProcess{},
		ProcessPathMap: map[string]*model.PostureResponseProcess{},
	}
	for _, r := range responses {
		r.Apply(pd)
	}
	return pd
}

// A process we have no specific check ID for (the PROCESS_MULTI case) must still be stored, keyed
// by its path, because that is how PROCESS_MULTI evaluation finds it. Also checks that the reported
// binary hash gets normalized (non-hex stripped, lowercased) as it is stored.
func TestPostureResponsesToModel_ProcessMulti_StoredByPath(t *testing.T) {
	const path = "C:\\Windows\\System32\\notepad.exe"

	responses := postureResponsesToModel(
		processListResponse(&sdkedge_pb.PostureResponse_Process{
			Path:               path,
			IsRunning:          true,
			Hash:               "AB:C1:23", // intentionally NOT hex-clean: Apply must normalize to "abc123"
			SignerFingerprints: []string{"FP1"},
		}),
		time.Now().UTC(),
		nil, // no single-PROCESS checks claim this path
	)

	if len(responses) != 1 {
		t.Fatalf("expected 1 model response, got %d", len(responses))
	}
	if got := responses[0].PostureCheckId; got != "ha-forwarded-process:"+path {
		t.Fatalf("expected synthetic check id for unclaimed path, got %q", got)
	}

	pd := applyAll(responses)
	stored, ok := pd.ProcessPathMap[path]
	if !ok {
		t.Fatalf("expected ProcessPathMap to contain path %q for PROCESS_MULTI evaluation", path)
	}
	if !stored.IsRunning {
		t.Fatalf("expected stored process IsRunning=true")
	}
	// Using an already-clean value would not prove anything. "AB:C1:23" must be hex-cleaned
	// (non-hex stripped, lowercased) to "abc123" as it is stored.
	if stored.BinaryHash != "abc123" {
		t.Fatalf("expected BinaryHash normalized to abc123, got %q", stored.BinaryHash)
	}
}

// A legacy single PROCESS check is matched by check ID, so when we know which checks watch this
// path, the conversion must emit a response tagged with each real check ID, not just the generic
// path-only entry. Two checks on the same path get two responses.
func TestPostureResponsesToModel_SingleProcess_UsesRealCheckIds(t *testing.T) {
	const path = "/usr/bin/myapp"
	pathMap := map[string][]string{
		path: {"check-A", "check-B"},
	}

	responses := postureResponsesToModel(
		processListResponse(&sdkedge_pb.PostureResponse_Process{Path: path, IsRunning: true}),
		time.Now().UTC(),
		pathMap,
	)

	if len(responses) != 2 {
		t.Fatalf("expected 2 model responses (one per claiming check), got %d", len(responses))
	}

	ids := map[string]bool{}
	for _, r := range responses {
		ids[r.PostureCheckId] = true
		if r.TypeId != model.PostureCheckTypeProcess {
			t.Fatalf("expected TypeId PROCESS, got %q", r.TypeId)
		}
	}
	if !ids["check-A"] || !ids["check-B"] {
		t.Fatalf("expected responses for check-A and check-B, got %v", ids)
	}

	// Both checks evaluate by PostureCheckId against pd.Processes.
	pd := applyAll(responses)
	for _, want := range []string{"check-A", "check-B"} {
		found := false
		for _, p := range pd.Processes {
			if p.PostureCheckId == want && p.IsRunning {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected a running process response for %q in pd.Processes", want)
		}
	}
}

// A path watched by a single PROCESS check must ALSO be stored by path, so that a PROCESS_MULTI
// check on the same path keeps working too. One reported process has to satisfy both check styles
// at once.
func TestPostureResponsesToModel_ClaimedPath_AlsoInPathMap(t *testing.T) {
	const path = "/usr/bin/myapp"
	pathMap := map[string][]string{path: {"check-A"}}

	responses := postureResponsesToModel(
		processListResponse(&sdkedge_pb.PostureResponse_Process{Path: path, IsRunning: true}),
		time.Now().UTC(),
		pathMap,
	)

	pd := applyAll(responses)
	if _, ok := pd.ProcessPathMap[path]; !ok {
		t.Fatalf("expected claimed path %q to also be present in ProcessPathMap", path)
	}
}

// OS, MAC, and Domain responses must convert to the matching model type with their fields copied
// across correctly. The process tests do not exercise these branches, so they are covered here.
// Empty input must produce no responses.
func TestPostureResponsesToModel_OsMacDomain_MapFields(t *testing.T) {
	now := time.Now().UTC()

	t.Run("os", func(t *testing.T) {
		out := postureResponsesToModel(&sdkedge_pb.PostureResponses{Responses: []*sdkedge_pb.PostureResponse{
			{Type: &sdkedge_pb.PostureResponse_Os{Os: &sdkedge_pb.PostureResponse_OperatingSystem{
				Type: "Windows", Version: "10.0.26200", Build: "build-1",
			}}},
		}}, now, nil)

		if len(out) != 1 {
			t.Fatalf("expected 1 response, got %d", len(out))
		}
		if out[0].TypeId != model.PostureCheckTypeOs {
			t.Fatalf("expected TypeId OS, got %q", out[0].TypeId)
		}
		os, ok := out[0].SubType.(*model.PostureResponseOs)
		if !ok {
			t.Fatalf("expected *PostureResponseOs subtype, got %T", out[0].SubType)
		}
		if os.Type != "Windows" || os.Version != "10.0.26200" || os.Build != "build-1" {
			t.Fatalf("OS fields mismatched: type=%q version=%q build=%q", os.Type, os.Version, os.Build)
		}
	})

	t.Run("mac", func(t *testing.T) {
		out := postureResponsesToModel(&sdkedge_pb.PostureResponses{Responses: []*sdkedge_pb.PostureResponse{
			{Type: &sdkedge_pb.PostureResponse_Macs_{Macs: &sdkedge_pb.PostureResponse_Macs{
				Addresses: []string{"aa:bb:cc:dd:ee:ff"},
			}}},
		}}, now, nil)

		if len(out) != 1 {
			t.Fatalf("expected 1 MAC response, got %d", len(out))
		}
		if out[0].TypeId != model.PostureCheckTypeMAC {
			t.Fatalf("expected TypeId MAC, got %q", out[0].TypeId)
		}
		mac, ok := out[0].SubType.(*model.PostureResponseMac)
		if !ok {
			t.Fatalf("expected *PostureResponseMac subtype, got %T", out[0].SubType)
		}
		if len(mac.Addresses) != 1 || mac.Addresses[0] != "aa:bb:cc:dd:ee:ff" {
			t.Fatalf("MAC addresses mismatched: %v", mac.Addresses)
		}
	})

	t.Run("domain", func(t *testing.T) {
		out := postureResponsesToModel(&sdkedge_pb.PostureResponses{Responses: []*sdkedge_pb.PostureResponse{
			{Type: &sdkedge_pb.PostureResponse_Domain_{Domain: &sdkedge_pb.PostureResponse_Domain{
				Name: "CORP",
			}}},
		}}, now, nil)

		if len(out) != 1 {
			t.Fatalf("expected 1 DOMAIN response, got %d", len(out))
		}
		if out[0].TypeId != model.PostureCheckTypeDomain {
			t.Fatalf("expected TypeId DOMAIN, got %q", out[0].TypeId)
		}
		dom, ok := out[0].SubType.(*model.PostureResponseDomain)
		if !ok {
			t.Fatalf("expected *PostureResponseDomain subtype, got %T", out[0].SubType)
		}
		if dom.Name != "CORP" {
			t.Fatalf("domain name mismatched: %q", dom.Name)
		}
	})

	t.Run("empty input yields no responses", func(t *testing.T) {
		out := postureResponsesToModel(&sdkedge_pb.PostureResponses{}, now, nil)
		if len(out) != 0 {
			t.Fatalf("expected no responses for empty input, got %d", len(out))
		}
	})
}
