package validations

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/controller/rest_client/link"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
)

const linkMeshPollInterval = 5 * time.Second

// ValidateLinkMesh waits until every controller reports exactly one usable link, connected and not down,
// between each pair of the routers routerSelector selects, with the same links for hold. It fails when that
// doesn't happen within timeout, naming each pair that is missing its link or still has more than one.
func ValidateLinkMesh(run model.Run, routerSelector string, timeout, hold time.Duration) error {
	routers := map[string]string{}
	for _, c := range run.GetModel().SelectComponents(routerSelector) {
		routers[c.Id] = routerLabel(c)
	}

	ctrls := run.GetModel().SelectComponents(".ctrl")
	errC := make(chan error, len(ctrls))
	deadline := time.Now().Add(timeout)
	for _, ctrl := range ctrls {
		go func() {
			errC <- ValidateLinkMeshForCtrl(run, ctrl, routers, deadline, hold)
		}()
	}

	var errs []error
	for range len(ctrls) {
		if err := <-errC; err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ValidateLinkMeshForCtrl runs ValidateLinkMesh's check against controller c until deadline. routers maps
// each router's name to the label its failures are reported under.
func ValidateLinkMeshForCtrl(run model.Run, c *model.Component, routers map[string]string, deadline time.Time, hold time.Duration) error {
	logger := tui.ValidationLogger().WithField("ctrl", c.Id)
	start := time.Now()

	held := false
	var heldIds []string
	var heldSince time.Time
	var lastErr error
	var lastReport string

	for {
		gaps, linkIds, err := checkLinkMesh(run, c, routers)
		switch {
		case err != nil:
			lastErr, held = err, false
		case len(gaps) > 0:
			lastErr, held = fmt.Errorf("link mesh on %s incomplete: %s", c.Id, strings.Join(gaps, "; ")), false
		case !held:
			lastErr, held, heldIds, heldSince = nil, true, linkIds, time.Now()
		case !slices.Equal(heldIds, linkIds):
			// a link replaced while the mesh stays complete is a link that dropped and was redialed
			lastErr = fmt.Errorf("link mesh on %s replaced links while held, from %v to %v", c.Id, heldIds, linkIds)
			heldIds, heldSince = linkIds, time.Now()
		case time.Since(heldSince) >= hold:
			logger.Infof("link mesh of %d routers complete and unchanged for %v, elapsed: %v", len(routers), hold, time.Since(start))
			return nil
		}

		report := fmt.Sprintf("link mesh complete with %d links, holding %v", len(heldIds), hold)
		if lastErr != nil {
			report = lastErr.Error()
		}
		if report != lastReport {
			logger.Infof("%s, elapsed: %v", report, time.Since(start))
			lastReport = report
		}

		if time.Now().After(deadline) {
			if lastErr == nil {
				lastErr = fmt.Errorf("link mesh on %s complete but not yet held for %v", c.Id, hold)
			}
			return lastErr
		}
		time.Sleep(linkMeshPollInterval)
	}
}

// meshLink is the part of a controller's link the mesh check reads.
type meshLink struct {
	id     string
	src    string
	dst    string
	state  string
	down   bool
	usable bool
}

func checkLinkMesh(run model.Run, c *model.Component, routers map[string]string) ([]string, []string, error) {
	clients, err := chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
	if err != nil {
		return nil, nil, err
	}

	ctx, cancelF := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelF()

	filter := "limit none"
	result, err := clients.Fabric.Link.ListLinks(&link.ListLinksParams{
		Filter:  &filter,
		Context: ctx,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("list links on %s: %w", c.Id, err)
	}

	var links []meshLink
	for _, detail := range result.Payload.Data {
		if detail.SourceRouter == nil || detail.DestRouter == nil {
			continue
		}
		l := meshLink{
			id:    deref(detail.ID),
			src:   detail.SourceRouter.Name,
			dst:   detail.DestRouter.Name,
			state: deref(detail.State),
			down:  deref(detail.Down),
		}
		l.usable = l.state == "Connected" && !l.down
		links = append(links, l)
	}

	gaps, linkIds := meshGaps(routers, links)
	return gaps, linkIds, nil
}

// meshGaps compares links to the full mesh of routers, which maps each router's name to its label. Each
// unordered pair of routers needs exactly one usable link. It returns a description of each pair that
// doesn't have one, and the sorted ids of the usable links between routers.
func meshGaps(routers map[string]string, links []meshLink) ([]string, []string) {
	type pair struct{ a, b string }
	byPair := map[pair][]meshLink{}
	var linkIds []string
	for _, l := range links {
		if _, found := routers[l.src]; !found {
			continue
		}
		if _, found := routers[l.dst]; !found {
			continue
		}
		p := pair{a: l.src, b: l.dst}
		if p.a > p.b {
			p.a, p.b = p.b, p.a
		}
		byPair[p] = append(byPair[p], l)
		if l.usable {
			linkIds = append(linkIds, l.id)
		}
	}
	sort.Strings(linkIds)

	names := make([]string, 0, len(routers))
	for name := range routers {
		names = append(names, name)
	}
	sort.Strings(names)

	var gaps []string
	for i, a := range names {
		for _, b := range names[i+1:] {
			var usable, unusable []string
			for _, l := range byPair[pair{a: a, b: b}] {
				if l.usable {
					usable = append(usable, fmt.Sprintf("%s %s -> %s", l.id, l.src, l.dst))
				} else {
					unusable = append(unusable, fmt.Sprintf("%s %s -> %s %s down=%v", l.id, l.src, l.dst, l.state, l.down))
				}
			}

			var gap string
			switch {
			case len(usable) == 0:
				gap = fmt.Sprintf("%s <-> %s: no usable link", routers[a], routers[b])
			case len(usable) > 1:
				gap = fmt.Sprintf("%s <-> %s: %d usable links (%s)", routers[a], routers[b], len(usable), strings.Join(usable, ", "))
			default:
				continue
			}
			if len(unusable) > 0 {
				gap += fmt.Sprintf(", plus %s", strings.Join(unusable, ", "))
			}
			gaps = append(gaps, gap)
		}
	}
	return gaps, linkIds
}

// routerLabel names router c with its version, when it doesn't run the local build.
func routerLabel(c *model.Component) string {
	if router, ok := c.Type.(*zitilab.RouterType); ok && router.Version != "" {
		return fmt.Sprintf("%s (%s)", c.Id, router.Version)
	}
	return c.Id
}

func deref[T any](v *T) T {
	var zero T
	if v == nil {
		return zero
	}
	return *v
}
