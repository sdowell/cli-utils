// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Prune functionality deletes previously applied objects
// which are subsequently omitted in further apply operations.
// This functionality relies on "inventory" objects to store
// object metadata for each apply operation. This file defines
// PruneOptions to encapsulate information necessary to
// calculate the prune set, and to delete the objects in
// this prune set.

package prune

import (
	"context"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply/filter"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/ordering"
)

// Pruner implements GetPruneObjs to calculate which objects to prune and Prune
// to delete them.
type Pruner struct {
	InvClient inventory.InventoryClient
	Client    dynamic.Interface
	Mapper    meta.RESTMapper
}

// NewPruner returns a new Pruner.
// Returns an error if dependency injection fails using the factory.
func NewPruner(factory util.Factory, invClient inventory.InventoryClient) (*Pruner, error) {
	// Client/Builder fields from the Factory.
	client, err := factory.DynamicClient()
	if err != nil {
		return nil, err
	}
	mapper, err := factory.ToRESTMapper()
	if err != nil {
		return nil, err
	}
	return &Pruner{
		InvClient: invClient,
		Client:    client,
		Mapper:    mapper,
	}, nil
}

// Options defines a set of parameters that can be used to tune
// the behavior of the pruner.
type Options struct {
	// DryRunStrategy defines whether objects should actually be pruned or if
	// we should just print what would happen without actually doing it.
	DryRunStrategy common.DryRunStrategy

	PropagationPolicy metav1.DeletionPropagation

	// True if we are destroying, which deletes the inventory object
	// as well (possibly) the inventory namespace.
	Destroy bool
}

// Prune deletes the set of passed objects. A prune skip/failure is
// captured in the TaskContext, so we do not lose track of these
// objects from the inventory. The passed prune filters are used to
// determine if permission exists to delete the object. An example
// of a prune filter is PreventDeleteFilter, which checks if an
// annotation exists on the object to ensure the objects is not
// deleted (e.g. a PersistentVolume that we do no want to
// automatically prune/delete).
//
// Parameters:
//   objs - objects to prune (delete)
//   pruneFilters - list of filters for deletion permission
//   taskContext - task for apply/prune
//   taskName - name of the parent task group, for events
//   opts - options for dry-run
func (p *Pruner) Prune(
	objs object.UnstructuredSet,
	pruneFilters []filter.ValidationFilter,
	taskContext *taskrunner.TaskContext,
	taskName string,
	opts Options,
) error {
	eventFactory := CreateEventFactory(opts.Destroy, taskName)
	// Iterate through objects to prune (delete). If an object is not pruned
	// and we need to keep it in the inventory, we must capture the prune failure.
	for _, obj := range objs {
		id := object.UnstructuredToObjMetaOrDie(obj)
		klog.V(5).Infof("evaluating prune filters (object: %q)", id)
		// Check filters to see if we're prevented from pruning/deleting object.
		var filtered bool
		var reason string
		var err error
		for _, pruneFilter := range pruneFilters {
			klog.V(6).Infof("evaluating prune filter (object: %q, filter: %q)", id, pruneFilter.Name())
			filtered, reason, err = pruneFilter.Filter(obj)
			if err != nil {
				if klog.V(5).Enabled() {
					klog.Errorf("error during %s, (%s): %v", pruneFilter.Name(), id, err)
				}
				taskContext.SendEvent(eventFactory.CreateFailedEvent(id, err))
				taskContext.AddFailedDelete(id)
				break
			}
			if filtered {
				klog.V(4).Infof("prune skipped (object: %q, filter: %q): %v", id, pruneFilter.Name(), reason)
				// If deletion was prevented, remove the inventory annotation.
				if pruneFilter.Name() == filter.PreventRemoveFilterName {
					if !opts.DryRunStrategy.ClientOrServerDryRun() {
						obj, err = p.removeInventoryAnnotation(obj)
						if err != nil {
							if klog.V(4).Enabled() {
								klog.Errorf("error removing annotation (object: %q, annotation: %q): %v", id, inventory.OwningInventoryKey, err)
							}
							taskContext.SendEvent(eventFactory.CreateFailedEvent(id, err))
							taskContext.AddFailedDelete(id)
							break
						} else {
							// Inventory annotation was successfully removed from the object.
							// Register for removal from the inventory.
							taskContext.AddAbandonedObject(id)
						}
					}
				}
				taskContext.SendEvent(eventFactory.CreateSkippedEvent(obj, reason))
				taskContext.AddSkippedDelete(id)
				break
			}
		}
		if filtered || err != nil {
			continue
		}
		// Filters passed--actually delete object if not dry run.
		if !opts.DryRunStrategy.ClientOrServerDryRun() {
			klog.V(4).Infof("deleting object (object: %q)", id)
			err := p.deleteObject(id, metav1.DeleteOptions{
				PropagationPolicy: &opts.PropagationPolicy,
			})
			if err != nil {
				if klog.V(4).Enabled() {
					klog.Errorf("error deleting object (object: %q): %v", id, err)
				}
				taskContext.SendEvent(eventFactory.CreateFailedEvent(id, err))
				taskContext.AddFailedDelete(id)
				continue
			}
		}
		taskContext.SendEvent(eventFactory.CreateSuccessEvent(obj))
	}
	return nil
}

// removeInventoryAnnotation removes the `config.k8s.io/owning-inventory` annotation from pruneObj.
func (p *Pruner) removeInventoryAnnotation(obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	// Make a copy of the input object to avoid modifying the input.
	// This prevents race conditions when writing to the underlying map.
	obj = obj.DeepCopy()
	id := object.UnstructuredToObjMetaOrDie(obj)
	annotations := obj.GetAnnotations()
	if annotations != nil {
		if _, ok := annotations[inventory.OwningInventoryKey]; ok {
			klog.V(4).Infof("removing annotation (object: %q, annotation: %q)", id, inventory.OwningInventoryKey)
			delete(annotations, inventory.OwningInventoryKey)
			obj.SetAnnotations(annotations)
			namespacedClient, err := p.namespacedClient(id)
			if err != nil {
				return obj, err
			}
			_, err = namespacedClient.Update(context.TODO(), obj, metav1.UpdateOptions{})
			return obj, err
		}
	}
	return obj, nil
}

// GetPruneObjs calculates the set of prune objects, and retrieves them
// from the cluster. Set of prune objects equals the set of inventory
// objects minus the set of currently applied objects. Returns an error
// if one occurs.
func (p *Pruner) GetPruneObjs(
	inv inventory.InventoryInfo,
	objs object.UnstructuredSet,
	opts Options,
) (object.UnstructuredSet, error) {
	ids := object.UnstructuredsToObjMetasOrDie(objs)
	invIDs, err := p.InvClient.GetClusterObjs(inv, opts.DryRunStrategy)
	if err != nil {
		return nil, err
	}
	// only return objects that were in the inventory but not in the object set
	ids = invIDs.Diff(ids)
	objs = object.UnstructuredSet{}
	for _, id := range ids {
		pruneObj, err := p.getObject(id)
		if err != nil {
			if meta.IsNoMatchError(err) {
				klog.V(4).Infof("skip pruning (object: %q): resource type not registered", id)
				continue
			}
			if apierrors.IsNotFound(err) {
				klog.V(4).Infof("skip pruning (object: %q): resource not found", id)
				continue
			}
			return nil, err
		}
		objs = append(objs, pruneObj)
	}
	sort.Sort(sort.Reverse(ordering.SortableUnstructureds(objs)))
	return objs, nil
}

func (p *Pruner) getObject(id object.ObjMetadata) (*unstructured.Unstructured, error) {
	namespacedClient, err := p.namespacedClient(id)
	if err != nil {
		return nil, err
	}
	return namespacedClient.Get(context.TODO(), id.Name, metav1.GetOptions{})
}

func (p *Pruner) deleteObject(id object.ObjMetadata, opts metav1.DeleteOptions) error {
	namespacedClient, err := p.namespacedClient(id)
	if err != nil {
		return err
	}
	return namespacedClient.Delete(context.TODO(), id.Name, opts)
}

func (p *Pruner) namespacedClient(id object.ObjMetadata) (dynamic.ResourceInterface, error) {
	mapping, err := p.Mapper.RESTMapping(id.GroupKind)
	if err != nil {
		return nil, err
	}
	return p.Client.Resource(mapping.Resource).Namespace(id.Namespace), nil
}
