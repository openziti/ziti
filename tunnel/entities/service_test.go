package entities

import (
	"sync"
	"testing"

	"github.com/openziti/ziti/tunnel"
	"github.com/stretchr/testify/require"
)

// Test_HostV1Config_GetAddress_ConcurrentFirstChecks locks in that a forwarded-address check is safe
// from concurrent dials, including the first ones, which parse the allowlist.
func Test_HostV1Config_GetAddress_ConcurrentFirstChecks(t *testing.T) {
	// arrange: a fresh config, asked for its last allowed address, which a partly parsed allowlist
	// would reject
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
		wg.Go(func() {
			<-start
			_, err := config.GetAddress(options)
			errs <- err
		})
	}

	// act
	close(start)
	wg.Wait()
	close(errs)

	// assert
	for err := range errs {
		require.NoError(t, err, "a concurrent check rejected an allowed address")
	}
}
