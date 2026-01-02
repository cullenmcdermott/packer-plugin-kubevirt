// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/iso"
)

var _ = Describe("StepValidateStorageClasses", func() {
	var (
		step      *iso.StepValidateStorageClasses
		state     *multistep.BasicStateBag
		ui        *packer.MockUi
		clientset *fake.Clientset
		ctx       context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		ui = &packer.MockUi{}
		state = new(multistep.BasicStateBag)
		state.Put("ui", ui)

		// Create a fake clientset with some storage classes
		clientset = fake.NewSimpleClientset(
			&storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "rook-ceph-block",
				},
			},
			&storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "rook-cephfs",
				},
			},
			&storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-path",
				},
			},
		)
	})

	Context("when no storage classes are specified", func() {
		It("should continue without validation", func() {
			step = &iso.StepValidateStorageClasses{
				Config: iso.Config{},
				Client: clientset,
			}

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionContinue))
		})
	})

	Context("when all specified storage classes exist", func() {
		It("should validate successfully with iso_storage_class", func() {
			step = &iso.StepValidateStorageClasses{
				Config: iso.Config{
					IsoStorageClass: "rook-cephfs",
				},
				Client: clientset,
			}

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionContinue))
		})

		It("should validate successfully with build_storage_class", func() {
			step = &iso.StepValidateStorageClasses{
				Config: iso.Config{
					BuildStorageClass: "local-path",
				},
				Client: clientset,
			}

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionContinue))
		})

		It("should validate successfully with output_storage_class", func() {
			step = &iso.StepValidateStorageClasses{
				Config: iso.Config{
					OutputStorageClass: "rook-ceph-block",
				},
				Client: clientset,
			}

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionContinue))
		})

		It("should validate successfully with all storage classes", func() {
			step = &iso.StepValidateStorageClasses{
				Config: iso.Config{
					IsoStorageClass:    "rook-cephfs",
					BuildStorageClass:  "local-path",
					OutputStorageClass: "rook-ceph-block",
				},
				Client: clientset,
			}

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionContinue))
		})
	})

	Context("when a specified storage class does not exist", func() {
		It("should halt with error for invalid iso_storage_class", func() {
			step = &iso.StepValidateStorageClasses{
				Config: iso.Config{
					IsoStorageClass: "nonexistent-sc",
				},
				Client: clientset,
			}

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionHalt))

			err, ok := state.GetOk("error")
			Expect(ok).To(BeTrue())
			Expect(err).To(MatchError(ContainSubstring("iso_storage_class")))
			Expect(err).To(MatchError(ContainSubstring("nonexistent-sc")))
			Expect(err).To(MatchError(ContainSubstring("does not exist")))
		})

		It("should halt with error for invalid build_storage_class", func() {
			step = &iso.StepValidateStorageClasses{
				Config: iso.Config{
					BuildStorageClass: "hostpath-local",
				},
				Client: clientset,
			}

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionHalt))

			err, ok := state.GetOk("error")
			Expect(ok).To(BeTrue())
			Expect(err).To(MatchError(ContainSubstring("build_storage_class")))
			Expect(err).To(MatchError(ContainSubstring("hostpath-local")))
			Expect(err).To(MatchError(ContainSubstring("does not exist")))
		})

		It("should halt with error for invalid output_storage_class", func() {
			step = &iso.StepValidateStorageClasses{
				Config: iso.Config{
					OutputStorageClass: "invalid-output",
				},
				Client: clientset,
			}

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionHalt))

			err, ok := state.GetOk("error")
			Expect(ok).To(BeTrue())
			Expect(err).To(MatchError(ContainSubstring("output_storage_class")))
			Expect(err).To(MatchError(ContainSubstring("invalid-output")))
			Expect(err).To(MatchError(ContainSubstring("does not exist")))
		})

		It("should halt on first invalid storage class when multiple are specified", func() {
			step = &iso.StepValidateStorageClasses{
				Config: iso.Config{
					IsoStorageClass:    "rook-cephfs",     // valid
					BuildStorageClass:  "invalid-build",   // invalid
					OutputStorageClass: "invalid-output",  // invalid
				},
				Client: clientset,
			}

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionHalt))

			err, ok := state.GetOk("error")
			Expect(ok).To(BeTrue())
			Expect(err.(error).Error()).To(ContainSubstring("does not exist"))
		})
	})
})
