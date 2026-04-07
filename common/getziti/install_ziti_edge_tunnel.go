package getziti

import (
	"fmt"
	"strings"

	"github.com/blang/semver"
	c "github.com/openziti/ziti/v2/ziti/constants"
)

func InstallZitiEdgeTunnel(targetVersion, targetOS, targetArch, binDir string, verbose bool) error {
	var newVersion semver.Version

	if targetVersion != "" {
		newVersion = semver.MustParse(strings.TrimPrefix(targetVersion, "v"))
	} else {
		v, err := GetLatestGitHubReleaseVersion(c.OpenZitiOrg, c.ZitiEdgeTunnelGithub, verbose)
		if err != nil {
			return err
		}
		newVersion = v
	}

	fmt.Println("Attempting to install '" + c.ZitiEdgeTunnel + "' version: " + newVersion.String())
	return FindVersionAndInstallGitHubRelease(
		c.OpenZitiOrg, c.ZitiEdgeTunnel, c.ZitiEdgeTunnelGithub, targetOS, targetArch, binDir, newVersion.String(), verbose)
}
