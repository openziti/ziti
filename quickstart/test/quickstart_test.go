//go:build manual

package test

import (
	"fmt"
	"testing"
)

func TestSimpleWebService(t *testing.T) {
	fmt.Println()
	fmt.Println("----------------------------------------------------------------")
	fmt.Println("  test has moved to ./ziti/cmd/edge/quickstart_manual_test.go.  ")
	fmt.Println("----------------------------------------------------------------")
	fmt.Println()
	fmt.Println("Run this command instead:")
	panic("go test -tags \"quickstart manual\" ./ziti/cmd/edge/...")
}
