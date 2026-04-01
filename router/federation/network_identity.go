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

package federation

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/identity"
	"gopkg.in/yaml.v3"
)

// NetworkIdentity represents the stored identity and connection details for a single
// federated network. It ties together the network's numeric identifier, its loaded
// TLS identity, controller endpoints, and the on-disk directory where these artifacts
// are persisted.
type NetworkIdentity struct {
	NetworkId uint16
	Id        *identity.TokenId
	Endpoints []string
	Dir       string
}

// NetworksDir returns the base directory used to store per-network identity
// directories, located under the router's configuration directory.
func NetworksDir(routerConfigDir string) string {
	return filepath.Join(routerConfigDir, "networks")
}

// NetworkDir returns the directory path for a specific network within the given
// base directory, using the numeric network ID as the subdirectory name.
func NetworkDir(baseDir string, networkId uint16) string {
	return filepath.Join(baseDir, fmt.Sprintf("%d", networkId))
}

// endpointsConfig is the YAML-serializable structure for the endpoints file stored
// in each network directory.
type endpointsConfig struct {
	Endpoints []string `yaml:"endpoints"`
}

// SaveNetworkIdentity persists a NetworkIdentity's endpoints to disk. It creates the
// network directory if it does not already exist and writes the endpoints.yml file.
// Certificate and key files are not written here; they are written during enrollment.
func SaveNetworkIdentity(ni *NetworkIdentity) error {
	if err := os.MkdirAll(ni.Dir, 0700); err != nil {
		return fmt.Errorf("unable to create network identity directory %s: %w", ni.Dir, err)
	}

	cfg := endpointsConfig{
		Endpoints: ni.Endpoints,
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("unable to marshal endpoints for network %d: %w", ni.NetworkId, err)
	}

	endpointsFile := filepath.Join(ni.Dir, "endpoints.yml")
	if err := os.WriteFile(endpointsFile, data, 0600); err != nil {
		return fmt.Errorf("unable to write endpoints file %s: %w", endpointsFile, err)
	}

	return nil
}

// LoadNetworkIdentity loads a NetworkIdentity from the given directory. It parses the
// network ID from the directory's base name, reads the endpoints.yml file, and loads
// the TLS identity from the standard cert.pem, key.pem, and ca.pem files.
func LoadNetworkIdentity(dir string) (*NetworkIdentity, error) {
	base := filepath.Base(dir)
	parsed, err := strconv.ParseUint(base, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("unable to parse network ID from directory name %q: %w", base, err)
	}
	networkId := uint16(parsed)

	endpointsFile := filepath.Join(dir, "endpoints.yml")
	data, err := os.ReadFile(endpointsFile)
	if err != nil {
		return nil, fmt.Errorf("unable to read endpoints file %s: %w", endpointsFile, err)
	}

	var cfg endpointsConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unable to parse endpoints file %s: %w", endpointsFile, err)
	}

	idConfig := identity.Config{
		Cert: filepath.Join(dir, "cert.pem"),
		Key:  filepath.Join(dir, "key.pem"),
		CA:   filepath.Join(dir, "ca.pem"),
	}

	id, err := identity.LoadIdentity(idConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to load identity for network %d from %s: %w", networkId, dir, err)
	}

	return &NetworkIdentity{
		NetworkId: networkId,
		Id:        identity.NewIdentity(id),
		Endpoints: cfg.Endpoints,
		Dir:       dir,
	}, nil
}

// LoadAllNetworkIdentities reads all subdirectories under baseDir and attempts to
// load a NetworkIdentity from each. Directories that fail to load are skipped with
// a warning log, and the successfully loaded identities are returned.
func LoadAllNetworkIdentities(baseDir string) ([]*NetworkIdentity, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, fmt.Errorf("unable to read networks directory %s: %w", baseDir, err)
	}

	log := pfxlog.Logger()
	var result []*NetworkIdentity

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dir := filepath.Join(baseDir, entry.Name())
		ni, err := LoadNetworkIdentity(dir)
		if err != nil {
			log.WithError(err).Warnf("skipping network identity directory %s", dir)
			continue
		}

		result = append(result, ni)
	}

	return result, nil
}
