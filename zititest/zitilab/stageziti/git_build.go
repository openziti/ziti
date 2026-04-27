/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package stageziti

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// defaultZitiRepo is the upstream source for from-git builds. Override at
// runtime with the ZITI_TRAFFIC_TEST_REPO_URL env var; a local filesystem path
// works (git clone accepts paths), which lets a developer build from their
// working tree by committing to a local branch and passing the SHA, branch
// name, or tag as the version.
const defaultZitiRepo = "https://github.com/openziti/ziti.git"

// gitBuildSpec describes how to build a single binary from a checkout of the
// ziti repo. workingDir is relative to the repo root; pkg is relative to
// workingDir; tags are additional build tags (the canonical CI invocations
// require these for some packages).
type gitBuildSpec struct {
	binaryName string
	workingDir string
	pkg        string
	tags       []string
}

var (
	zitiTrafficTestBuildSpec = gitBuildSpec{
		binaryName: "ziti-traffic-test",
		workingDir: "zititest", // separate Go module
		pkg:        "./ziti-traffic-test",
		tags:       []string{"all"}, // matches `go install -tags all` in CI
	}
	zitiBuildSpec = gitBuildSpec{
		binaryName: "ziti",
		workingDir: "", // root module
		pkg:        "./ziti",
		tags:       nil,
	}
)

// buildZitiTrafficTestFromGit clones the ziti repo at ref and builds
// zititest/ziti-traffic-test for linux/amd64, writing the binary to outputPath.
func buildZitiTrafficTestFromGit(ref, outputPath string) error {
	return buildFromGit(ref, outputPath, zitiTrafficTestBuildSpec)
}

// buildZitiFromGit clones the ziti repo at ref and builds the ziti multi-tool
// binary for linux/amd64, writing it to outputPath.
func buildZitiFromGit(ref, outputPath string) error {
	return buildFromGit(ref, outputPath, zitiBuildSpec)
}

// buildFromGit clones, checks out, and builds a binary from the openziti/ziti
// repo per spec. Caller is responsible for caching / DoOnce gating; this
// function unconditionally rebuilds when invoked.
//
// Cross-compile target is fixed at linux/amd64 (fablab nodes are AWS Linux);
// CGO is disabled to keep the binary statically linked.
func buildFromGit(ref, outputPath string, spec gitBuildSpec) error {
	if ref == "" {
		return errors.Errorf("buildFromGit requires a non-empty git ref (binary=%s)", spec.binaryName)
	}

	repoURL := os.Getenv("ZITI_TRAFFIC_TEST_REPO_URL")
	if repoURL == "" {
		repoURL = defaultZitiRepo
	}

	tmpDir, err := os.MkdirTemp("", spec.binaryName+"-build-*")
	if err != nil {
		return errors.Wrap(err, "creating build temp dir")
	}
	defer func() {
		if rmErr := os.RemoveAll(tmpDir); rmErr != nil {
			logrus.WithError(rmErr).Warnf("failed to remove build temp dir %s", tmpDir)
		}
	}()

	logrus.Infof("%s: cloning %s -> %s", spec.binaryName, repoURL, tmpDir)
	if err := runStreamed(exec.Command("git", "clone", repoURL, tmpDir)); err != nil {
		return errors.Wrapf(err, "git clone %s", repoURL)
	}

	logrus.Infof("%s: checking out %s", spec.binaryName, ref)
	if err := runStreamed(exec.Command("git", "-C", tmpDir, "checkout", ref)); err != nil {
		return errors.Wrapf(err, "git checkout %s", ref)
	}

	logrus.Infof("%s: building -> %s", spec.binaryName, outputPath)
	args := []string{"build", "-trimpath"}
	if len(spec.tags) > 0 {
		args = append(args, "-tags", strings.Join(spec.tags, ","))
	}
	args = append(args, "-o", outputPath, spec.pkg)
	build := exec.Command("go", args...)
	build.Dir = filepath.Join(tmpDir, spec.workingDir)
	build.Env = append(os.Environ(),
		"GOOS=linux",
		"GOARCH=amd64",
		"CGO_ENABLED=0",
	)
	if err := runStreamed(build); err != nil {
		return errors.Wrapf(err, "go build %s", spec.binaryName)
	}
	return nil
}

func runStreamed(cmd *exec.Cmd) error {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
