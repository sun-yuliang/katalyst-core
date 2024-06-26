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

package adminqos

import (
	"github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic/adminqos/advisor"
	"github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic/adminqos/eviction"
	"github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic/adminqos/reclaimedresource"
	"github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic/crd"
)

type AdminQoSConfiguration struct {
	*reclaimedresource.ReclaimedResourceConfiguration
	*eviction.EvictionConfiguration
	*advisor.AdvisorConfiguration
}

func NewAdminQoSConfiguration() *AdminQoSConfiguration {
	return &AdminQoSConfiguration{
		ReclaimedResourceConfiguration: reclaimedresource.NewReclaimedResourceConfiguration(),
		EvictionConfiguration:          eviction.NewEvictionConfiguration(),
		AdvisorConfiguration:           advisor.NewAdvisorConfiguration(),
	}
}

func (c *AdminQoSConfiguration) ApplyConfiguration(conf *crd.DynamicConfigCRD) {
	c.ReclaimedResourceConfiguration.ApplyConfiguration(conf)
	c.EvictionConfiguration.ApplyConfiguration(conf)
	c.AdvisorConfiguration.ApplyConfiguration(conf)
}
