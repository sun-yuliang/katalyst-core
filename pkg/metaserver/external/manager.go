// Copyright 2022 The Katalyst Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package external

import (
	"context"
	"sync"

	"k8s.io/klog/v2"

	"github.com/kubewharf/katalyst-core/pkg/metaserver/agent/pod"
	"github.com/kubewharf/katalyst-core/pkg/metaserver/external/cgroupid"
	"github.com/kubewharf/katalyst-core/pkg/util/external/network"
	"github.com/kubewharf/katalyst-core/pkg/util/external/rdt"
)

var (
	initManagerOnce sync.Once
	manager         *ExternalManager
)

// ExternalManager contains a set of managers that execute configurations beyond the OCI spec.
type ExternalManager struct {
	mutex sync.Mutex
	start bool
	*cgroupid.CgroupIDManager

	network.NetworkManager
	rdt.RDTManager
}

// InitExternalManager initializes an ExternalManager
func InitExternalManager(podFetcher pod.PodFetcher) *ExternalManager {
	initManagerOnce.Do(func() {
		manager = &ExternalManager{
			start:           false,
			CgroupIDManager: cgroupid.NewCgroupIDManager(podFetcher),
			NetworkManager:  network.NewDefaultManager(),
			RDTManager:      rdt.NewDefaultManager(),
		}
	})

	return manager
}

// Run starts an ExternalManager
func (m *ExternalManager) Run(ctx context.Context) {
	m.Lock()
	if m.start {
		m.Unlock()
		return
	}
	m.start = true

	go m.CgroupIDManager.Run(ctx)

	m.Unlock()
	<-ctx.Done()
}

// SetNetworkManager replaces defaultNetworkManager with a custom implementation
func (m *ExternalManager) SetNetworkManager(n network.NetworkManager) {
	m.setComponentImplementation(func() {
		m.NetworkManager = n
	})
}

func (m *ExternalManager) setComponentImplementation(setter func()) {
	m.Lock()
	defer m.Unlock()

	if m.start {
		klog.Warningf("external manager has already started, not allowed to set implementations")
		return
	}

	setter()
}