//go:build cli_tests

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
package cli_tests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func (s *cliTestState) fabricTests(t *testing.T) {
	test(t, `fabric validate router-data-model`, s.fabricValidateRDM)
	//test(t, `fabric inspect config`, s.fabricInspectConfig)
	test(t, "fabric list links", s.fabricListLinks)
	test(t, "fabric validate router-links", s.fabricValidateRouterLinks)
	test(t, "fabric stream events", s.fabricStreamEvents)
}

func (s *cliTestState) fabricInspectConfig(t *testing.T) {
	out, err := s.runCLI(`fabric inspect config`)
	fmt.Println(out)
	require.NoError(t, err, out)
	require.Contains(t, out, "ctrl:")
	t.Log(out)
}

func (s *cliTestState) fabricValidateRDM(t *testing.T) {
	out, err := s.runCLI(`fabric validate router-data-model`)
	require.NoError(t, err, out)
	require.Contains(t, out, "started validation of")
	t.Log(out)
}

func (s *cliTestState) fabricListLinks(t *testing.T) {
	out, err := s.runCLI(`fabric list links`)
	require.NoError(t, err, out)
	require.Contains(t, out, "results: none")
	t.Log(out)
}

func (s *cliTestState) fabricValidateRouterLinks(t *testing.T) {
	out, err := s.runCLI(`fabric validate router-links --include-valid-routers`)
	require.NoError(t, err, out)
	require.Contains(t, out, "routerName: router-quickstart")
	t.Log(out)
}

func (s *cliTestState) fabricStreamEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var out string
	var err error
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		out, err = s.runCLIWithContext(ctx, `fabric stream events --api-sessions --verbose`)
		wg.Done()
	}()

	go func() {
		time.Sleep(1 * time.Second)      // a small wait to make sure the stream events command connects
		s.testCorrectPasswordSucceeds(t) // logging in forces an event to fire
		cancel()
	}()

	wg.Wait()
	require.NoError(t, err, out)
	require.Contains(t, out, "event streaming started: success")
	require.Contains(t, out, `"apiSession","event_src_id"`)
	t.Log(out)
}
