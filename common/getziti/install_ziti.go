package getziti

import (
	"fmt"
	c "github.com/openziti/ziti/ziti/constants"
)

func InstallZiti(targetVersion, targetOS, targetArch, binDir string, verbose bool) error {
	fmt.Println("Attempting to install '" + c.ZITI + "' version: " + targetVersion)
	return FindVersionAndInstallGitHubRelease(
		c.ZITI, c.ZITI, targetOS, targetArch, binDir, targetVersion, verbose)
}

func InstallZrok(targetVersion, targetOS, targetArch, binDir string, verbose bool) error {
	fmt.Println("Attempting to install '" + c.ZROK + "' version: " + targetVersion)
	return FindVersionAndInstallGitHubRelease(
		c.ZROK, c.ZROK, targetOS, targetArch, binDir, targetVersion, verbose)
}
