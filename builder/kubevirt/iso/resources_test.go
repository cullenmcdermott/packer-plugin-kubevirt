// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"context"
	"io"
	"strings"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/iso"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	"k8s.io/apimachinery/pkg/runtime"
	fakek8sclient "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	v1 "kubevirt.io/api/core/v1"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	fakecdiclient "kubevirt.io/client-go/containerizeddataimporter/fake"
	"kubevirt.io/client-go/kubecli"
	kubevirtfake "kubevirt.io/client-go/kubevirt/fake"
)

var _ = Describe("Storage Class Integration", func() {
	const (
		namespace          = "test-ns"
		name               = "test-vm"
		buildStorageClass  = "fast-nvme"
		outputStorageClass = "ceph-rbd"
	)

	var (
		ctrl       *gomock.Controller
		kubeClient *fakek8sclient.Clientset
		cdiClient  *fakecdiclient.Clientset
		virtClient kubecli.KubevirtClient
		vmClient   *kubevirtfake.Clientset
		state      *multistep.BasicStateBag
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())

		uiErr := &strings.Builder{}
		ui := &packer.BasicUi{
			Reader:      strings.NewReader(""),
			Writer:      io.Discard,
			ErrorWriter: uiErr,
		}
		state = new(multistep.BasicStateBag)
		state.Put("ui", ui)

		kubeClient = fakek8sclient.NewSimpleClientset()
		cdiClient = fakecdiclient.NewSimpleClientset()
		vmClient = kubevirtfake.NewSimpleClientset()

		kubecli.GetKubevirtClientFromClientConfig = kubecli.GetMockKubevirtClientFromClientConfig
		kubecli.MockKubevirtClientInstance = kubecli.NewMockKubevirtClient(ctrl)
		kubecli.MockKubevirtClientInstance.EXPECT().CoreV1().Return(kubeClient.CoreV1()).AnyTimes()
		kubecli.MockKubevirtClientInstance.EXPECT().
			VirtualMachine(gomock.Any()).
			DoAndReturn(func(ns string) kubecli.VirtualMachineInterface {
				return vmClient.KubevirtV1().VirtualMachines(ns)
			}).AnyTimes()
		kubecli.MockKubevirtClientInstance.EXPECT().CdiClient().Return(cdiClient).AnyTimes()

		virtClient, _ = kubecli.GetKubevirtClientFromClientConfig(nil)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("VirtualMachine resource", func() {
		It("applies storage class to DataVolumeTemplateSpec when specified", func() {
			vm := iso.VirtualMachine(iso.VirtualMachineOptions{
				Name:              name,
				IsoVolumeName:     "iso-vol",
				DiskSize:          "10Gi",
				InstanceType:      "cx1.medium",
				PreferenceName:    "fedora",
				InstanceTypeKind:  "",
				PreferenceKind:    "",
				OSType:            "linux",
				Networks:          nil,
				BuildStorageClass: buildStorageClass,
			})

			Expect(vm).NotTo(BeNil())
			Expect(vm.Spec.DataVolumeTemplates).To(HaveLen(1))
			dvTemplate := vm.Spec.DataVolumeTemplates[0]
			Expect(dvTemplate.Spec.PVC).NotTo(BeNil())
			Expect(dvTemplate.Spec.PVC.StorageClassName).NotTo(BeNil())
			Expect(*dvTemplate.Spec.PVC.StorageClassName).To(Equal(buildStorageClass))
		})

		It("uses default (nil) when no storage class specified", func() {
			vm := iso.VirtualMachine(iso.VirtualMachineOptions{
				Name:              name,
				IsoVolumeName:     "iso-vol",
				DiskSize:          "10Gi",
				InstanceType:      "cx1.medium",
				PreferenceName:    "fedora",
				InstanceTypeKind:  "",
				PreferenceKind:    "",
				OSType:            "linux",
				Networks:          nil,
				BuildStorageClass: "", // empty storage class
			})

			Expect(vm).NotTo(BeNil())
			Expect(vm.Spec.DataVolumeTemplates).To(HaveLen(1))
			dvTemplate := vm.Spec.DataVolumeTemplates[0]
			Expect(dvTemplate.Spec.PVC).NotTo(BeNil())
			Expect(dvTemplate.Spec.PVC.StorageClassName).To(BeNil())
		})
	})

	Describe("CloneVolume resource", func() {
		It("applies storage class to PVC spec when specified", func() {
			dv := iso.CloneVolume(name, namespace, "10Gi", outputStorageClass)

			Expect(dv).NotTo(BeNil())
			Expect(dv.Spec.PVC).NotTo(BeNil())
			Expect(dv.Spec.PVC.StorageClassName).NotTo(BeNil())
			Expect(*dv.Spec.PVC.StorageClassName).To(Equal(outputStorageClass))
		})

		It("uses default (nil) when no storage class specified", func() {
			dv := iso.CloneVolume(name, namespace, "10Gi", "")

			Expect(dv).NotTo(BeNil())
			Expect(dv.Spec.PVC).NotTo(BeNil())
			Expect(dv.Spec.PVC.StorageClassName).To(BeNil())
		})
	})

	Describe("StepCreateVirtualMachine", func() {
		It("passes build storage class to virtualMachine function", func() {
			var capturedVM *v1.VirtualMachine

			// Capture the VM being created
			vmClient.Fake.PrependReactor("create", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
				create := action.(k8stesting.CreateAction)
				capturedVM = create.GetObject().(*v1.VirtualMachine)
				capturedVM.Status.Ready = true
				return false, capturedVM, nil
			})

			// Set iso_volume_name in state bag (simulating StepCreateIsoDataVolume)
			state.Put("iso_volume_name", "iso-vol")

			step := &iso.StepCreateVirtualMachine{
				Config: iso.Config{
					Name:                name,
					Namespace:           namespace,
					IsoVolumeName:       "iso-vol",
					DiskSize:            "10Gi",
					InstanceType:        "cx1.medium",
					Preference:          "fedora",
					OperatingSystemType: "linux",
					BuildStorageClass:   buildStorageClass,
				},
				Client: virtClient,
			}

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))

			// Verify storage class was applied
			Expect(capturedVM).NotTo(BeNil())
			Expect(capturedVM.Spec.DataVolumeTemplates).To(HaveLen(1))
			dvTemplate := capturedVM.Spec.DataVolumeTemplates[0]
			Expect(dvTemplate.Spec.PVC.StorageClassName).NotTo(BeNil())
			Expect(*dvTemplate.Spec.PVC.StorageClassName).To(Equal(buildStorageClass))
		})

		It("gets iso_volume_name from state bag when available", func() {
			var capturedVM *v1.VirtualMachine
			stateIsoName := "state-iso-volume"

			// Capture the VM being created
			vmClient.Fake.PrependReactor("create", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
				create := action.(k8stesting.CreateAction)
				capturedVM = create.GetObject().(*v1.VirtualMachine)
				capturedVM.Status.Ready = true
				return false, capturedVM, nil
			})

			// Set iso_volume_name in state bag (simulates StepCreateIsoDataVolume setting it)
			state.Put("iso_volume_name", stateIsoName)

			step := &iso.StepCreateVirtualMachine{
				Config: iso.Config{
					Name:                name,
					Namespace:           namespace,
					IsoVolumeName:       "config-iso-volume", // Different from state
					DiskSize:            "10Gi",
					InstanceType:        "cx1.medium",
					Preference:          "fedora",
					OperatingSystemType: "linux",
				},
				Client: virtClient,
			}

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))

			// Verify state bag iso name was used
			Expect(capturedVM).NotTo(BeNil())
			// Find the cdrom volume that references the ISO
			var isoVolumeName string
			for _, vol := range capturedVM.Spec.Template.Spec.Volumes {
				if vol.Name == "cdrom" && vol.DataVolume != nil {
					isoVolumeName = vol.DataVolume.Name
					break
				}
			}
			Expect(isoVolumeName).To(Equal(stateIsoName))
		})
	})

	Describe("StepCreateBootableVolume", func() {
		It("passes output storage class to cloneVolume function", func() {
			var capturedDV *cdiv1beta1.DataVolume

			// Capture the DataVolume being created
			cdiClient.PrependReactor("create", "datavolumes", func(action k8stesting.Action) (bool, runtime.Object, error) {
				create := action.(k8stesting.CreateAction)
				capturedDV = create.GetObject().(*cdiv1beta1.DataVolume)
				capturedDV.Status.Phase = cdiv1beta1.Succeeded
				_ = cdiClient.Tracker().Add(capturedDV)
				return true, capturedDV, nil
			})

			cdiClient.PrependReactor("create", "datasources", func(action k8stesting.Action) (bool, runtime.Object, error) {
				create := action.(k8stesting.CreateAction)
				ds := create.GetObject().(*cdiv1beta1.DataSource)
				_ = cdiClient.Tracker().Add(ds)
				return true, ds, nil
			})

			step := &iso.StepCreateBootableVolume{
				Config: iso.Config{
					Name:               name,
					Namespace:          namespace,
					DiskSize:           "10Gi",
					InstanceType:       "cx1.medium",
					Preference:         "fedora",
					OutputStorageClass: outputStorageClass,
				},
				Client: virtClient,
			}

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))

			// Verify storage class was applied
			Expect(capturedDV).NotTo(BeNil())
			Expect(capturedDV.Spec.PVC).NotTo(BeNil())
			Expect(capturedDV.Spec.PVC.StorageClassName).NotTo(BeNil())
			Expect(*capturedDV.Spec.PVC.StorageClassName).To(Equal(outputStorageClass))
		})

		It("uses default storage class when not specified", func() {
			var capturedDV *cdiv1beta1.DataVolume

			// Capture the DataVolume being created
			cdiClient.PrependReactor("create", "datavolumes", func(action k8stesting.Action) (bool, runtime.Object, error) {
				create := action.(k8stesting.CreateAction)
				capturedDV = create.GetObject().(*cdiv1beta1.DataVolume)
				capturedDV.Status.Phase = cdiv1beta1.Succeeded
				_ = cdiClient.Tracker().Add(capturedDV)
				return true, capturedDV, nil
			})

			cdiClient.PrependReactor("create", "datasources", func(action k8stesting.Action) (bool, runtime.Object, error) {
				create := action.(k8stesting.CreateAction)
				ds := create.GetObject().(*cdiv1beta1.DataSource)
				_ = cdiClient.Tracker().Add(ds)
				return true, ds, nil
			})

			step := &iso.StepCreateBootableVolume{
				Config: iso.Config{
					Name:               name,
					Namespace:          namespace,
					DiskSize:           "10Gi",
					InstanceType:       "cx1.medium",
					Preference:         "fedora",
					OutputStorageClass: "", // empty - use cluster default
				},
				Client: virtClient,
			}

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))

			// Verify storage class is nil (cluster default)
			Expect(capturedDV).NotTo(BeNil())
			Expect(capturedDV.Spec.PVC).NotTo(BeNil())
			Expect(capturedDV.Spec.PVC.StorageClassName).To(BeNil())
		})
	})
})

