// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// FakeClient is a testing implementation of the Client interface.
type FakeClient struct {
	Inv Inventory
	Err error
}

var (
	_ Client        = &FakeClient{}
	_ ClientFactory = FakeClientFactory{}

	_ Inventory = &FakeInventory{}
)

type FakeClientFactory object.ObjMetadataSet

func (f FakeClientFactory) NewClient(cmdutil.Factory) (Client, error) {
	return NewFakeClient(object.ObjMetadataSet(f)), nil
}

// NewFakeClient returns a FakeClient.
func NewFakeClient(objs object.ObjMetadataSet) *FakeClient {
	return &FakeClient{
		Inv: &FakeInventory{
			InventoryID:   "fake-inventory",
			BaseInventory: NewBaseInventory(objs, nil),
		},
		Err: nil,
	}
}

// Get returns currently stored inventory.
func (fic *FakeClient) Get(ctx context.Context, id Inventory, opts GetOptions) (Inventory, error) {
	if fic.Err != nil {
		return nil, fic.Err
	}
	return fic.Inv, nil
}

// Update the stored cluster inventory objs with the passed obj, or an
// error if one is set up.
func (fic *FakeClient) Update(ctx context.Context, inv Inventory, opts UpdateOptions) error {
	if fic.Err != nil {
		return fic.Err
	}
	fic.Inv = inv
	return nil
}

// UpdateStatus the stored cluster inventory objs with the passed obj, or an
// error if one is set up.
func (fic *FakeClient) UpdateStatus(ctx context.Context, inv Inventory, opts UpdateOptions) error {
	if fic.Err != nil {
		return fic.Err
	}
	fic.Inv = inv
	return nil
}

// Delete returns an error if one is forced; does nothing otherwise.
func (fic *FakeClient) Delete(ctx context.Context, id Inventory, opts DeleteOptions) error {
	if fic.Err != nil {
		return fic.Err
	}
	return nil
}

// List the in-cluster inventory
// Performs a simple in-place update on the ConfigMap
func (fic *FakeClient) List(ctx context.Context, opts ListOptions) ([]Inventory, error) {
	return nil, nil
}

// SetError forces an error on the subsequent client call if it returns an error.
func (fic *FakeClient) SetError(err error) {
	fic.Err = err
}

// ClearError clears the force error
func (fic *FakeClient) ClearError() {
	fic.Err = nil
}

type FakeInventory struct {
	BaseInventory
	InventoryID        string
	InventoryNamespace string
	InventoryName      string
}

func (fi *FakeInventory) ID() string {
	return fi.InventoryID
}

func (fi *FakeInventory) Namespace() string {
	return fi.InventoryNamespace
}

func (fi *FakeInventory) ToUnstructured() *unstructured.Unstructured {
	invMap := make(map[string]interface{})
	for _, objMeta := range fi.OldObjects {
		invMap[objMeta.String()] = ""
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      fi.InventoryName,
				"namespace": fi.InventoryNamespace,
				"labels": map[string]interface{}{
					common.InventoryLabel: fi.InventoryID,
				},
			},
			"data": invMap,
		},
	}
}
