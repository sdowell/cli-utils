// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package list

import (
	"fmt"

	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

type Formatter interface {
	FormatApplyEvent(ae event.ApplyEvent) error
	FormatStatusEvent(se event.StatusEvent) error
	FormatPruneEvent(pe event.PruneEvent) error
	FormatDeleteEvent(de event.DeleteEvent) error
	FormatWaitEvent(we event.WaitEvent) error
	FormatErrorEvent(ee event.ErrorEvent) error
	FormatActionGroupEvent(
		age event.ActionGroupEvent,
		ags []event.ActionGroup,
		as *ApplyStats,
		ps *PruneStats,
		ds *DeleteStats,
		ws *WaitStats,
		c Collector,
	) error
}

type FormatterFactory func(previewStrategy common.DryRunStrategy) Formatter

type BaseListPrinter struct {
	FormatterFactory FormatterFactory
}

type ApplyStats struct {
	ServersideApplied int
	Created           int
	Unchanged         int
	Configured        int
	Failed            int
}

func (a *ApplyStats) inc(op event.ApplyEventOperation) {
	switch op {
	case event.ApplyUnspecified:
	case event.ServersideApplied:
		a.ServersideApplied++
	case event.Created:
		a.Created++
	case event.Unchanged:
		a.Unchanged++
	case event.Configured:
		a.Configured++
	default:
		panic(fmt.Errorf("unknown apply operation %s", op.String()))
	}
}

func (a *ApplyStats) incFailed() {
	a.Failed++
}

func (a *ApplyStats) Sum() int {
	return a.ServersideApplied + a.Configured + a.Unchanged + a.Created + a.Failed
}

type PruneStats struct {
	Pruned  int
	Skipped int
	Failed  int
}

func (p *PruneStats) incPruned() {
	p.Pruned++
}

func (p *PruneStats) incSkipped() {
	p.Skipped++
}

func (p *PruneStats) incFailed() {
	p.Failed++
}

type DeleteStats struct {
	Deleted int
	Skipped int
	Failed  int
}

func (d *DeleteStats) incDeleted() {
	d.Deleted++
}

func (d *DeleteStats) incSkipped() {
	d.Skipped++
}

func (d *DeleteStats) incFailed() {
	d.Failed++
}

type WaitStats struct {
	Reconciled int
	Timeout    int
	Skipped    int
}

func (w *WaitStats) incReconciled() {
	w.Reconciled++
}

func (w *WaitStats) incTimeout() {
	w.Timeout++
}

func (w *WaitStats) incSkipped() {
	w.Skipped++
}

type Collector interface {
	LatestStatus() map[object.ObjMetadata]event.StatusEvent
}

type StatusCollector struct {
	latestStatus map[object.ObjMetadata]event.StatusEvent
}

func (sc *StatusCollector) updateStatus(id object.ObjMetadata, se event.StatusEvent) {
	sc.latestStatus[id] = se
}

func (sc *StatusCollector) LatestStatus() map[object.ObjMetadata]event.StatusEvent {
	return sc.latestStatus
}

// Print outputs the events from the provided channel in a simple
// format on StdOut. As we support other printer implementations
// this should probably be an interface.
// This function will block until the channel is closed.
//nolint:gocyclo
func (b *BaseListPrinter) Print(ch <-chan event.Event, previewStrategy common.DryRunStrategy, printStatus bool) error {
	var actionGroups []event.ActionGroup
	applyStats := &ApplyStats{}
	pruneStats := &PruneStats{}
	deleteStats := &DeleteStats{}
	waitStats := &WaitStats{}
	statusCollector := &StatusCollector{
		latestStatus: make(map[object.ObjMetadata]event.StatusEvent),
	}
	formatter := b.FormatterFactory(previewStrategy)
	for e := range ch {
		switch e.Type {
		case event.InitType:
			actionGroups = e.InitEvent.ActionGroups
		case event.ErrorType:
			_ = formatter.FormatErrorEvent(e.ErrorEvent)
			return e.ErrorEvent.Err
		case event.ApplyType:
			applyStats.inc(e.ApplyEvent.Operation)
			if e.ApplyEvent.Error != nil {
				applyStats.incFailed()
			}
			if err := formatter.FormatApplyEvent(e.ApplyEvent); err != nil {
				return err
			}
		case event.StatusType:
			statusCollector.updateStatus(e.StatusEvent.Identifier, e.StatusEvent)
			if printStatus {
				if err := formatter.FormatStatusEvent(e.StatusEvent); err != nil {
					return err
				}
			}
		case event.PruneType:
			switch e.PruneEvent.Operation {
			case event.Pruned:
				pruneStats.incPruned()
			case event.PruneSkipped:
				pruneStats.incSkipped()
			}
			if e.PruneEvent.Error != nil {
				pruneStats.incFailed()
			}
			if err := formatter.FormatPruneEvent(e.PruneEvent); err != nil {
				return err
			}
		case event.DeleteType:
			switch e.DeleteEvent.Operation {
			case event.Deleted:
				deleteStats.incDeleted()
			case event.DeleteSkipped:
				deleteStats.incSkipped()
			}
			if e.DeleteEvent.Error != nil {
				deleteStats.incFailed()
			}
			if err := formatter.FormatDeleteEvent(e.DeleteEvent); err != nil {
				return err
			}
		case event.WaitType:
			switch e.WaitEvent.Operation {
			case event.Reconciled:
				waitStats.incReconciled()
			case event.ReconcileSkipped:
				waitStats.incSkipped()
			case event.ReconcileTimeout:
				waitStats.incTimeout()
			}
			if err := formatter.FormatWaitEvent(e.WaitEvent); err != nil {
				return err
			}
		case event.ActionGroupType:
			if err := formatter.FormatActionGroupEvent(
				e.ActionGroupEvent,
				actionGroups,
				applyStats,
				pruneStats,
				deleteStats,
				waitStats,
				statusCollector,
			); err != nil {
				return err
			}
		}
	}
	failedSum := applyStats.Failed + pruneStats.Failed + deleteStats.Failed
	switch {
	case failedSum > 0 && waitStats.Timeout > 0:
		return fmt.Errorf("%d resources failed, %d resources failed to reconcile before timeout",
			failedSum, waitStats.Timeout)
	case failedSum > 0:
		return fmt.Errorf("%d resources failed", failedSum)
	case waitStats.Timeout > 0:
		return fmt.Errorf("%d resources failed to reconcile before timeout",
			waitStats.Timeout)
	default:
		return nil
	}
}

func ActionGroupByName(name string, ags []event.ActionGroup) (event.ActionGroup, bool) {
	for _, ag := range ags {
		if ag.Name == name {
			return ag, true
		}
	}
	return event.ActionGroup{}, false
}

// IsLastActionGroup returns true if the passed ActionGroupEvent is the
// last of its type in the slice of ActionGroup; false otherwise. For example,
// this function will determine if an ApplyAction is the last ApplyAction in
// the initialized task queue. This functionality is current used to determine
// when to print stats.
func IsLastActionGroup(age event.ActionGroupEvent, ags []event.ActionGroup) bool {
	var found bool
	var action event.ResourceAction
	for _, ag := range ags {
		if found && (action == ag.Action) {
			return false
		}
		if age.GroupName == ag.Name {
			found = true
			action = age.Action
		}
	}
	return true
}
