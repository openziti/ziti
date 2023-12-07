package db

import (
	"fmt"
	"testing"
)

func Test_GenerateSymbols(t *testing.T) {
	t.SkipNow()
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	for k, store := range ctx.stores.storeMap {
		fmt.Printf("%v (%v)\n", store.GetEntityType(), k)
		for _, symbol := range store.GetPublicSymbols() {
			fmt.Printf("\t%v\n", symbol)
		}
	}
}
