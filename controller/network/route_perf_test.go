/*
	Copyright NetFoundry, Inc.

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

package network

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/db"
	"github.com/sirupsen/logrus"
	"math/rand"
	"testing"
)

func TestShortestPathAgainstEstablished(t *testing.T) {
	pfxlog.GlobalInit(logrus.WarnLevel, pfxlog.DefaultOptions())

	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	closeNotify := make(chan struct{})
	defer close(closeNotify)

	options := DefaultOptions()
	options.MinRouterCost = 0
	network, err := NewNetwork("test", options, ctx.GetDb(), nil, NewVersionProviderTest(), closeNotify)
	ctx.NoError(err)

	entityHelper := newTestEntityHelper(ctx, network)

	var routers []*Router

	for i := 0; i < 50; i++ {
		router := entityHelper.addTestRouter()
		routers = append(routers, router)
	}

	linkIdx := 0

	r := rand.New(rand.NewSource(1))

	nextCost := func() int64 {
		v := r.Uint32()
		return int64(v % 1000)
	}

	addLink := func(srcRouter, dstRouter *Router) {
		if srcRouter != dstRouter {
			link := newTestLink(fmt.Sprintf("link-%04d", linkIdx), "tls")
			link.SetStaticCost(int32(nextCost()))
			link.SetDstLatency(nextCost() * 100_000)
			link.SetSrcLatency(nextCost() * 100_000)
			link.Src = srcRouter
			link.Dst = dstRouter
			link.addState(newLinkState(Connected))
			network.linkController.add(link)
			linkIdx++
		}
	}

	for _, srcRouter := range routers {
		for _, dstRouter := range routers {
			addLink(srcRouter, dstRouter)
		}
	}

	expectedRoutes := []*expectedRoute{
		{cost: 263, path: []string{"router-000", "router-036", "router-001"}},
		{cost: 100, path: []string{"router-000", "router-002"}},
		{cost: 232, path: []string{"router-000", "router-035", "router-003"}},
		{cost: 233, path: []string{"router-000", "router-019", "router-004"}},
		{cost: 315, path: []string{"router-000", "router-020", "router-005"}},
		{cost: 325, path: []string{"router-000", "router-002", "router-006"}},
		{cost: 355, path: []string{"router-000", "router-009", "router-007"}},
		{cost: 256, path: []string{"router-000", "router-002", "router-028", "router-008"}},
		{cost: 240, path: []string{"router-000", "router-009"}},
		{cost: 213, path: []string{"router-000", "router-002", "router-010"}},
		{cost: 198, path: []string{"router-000", "router-050", "router-011"}},
		{cost: 187, path: []string{"router-000", "router-035", "router-012"}},
		{cost: 204, path: []string{"router-000", "router-002", "router-013"}},
		{cost: 153, path: []string{"router-000", "router-014"}},
		{cost: 306, path: []string{"router-000", "router-051", "router-015"}},
		{cost: 253, path: []string{"router-000", "router-056", "router-016"}},
		{cost: 232, path: []string{"router-000", "router-050", "router-011", "router-017"}},
		{cost: 226, path: []string{"router-000", "router-020", "router-018"}},
		{cost: 119, path: []string{"router-000", "router-019"}},
		{cost: 188, path: []string{"router-000", "router-020"}},
		{cost: 268, path: []string{"router-000", "router-002", "router-021"}},
		{cost: 239, path: []string{"router-000", "router-051", "router-022"}},
		{cost: 118, path: []string{"router-000", "router-023"}},
		{cost: 190, path: []string{"router-000", "router-002", "router-024"}},
		{cost: 137, path: []string{"router-000", "router-025"}},
		{cost: 252, path: []string{"router-000", "router-023", "router-073"}},
		{cost: 237, path: []string{"router-000", "router-002", "router-054", "router-072"}},
		{cost: 218, path: []string{"router-000", "router-051", "router-071"}},
		{cost: 197, path: []string{"router-000", "router-002", "router-070"}},
		{cost: 254, path: []string{"router-000", "router-050", "router-069"}},
		{cost: 238, path: []string{"router-000", "router-019", "router-068"}},
		{cost: 197, path: []string{"router-000", "router-002", "router-067"}},
		{cost: 290, path: []string{"router-000", "router-050", "router-066"}},
		{cost: 230, path: []string{"router-000", "router-051", "router-065"}},
		{cost: 283, path: []string{"router-000", "router-002", "router-054", "router-064"}},
		{cost: 320, path: []string{"router-000", "router-063"}},
		{cost: 248, path: []string{"router-000", "router-002", "router-062"}},
		{cost: 329, path: []string{"router-000", "router-002", "router-061"}},
		{cost: 280, path: []string{"router-000", "router-056", "router-060"}},
		{cost: 257, path: []string{"router-000", "router-084", "router-059"}},
		{cost: 231, path: []string{"router-000", "router-084", "router-058"}},
		{cost: 211, path: []string{"router-000", "router-002", "router-054", "router-057"}},
		{cost: 155, path: []string{"router-000", "router-056"}},
		{cost: 280, path: []string{"router-000", "router-002", "router-055"}},
		{cost: 198, path: []string{"router-000", "router-002", "router-054"}},
		{cost: 308, path: []string{"router-000", "router-002", "router-053"}},
		{cost: 273, path: []string{"router-000", "router-052"}},
		{cost: 133, path: []string{"router-000", "router-051"}},
		{cost: 141, path: []string{"router-000", "router-050"}},
	}

	srcRouter := routers[0]
	replaceIdx := len(routers) - 1
	for i := 1; i < len(routers); i++ {
		p, c, err := network.shortestPath(srcRouter, routers[i])
		ctx.NoError(err)

		ctx.Equal(expectedRoutes[i-1].cost, c)
		for idx, r := range p {
			ctx.Equal(expectedRoutes[i-1].path[idx], r.Id)
		}

		network.DisconnectRouter(routers[replaceIdx])
		newRouter := entityHelper.addTestRouter()
		routers[replaceIdx] = newRouter
		for _, r := range routers {
			addLink(newRouter, r)
			addLink(r, newRouter)
		}
		replaceIdx--
		entityHelper.discardControllerEvents()
	}
}

func BenchmarkShortestPathPerfWithRouterChanges(b *testing.B) {
	b.StopTimer()
	pfxlog.GlobalInit(logrus.WarnLevel, pfxlog.DefaultOptions())

	ctx := db.NewTestContext(b)
	defer ctx.Cleanup()

	closeNotify := make(chan struct{})
	defer close(closeNotify)

	network, err := NewNetwork("test", nil, ctx.GetDb(), nil, NewVersionProviderTest(), closeNotify)
	ctx.NoError(err)

	entityHelper := newTestEntityHelper(ctx, network)

	var routers []*Router

	for i := 0; i < 50; i++ {
		router := entityHelper.addTestRouter()
		routers = append(routers, router)
	}

	linkIdx := 0

	r := rand.New(rand.NewSource(1))

	nextCost := func() int64 {
		v := r.Uint32()
		return int64(v % 1000)
	}

	addLink := func(srcRouter, dstRouter *Router) {
		if srcRouter != dstRouter {
			link := newTestLink(fmt.Sprintf("link-%04d", linkIdx), "tls")
			link.SetStaticCost(int32(nextCost()))
			link.SetDstLatency(nextCost() * 100_000)
			link.SetSrcLatency(nextCost() * 100_000)
			link.Src = srcRouter
			link.Dst = dstRouter
			link.addState(newLinkState(Connected))
			network.linkController.add(link)
			linkIdx++
		}
	}

	for _, srcRouter := range routers {
		for _, dstRouter := range routers {
			addLink(srcRouter, dstRouter)
		}
	}

	b.StartTimer()
	replaceIdx := len(routers) - 1
	srcIndex := 0
	dstIndex := 1
	for i := 0; i < b.N; i++ {
		srcRouter := routers[srcIndex]
		dstRouter := routers[dstIndex]
		_, _, err := network.shortestPath(srcRouter, dstRouter)
		ctx.NoError(err)

		network.DisconnectRouter(routers[replaceIdx])
		newRouter := entityHelper.addTestRouter()
		routers[replaceIdx] = newRouter
		for _, r := range routers {
			addLink(newRouter, r)
			addLink(r, newRouter)
		}
		replaceIdx--
		if replaceIdx < 0 {
			replaceIdx = len(routers) - 1
		}
		entityHelper.discardControllerEvents()

		dstIndex++
		for dstIndex >= len(routers) {
			srcIndex++
			if srcIndex >= len(routers) {
				srcIndex = 0
			}
			dstIndex = 0
			if dstIndex == srcIndex {
				dstIndex++
			}
		}

	}
}

type expectedRoute struct {
	cost int64
	path []string
}

func BenchmarkShortestPathPerf(b *testing.B) {
	b.StopTimer()
	pfxlog.GlobalInit(logrus.WarnLevel, pfxlog.DefaultOptions())

	ctx := db.NewTestContext(b)
	defer ctx.Cleanup()

	closeNotify := make(chan struct{})
	defer close(closeNotify)

	network, err := NewNetwork("test", nil, ctx.GetDb(), nil, NewVersionProviderTest(), closeNotify)
	ctx.NoError(err)

	entityHelper := newTestEntityHelper(ctx, network)

	var routers []*Router

	for i := 0; i < 400; i++ {
		router := entityHelper.addTestRouter()
		routers = append(routers, router)
	}

	linkIdx := 0

	r := rand.New(rand.NewSource(1))

	nextCost := func() int64 {
		v := r.Uint32()
		return int64(v % 1000)
	}

	addLink := func(srcRouter, dstRouter *Router) {
		if srcRouter != dstRouter {
			link := newTestLink(fmt.Sprintf("link-%04d", linkIdx), "tls")
			link.SetStaticCost(int32(nextCost()))
			link.SetDstLatency(nextCost() * 100_000)
			link.SetSrcLatency(nextCost() * 100_000)
			link.Src = srcRouter
			link.Dst = dstRouter
			link.addState(newLinkState(Connected))
			network.linkController.add(link)
			linkIdx++
		}
	}

	for _, srcRouter := range routers {
		for _, dstRouter := range routers {
			addLink(srcRouter, dstRouter)
		}
	}

	b.StartTimer()
	srcIndex := 0
	dstIndex := 1
	for i := 0; i < b.N; i++ {
		srcRouter := routers[srcIndex]
		dstRouter := routers[dstIndex]
		_, _, err := network.shortestPath(srcRouter, dstRouter)
		ctx.NoError(err)

		dstIndex++
		for dstIndex >= len(routers) {
			srcIndex++
			if srcIndex >= len(routers) {
				srcIndex = 0
			}
			dstIndex = 0
			if dstIndex == srcIndex {
				dstIndex++
			}
		}
	}
}

func BenchmarkMoreRealisticShortestPathPerf(b *testing.B) {
	//b.StopTimer()
	pfxlog.GlobalInit(logrus.WarnLevel, pfxlog.DefaultOptions())

	ctx := db.NewTestContext(b)
	defer ctx.Cleanup()

	closeNotify := make(chan struct{})
	defer close(closeNotify)

	network, err := NewNetwork("test", nil, ctx.GetDb(), nil, NewVersionProviderTest(), closeNotify)
	ctx.NoError(err)

	entityHelper := newTestEntityHelper(ctx, network)

	var routers []*Router

	for i := 0; i < 200; i++ {
		router := entityHelper.addTestRouter()
		routers = append(routers, router)
	}

	linkIdx := 0

	r := rand.New(rand.NewSource(1))

	nextCost := func() int64 {
		v := r.Uint32()
		return int64(v % 1000)
	}

	addLink := func(srcRouter, dstRouter *Router) {
		if srcRouter != dstRouter {
			link := newTestLink(fmt.Sprintf("link-%04d", linkIdx), "tls")
			link.SetStaticCost(int32(nextCost()))
			link.SetDstLatency(nextCost() * 100_000)
			link.SetSrcLatency(nextCost() * 100_000)
			link.Src = srcRouter
			link.Dst = dstRouter
			link.addState(newLinkState(Connected))
			network.linkController.add(link)
			linkIdx++
		}
	}

	// make half the routers private routers
	var privateRouters []*Router
	var publicRouters []*Router
	for idx, router := range routers {
		if idx <= len(routers)/2 {
			privateRouters = append(privateRouters, router)
		} else {
			publicRouters = append(publicRouters, router)
		}
	}

	for _, srcRouter := range privateRouters {
		for _, dstRouter := range publicRouters {
			addLink(srcRouter, dstRouter)
		}
	}

	for _, srcRouter := range publicRouters {
		for _, dstRouter := range publicRouters {
			addLink(srcRouter, dstRouter)
		}
	}

	b.StartTimer()
	srcIndex := 0
	dstIndex := 1
	for i := 0; i < b.N; i++ {
		srcRouter := privateRouters[srcIndex]
		dstRouter := privateRouters[dstIndex]
		_, _, err := network.shortestPath(srcRouter, dstRouter)
		ctx.NoError(err)

		dstIndex++
		for dstIndex >= len(privateRouters) {
			srcIndex++
			if srcIndex >= len(privateRouters) {
				srcIndex = 0
			}
			dstIndex = 0
			if dstIndex == srcIndex {
				dstIndex++
			}
		}
	}
}
