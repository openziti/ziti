//go:build quickstart && manual

package edge

import (
	"testing"
)

/*
This is a manually run test that will, with the default values except the admin password, confirm the docker-compose
ziti network is running as expected. The values can be edited to confirm other ziti networks but will require an http
server on the back end.
*/
func TestEdgeQuickstartManual(t *testing.T) {
	performQuickstartTest(t)
}
