// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"testing"

	"sigs.k8s.io/cli-utils/pkg/apply/cache"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

func TestInvSetTask(t *testing.T) {
	id1 := object.UnstructuredToObjMetaOrDie(obj1)
	id2 := object.UnstructuredToObjMetaOrDie(obj2)
	id3 := object.UnstructuredToObjMetaOrDie(obj3)

	tests := map[string]struct {
		prevInventory  object.ObjMetadataSet
		appliedObjs    object.ObjMetadataSet
		failedApplies  object.ObjMetadataSet
		failedDeletes  object.ObjMetadataSet
		skippedApplies object.ObjMetadataSet
		skippedDeletes object.ObjMetadataSet
		abandonedObjs  object.ObjMetadataSet
		expectedObjs   object.ObjMetadataSet
	}{
		"no apply objs, no prune failures; no inventory": {
			appliedObjs:   object.ObjMetadataSet{},
			failedDeletes: object.ObjMetadataSet{},
			expectedObjs:  object.ObjMetadataSet{},
		},
		"one apply objs, no prune failures; one inventory": {
			appliedObjs:   object.ObjMetadataSet{id1},
			failedDeletes: object.ObjMetadataSet{},
			expectedObjs:  object.ObjMetadataSet{id1},
		},
		"no apply objs, one prune failure, in prev inventory; one inventory": {
			prevInventory: object.ObjMetadataSet{id1},
			appliedObjs:   object.ObjMetadataSet{},
			failedDeletes: object.ObjMetadataSet{id1},
			expectedObjs:  object.ObjMetadataSet{id1},
		},
		"no apply objs, one prune failure, not in prev inventory; no inventory": {
			// aritifical use case: prunes come from the inventory
			prevInventory: object.ObjMetadataSet{},
			appliedObjs:   object.ObjMetadataSet{},
			failedDeletes: object.ObjMetadataSet{id1},
			expectedObjs:  object.ObjMetadataSet{},
		},
		"one apply objs, one prune failures; one inventory": {
			// aritifical use case: applies and prunes are mutually exclusive
			appliedObjs:   object.ObjMetadataSet{id3},
			failedDeletes: object.ObjMetadataSet{id3},
			expectedObjs:  object.ObjMetadataSet{id3},
		},
		"two apply objs, two prune failures; three inventory": {
			// aritifical use case: applies and prunes are mutually exclusive
			prevInventory: object.ObjMetadataSet{id2, id3},
			appliedObjs:   object.ObjMetadataSet{id1, id2},
			failedDeletes: object.ObjMetadataSet{id2, id3},
			expectedObjs:  object.ObjMetadataSet{id1, id2, id3},
		},
		"no apply objs, no apply failures, no prune failures; no inventory": {
			appliedObjs:   object.ObjMetadataSet{},
			failedApplies: object.ObjMetadataSet{id3},
			prevInventory: object.ObjMetadataSet{},
			failedDeletes: object.ObjMetadataSet{},
			expectedObjs:  object.ObjMetadataSet{},
		},
		"one apply failure not in prev inventory; no inventory": {
			appliedObjs:   object.ObjMetadataSet{},
			failedApplies: object.ObjMetadataSet{id3},
			prevInventory: object.ObjMetadataSet{},
			failedDeletes: object.ObjMetadataSet{},
			expectedObjs:  object.ObjMetadataSet{},
		},
		"one apply obj, one apply failure not in prev inventory; one inventory": {
			appliedObjs:   object.ObjMetadataSet{id2},
			failedApplies: object.ObjMetadataSet{id3},
			prevInventory: object.ObjMetadataSet{},
			failedDeletes: object.ObjMetadataSet{},
			expectedObjs:  object.ObjMetadataSet{id2},
		},
		"one apply obj, one apply failure in prev inventory; one inventory": {
			appliedObjs:   object.ObjMetadataSet{id2},
			failedApplies: object.ObjMetadataSet{id3},
			prevInventory: object.ObjMetadataSet{id3},
			failedDeletes: object.ObjMetadataSet{},
			expectedObjs:  object.ObjMetadataSet{id2, id3},
		},
		"one apply obj, two apply failures with one in prev inventory; two inventory": {
			appliedObjs:   object.ObjMetadataSet{id2},
			failedApplies: object.ObjMetadataSet{id1, id3},
			prevInventory: object.ObjMetadataSet{id3},
			failedDeletes: object.ObjMetadataSet{},
			expectedObjs:  object.ObjMetadataSet{id2, id3},
		},
		"three apply failures with two in prev inventory; two inventory": {
			appliedObjs:   object.ObjMetadataSet{},
			failedApplies: object.ObjMetadataSet{id1, id2, id3},
			prevInventory: object.ObjMetadataSet{id2, id3},
			failedDeletes: object.ObjMetadataSet{},
			expectedObjs:  object.ObjMetadataSet{id2, id3},
		},
		"three apply failures with three in prev inventory; three inventory": {
			appliedObjs:   object.ObjMetadataSet{},
			failedApplies: object.ObjMetadataSet{id1, id2, id3},
			prevInventory: object.ObjMetadataSet{id2, id3, id1},
			failedDeletes: object.ObjMetadataSet{},
			expectedObjs:  object.ObjMetadataSet{id2, id1, id3},
		},
		"one skipped apply from prev inventory; one inventory": {
			prevInventory:  object.ObjMetadataSet{id1},
			appliedObjs:    object.ObjMetadataSet{},
			failedApplies:  object.ObjMetadataSet{},
			failedDeletes:  object.ObjMetadataSet{},
			skippedApplies: object.ObjMetadataSet{id1},
			skippedDeletes: object.ObjMetadataSet{},
			abandonedObjs:  object.ObjMetadataSet{},
			expectedObjs:   object.ObjMetadataSet{id1},
		},
		"one skipped apply, no prev inventory; no inventory": {
			prevInventory:  object.ObjMetadataSet{},
			appliedObjs:    object.ObjMetadataSet{},
			failedApplies:  object.ObjMetadataSet{},
			failedDeletes:  object.ObjMetadataSet{},
			skippedApplies: object.ObjMetadataSet{id1},
			skippedDeletes: object.ObjMetadataSet{},
			abandonedObjs:  object.ObjMetadataSet{},
			expectedObjs:   object.ObjMetadataSet{},
		},
		"one apply obj, one skipped apply, two prev inventory; two inventory": {
			prevInventory:  object.ObjMetadataSet{id1, id2},
			appliedObjs:    object.ObjMetadataSet{id2},
			failedApplies:  object.ObjMetadataSet{},
			failedDeletes:  object.ObjMetadataSet{},
			skippedApplies: object.ObjMetadataSet{id1},
			skippedDeletes: object.ObjMetadataSet{},
			abandonedObjs:  object.ObjMetadataSet{},
			expectedObjs:   object.ObjMetadataSet{id1, id2},
		},
		"one skipped delete from prev inventory; one inventory": {
			prevInventory:  object.ObjMetadataSet{id1},
			appliedObjs:    object.ObjMetadataSet{},
			failedApplies:  object.ObjMetadataSet{},
			failedDeletes:  object.ObjMetadataSet{},
			skippedApplies: object.ObjMetadataSet{},
			skippedDeletes: object.ObjMetadataSet{id1},
			abandonedObjs:  object.ObjMetadataSet{},
			expectedObjs:   object.ObjMetadataSet{id1},
		},
		"one apply obj, one skipped delete, two prev inventory; two inventory": {
			prevInventory:  object.ObjMetadataSet{id1, id2},
			appliedObjs:    object.ObjMetadataSet{id2},
			failedApplies:  object.ObjMetadataSet{},
			failedDeletes:  object.ObjMetadataSet{},
			skippedApplies: object.ObjMetadataSet{},
			skippedDeletes: object.ObjMetadataSet{id1},
			abandonedObjs:  object.ObjMetadataSet{},
			expectedObjs:   object.ObjMetadataSet{id1, id2},
		},
		"two apply obj, one abandoned, three in prev inventory; two inventory": {
			prevInventory: object.ObjMetadataSet{id1, id2, id3},
			appliedObjs:   object.ObjMetadataSet{id1, id2},
			failedApplies: object.ObjMetadataSet{},
			failedDeletes: object.ObjMetadataSet{},
			abandonedObjs: object.ObjMetadataSet{id3},
			expectedObjs:  object.ObjMetadataSet{id1, id2},
		},
		"two abandoned, two in prev inventory; no inventory": {
			prevInventory: object.ObjMetadataSet{id2, id3},
			appliedObjs:   object.ObjMetadataSet{},
			failedApplies: object.ObjMetadataSet{},
			failedDeletes: object.ObjMetadataSet{},
			abandonedObjs: object.ObjMetadataSet{id2, id3},
			expectedObjs:  object.ObjMetadataSet{},
		},
		"same obj skipped delete and abandoned, one in prev inventory; no inventory": {
			prevInventory:  object.ObjMetadataSet{id3},
			appliedObjs:    object.ObjMetadataSet{},
			failedApplies:  object.ObjMetadataSet{},
			failedDeletes:  object.ObjMetadataSet{},
			skippedDeletes: object.ObjMetadataSet{id3},
			abandonedObjs:  object.ObjMetadataSet{id3},
			expectedObjs:   object.ObjMetadataSet{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client := inventory.NewFakeInventoryClient(object.ObjMetadataSet{})
			eventChannel := make(chan event.Event)
			resourceCache := cache.NewResourceCacheMap()
			context := taskrunner.NewTaskContext(eventChannel, resourceCache)

			task := InvSetTask{
				TaskName:      taskName,
				InvClient:     client,
				InvInfo:       nil,
				PrevInventory: tc.prevInventory,
			}
			for _, applyObj := range tc.appliedObjs {
				context.AddSuccessfulApply(applyObj, "unusued-uid", int64(0))
			}
			for _, applyFailure := range tc.failedApplies {
				context.AddFailedApply(applyFailure)
			}
			for _, pruneObj := range tc.failedDeletes {
				context.AddFailedDelete(pruneObj)
			}
			for _, skippedApply := range tc.skippedApplies {
				context.AddSkippedApply(skippedApply)
			}
			for _, skippedDelete := range tc.skippedDeletes {
				context.AddSkippedDelete(skippedDelete)
			}
			for _, abandonedObj := range tc.abandonedObjs {
				context.AddAbandonedObject(abandonedObj)
			}
			if taskName != task.Name() {
				t.Errorf("expected task name (%s), got (%s)", taskName, task.Name())
			}
			task.Start(context)
			result := <-context.TaskChannel()
			if result.Err != nil {
				t.Errorf("unexpected error running InvAddTask: %s", result.Err)
			}
			actual, _ := client.GetClusterObjs(nil, common.DryRunNone)
			testutil.AssertEqual(t, tc.expectedObjs, actual,
				"Actual cluster objects (%d) do not match expected cluster objects (%d)",
				len(actual), len(tc.expectedObjs))
		})
	}
}
