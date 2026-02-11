package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/parallel"
	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/v2/zitirest"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/openziti/ziti/zititest/zitilab/models"
)

const lifecycleLabelPrefix = "lifecycle:"

type RouterState int

const (
	Absent          RouterState = iota // not created in controller
	Present                            // running, no ctrl listener
	PresentDialable                    // running, with ctrl listener
)

func (s RouterState) String() string {
	switch s {
	case Absent:
		return "Absent"
	case Present:
		return "Present"
	case PresentDialable:
		return "PresentDialable"
	default:
		return fmt.Sprintf("Unknown(%d)", int(s))
	}
}

func parseRouterState(s string) RouterState {
	switch s {
	case "Present":
		return Present
	case "PresentDialable":
		return PresentDialable
	default:
		return Absent
	}
}

type lifecycleBootstrapExtension struct{}

func (self lifecycleBootstrapExtension) Bootstrap(m *model.Model) error {
	logger := tui.ActionsLogger()
	label := model.GetLabel()
	if label == nil {
		return nil
	}

	count := 0
	for k, v := range label.Bindings {
		if id, ok := strings.CutPrefix(k, lifecycleLabelPrefix); ok {
			if stateStr, ok := v.(string); ok {
				lifecycleStates[id] = parseRouterState(stateStr)
				count++
			}
		}
	}

	if count > 0 {
		logger.Infof("loaded %d lifecycle router states from label", count)
	}
	return nil
}

func saveLifecycleStatesToLabel(run model.Run) error {
	label := run.GetLabel()

	// clear old lifecycle entries
	for k := range label.Bindings {
		if strings.HasPrefix(k, lifecycleLabelPrefix) {
			delete(label.Bindings, k)
		}
	}

	// save current lifecycle states
	for _, c := range run.GetModel().SelectComponents(".lifecycle") {
		state := lifecycleStates[c.Id]
		label.Bindings[lifecycleLabelPrefix+c.Id] = state.String()
	}

	return label.Save()
}

var lifecycleStates = map[string]RouterState{}

func clearLifecycleStateFromLabel(run model.Run) error {
	label := run.GetLabel()
	for k := range label.Bindings {
		if strings.HasPrefix(k, lifecycleLabelPrefix) {
			delete(label.Bindings, k)
		}
	}
	lifecycleStates = map[string]RouterState{}
	initStaticRouterStates(run.GetModel())
	return label.Save()
}

func resetLifecycleRouters(run model.Run) error {
	logger := tui.ActionsLogger()
	components := run.GetModel().SelectComponents(".lifecycle")
	logger.Infof("resetting %d lifecycle routers to Absent", len(components))

	// best-effort stop, may not be running
	if err := component.StopInParallel(".lifecycle", 15).Execute(run); err != nil {
		return err
	}

	for _, c := range components {
		lifecycleStates[c.Id] = Absent
	}

	// authenticate via REST API
	ctrl, err := run.GetModel().SelectComponent("#ctrl-east")
	if err != nil {
		return err
	}
	clients, err := chaos.EnsureLoggedIntoCtrl(run, ctrl, time.Minute)
	if err != nil {
		return err
	}

	// list and delete lifecycle routers via REST API
	routers, err := models.ListEdgeRouters(clients, `name contains "lifecycle-" limit none`, 15*time.Second)
	if err != nil {
		logger.WithError(err).Info("failed to list lifecycle routers for deletion")
	} else {
		var tasks []parallel.LabeledTask
		for _, r := range routers {
			id := *r.ID
			name := *r.Name
			tasks = append(tasks, parallel.TaskWithLabel("delete.edge-router",
				fmt.Sprintf("delete lifecycle router %s", name),
				parallel.Task(func() error {
					return models.DeleteEdgeRouter(clients, id, 15*time.Second)
				}),
			))
		}
		if err := parallel.ExecuteLabeled(tasks, 10, models.RetryPolicy); err != nil {
			logger.WithError(err).Info("some lifecycle router deletions failed")
		}
	}

	if err := saveLifecycleStatesToLabel(run); err != nil {
		return fmt.Errorf("failed to save lifecycle states to label: %w", err)
	}

	logger.Info("all lifecycle routers reset to Absent")
	return nil
}

func transitionLifecycleRouters(run model.Run) error {
	logger := tui.ActionsLogger()

	// Authenticate once via REST API
	ctrl, err := run.GetModel().SelectComponent("#ctrl-east")
	if err != nil {
		return err
	}
	clients, err := chaos.EnsureLoggedIntoCtrl(run, ctrl, time.Minute)
	if err != nil {
		return err
	}

	components := run.GetModel().SelectComponents(".lifecycle")
	logger.Infof("transitioning %d lifecycle routers", len(components))

	var tasks []parallel.LabeledTask
	newStates := map[string]RouterState{}

	for _, c := range components {
		currentState := lifecycleStates[c.Id]
		newState := pickNewState()

		if newState == currentState {
			continue
		}

		logger.Infof("transitioning %s: %s -> %s", c.Id, currentState, newState)
		newStates[c.Id] = newState
		tasks = append(tasks, buildTransitionTasks(run, c, clients, currentState, newState)...)
	}

	if len(tasks) == 0 {
		return saveLifecycleStatesToLabel(run)
	}

	if err := parallel.ExecuteLabeled(tasks, 10, models.RetryPolicy); err != nil {
		return err
	}

	for id, state := range newStates {
		lifecycleStates[id] = state
	}
	return saveLifecycleStatesToLabel(run)
}

func pickNewState() RouterState {
	states := []RouterState{Absent, Present, PresentDialable}
	return states[rand.Intn(len(states))]
}

func buildTransitionTasks(run model.Run, c *model.Component, clients *zitirest.Clients, from, to RouterState) []parallel.LabeledTask {
	switch {
	case from == Absent && to == Present:
		return buildCreateAndStartTasks(run, c, clients, zitilab.DefaultConfigId)
	case from == Absent && to == PresentDialable:
		return buildCreateAndStartTasks(run, c, clients, "dialable")
	case from == Present && to == PresentDialable:
		return buildReconfigureTasks(run, c, "dialable")
	case from == PresentDialable && to == Present:
		return buildReconfigureTasks(run, c, zitilab.DefaultConfigId)
	case to == Absent:
		return buildDeleteTasks(run, c, clients)
	default:
		return nil
	}
}

func buildCreateAndStartTasks(run model.Run, c *model.Component, clients *zitirest.Clients, configId string) []parallel.LabeledTask {
	c.PutVariable(zitilab.ConfigIdKey, configId)
	routerType := c.Type.(*zitilab.RouterType)

	// Reuse CreateAndEnrollTasks for create, get-jwt, send-jwt, enroll steps
	tasks := routerType.CreateAndEnrollTasks(run, c, clients)

	startTask := parallel.TaskWithLabel("lifecycle.start", fmt.Sprintf("start lifecycle router %s", c.Id), func() error {
		return routerType.Start(run, c)
	})
	startTask.DependsOn(tasks[len(tasks)-1], time.Minute)

	tasks = append(tasks, startTask)
	return tasks
}

func buildReconfigureTasks(run model.Run, c *model.Component, configId string) []parallel.LabeledTask {
	routerType := c.Type.(*zitilab.RouterType)

	stopTask := parallel.TaskWithLabel("lifecycle.stop", fmt.Sprintf("stop lifecycle router %s", c.Id), func() error {
		err := routerType.Stop(run, c)
		if err == nil {
			c.PutVariable(zitilab.ConfigIdKey, configId)
		}
		return err
	})

	startTask := parallel.TaskWithLabel("lifecycle.start", fmt.Sprintf("start lifecycle router %s", c.Id), func() error {
		return routerType.Start(run, c)
	})

	startTask.DependsOn(stopTask, time.Minute)

	return []parallel.LabeledTask{stopTask, startTask}
}

func buildDeleteTasks(run model.Run, c *model.Component, clients *zitirest.Clients) []parallel.LabeledTask {
	routerType := c.Type.(*zitilab.RouterType)
	var idHolder concurrenz.AtomicValue[string]

	stopTask := parallel.TaskWithLabel("lifecycle.stop", fmt.Sprintf("stop lifecycle router %s", c.Id), func() error {
		return routerType.Stop(run, c)
	})

	lookupTask := parallel.TaskWithLabel("lifecycle.lookup", fmt.Sprintf("lookup lifecycle router %s", c.Id), func() error {
		id, err := models.GetEdgeRouterId(clients, c.Id, 15*time.Second)
		if err != nil {
			return err
		}
		idHolder.Store(id)
		return nil
	})
	lookupTask.DependsOn(stopTask, time.Minute)

	deleteTask := parallel.TaskWithLabel("delete.edge-router", fmt.Sprintf("delete lifecycle router %s", c.Id), func() error {
		return models.DeleteEdgeRouter(clients, idHolder.Load(), 15*time.Second)
	})
	deleteTask.DependsOn(lookupTask, time.Minute)

	return []parallel.LabeledTask{stopTask, lookupTask, deleteTask}
}

func selectRunningLifecycleComponents(m *model.Model) []*model.Component {
	var result []*model.Component
	for _, c := range m.SelectComponents(".lifecycle") {
		if state, ok := lifecycleStates[c.Id]; ok && state != Absent {
			result = append(result, c)
		}
	}
	return result
}

func initStaticRouterStates(m *model.Model) {
	for _, c := range m.SelectComponents(".router") {
		lifecycleStates[c.Id] = PresentDialable
	}
}

func expectedRoutersForCtrl(m *model.Model, ctrl *model.Component) map[string]bool {
	isWestCtrl := ctrl.HasTag("west")

	componentById := map[string]*model.Component{}
	for _, c := range m.SelectComponents(".router") {
		componentById[c.Id] = c
	}
	for _, c := range m.SelectComponents(".lifecycle") {
		componentById[c.Id] = c
	}

	result := map[string]bool{}
	for id, state := range lifecycleStates {
		if state == Absent {
			continue
		}

		if !isWestCtrl {
			// ctrl-east: connected to all non-Absent routers
			result[id] = true
		} else {
			// ctrl-west: connected to west non-Absent + non-west PresentDialable
			c := componentById[id]
			if c == nil {
				continue
			}
			if c.HasTag("west") {
				result[id] = true
			} else if state == PresentDialable {
				result[id] = true
			}
		}
	}
	return result
}
