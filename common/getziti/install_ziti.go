package getziti

import (
	"fmt"
	"github.com/blang/semver"
	c "github.com/openziti/ziti/ziti/constants"
	"strings"
)

func InstallZiti(targetVersion, targetOS, targetArch, binDir string, verbose bool) error {
	var newVersion semver.Version

	if targetVersion != "" {
		newVersion = semver.MustParse(strings.TrimPrefix(targetVersion, "v"))
	} else {
		v, err := GetLatestGitHubReleaseVersion(c.ZITI, verbose)
		if err != nil {
			return err
		}
		newVersion = v
	}

	fmt.Println("Attempting to install '" + c.ZITI + "' version: v" + newVersion.String())
	return FindVersionAndInstallGitHubRelease(
		c.ZITI, c.ZITI, targetOS, targetArch, binDir, "v"+newVersion.String(), verbose)
}
