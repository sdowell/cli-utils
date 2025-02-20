// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func TestGetClusterInventoryInfo(t *testing.T) {
	tests := map[string]struct {
		inv       InventoryInfo
		localObjs object.ObjMetadataSet
		isError   bool
	}{
		"Nil local inventory object is an error": {
			inv:       nil,
			localObjs: object.ObjMetadataSet{},
			isError:   true,
		},
		"Empty local inventory object": {
			inv:       localInv,
			localObjs: object.ObjMetadataSet{},
			isError:   false,
		},
		"Local inventory with a single object": {
			inv: localInv,
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod2Info),
			},
			isError: false,
		},
		"Local inventory with multiple objects": {
			inv: localInv,
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod2Info),
				ignoreErrInfoToObjMeta(pod3Info)},
			isError: false,
		},
	}

	tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
	defer tf.Cleanup()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			invClient, err := NewInventoryClient(tf,
				WrapInventoryObj, InvInfoToConfigMap)
			require.NoError(t, err)
			fakeBuilder := FakeBuilder{}
			fakeBuilder.SetInventoryObjs(tc.localObjs)
			invClient.builderFunc = fakeBuilder.GetBuilder()
			var inv *unstructured.Unstructured
			if tc.inv != nil {
				inv = storeObjsInInventory(tc.inv, tc.localObjs)
			}
			clusterInv, err := invClient.GetClusterInventoryInfo(WrapInventoryInfoObj(inv), common.DryRunNone)
			if tc.isError {
				if err == nil {
					t.Fatalf("expected error but received none")
				}
				return
			}
			if !tc.isError && err != nil {
				t.Fatalf("unexpected error received: %s", err)
			}
			if clusterInv != nil {
				wrapped := WrapInventoryObj(clusterInv)
				clusterObjs, err := wrapped.Load()
				if err != nil {
					t.Fatalf("unexpected error received: %s", err)
				}
				if !tc.localObjs.Equal(clusterObjs) {
					t.Fatalf("expected cluster objs (%v), got (%v)", tc.localObjs, clusterObjs)
				}
			}
		})
	}
}

func TestMerge(t *testing.T) {
	tests := map[string]struct {
		localInv    InventoryInfo
		localObjs   object.ObjMetadataSet
		clusterObjs object.ObjMetadataSet
		pruneObjs   object.ObjMetadataSet
		isError     bool
	}{
		"Nil local inventory object is error": {
			localInv:    nil,
			localObjs:   object.ObjMetadataSet{},
			clusterObjs: object.ObjMetadataSet{},
			pruneObjs:   object.ObjMetadataSet{},
			isError:     true,
		},
		"Cluster and local inventories empty: no prune objects; no change": {
			localInv:    copyInventory(),
			localObjs:   object.ObjMetadataSet{},
			clusterObjs: object.ObjMetadataSet{},
			pruneObjs:   object.ObjMetadataSet{},
			isError:     false,
		},
		"Cluster and local inventories same: no prune objects; no change": {
			localInv: copyInventory(),
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			clusterObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			pruneObjs: object.ObjMetadataSet{},
			isError:   false,
		},
		"Cluster two obj, local one: prune obj": {
			localInv: copyInventory(),
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			clusterObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod3Info),
			},
			pruneObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod3Info),
			},
			isError: false,
		},
		"Cluster multiple objs, local multiple different objs: prune objs": {
			localInv: copyInventory(),
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod2Info),
			},
			clusterObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod2Info),
				ignoreErrInfoToObjMeta(pod3Info)},
			pruneObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod3Info),
			},
			isError: false,
		},
	}

	tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
	defer tf.Cleanup()

	for name, tc := range tests {
		for i := range common.Strategies {
			drs := common.Strategies[i]
			t.Run(name, func(t *testing.T) {
				// Create the local inventory object storing "tc.localObjs"
				invClient, err := NewInventoryClient(tf,
					WrapInventoryObj, InvInfoToConfigMap)
				require.NoError(t, err)
				// Create a fake builder to return "tc.clusterObjs" from
				// the cluster inventory object.
				fakeBuilder := FakeBuilder{}
				fakeBuilder.SetInventoryObjs(tc.clusterObjs)
				invClient.builderFunc = fakeBuilder.GetBuilder()
				// Call "Merge" to create the union of clusterObjs and localObjs.
				pruneObjs, err := invClient.Merge(tc.localInv, tc.localObjs, drs)
				if tc.isError {
					if err == nil {
						t.Fatalf("expected error but received none")
					}
					return
				}
				if !tc.isError && err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if !tc.pruneObjs.Equal(pruneObjs) {
					t.Errorf("expected (%v) prune objs; got (%v)", tc.pruneObjs, pruneObjs)
				}
			})
		}
	}
}

func TestCreateInventory(t *testing.T) {
	tests := map[string]struct {
		inv       InventoryInfo
		localObjs object.ObjMetadataSet
		error     string
	}{
		"Nil local inventory object is an error": {
			inv:       nil,
			localObjs: object.ObjMetadataSet{},
			error:     "attempting create a nil inventory object",
		},
		"Empty local inventory object": {
			inv:       localInv,
			localObjs: object.ObjMetadataSet{},
		},
		"Local inventory with a single object": {
			inv: localInv,
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod2Info),
			},
		},
		"Local inventory with multiple objects": {
			inv: localInv,
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod2Info),
				ignoreErrInfoToObjMeta(pod3Info)},
		},
	}

	tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
	defer tf.Cleanup()

	// The fake client must see a POST to the confimap URL.
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: resource.UnstructuredPlusDefaultContentConfig().NegotiatedSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			if req.Method == "POST" && cmPathRegex.Match([]byte(req.URL.Path)) {
				b, err := ioutil.ReadAll(req.Body)
				if err != nil {
					return nil, err
				}
				cm := corev1.ConfigMap{}
				err = runtime.DecodeInto(codec, b, &cm)
				if err != nil {
					return nil, err
				}
				bodyRC := ioutil.NopCloser(bytes.NewReader(b))
				return &http.Response{StatusCode: http.StatusCreated, Header: cmdtesting.DefaultHeader(), Body: bodyRC}, nil
			}
			return nil, nil
		}),
	}
	tf.ClientConfigVal = cmdtesting.DefaultClientConfig()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			invClient, err := NewInventoryClient(tf,
				WrapInventoryObj, InvInfoToConfigMap)
			require.NoError(t, err)
			inv := invClient.invToUnstructuredFunc(tc.inv)
			if inv != nil {
				inv = storeObjsInInventory(tc.inv, tc.localObjs)
			}
			err = invClient.createInventoryObj(inv, common.DryRunNone)
			if tc.error != "" {
				assert.EqualError(t, err, tc.error)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestReplace(t *testing.T) {
	tests := map[string]struct {
		localObjs   object.ObjMetadataSet
		clusterObjs object.ObjMetadataSet
	}{
		"Cluster and local inventories empty": {
			localObjs:   object.ObjMetadataSet{},
			clusterObjs: object.ObjMetadataSet{},
		},
		"Cluster and local inventories same": {
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			clusterObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
		},
		"Cluster two obj, local one": {
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			clusterObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod3Info),
			},
		},
		"Cluster multiple objs, local multiple different objs": {
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod2Info),
			},
			clusterObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod2Info),
				ignoreErrInfoToObjMeta(pod3Info)},
		},
	}

	tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
	defer tf.Cleanup()

	// Client and server dry-run do not throw errors.
	invClient, err := NewInventoryClient(tf, WrapInventoryObj, InvInfoToConfigMap)
	require.NoError(t, err)
	err = invClient.Replace(copyInventory(), object.ObjMetadataSet{}, common.DryRunClient)
	if err != nil {
		t.Fatalf("unexpected error received: %s", err)
	}
	err = invClient.Replace(copyInventory(), object.ObjMetadataSet{}, common.DryRunServer)
	if err != nil {
		t.Fatalf("unexpected error received: %s", err)
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create inventory client, and store the cluster objs in the inventory object.
			invClient, err := NewInventoryClient(tf,
				WrapInventoryObj, InvInfoToConfigMap)
			require.NoError(t, err)
			wrappedInv := invClient.InventoryFactoryFunc(inventoryObj)
			if err := wrappedInv.Store(tc.clusterObjs); err != nil {
				t.Fatalf("unexpected error storing inventory objects: %s", err)
			}
			inv, err := wrappedInv.GetObject()
			if err != nil {
				t.Fatalf("unexpected error storing inventory objects: %s", err)
			}
			// Call replaceInventory with the new set of "localObjs"
			inv, err = invClient.replaceInventory(inv, tc.localObjs)
			if err != nil {
				t.Fatalf("unexpected error received: %s", err)
			}
			wrappedInv = invClient.InventoryFactoryFunc(inv)
			// Validate that the stored objects are now the "localObjs".
			actualObjs, err := wrappedInv.Load()
			if err != nil {
				t.Fatalf("unexpected error received: %s", err)
			}
			if !tc.localObjs.Equal(actualObjs) {
				t.Errorf("expected objects (%s), got (%s)", tc.localObjs, actualObjs)
			}
		})
	}
}

func TestGetClusterObjs(t *testing.T) {
	tests := map[string]struct {
		localInv    InventoryInfo
		clusterObjs object.ObjMetadataSet
		isError     bool
	}{
		"Nil cluster inventory is error": {
			localInv:    nil,
			clusterObjs: object.ObjMetadataSet{},
			isError:     true,
		},
		"No cluster objs": {
			localInv:    copyInventory(),
			clusterObjs: object.ObjMetadataSet{},
			isError:     false,
		},
		"Single cluster obj": {
			localInv:    copyInventory(),
			clusterObjs: object.ObjMetadataSet{ignoreErrInfoToObjMeta(pod1Info)},
			isError:     false,
		},
		"Multiple cluster objs": {
			localInv:    copyInventory(),
			clusterObjs: object.ObjMetadataSet{ignoreErrInfoToObjMeta(pod1Info), ignoreErrInfoToObjMeta(pod3Info)},
			isError:     false,
		},
	}

	tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
	defer tf.Cleanup()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			invClient, err := NewInventoryClient(tf,
				WrapInventoryObj, InvInfoToConfigMap)
			require.NoError(t, err)
			// Create fake builder returning "tc.clusterObjs" from cluster inventory.
			fakeBuilder := FakeBuilder{}
			fakeBuilder.SetInventoryObjs(tc.clusterObjs)
			invClient.builderFunc = fakeBuilder.GetBuilder()
			// Call "GetClusterObjs" and compare returned cluster inventory objs to expected.
			clusterObjs, err := invClient.GetClusterObjs(tc.localInv, common.DryRunNone)
			if tc.isError {
				if err == nil {
					t.Fatalf("expected error but received none")
				}
				return
			}
			if !tc.isError && err != nil {
				t.Fatalf("unexpected error received: %s", err)
			}
			if !tc.clusterObjs.Equal(clusterObjs) {
				t.Errorf("expected (%v) cluster inventory objs; got (%v)", tc.clusterObjs, clusterObjs)
			}
		})
	}
}

func TestDeleteInventoryObj(t *testing.T) {
	tests := map[string]struct {
		inv       InventoryInfo
		localObjs object.ObjMetadataSet
	}{
		"Nil local inventory object is an error": {
			inv:       nil,
			localObjs: object.ObjMetadataSet{},
		},
		"Empty local inventory object": {
			inv:       localInv,
			localObjs: object.ObjMetadataSet{},
		},
		"Local inventory with a single object": {
			inv: localInv,
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod2Info),
			},
		},
		"Local inventory with multiple objects": {
			inv: localInv,
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod2Info),
				ignoreErrInfoToObjMeta(pod3Info)},
		},
	}

	tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
	defer tf.Cleanup()

	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: resource.UnstructuredPlusDefaultContentConfig().NegotiatedSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			if req.Method == "DELETE" && cmPathRegex.Match([]byte(req.URL.Path)) {
				b, err := ioutil.ReadAll(req.Body)
				if err != nil {
					return nil, err
				}
				cm := corev1.ConfigMap{}
				err = runtime.DecodeInto(codec, b, &cm)
				if err != nil {
					return nil, err
				}
				bodyRC := ioutil.NopCloser(bytes.NewReader(b))
				return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: bodyRC}, nil
			}
			return nil, nil
		}),
	}
	tf.ClientConfigVal = cmdtesting.DefaultClientConfig()

	for name, tc := range tests {
		for i := range common.Strategies {
			drs := common.Strategies[i]
			t.Run(name, func(t *testing.T) {
				invClient, err := NewInventoryClient(tf,
					WrapInventoryObj, InvInfoToConfigMap)
				require.NoError(t, err)
				inv := invClient.invToUnstructuredFunc(tc.inv)
				if inv != nil {
					inv = storeObjsInInventory(tc.inv, tc.localObjs)
				}
				err = invClient.deleteInventoryObjByName(inv, drs)
				if err != nil {
					t.Fatalf("unexpected error received: %s", err)
				}
			})
		}
	}
}

type invAndObjs struct {
	inv     InventoryInfo
	invObjs object.ObjMetadataSet
}

func TestMergeInventoryObjs(t *testing.T) {
	pod1Obj := ignoreErrInfoToObjMeta(pod1Info)
	pod2Obj := ignoreErrInfoToObjMeta(pod2Info)
	pod3Obj := ignoreErrInfoToObjMeta(pod3Info)
	tests := map[string]struct {
		invs     []invAndObjs
		expected object.ObjMetadataSet
	}{
		"Single inventory object with no inventory is valid": {
			invs: []invAndObjs{
				{
					inv:     copyInventory(),
					invObjs: object.ObjMetadataSet{},
				},
			},
			expected: object.ObjMetadataSet{},
		},
		"Single inventory object returns same objects": {
			invs: []invAndObjs{
				{
					inv:     copyInventory(),
					invObjs: object.ObjMetadataSet{pod1Obj},
				},
			},
			expected: object.ObjMetadataSet{pod1Obj},
		},
		"Two inventories with the same objects returns them": {
			invs: []invAndObjs{
				{
					inv:     copyInventory(),
					invObjs: object.ObjMetadataSet{pod1Obj},
				},
				{
					inv:     copyInventory(),
					invObjs: object.ObjMetadataSet{pod1Obj},
				},
			},
			expected: object.ObjMetadataSet{pod1Obj},
		},
		"Two inventories with different retain the union": {
			invs: []invAndObjs{
				{
					inv:     copyInventory(),
					invObjs: object.ObjMetadataSet{pod1Obj},
				},
				{
					inv:     copyInventory(),
					invObjs: object.ObjMetadataSet{pod2Obj},
				},
			},
			expected: object.ObjMetadataSet{pod1Obj, pod2Obj},
		},
		"More than two inventory objects retains all objects": {
			invs: []invAndObjs{
				{
					inv:     copyInventory(),
					invObjs: object.ObjMetadataSet{pod1Obj, pod2Obj},
				},
				{
					inv:     copyInventory(),
					invObjs: object.ObjMetadataSet{pod2Obj},
				},
				{
					inv:     copyInventory(),
					invObjs: object.ObjMetadataSet{pod3Obj},
				},
			},
			expected: object.ObjMetadataSet{pod1Obj, pod2Obj, pod3Obj},
		},
	}

	tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
	defer tf.Cleanup()

	for name, tc := range tests {
		for i := range common.Strategies {
			drs := common.Strategies[i]
			t.Run(name, func(t *testing.T) {
				invClient, err := NewInventoryClient(tf,
					WrapInventoryObj, InvInfoToConfigMap)
				require.NoError(t, err)
				inventories := []*unstructured.Unstructured{}
				for _, i := range tc.invs {
					inv := storeObjsInInventory(i.inv, i.invObjs)
					inventories = append(inventories, inv)
				}
				retained, err := invClient.mergeClusterInventory(inventories, drs)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				wrapped := WrapInventoryObj(retained)
				mergedObjs, _ := wrapped.Load()
				if !tc.expected.Equal(mergedObjs) {
					t.Errorf("expected merged inventory objects (%v), got (%v)", tc.expected, mergedObjs)
				}
			})
		}
	}
}

func ignoreErrInfoToObjMeta(info *resource.Info) object.ObjMetadata {
	objMeta, _ := object.InfoToObjMeta(info)
	return objMeta
}
