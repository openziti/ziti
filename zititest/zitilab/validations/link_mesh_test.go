package validations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMeshGaps(t *testing.T) {
	routers := map[string]string{
		"router-east-1": "router-east-1 (v1.5.4)",
		"router-east-2": "router-east-2 (v1.5.4)",
		"router-west":   "router-west",
	}
	connected := func(id, src, dst string) meshLink {
		return meshLink{id: id, src: src, dst: dst, state: "Connected", usable: true}
	}
	fullMesh := []meshLink{
		connected("l1", "router-east-1", "router-east-2"),
		connected("l2", "router-west", "router-east-1"),
		connected("l3", "router-east-2", "router-west"),
	}

	tests := []struct {
		name    string
		links   []meshLink
		gaps    []string
		linkIds []string
	}{
		{
			name:    "full mesh, either dial direction",
			links:   fullMesh,
			linkIds: []string{"l1", "l2", "l3"},
		},
		{
			name: "a pair without a link",
			links: []meshLink{
				connected("l1", "router-east-1", "router-east-2"),
				connected("l3", "router-east-2", "router-west"),
			},
			gaps:    []string{"router-east-1 (v1.5.4) <-> router-west: no usable link"},
			linkIds: []string{"l1", "l3"},
		},
		{
			name:  "a router with no links",
			links: []meshLink{connected("l1", "router-east-1", "router-east-2")},
			gaps: []string{
				"router-east-1 (v1.5.4) <-> router-west: no usable link",
				"router-east-2 (v1.5.4) <-> router-west: no usable link",
			},
			linkIds: []string{"l1"},
		},
		{
			name:  "no links",
			links: nil,
			gaps: []string{
				"router-east-1 (v1.5.4) <-> router-east-2 (v1.5.4): no usable link",
				"router-east-1 (v1.5.4) <-> router-west: no usable link",
				"router-east-2 (v1.5.4) <-> router-west: no usable link",
			},
		},
		{
			name:    "both directions dialed and not deduplicated",
			links:   append(fullMesh, connected("l4", "router-east-1", "router-west")),
			gaps:    []string{"router-east-1 (v1.5.4) <-> router-west: 2 usable links (l2 router-west -> router-east-1, l4 router-east-1 -> router-west)"},
			linkIds: []string{"l1", "l2", "l3", "l4"},
		},
		{
			name: "the losing dial marked duplicate",
			links: append(fullMesh,
				meshLink{id: "l4", src: "router-east-1", dst: "router-west", state: "Duplicate"}),
			linkIds: []string{"l1", "l2", "l3"},
		},
		{
			name: "a failed or down link is not usable",
			links: []meshLink{
				connected("l1", "router-east-1", "router-east-2"),
				{id: "l2", src: "router-east-1", dst: "router-west", state: "Failed"},
				{id: "l3", src: "router-east-2", dst: "router-west", state: "Connected", down: true},
			},
			gaps: []string{
				"router-east-1 (v1.5.4) <-> router-west: no usable link, plus l2 router-east-1 -> router-west Failed down=false",
				"router-east-2 (v1.5.4) <-> router-west: no usable link, plus l3 router-east-2 -> router-west Connected down=true",
			},
			linkIds: []string{"l1"},
		},
		{
			name:    "links to routers outside the mesh are ignored",
			links:   append(fullMesh, connected("l9", "router-east-1", "router-other")),
			linkIds: []string{"l1", "l2", "l3"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gaps, linkIds := meshGaps(routers, test.links)
			require.Equal(t, test.gaps, gaps)
			require.Equal(t, test.linkIds, linkIds)
		})
	}
}
