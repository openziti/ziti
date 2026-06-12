package posture

import (
	"testing"

	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
)

const testOsType = "Windows"
const testOsVersion = "10.0.26200"

// newOsCheck builds an OsCheck whose single OS entry matches testOsType. osVersions are the
// version range constraints; pass nil/empty to model a check with no version constraint.
func newOsCheck(osVersions []string) *OsCheck {
	return &OsCheck{
		DataState_PostureCheck: &edge_ctrl_pb.DataState_PostureCheck{
			Id:   "test-os-id",
			Name: "my-os-check",
		},
		DataState_PostureCheck_OsList: &edge_ctrl_pb.DataState_PostureCheck_OsList{
			OsList: []*edge_ctrl_pb.DataState_PostureCheck_Os{
				{
					OsType:     testOsType,
					OsVersions: osVersions,
				},
			},
		},
	}
}

func osData(osType, version string) *InstanceData {
	return &InstanceData{
		Os: &edge_client_pb.PostureResponse_Os{
			Os: &edge_client_pb.PostureResponse_OperatingSystem{
				Type:    osType,
				Version: version,
			},
		},
	}
}

// Regression: an OS check whose OS type matches but that has NO version constraint must pass
// for any reported version. Previously the empty-OsVersions loop fell through to a failure with
// "none of the given values were in the valid values, given: [<version>], valid: []".
func TestOsCheck_MatchingTypeNoVersionConstraint_Passes(t *testing.T) {
	check := newOsCheck(nil)

	if err := check.Evaluate(osData(testOsType, testOsVersion)); err != nil {
		t.Fatalf("expected nil (pass) for matching OS type with no version constraint, got: %v", err)
	}
}

// An OS check with no version constraint must pass even when the reported version is not valid
// semver. This guards the empty-OsVersions early return staying ahead of semver.Make: if that
// guard ever moves below the parse, a client reporting e.g. "unknown" regresses to a failure.
func TestOsCheck_NoVersionConstraintNonSemverVersion_Passes(t *testing.T) {
	check := newOsCheck(nil)

	if err := check.Evaluate(osData(testOsType, "unknown")); err != nil {
		t.Fatalf("expected nil (pass) for no version constraint with non-semver version, got: %v", err)
	}
}

func TestOsCheck_MatchingTypeAndVersionInRange_Passes(t *testing.T) {
	check := newOsCheck([]string{">=10.0.0 <=11.0.0"})

	if err := check.Evaluate(osData(testOsType, testOsVersion)); err != nil {
		t.Fatalf("expected nil (pass) for version in range, got: %v", err)
	}
}

func TestOsCheck_MatchingTypeVersionOutOfRange_Fails(t *testing.T) {
	check := newOsCheck([]string{">=11.0.0"})

	if err := check.Evaluate(osData(testOsType, testOsVersion)); err == nil {
		t.Fatalf("expected a CheckError for version below the required range, got nil")
	}
}

func TestOsCheck_TypeMismatch_Fails(t *testing.T) {
	check := newOsCheck(nil)

	if err := check.Evaluate(osData("Linux", "5.15.0")); err == nil {
		t.Fatalf("expected a CheckError for non-matching OS type, got nil")
	}
}

// No OS posture reported at all must fail cleanly, not panic.
func TestOsCheck_NilOsData_DoesNotPanic(t *testing.T) {
	check := newOsCheck(nil)

	if err := check.Evaluate(&InstanceData{}); err == nil {
		t.Fatalf("expected a CheckError when no OS posture data is present, got nil")
	}
}
