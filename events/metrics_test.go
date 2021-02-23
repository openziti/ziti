package events

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func Test_ExtractId(t *testing.T) {
	name := "ctrl.3tOOkKfDn.tx.bytesrate"

	req := require.New(t)
	name, entityId, _ := ExtractId(name, "ctrl.", 2)
	req.Equal(name, "ctrl.tx.bytesrate")
	req.Equal(entityId, "3tOOkKfDn")

	name = "ctrl.3tO.kKfDn.tx.bytesrate"
	name, entityId, _ = ExtractId(name, "ctrl.", 2)
	req.Equal(name, "ctrl.tx.bytesrate")
	req.Equal(entityId, "3tO.kKfDn")

	name = "ctrl.3tO.kK.Dn.tx.bytesrate"
	name, entityId, _ = ExtractId(name, "ctrl.", 2)
	req.Equal(name, "ctrl.tx.bytesrate")
	req.Equal(entityId, "3tO.kK.Dn")

	name = "ctrl..tO.kK.Dn.tx.bytesrate"
	name, entityId, _ = ExtractId(name, "ctrl.", 2)
	req.Equal(name, "ctrl.tx.bytesrate")
	req.Equal(entityId, ".tO.kK.Dn")

	name = "ctrl..tO.kK.D..tx.bytesrate"
	name, entityId, _ = ExtractId(name, "ctrl.", 2)
	req.Equal(name, "ctrl.tx.bytesrate")
	req.Equal(entityId, ".tO.kK.D.")
}
