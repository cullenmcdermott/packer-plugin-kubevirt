// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"context"
	"fmt"
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/golang/mock/gomock"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/iso"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	fakecdiclient "kubevirt.io/client-go/containerizeddataimporter/fake"
	"kubevirt.io/client-go/kubecli"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/testing"
)

var _ = Describe("StepCreateIsoDataVolume", func() {
	const (
		namespace = "test-ns"
		isoUrl    = "https://example.com/fedora.iso"
	)

	var (
		ctrl       *gomock.Controller
		state      *multistep.BasicStateBag
		step       *iso.StepCreateIsoDataVolume
		cdiClient  *fakecdiclient.Clientset
		virtClient kubecli.KubevirtClient
	)

	BeforeEach(func() {
		uiErr := &strings.Builder{}
		ui := &packer.BasicUi{
			Reader:      strings.NewReader(""),
			Writer:      io.Discard,
			ErrorWriter: uiErr,
		}
		state = new(multistep.BasicStateBag)
		state.Put("ui", ui)

		ctrl = gomock.NewController(GinkgoT())
		cdiClient = fakecdiclient.NewSimpleClientset()
		kubecli.GetKubevirtClientFromClientConfig = kubecli.GetMockKubevirtClientFromClientConfig
		kubecli.MockKubevirtClientInstance = kubecli.NewMockKubevirtClient(ctrl)
		kubecli.MockKubevirtClientInstance.EXPECT().CdiClient().Return(cdiClient).AnyTimes()
		virtClient, _ = kubecli.GetKubevirtClientFromClientConfig(nil)

		step = &iso.StepCreateIsoDataVolume{
			Config: iso.Config{
				Namespace: namespace,
				IsoUrl:    isoUrl,
			},
			Client: virtClient,
		}
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Context("Run", func() {
		It("creates DataVolume from HTTP URL and stores name in state", func() {
			cdiClient.PrependReactor("create", "datavolumes", func(action testing.Action) (bool, runtime.Object, error) {
				create := action.(testing.CreateAction)
				dv := create.GetObject().(*cdiv1beta1.DataVolume)
				dv.Status.Phase = cdiv1beta1.Succeeded

				// Store DV in the fake client's tracker for Get() calls
				_ = cdiClient.Tracker().Add(dv)

				return true, dv, nil
			})

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))

			// Verify resolved ISO volume name is stored in state bag
			isoVolumeName := state.Get("iso_volume_name")
			Expect(isoVolumeName).NotTo(BeNil())
			Expect(isoVolumeName.(string)).To(HavePrefix("packer-iso-"))
		})

		It("skips creation when DataVolume already exists and is Succeeded", func() {
			expectedName := iso.GenerateIsoName(isoUrl)

			// Pre-create the DataVolume in Succeeded state
			_, err := cdiClient.CdiV1beta1().DataVolumes(namespace).Create(context.Background(), &cdiv1beta1.DataVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:      expectedName,
					Namespace: namespace,
				},
				Status: cdiv1beta1.DataVolumeStatus{Phase: cdiv1beta1.Succeeded},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Track if create was called
			createCalled := false
			cdiClient.PrependReactor("create", "datavolumes", func(action testing.Action) (bool, runtime.Object, error) {
				createCalled = true
				return false, nil, nil
			})

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))
			Expect(createCalled).To(BeFalse(), "should skip creation for existing Succeeded DataVolume")

			// Verify name is stored in state
			isoVolumeName := state.Get("iso_volume_name")
			Expect(isoVolumeName).To(Equal(expectedName))
		})

		It("applies storage class when iso_storage_class is set", func() {
			storageClass := "fast-nvme"
			step.Config.IsoStorageClass = storageClass

			var createdDV *cdiv1beta1.DataVolume
			cdiClient.PrependReactor("create", "datavolumes", func(action testing.Action) (bool, runtime.Object, error) {
				create := action.(testing.CreateAction)
				createdDV = create.GetObject().(*cdiv1beta1.DataVolume)
				createdDV.Status.Phase = cdiv1beta1.Succeeded

				_ = cdiClient.Tracker().Add(createdDV)

				return true, createdDV, nil
			})

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))

			// Verify storage class was applied
			Expect(createdDV).NotTo(BeNil())
			Expect(createdDV.Spec.PVC).NotTo(BeNil())
			Expect(createdDV.Spec.PVC.StorageClassName).NotTo(BeNil())
			Expect(*createdDV.Spec.PVC.StorageClassName).To(Equal(storageClass))
		})

		It("halts on DataVolume creation error", func() {
			cdiClient.PrependReactor("create", "datavolumes", func(action testing.Action) (bool, runtime.Object, error) {
				return true, nil, fmt.Errorf("boom: DV create failed")
			})

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
		})

		It("validates and stores IsoVolumeName when iso_url is not set", func() {
			existingName := "existing-iso-dv"
			step.Config.IsoUrl = ""
			step.Config.IsoVolumeName = existingName

			// Pre-create the DataVolume in Succeeded state
			_, err := cdiClient.CdiV1beta1().DataVolumes(namespace).Create(context.Background(), &cdiv1beta1.DataVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:      existingName,
					Namespace: namespace,
				},
				Status: cdiv1beta1.DataVolumeStatus{Phase: cdiv1beta1.Succeeded},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))

			// Verify existing ISO volume name is stored in state
			isoVolumeName := state.Get("iso_volume_name")
			Expect(isoVolumeName).To(Equal(existingName))
		})

		It("halts when iso_volume_name DataVolume not found", func() {
			step.Config.IsoUrl = ""
			step.Config.IsoVolumeName = "nonexistent-iso"

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
		})

		It("halts when iso_volume_name DataVolume never succeeds", func() {
			existingName := "pending-iso"
			step.Config.IsoUrl = ""
			step.Config.IsoVolumeName = existingName

			// Pre-create the DataVolume in Pending state
			_, err := cdiClient.CdiV1beta1().DataVolumes(namespace).Create(context.Background(), &cdiv1beta1.DataVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:      existingName,
					Namespace: namespace,
				},
				Status: cdiv1beta1.DataVolumeStatus{Phase: cdiv1beta1.Pending},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Cancel context early to simulate stuck Pending
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionHalt))
		})

		It("uses IsoDataVolumeName override when specified", func() {
			customName := "my-custom-iso"
			step.Config.IsoDataVolumeName = customName

			cdiClient.PrependReactor("create", "datavolumes", func(action testing.Action) (bool, runtime.Object, error) {
				create := action.(testing.CreateAction)
				dv := create.GetObject().(*cdiv1beta1.DataVolume)
				dv.Status.Phase = cdiv1beta1.Succeeded

				_ = cdiClient.Tracker().Add(dv)

				return true, dv, nil
			})

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))

			// Verify custom name is used and stored in state
			isoVolumeName := state.Get("iso_volume_name")
			Expect(isoVolumeName).To(Equal(customName))
		})
	})

	Context("Cleanup", func() {
		It("deletes ISO DataVolume when delete_iso is true and ISO was created", func() {
			step.Config.DeleteIso = true
			expectedName := iso.GenerateIsoName(isoUrl)

			// First run to create the DataVolume
			cdiClient.PrependReactor("create", "datavolumes", func(action testing.Action) (bool, runtime.Object, error) {
				create := action.(testing.CreateAction)
				dv := create.GetObject().(*cdiv1beta1.DataVolume)
				dv.Status.Phase = cdiv1beta1.Succeeded
				_ = cdiClient.Tracker().Add(dv)
				return true, dv, nil
			})

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))

			// Track if delete was called
			deleteCalled := false
			cdiClient.PrependReactor("delete", "datavolumes", func(action testing.Action) (bool, runtime.Object, error) {
				delete := action.(testing.DeleteAction)
				Expect(delete.GetName()).To(Equal(expectedName))
				deleteCalled = true
				return true, nil, nil
			})

			step.Cleanup(state)
			Expect(deleteCalled).To(BeTrue(), "should delete ISO DataVolume when delete_iso is true")
		})

		It("does not delete ISO DataVolume when delete_iso is false", func() {
			step.Config.DeleteIso = false

			// First run to create the DataVolume
			cdiClient.PrependReactor("create", "datavolumes", func(action testing.Action) (bool, runtime.Object, error) {
				create := action.(testing.CreateAction)
				dv := create.GetObject().(*cdiv1beta1.DataVolume)
				dv.Status.Phase = cdiv1beta1.Succeeded
				_ = cdiClient.Tracker().Add(dv)
				return true, dv, nil
			})

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))

			// Track if delete was called
			deleteCalled := false
			cdiClient.PrependReactor("delete", "datavolumes", func(action testing.Action) (bool, runtime.Object, error) {
				deleteCalled = true
				return true, nil, nil
			})

			step.Cleanup(state)
			Expect(deleteCalled).To(BeFalse(), "should not delete ISO DataVolume when delete_iso is false")
		})
	})
})
