package posture

import (
	"testing"

	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/stretchr/testify/require"
)

func newOsCheck(oses ...*edge_ctrl_pb.DataState_PostureCheck_Os) *OsCheck {
	return &OsCheck{
		DataState_PostureCheck: &edge_ctrl_pb.DataState_PostureCheck{Id: "os-check", Name: "os-check"},
		DataState_PostureCheck_OsList: &edge_ctrl_pb.DataState_PostureCheck_OsList{
			OsList: oses,
		},
	}
}

func osState(osType, version string) *InstanceData {
	return &InstanceData{
		Os: &edge_client_pb.PostureResponse_Os{
			Os: &edge_client_pb.PostureResponse_OperatingSystem{
				Type:    osType,
				Version: version,
			},
		},
	}
}

// Test_OsCheck_NoVersions_TypeMatches locks in that an OS type declared with no versions passes on
// a type match, matching the controller, rather than falling through to the version failure.
func Test_OsCheck_NoVersions_TypeMatches(t *testing.T) {
	check := newOsCheck(&edge_ctrl_pb.DataState_PostureCheck_Os{OsType: "Windows"})

	require.Nil(t, check.Evaluate(osState("Windows", "10.0.19045")), "no declared versions: the type match alone passes")
}

// Test_OsCheck_NoVersions_UnparsableReportedVersion locks in that the reported version is only
// parsed when a version constraint exists. sdk-golang reports "unknown" on some Linux distros.
func Test_OsCheck_NoVersions_UnparsableReportedVersion(t *testing.T) {
	check := newOsCheck(&edge_ctrl_pb.DataState_PostureCheck_Os{OsType: "Linux"})

	require.Nil(t, check.Evaluate(osState("Linux", "unknown")), "no declared versions: the reported version is never parsed")
}

// Test_OsCheck_VersionInRange locks in that a reported version inside a declared range passes.
func Test_OsCheck_VersionInRange(t *testing.T) {
	check := newOsCheck(&edge_ctrl_pb.DataState_PostureCheck_Os{
		OsType:     "Windows",
		OsVersions: []string{">=10.0.0"},
	})

	require.Nil(t, check.Evaluate(osState("Windows", "10.0.19045")))
}

// Test_OsCheck_VersionOutOfRange locks in that a reported version outside every declared range fails.
func Test_OsCheck_VersionOutOfRange(t *testing.T) {
	check := newOsCheck(&edge_ctrl_pb.DataState_PostureCheck_Os{
		OsType:     "Windows",
		OsVersions: []string{">=10.0.0"},
	})

	require.NotNil(t, check.Evaluate(osState("Windows", "6.1.7601")))
}

// Test_OsCheck_TypeMismatch locks in that an unlisted OS type fails regardless of version.
func Test_OsCheck_TypeMismatch(t *testing.T) {
	check := newOsCheck(&edge_ctrl_pb.DataState_PostureCheck_Os{
		OsType:     "Windows",
		OsVersions: []string{">=10.0.0"},
	})

	require.NotNil(t, check.Evaluate(osState("Linux", "10.0.19045")))
}

// Test_OsCheck_UnparsableRangeSkipped locks in that an unparsable declared range is skipped rather
// than failing the whole check, leaving the remaining ranges to decide the outcome.
func Test_OsCheck_UnparsableRangeSkipped(t *testing.T) {
	check := newOsCheck(&edge_ctrl_pb.DataState_PostureCheck_Os{
		OsType:     "Windows",
		OsVersions: []string{"not a range", ">=10.0.0"},
	})

	require.Nil(t, check.Evaluate(osState("Windows", "10.0.19045")), "the parsable range still matches")
}

// Test_OsCheck_AllRangesUnparsable locks in that a check whose declared ranges all fail to parse
// fails closed: versions were required, and none of them can be satisfied.
func Test_OsCheck_AllRangesUnparsable(t *testing.T) {
	check := newOsCheck(&edge_ctrl_pb.DataState_PostureCheck_Os{
		OsType:     "Windows",
		OsVersions: []string{"not a range"},
	})

	require.NotNil(t, check.Evaluate(osState("Windows", "10.0.19045")))
}

// Test_OsCheck_FirstDeclarationWins locks in that a duplicated OS type is decided by the first
// declaration, so a later redeclaration cannot widen or narrow the match.
func Test_OsCheck_FirstDeclarationWins(t *testing.T) {
	check := newOsCheck(
		&edge_ctrl_pb.DataState_PostureCheck_Os{OsType: "Windows", OsVersions: []string{">=10.0.0"}},
		&edge_ctrl_pb.DataState_PostureCheck_Os{OsType: "Windows", OsVersions: []string{"<7.0.0"}},
	)

	require.Nil(t, check.Evaluate(osState("Windows", "10.0.19045")), "the first declaration decides the match")
}
