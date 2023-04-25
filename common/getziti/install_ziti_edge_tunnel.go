package getziti

import (
	"fmt"
	"github.com/blang/semver"
	c "github.com/openziti/ziti/ziti/constants"
	"strings"
)

func InstallZitiEdgeTunnel(targetVersion, targetOS, targetArch, binDir string, verbose bool) error {
	var newVersion semver.Version

	if targetVersion != "" {
		newVersion = semver.MustParse(strings.TrimPrefix(targetVersion, "v"))
	} else {
		v, err := GetLatestGitHubReleaseVersion(c.ZITI_EDGE_TUNNEL_GITHUB, verbose)
		if err != nil {
			return err
		}
		newVersion = v
	}

	fmt.Println("Attempting to install '" + c.ZITI_EDGE_TUNNEL + "' version: " + newVersion.String())
	return FindVersionAndInstallGitHubRelease(
		c.ZITI_EDGE_TUNNEL, c.ZITI_EDGE_TUNNEL_GITHUB, targetOS, targetArch, binDir, newVersion.String(), verbose)
}
