/*
Copyright 2022 The Katalyst Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cpu

import (
	"context"
	"io/ioutil"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kubewharf/katalyst-api/pkg/consts"
	katalyst_base "github.com/kubewharf/katalyst-core/cmd/base"
	"github.com/kubewharf/katalyst-core/cmd/katalyst-agent/app/options"
	"github.com/kubewharf/katalyst-core/pkg/agent/qrm-plugins/cpu/dynamicpolicy/state"
	"github.com/kubewharf/katalyst-core/pkg/agent/sysadvisor/metacache"
	"github.com/kubewharf/katalyst-core/pkg/agent/sysadvisor/types"
	"github.com/kubewharf/katalyst-core/pkg/config"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metaserver/agent"
	"github.com/kubewharf/katalyst-core/pkg/metaserver/agent/metric"
	"github.com/kubewharf/katalyst-core/pkg/metaserver/agent/pod"
	"github.com/kubewharf/katalyst-core/pkg/metaserver/spd"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	"github.com/kubewharf/katalyst-core/pkg/util/machine"
)

func generateTestConfiguration(t *testing.T, checkpointDir, stateFileDir string) *config.Configuration {
	conf, err := options.NewOptions().Config()
	require.NoError(t, err)
	require.NotNil(t, conf)

	conf.GenericSysAdvisorConfiguration.StateFileDirectory = stateFileDir
	conf.MetaServerConfiguration.CheckpointManagerDir = checkpointDir

	return conf
}

func newTestCPUResourceAdvisor(t *testing.T, pods []*v1.Pod, checkpointDir, stateFileDir string) (*cpuResourceAdvisor, metacache.MetaCache) {
	conf := generateTestConfiguration(t, checkpointDir, stateFileDir)

	metaCache, err := metacache.NewMetaCacheImp(conf, metric.NewFakeMetricsFetcher(metrics.DummyMetrics{}))
	require.NoError(t, err)

	// numa node0 cpu(s): 0-23,48-71
	// numa node1 cpu(s): 24-47,72-95
	cpuTopology, err := machine.GenerateDummyCPUTopology(96, 2, 2)
	assert.NoError(t, err)

	genericCtx, err := katalyst_base.GenerateFakeGenericContext([]runtime.Object{})
	require.NoError(t, err)

	metaServer, err := metaserver.NewMetaServer(genericCtx.Client, metrics.DummyMetrics{}, conf)
	require.NoError(t, err)

	metaServer.MetaAgent = &agent.MetaAgent{
		KatalystMachineInfo: &machine.KatalystMachineInfo{
			CPUTopology: cpuTopology,
		},
		PodFetcher: &pod.PodFetcherStub{
			PodList: pods,
		},
	}

	err = metaServer.SetServiceProfilingManager(&spd.DummyServiceProfilingManager{})
	require.NoError(t, err)

	cra := NewCPUResourceAdvisor(conf, struct{}{}, metaCache, metaServer, nil)
	require.NotNil(t, cra)

	return cra, metaCache
}

func makeContainerInfo(podUID, namespace, podName, containerName, qoSLevel, ownerPoolName string, annotations map[string]string,
	topologyAwareAssignments types.TopologyAwareAssignment, cpuRequest float64) *types.ContainerInfo {
	return &types.ContainerInfo{
		PodUID:                           podUID,
		PodNamespace:                     namespace,
		PodName:                          podName,
		ContainerName:                    containerName,
		ContainerIndex:                   0,
		Labels:                           nil,
		Annotations:                      annotations,
		QoSLevel:                         qoSLevel,
		CPURequest:                       cpuRequest,
		MemoryRequest:                    0,
		RampUp:                           false,
		OwnerPoolName:                    ownerPoolName,
		TopologyAwareAssignments:         topologyAwareAssignments,
		OriginalTopologyAwareAssignments: topologyAwareAssignments,
	}
}

func TestAdvisorUpdate(t *testing.T) {
	tests := []struct {
		name                          string
		pools                         map[string]*types.PoolInfo
		containers                    []*types.ContainerInfo
		pods                          []*v1.Pod
		enableReclaim                 bool
		wantInternalCalculationResult types.InternalCalculationResult
		wantHeadroom                  resource.Quantity
	}{
		{
			name:                          "missing_reserve_pool",
			pools:                         map[string]*types.PoolInfo{},
			wantInternalCalculationResult: types.InternalCalculationResult{},
			wantHeadroom:                  resource.Quantity{},
		},
		{
			name: "provision:reserve_pool_only",
			pools: map[string]*types.PoolInfo{
				state.PoolNameReserve: {
					PoolName: state.PoolNameReserve,
					TopologyAwareAssignments: map[int]machine.CPUSet{
						0: machine.MustParse("0"),
						1: machine.MustParse("24"),
					},
				},
			},
			enableReclaim: true,
			wantInternalCalculationResult: types.InternalCalculationResult{
				PoolEntries: map[string]map[int]int{
					state.PoolNameReserve: {-1: 2},
					state.PoolNameReclaim: {-1: 94},
				},
			},
			wantHeadroom: resource.Quantity{},
		},
		{
			name: "provision:single_small_share_pool",
			pools: map[string]*types.PoolInfo{
				state.PoolNameReserve: {
					PoolName: state.PoolNameReserve,
					TopologyAwareAssignments: map[int]machine.CPUSet{
						0: machine.MustParse("0"),
						1: machine.MustParse("24"),
					},
				},
				state.PoolNameShare: {
					PoolName: state.PoolNameShare,
					TopologyAwareAssignments: map[int]machine.CPUSet{
						0: machine.MustParse("1"),
						1: machine.MustParse("25"),
					},
				},
			},
			containers: []*types.ContainerInfo{
				makeContainerInfo("uid1", "default", "pod1", "c1", consts.PodAnnotationQoSLevelSharedCores, state.PoolNameShare, nil,
					map[int]machine.CPUSet{
						0: machine.MustParse("1"),
						1: machine.MustParse("25"),
					}, 4),
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
						UID:       "uid1",
					},
				},
			},
			enableReclaim: true,
			wantInternalCalculationResult: types.InternalCalculationResult{
				PoolEntries: map[string]map[int]int{
					state.PoolNameReserve: {-1: 2},
					state.PoolNameShare:   {-1: 8},
					state.PoolNameReclaim: {-1: 86},
				},
			},
			wantHeadroom: resource.Quantity{},
		},
		{
			name: "provision:single_large_share_pool",
			pools: map[string]*types.PoolInfo{
				state.PoolNameReserve: {
					PoolName: state.PoolNameReserve,
					TopologyAwareAssignments: map[int]machine.CPUSet{
						0: machine.MustParse("0"),
						1: machine.MustParse("24"),
					},
				},
				state.PoolNameShare: {
					PoolName: state.PoolNameShare,
					TopologyAwareAssignments: map[int]machine.CPUSet{
						0: machine.MustParse("1-23,48-71"),
						1: machine.MustParse("25-47,72-95"),
					},
				},
			},
			containers: []*types.ContainerInfo{
				makeContainerInfo("uid1", "default", "pod1", "c1", consts.PodAnnotationQoSLevelSharedCores, state.PoolNameShare, nil,
					map[int]machine.CPUSet{
						0: machine.MustParse("1-22,48-70"),
						1: machine.MustParse("25-46,72-94"),
					}, 100),
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
						UID:       "uid1",
					},
				},
			},
			enableReclaim: true,
			wantInternalCalculationResult: types.InternalCalculationResult{
				PoolEntries: map[string]map[int]int{
					state.PoolNameReserve: {-1: 2},
					state.PoolNameShare:   {-1: 90},
					state.PoolNameReclaim: {-1: 4},
				},
			},
			wantHeadroom: resource.Quantity{},
		},
		{
			name: "provision:multi_small_share_pools",
			pools: map[string]*types.PoolInfo{
				state.PoolNameReserve: {
					PoolName: state.PoolNameReserve,
					TopologyAwareAssignments: map[int]machine.CPUSet{
						0: machine.MustParse("0"),
						1: machine.MustParse("24"),
					},
				},
				state.PoolNameShare: {
					PoolName: state.PoolNameShare,
					TopologyAwareAssignments: map[int]machine.CPUSet{
						0: machine.MustParse("1"),
						1: machine.MustParse("25"),
					},
				},
				"batch": {
					PoolName: "batch",
					TopologyAwareAssignments: map[int]machine.CPUSet{
						0: machine.MustParse("2"),
						1: machine.MustParse("26"),
					},
				},
			},
			containers: []*types.ContainerInfo{
				makeContainerInfo("uid1", "default", "pod1", "c1", consts.PodAnnotationQoSLevelSharedCores, state.PoolNameShare, nil,
					map[int]machine.CPUSet{
						0: machine.MustParse("1"),
						1: machine.MustParse("25"),
					}, 4),
				makeContainerInfo("uid2", "default", "pod2", "c2", consts.PodAnnotationQoSLevelSharedCores, "batch", nil,
					map[int]machine.CPUSet{
						0: machine.MustParse("2"),
						1: machine.MustParse("26"),
					}, 6),
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
						UID:       "uid1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "default",
						UID:       "uid2",
					},
				},
			},
			enableReclaim: true,
			wantInternalCalculationResult: types.InternalCalculationResult{
				PoolEntries: map[string]map[int]int{
					state.PoolNameReserve: {-1: 2},
					state.PoolNameShare:   {-1: 6},
					"batch":               {-1: 8},
					state.PoolNameReclaim: {-1: 80},
				},
			},
			wantHeadroom: resource.Quantity{},
		},
		{
			name: "provision:multi_large_share_pools",
			pools: map[string]*types.PoolInfo{
				state.PoolNameReserve: {
					PoolName: state.PoolNameReserve,
					TopologyAwareAssignments: map[int]machine.CPUSet{
						0: machine.MustParse("0"),
						1: machine.MustParse("24"),
					},
				},
				state.PoolNameShare: {
					PoolName: state.PoolNameShare,
					TopologyAwareAssignments: map[int]machine.CPUSet{
						0: machine.MustParse("1-5,48-52"),
						1: machine.MustParse("25-29,72-76"),
					},
				},
				"batch": {
					PoolName: "batch",
					TopologyAwareAssignments: map[int]machine.CPUSet{
						0: machine.MustParse("1-12,48-60"),
						1: machine.MustParse("25-36,72-84"),
					},
				},
			},
			containers: []*types.ContainerInfo{
				makeContainerInfo("uid1", "default", "pod1", "c1", consts.PodAnnotationQoSLevelSharedCores, state.PoolNameShare, nil,
					map[int]machine.CPUSet{
						0: machine.MustParse("1-5,48-52"),
						1: machine.MustParse("25-29,72-76"),
					}, 100),
				makeContainerInfo("uid2", "default", "pod2", "c2", consts.PodAnnotationQoSLevelSharedCores, "batch", nil,
					map[int]machine.CPUSet{
						0: machine.MustParse("1-12,48-60"),
						1: machine.MustParse("25-36,72-84"),
					}, 200),
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
						UID:       "uid1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "default",
						UID:       "uid2",
					},
				},
			},
			enableReclaim: true,
			wantInternalCalculationResult: types.InternalCalculationResult{
				PoolEntries: map[string]map[int]int{
					state.PoolNameReserve: {-1: 2},
					state.PoolNameShare:   {-1: 30},
					"batch":               {-1: 60},
					state.PoolNameReclaim: {-1: 4},
				},
			},
			wantHeadroom: resource.Quantity{},
		},
		{
			name: "provision:single_dedicated_numa_exclusive",
			pools: map[string]*types.PoolInfo{
				state.PoolNameReserve: {
					PoolName: state.PoolNameReserve,
					TopologyAwareAssignments: map[int]machine.CPUSet{
						0: machine.MustParse("0"),
						1: machine.MustParse("24"),
					},
				},
			},
			containers: []*types.ContainerInfo{
				makeContainerInfo("uid1", "default", "pod1", "c1", consts.PodAnnotationQoSLevelDedicatedCores, state.PoolNameDedicated,
					map[string]string{consts.PodAnnotationMemoryEnhancementNumaBinding: consts.PodAnnotationMemoryEnhancementNumaBindingEnable},
					map[int]machine.CPUSet{
						0: machine.MustParse("1-23,48-71"),
					}, 48),
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
						UID:       "uid1",
					},
				},
			},
			enableReclaim: true,
			wantInternalCalculationResult: types.InternalCalculationResult{
				PoolEntries: map[string]map[int]int{
					state.PoolNameReserve: {
						-1: 2,
					},
					state.PoolNameReclaim: {
						0:  2,
						-1: 47,
					},
				},
			},
			wantHeadroom: resource.Quantity{},
		},
		{
			name: "dedicated_numa_exclusive_&_share",
			pools: map[string]*types.PoolInfo{
				state.PoolNameReserve: {
					PoolName: state.PoolNameReserve,
					TopologyAwareAssignments: map[int]machine.CPUSet{
						0: machine.MustParse("0"),
						1: machine.MustParse("24"),
					},
				},
				state.PoolNameShare: {
					PoolName: state.PoolNameShare,
					TopologyAwareAssignments: map[int]machine.CPUSet{
						1: machine.MustParse("25-30"),
					},
				},
				state.PoolNameReclaim: {
					PoolName: state.PoolNameReclaim,
					TopologyAwareAssignments: map[int]machine.CPUSet{
						0: machine.MustParse("70-71"),
						1: machine.MustParse("31-47,72-95"),
					},
				},
			},
			containers: []*types.ContainerInfo{
				makeContainerInfo("uid1", "default", "pod1", "c1", consts.PodAnnotationQoSLevelDedicatedCores, state.PoolNameDedicated,
					map[string]string{consts.PodAnnotationMemoryEnhancementNumaBinding: consts.PodAnnotationMemoryEnhancementNumaBindingEnable},
					map[int]machine.CPUSet{
						0: machine.MustParse("1-23,48-71"),
					}, 48),
				makeContainerInfo("uid2", "default", "pod2", "c2", consts.PodAnnotationQoSLevelSharedCores, state.PoolNameShare, nil,
					map[int]machine.CPUSet{
						1: machine.MustParse("25-28"),
					}, 4),
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
						UID:       "uid1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "default",
						UID:       "uid2",
					},
				},
			},
			enableReclaim: true,
			wantInternalCalculationResult: types.InternalCalculationResult{
				PoolEntries: map[string]map[int]int{
					state.PoolNameReserve: {
						-1: 2,
					},
					state.PoolNameShare: {-1: 6},
					state.PoolNameReclaim: {
						0:  2,
						-1: 41,
					},
				},
			},
			wantHeadroom: *resource.NewQuantity(43, resource.DecimalSI),
		},
		{
			name: "dedicated_numa_exclusive_&_share_disable_reclaim",
			pools: map[string]*types.PoolInfo{
				state.PoolNameReserve: {
					PoolName: state.PoolNameReserve,
					TopologyAwareAssignments: map[int]machine.CPUSet{
						0: machine.MustParse("0"),
						1: machine.MustParse("24"),
					},
				},
				state.PoolNameShare: {
					PoolName: state.PoolNameShare,
					TopologyAwareAssignments: map[int]machine.CPUSet{
						1: machine.MustParse("25-30"),
					},
				},
				state.PoolNameReclaim: {
					PoolName: state.PoolNameReclaim,
					TopologyAwareAssignments: map[int]machine.CPUSet{
						0: machine.MustParse("70-71"),
						1: machine.MustParse("31-47,72-95"),
					},
				},
			},
			containers: []*types.ContainerInfo{
				makeContainerInfo("uid1", "default", "pod1", "c1", consts.PodAnnotationQoSLevelDedicatedCores, state.PoolNameDedicated,
					map[string]string{consts.PodAnnotationMemoryEnhancementNumaBinding: consts.PodAnnotationMemoryEnhancementNumaBindingEnable},
					map[int]machine.CPUSet{
						0: machine.MustParse("1-23,48-71"),
					}, 48),
				makeContainerInfo("uid2", "default", "pod2", "c2", consts.PodAnnotationQoSLevelSharedCores, state.PoolNameShare, nil,
					map[int]machine.CPUSet{
						1: machine.MustParse("25-28"),
					}, 4),
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
						UID:       "uid1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "default",
						UID:       "uid2",
					},
				},
			},
			enableReclaim: false,
			wantInternalCalculationResult: types.InternalCalculationResult{
				PoolEntries: map[string]map[int]int{
					state.PoolNameReserve: {
						-1: 2,
					},
					state.PoolNameShare: {-1: 45},
					state.PoolNameReclaim: {
						0:  2,
						-1: 2,
					},
				},
			},
			wantHeadroom: *resource.NewQuantity(0, resource.DecimalSI),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ckDir, err := ioutil.TempDir("", "checkpoint")
			require.NoError(t, err)
			defer os.RemoveAll(ckDir)

			sfDir, err := ioutil.TempDir("", "statefile")
			require.NoError(t, err)
			defer os.RemoveAll(sfDir)

			advisor, metaCache := newTestCPUResourceAdvisor(t, tt.pods, ckDir, sfDir)
			advisor.startTime = time.Now().Add(-types.StartUpPeriod * 2)
			advisor.conf.ReclaimedResourceConfiguration.SetEnableReclaim(tt.enableReclaim)

			recvChInterface, sendChInterface := advisor.GetChannels()
			recvCh := recvChInterface.(chan struct{})
			sendCh := sendChInterface.(chan types.InternalCalculationResult)

			for poolName, poolInfo := range tt.pools {
				_ = metaCache.SetPoolInfo(poolName, poolInfo)
			}
			for _, c := range tt.containers {
				_ = metaCache.SetContainerInfo(c.PodUID, c.ContainerName, c)
			}

			ctx, cancel := context.WithCancel(context.Background())
			go advisor.Run(ctx)

			// trigger advisor update
			recvCh <- struct{}{}

			// check provision
			if !reflect.DeepEqual(tt.wantInternalCalculationResult, types.InternalCalculationResult{}) {
				advisorResp := <-sendCh
				if !reflect.DeepEqual(tt.wantInternalCalculationResult, advisorResp) {
					t.Errorf("cpu provision\nexpected: %+v,\nactual: %+v", tt.wantInternalCalculationResult, advisorResp)
				}

			}

			// check headroom
			if !reflect.DeepEqual(tt.wantHeadroom, resource.Quantity{}) {
				headroom, err := advisor.GetHeadroom()
				assert.NoError(t, err)
				if !reflect.DeepEqual(tt.wantHeadroom.MilliValue(), headroom.MilliValue()) {
					t.Errorf("headroom\nexpected: %+v\nactual: %+v", tt.wantHeadroom, headroom)
				}
			}

			cancel()
		})
	}
}
