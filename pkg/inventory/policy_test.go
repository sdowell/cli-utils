// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type fakeInventoryInfo struct {
	id string
}

func (i *fakeInventoryInfo) Name() string {
	return ""
}

func (i *fakeInventoryInfo) Namespace() string {
	return ""
}

func (i *fakeInventoryInfo) ID() string {
	return i.id
}

func (i *fakeInventoryInfo) Strategy() InventoryStrategy {
	return NameStrategy
}

func testObjectWithAnnotation(key, val string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "foo",
				"namespace": "ns",
			},
		},
	}
	if key != "" {
		obj.SetAnnotations(map[string]string{
			key: val,
		})
	}
	return obj
}

func TestInventoryIDMatch(t *testing.T) {
	testcases := []struct {
		name     string
		obj      *unstructured.Unstructured
		inv      InventoryInfo
		expected inventoryIDMatchStatus
	}{
		{
			name:     "empty",
			obj:      testObjectWithAnnotation("", ""),
			inv:      &fakeInventoryInfo{id: "random-id"},
			expected: Empty,
		},
		{
			name:     "matched",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "matched"),
			inv:      &fakeInventoryInfo{id: "matched"},
			expected: Match,
		},
		{
			name:     "unmatched",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "unmatched"),
			inv:      &fakeInventoryInfo{id: "random-id"},
			expected: NoMatch,
		},
	}
	for _, tc := range testcases {
		actual := InventoryIDMatch(tc.inv, tc.obj)
		if actual != tc.expected {
			t.Errorf("expected %v, but got %v", tc.expected, actual)
		}
	}
}

func TestCanApply(t *testing.T) {
	testcases := []struct {
		name     string
		obj      *unstructured.Unstructured
		inv      InventoryInfo
		policy   InventoryPolicy
		canApply bool
	}{
		{
			name:     "nil object",
			obj:      nil,
			inv:      &fakeInventoryInfo{id: "random-id"},
			canApply: true,
		},
		{
			name:     "empty with AdoptIfNoInventory",
			obj:      testObjectWithAnnotation("", ""),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   AdoptIfNoInventory,
			canApply: true,
		},
		{
			name:     "empty with AdoptAll",
			obj:      testObjectWithAnnotation("", ""),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   AdoptAll,
			canApply: true,
		},
		{
			name:     "empty with InventoryPolicyMustMatch",
			obj:      testObjectWithAnnotation("", ""),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   InventoryPolicyMustMatch,
			canApply: false,
		},
		{
			name:     "matched with InventoryPolicyMustMatch",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "matched"),
			inv:      &fakeInventoryInfo{id: "matched"},
			policy:   InventoryPolicyMustMatch,
			canApply: true,
		},
		{
			name:     "matched with AdoptIfNoInventory",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "matched"),
			inv:      &fakeInventoryInfo{id: "matched"},
			policy:   AdoptIfNoInventory,
			canApply: true,
		},
		{
			name:     "matched with AloptAll",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "matched"),
			inv:      &fakeInventoryInfo{id: "matched"},
			policy:   AdoptAll,
			canApply: true,
		},
		{
			name:     "unmatched with InventoryPolicyMustMatch",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "unmatched"),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   InventoryPolicyMustMatch,
			canApply: false,
		},
		{
			name:     "unmatched with AdoptIfNoInventory",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "unmatched"),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   AdoptIfNoInventory,
			canApply: false,
		},
		{
			name:     "unmatched with AdoptAll",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "unmatched"),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   AdoptAll,
			canApply: true,
		},
	}
	for _, tc := range testcases {
		actual, _ := CanApply(tc.inv, tc.obj, tc.policy)
		if actual != tc.canApply {
			t.Errorf("expected %v, but got %v", tc.canApply, actual)
		}
	}
}

func TestCanPrune(t *testing.T) {
	testcases := []struct {
		name     string
		obj      *unstructured.Unstructured
		inv      InventoryInfo
		policy   InventoryPolicy
		canPrune bool
	}{
		{
			name:     "nil object",
			obj:      nil,
			inv:      &fakeInventoryInfo{id: "random-id"},
			canPrune: false,
		},
		{
			name:     "empty with AdoptIfNoInventory",
			obj:      testObjectWithAnnotation("", ""),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   AdoptIfNoInventory,
			canPrune: true,
		},
		{
			name:     "empty with AdoptAll",
			obj:      testObjectWithAnnotation("", ""),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   AdoptAll,
			canPrune: true,
		},
		{
			name:     "empty with InventoryPolicyMustMatch",
			obj:      testObjectWithAnnotation("", ""),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   InventoryPolicyMustMatch,
			canPrune: false,
		},
		{
			name:     "matched with InventoryPolicyMustMatch",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "matched"),
			inv:      &fakeInventoryInfo{id: "matched"},
			policy:   InventoryPolicyMustMatch,
			canPrune: true,
		},
		{
			name:     "matched with AdoptIfNoInventory",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "matched"),
			inv:      &fakeInventoryInfo{id: "matched"},
			policy:   AdoptIfNoInventory,
			canPrune: true,
		},
		{
			name:     "matched with AloptAll",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "matched"),
			inv:      &fakeInventoryInfo{id: "matched"},
			policy:   AdoptAll,
			canPrune: true,
		},
		{
			name:     "unmatched with InventoryPolicyMustMatch",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "unmatched"),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   InventoryPolicyMustMatch,
			canPrune: false,
		},
		{
			name:     "unmatched with AdoptIfNoInventory",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "unmatched"),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   AdoptIfNoInventory,
			canPrune: false,
		},
		{
			name:     "unmatched with AdoptAll",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "unmatched"),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   AdoptAll,
			canPrune: true,
		},
	}
	for _, tc := range testcases {
		actual := CanPrune(tc.inv, tc.obj, tc.policy)
		if actual != tc.canPrune {
			t.Errorf("expected %v, but got %v", tc.canPrune, actual)
		}
	}
}
