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

package types

import (
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubelet/pkg/apis/resourceplugin/v1alpha1"

	"github.com/kubewharf/katalyst-core/pkg/util/machine"
)

const (
	AdvisorPluginNameQoSAware      = "qos_aware"
	AdvisorPluginNameMetaCache     = "metacache"
	AdvisorPluginNameMetricEmitter = "metric_emitter"
)

// QoSResourceName describes different resources under qos aware control
type QoSResourceName string

const (
	QoSResourceCPU    QoSResourceName = "cpu"
	QoSResourceMemory QoSResourceName = "memory"
)

// CPUProvisionPolicyName defines policy names for cpu advisor resource provision
type CPUProvisionPolicyName string

const (
	CPUProvisionPolicyNone      CPUProvisionPolicyName = "none"
	CPUProvisionPolicyCanonical CPUProvisionPolicyName = "canonical"
	CPUProvisionPolicyRama      CPUProvisionPolicyName = "rama"
)

// CPUHeadroomPolicyName defines policy names for cpu advisor headroom estimation
type CPUHeadroomPolicyName string

const (
	CPUHeadroomPolicyNone        CPUHeadroomPolicyName = "none"
	CPUHeadroomPolicyCanonical   CPUHeadroomPolicyName = "canonical"
	CPUHeadroomPolicyUtilization CPUHeadroomPolicyName = "utilization"
)

// CPUProvisionAssemblerName defines assemblers for cpu advisor to generate node
// provision result from region control knobs
type CPUProvisionAssemblerName string

const (
	CPUProvisionAssemblerNone   CPUProvisionAssemblerName = "none"
	CPUProvisionAssemblerCommon CPUProvisionAssemblerName = "common"
)

// CPUHeadroomAssemblerName defines assemblers for cpu advisor to generate node
// headroom from region headroom or node level policy
type CPUHeadroomAssemblerName string

const (
	CPUHeadroomAssemblerNone   CPUHeadroomAssemblerName = "none"
	CPUHeadroomAssemblerCommon CPUHeadroomAssemblerName = "common"
)

// MemoryHeadroomPolicyName defines policy names for memory advisor headroom estimation
type MemoryHeadroomPolicyName string

const (
	MemoryHeadroomPolicyNone      MemoryHeadroomPolicyName = "none"
	MemoryHeadroomPolicyCanonical MemoryHeadroomPolicyName = "canonical"
)

// QoSRegionType declares pre-defined region types
type QoSRegionType string

const (
	// QoSRegionTypeShare for each share pool
	QoSRegionTypeShare QoSRegionType = "share"

	// QoSRegionTypeDedicatedNumaExclusive for each dedicated core with numa binding
	// and numa exclusive container
	QoSRegionTypeDedicatedNumaExclusive QoSRegionType = "dedicated-numa-exclusive"

	// QoSRegionTypeEmpty works as a wrapper for empty numas
	QoSRegionTypeEmpty QoSRegionType = "empty"
)

type TopologyAwareAssignment map[int]machine.CPUSet

// ContainerInfo contains container information for sysadvisor plugins
type ContainerInfo struct {
	// Metadata unchanged during container's lifecycle
	PodUID         string
	PodNamespace   string
	PodName        string
	ContainerName  string
	ContainerType  v1alpha1.ContainerType
	ContainerIndex int
	Labels         map[string]string
	Annotations    map[string]string
	QoSLevel       string
	CPURequest     float64
	MemoryRequest  float64

	// Allocation information changing by list and watch
	RampUp                           bool
	OwnerPoolName                    string
	TopologyAwareAssignments         TopologyAwareAssignment
	OriginalTopologyAwareAssignments TopologyAwareAssignment
	RegionNames                      sets.String
}

// PoolInfo contains pool information for sysadvisor plugins
type PoolInfo struct {
	PoolName                         string
	TopologyAwareAssignments         TopologyAwareAssignment
	OriginalTopologyAwareAssignments TopologyAwareAssignment
	RegionNames                      sets.String
}

// RegionInfo contains region information generated by sysadvisor resource advisor
type RegionInfo struct {
	RegionType   QoSRegionType  `json:"region_type"`
	BindingNumas machine.CPUSet `json:"binding_numas"`

	HeadroomPolicyTopPriority CPUHeadroomPolicyName `json:"headroom_policy_top_priority"`
	HeadroomPolicyInUse       CPUHeadroomPolicyName `json:"headroom_policy_in_use"`
	Headroom                  float64               `json:"headroom"`

	ControlKnobMap             ControlKnob            `json:"control_knob_map"`
	ProvisionPolicyTopPriority CPUProvisionPolicyName `json:"provision_policy_top_priority"`
	ProvisionPolicyInUse       CPUProvisionPolicyName `json:"provision_policy_in_use"`
}

// ContainerEntries stores container info keyed by container name
type ContainerEntries map[string]*ContainerInfo

// PodEntries stores container info keyed by pod uid and container name
type PodEntries map[string]ContainerEntries

// PoolEntries stores pool info keyed by pool name
type PoolEntries map[string]*PoolInfo

// RegionEntries stores region info keyed by region name
type RegionEntries map[string]*RegionInfo

// PodSet stores container names keyed by pod uid
type PodSet map[string]sets.String

// InternalCalculationResult conveys minimal information to cpu server for composing
// calculation result
type InternalCalculationResult struct {
	PoolEntries map[string]map[int]int // map[poolName][numaId]cpuSize
}

// ResourceEssentials defines essential (const) variables, and those variables may be adjusted by KCC
type ResourceEssentials struct {
	EnableReclaim       bool
	ResourceUpperBound  float64
	ResourceLowerBound  float64
	ReservedForAllocate float64
}

// ControlKnob holds tunable system entries affecting indicator metrics
type ControlKnob map[ControlKnobName]ControlKnobValue

// ControlKnobName defines available control knob key for provision policy
type ControlKnobName string

const (
	// ControlKnobNonReclaimedCPUSetSize refers to the cpuset size of the pods with high Qos level including dedicated_cores and shared_cores
	ControlKnobNonReclaimedCPUSetSize ControlKnobName = "non-reclaimed-cpuset-size"

	// ControlKnobReclaimedCPUSupplied refers to the cpu resource could be supplied to the pods with reclaimed_cores QoS level
	ControlKnobReclaimedCPUSupplied ControlKnobName = "reclaimed-cpu-supplied"
)

// ControlKnobValue holds control knob value and action
type ControlKnobValue struct {
	Value  float64
	Action ControlKnobAction
}

// ControlKnobAction defines control knob adjustment actions
type ControlKnobAction string

const (
	ControlKnobActionNone ControlKnobAction = "none"
)

// Indicator holds system metrics related to service stability keyed by metric name
type Indicator map[string]IndicatorValue

// IndicatorValue holds indicator values of different levels
type IndicatorValue struct {
	Current float64
	Target  float64
	High    float64
	Low     float64
}

// PolicyUpdateStatus works as a flag indicating update result
type PolicyUpdateStatus string

const (
	PolicyUpdateSucceeded PolicyUpdateStatus = "succeeded"
	PolicyUpdateFailed    PolicyUpdateStatus = "failed"
)
