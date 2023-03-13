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
	"strconv"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	computev1alpha1 "github.com/onmetal/onmetal-api/api/compute/v1alpha1"
	storagev1alpha1 "github.com/onmetal/onmetal-api/api/storage/v1alpha1"
	testutils "github.com/onmetal/onmetal-api/utils/testing"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("Controller", func() {
	ctx := testutils.SetupContext()
	ns, drv := SetupTest(ctx)

	var (
		volume     = &storagev1alpha1.Volume{}
		volumePool = &storagev1alpha1.VolumePool{}
	)

	BeforeEach(func() {
		By("creating a volume pool")
		volumePool = &storagev1alpha1.VolumePool{
			ObjectMeta: metav1.ObjectMeta{
				Name: "volumepool",
			},
			Spec: storagev1alpha1.VolumePoolSpec{
				ProviderID: "foo",
			},
		}
		Expect(k8sClient.Create(ctx, volumePool)).To(Succeed())

		By("creating a volume through the csi driver")
		volSize := int64(5 * 1024 * 1024 * 1024)
		res, err := drv.CreateVolume(ctx, &csi.CreateVolumeRequest{
			Name:          "volume",
			CapacityRange: &csi.CapacityRange{RequiredBytes: volSize},
			VolumeCapabilities: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			Parameters: map[string]string{
				"type":   "slow",
				"fstype": "ext4",
			},
			AccessibilityRequirements: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{
							topologyKey: "volumepool",
						},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							topologyKey: "volumepool",
						},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Volume).To(SatisfyAll(
			HaveField("VolumeId", "volume"),
			HaveField("CapacityBytes", volSize),
			HaveField("AccessibleTopology", ContainElement(
				HaveField("Segments", HaveKeyWithValue("topology.csi.onmetal.de/zone", "volumepool"))),
			),
			HaveField("VolumeContext", SatisfyAll(
				HaveKeyWithValue("volume_id", "volume"),
				HaveKeyWithValue("volume_name", "volume"),
				HaveKeyWithValue("volume_pool", "volumepool"),
				HaveKeyWithValue("fstype", "ext4"),
				HaveKeyWithValue("creation_time", ContainSubstring(strconv.Itoa(time.Now().Year()))))),
		))

		By("patching the volume state to be available")
		volume = &storagev1alpha1.Volume{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      "volume",
			},
		}
		Eventually(Object(volume)).Should(SatisfyAll(
			HaveField("Status.State", storagev1alpha1.VolumeStatePending),
		))
		volumeBase := volume.DeepCopy()
		volume.Status.State = storagev1alpha1.VolumeStateAvailable
		Expect(k8sClient.Status().Patch(ctx, volume, client.MergeFrom(volumeBase))).To(Succeed())
	})

	It("should not assign the volume to a volume pool if the pool is not available", func() {
		By("creating a volume through the csi driver")
		volSize := int64(5 * 1024 * 1024 * 1024)
		res, err := drv.CreateVolume(ctx, &csi.CreateVolumeRequest{
			Name:          "volume-wrong-pool",
			CapacityRange: &csi.CapacityRange{RequiredBytes: volSize},
			VolumeCapabilities: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			Parameters: map[string]string{
				"type":   "slow",
				"fstype": "ext4",
			},
			AccessibilityRequirements: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{
							topologyKey: "foo",
						},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							topologyKey: "foo",
						},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Volume).To(SatisfyAll(
			HaveField("VolumeId", "volume-wrong-pool"),
			HaveField("CapacityBytes", volSize),
			HaveField("AccessibleTopology", ContainElement(
				HaveField("Segments", HaveKeyWithValue("topology.csi.onmetal.de/zone", "foo"))),
			),
			HaveField("VolumeContext", SatisfyAll(
				HaveKeyWithValue("volume_id", "volume-wrong-pool"),
				HaveKeyWithValue("volume_name", "volume-wrong-pool"),
				HaveKeyWithValue("volume_pool", ""),
				HaveKeyWithValue("fstype", "ext4"),
				HaveKeyWithValue("creation_time", ContainSubstring(strconv.Itoa(time.Now().Year()))))),
		))
	})

	It("should delete a volume", func() {
		By("creating a volume through the csi driver")
		volSize := int64(5 * 1024 * 1024 * 1024)
		_, err := drv.CreateVolume(ctx, &csi.CreateVolumeRequest{
			Name:          "volume-to-delete",
			CapacityRange: &csi.CapacityRange{RequiredBytes: volSize},
			VolumeCapabilities: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			Parameters: map[string]string{
				"type":   "slow",
				"fstype": "ext4",
			},
			AccessibilityRequirements: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{
							topologyKey: "volumepool",
						},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							topologyKey: "volumepool",
						},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		By("deleting the volume through the csi driver")
		_, err = drv.DeleteVolume(ctx, &csi.DeleteVolumeRequest{
			VolumeId: "volume-to-delete",
		})
		Expect(err).NotTo(HaveOccurred())

		By("waiting for the volume to be deleted")
		deletedVolume := &storagev1alpha1.Volume{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      "volume-to-delete",
			},
		}
		Eventually(Get(deletedVolume)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("should publish/unpublish a volume on a node", func() {
		By("calling ControllerPublishVolume")
		_, err := drv.ControllerPublishVolume(ctx, &csi.ControllerPublishVolumeRequest{
			VolumeId:         "volume",
			NodeId:           "node",
			VolumeCapability: nil,
			Readonly:         false,
			VolumeContext:    nil,
		})
		// as long as the volume is pending or not available we fail
		Expect(err).To(HaveOccurred())

		By("patching the volume phase to be bound")
		volumeBase := volume.DeepCopy()
		volume.Status.Phase = storagev1alpha1.VolumePhaseBound
		Expect(k8sClient.Status().Patch(ctx, volume, client.MergeFrom(volumeBase))).To(Succeed())

		By("ensuring that the volume attachment is reflected in the machine spec")
		machine := &computev1alpha1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      "node",
			},
		}
		Eventually(Object(machine)).Should(SatisfyAll(
			HaveField("Spec.Volumes", ConsistOf(
				MatchFields(IgnoreMissing|IgnoreExtras, Fields{
					"Name":   Equal("volume-attachment"),
					"Phase":  Equal(computev1alpha1.VolumePhaseBound),
					"Device": Equal(pointer.String("oda")),
					// TODO: validate VolumeSource
				}),
			)),
		))

		By("patching the machine volume status to be available and bound")
		machineBase := machine.DeepCopy()
		machine.Status.Volumes = []computev1alpha1.VolumeStatus{
			{
				Name:  "volume-attachment",
				State: computev1alpha1.VolumeStateAttached,
				Phase: computev1alpha1.VolumePhaseBound,
			},
		}
		Expect(k8sClient.Patch(ctx, machine, client.MergeFrom(machineBase))).To(Succeed())

		By("patching the volume device information")
		volumeBase = volume.DeepCopy()
		volume.Status = storagev1alpha1.VolumeStatus{
			State: storagev1alpha1.VolumeStateAvailable,
			Phase: storagev1alpha1.VolumePhaseBound,
			Conditions: []storagev1alpha1.VolumeCondition{
				{
					Type:   storagev1alpha1.VolumeConditionType(storagev1alpha1.VolumePhaseBound),
					Status: corev1.ConditionTrue,
				},
			},
			Access: &storagev1alpha1.VolumeAccess{
				Handle: "bar",
				VolumeAttributes: map[string]string{
					"WWN": "/dev/disk/by-id/virtio-foo-bar",
				},
			},
		}
		Expect(k8sClient.Patch(ctx, volume, client.MergeFrom(volumeBase))).To(Succeed())

		By("calling ControllerPublishVolume")
		publishRes, err := drv.ControllerPublishVolume(ctx, &csi.ControllerPublishVolumeRequest{
			VolumeId:         "volume",
			NodeId:           "node",
			VolumeCapability: nil,
			Readonly:         false,
			VolumeContext:    nil,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(publishRes.PublishContext).To(Equal(map[string]string{
			ParameterNodeID:     "node",
			ParameterVolumeID:   "volume",
			ParameterDeviceName: "/dev/disk/by-id/virtio-oda-bar",
		}))

		By("calling ControllerUnpublishVolume")
		_, err = drv.ControllerUnpublishVolume(ctx, &csi.ControllerUnpublishVolumeRequest{
			VolumeId: "volume",
			NodeId:   "node",
		})
		Expect(err).NotTo(HaveOccurred())

		By("ensuring that the volume is removed from machine")
		var volumeAttachments []computev1alpha1.Volume
		Eventually(Object(machine)).Should(SatisfyAll(HaveField("Spec.Volumes", volumeAttachments)))
	})

	AfterEach(func() {
		DeferCleanup(k8sClient.Delete, ctx, volume)
		DeferCleanup(k8sClient.Delete, ctx, volumePool)
	})
})
