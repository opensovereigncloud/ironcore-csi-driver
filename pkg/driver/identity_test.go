// Copyright 2023 OnMetal authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package driver

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	testutils "github.com/onmetal/onmetal-api/utils/testing"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Controller", func() {
	ctx := testutils.SetupContext()
	_, drv := SetupTest(ctx)

	It("should get the correct driver plugin information", func() {
		By("calling GetPluginInfo")
		res, err := drv.GetPluginInfo(ctx, &csi.GetPluginInfoRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(SatisfyAll(
			HaveField("Name", "csi.onmetal.de"),
			HaveField("VendorVersion", "dev"),
		))
	})

	It("should get the correct driver plugin capabilities", func() {
		By("calling GetPluginCapabilities")
		res, err := drv.GetPluginCapabilities(ctx, &csi.GetPluginCapabilitiesRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Capabilities).To(ConsistOf(
			&csi.PluginCapability{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
			&csi.PluginCapability{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_VOLUME_ACCESSIBILITY_CONSTRAINTS,
					},
				},
			},
		))
	})
})
