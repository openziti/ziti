package events

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/openziti/ziti/v2/controller/event"
	"github.com/stretchr/testify/require"
)

func TestTypePrinting(t *testing.T) {
	v := fmt.Sprintf("%v", reflect.TypeOf((*event.AlertEventHandler)(nil)).Elem())
	require.Equal(t, "event.AlertEventHandler", v)
}
