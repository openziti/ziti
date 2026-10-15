package entities

import (
	"sync"
	"testing"

	"github.com/openziti/ziti/v2/tunnel"
	"github.com/stretchr/testify/require"
)

func Test_HostV1Config_GetAddress_Concurrent(t *testing.T) {
	config := &HostV1Config{
		ForwardAddress:   true,
		AllowedAddresses: []string{"10.0.0.0/8", "*.example.com", "db.internal"},
	}
	options := map[string]interface{}{tunnel.DestinationHostname: "db.internal"}

	const callers = 16
	start := make(chan struct{})
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := config.GetAddress(options)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
}
