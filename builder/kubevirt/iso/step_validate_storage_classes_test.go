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
		DescribeTable("should validate successfully",
			func(config iso.Config) {
				step = &iso.StepValidateStorageClasses{
					Config: config,
					Client: clientset,
				}
				action := step.Run(ctx, state)
				Expect(action).To(Equal(multistep.ActionContinue))
			},
			Entry("with iso_storage_class", iso.Config{IsoStorageClass: "rook-cephfs"}),
			Entry("with build_storage_class", iso.Config{BuildStorageClass: "local-path"}),
			Entry("with output_storage_class", iso.Config{OutputStorageClass: "rook-ceph-block"}),
			Entry("with all storage classes", iso.Config{
				IsoStorageClass:    "rook-cephfs",
				BuildStorageClass:  "local-path",
				OutputStorageClass: "rook-ceph-block",
			}),
		)
	})

	Context("when a specified storage class does not exist", func() {
		DescribeTable("should halt with descriptive error",
			func(config iso.Config, expectedField, expectedValue string) {
				step = &iso.StepValidateStorageClasses{
					Config: config,
					Client: clientset,
				}

				action := step.Run(ctx, state)
				Expect(action).To(Equal(multistep.ActionHalt))

				err, ok := state.GetOk("error")
				Expect(ok).To(BeTrue())
				Expect(err).To(MatchError(ContainSubstring(expectedField)))
				Expect(err).To(MatchError(ContainSubstring(expectedValue)))
				Expect(err).To(MatchError(ContainSubstring("does not exist")))
			},
			Entry("for invalid iso_storage_class",
				iso.Config{IsoStorageClass: "nonexistent-sc"},
				"iso_storage_class", "nonexistent-sc"),
			Entry("for invalid build_storage_class",
				iso.Config{BuildStorageClass: "hostpath-local"},
				"build_storage_class", "hostpath-local"),
			Entry("for invalid output_storage_class",
				iso.Config{OutputStorageClass: "invalid-output"},
				"output_storage_class", "invalid-output"),
		)

		It("should halt on first invalid storage class when multiple are specified", func() {
			step = &iso.StepValidateStorageClasses{
				Config: iso.Config{
					IsoStorageClass:    "rook-cephfs",    // valid
					BuildStorageClass:  "invalid-build",  // invalid
					OutputStorageClass: "invalid-output", // invalid
				},
				Client: clientset,
			}

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionHalt))

			err, ok := state.GetOk("error")
			Expect(ok).To(BeTrue())
			// With deterministic ordering, build_storage_class should fail first
			Expect(err).To(MatchError(ContainSubstring("build_storage_class")))
			Expect(err).To(MatchError(ContainSubstring("does not exist")))
		})
	})
})
