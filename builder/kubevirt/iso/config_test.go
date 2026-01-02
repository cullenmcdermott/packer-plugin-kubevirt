// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/iso"
)

var _ = Describe("GenerateIsoName", func() {
	It("produces deterministic output for the same URL", func() {
		url := "https://example.com/path/to/fedora.iso"
		name1 := iso.GenerateIsoName(url)
		name2 := iso.GenerateIsoName(url)
		Expect(name1).To(Equal(name2))
	})

	It("produces different output for different URLs", func() {
		url1 := "https://example.com/fedora.iso"
		url2 := "https://example.com/ubuntu.iso"
		name1 := iso.GenerateIsoName(url1)
		name2 := iso.GenerateIsoName(url2)
		Expect(name1).NotTo(Equal(name2))
	})

	It("produces DNS-compliant names (lowercase, max 63 chars)", func() {
		// Test with a URL that has uppercase letters
		url := "https://EXAMPLE.COM/PATH/TO/FEDORA.ISO"
		name := iso.GenerateIsoName(url)

		// Check format: packer-iso-<hash>
		Expect(name).To(HavePrefix("packer-iso-"))

		// Check max length (63 chars for DNS compliance)
		Expect(len(name)).To(BeNumerically("<=", 63))

		// Check lowercase (DNS names must be lowercase)
		Expect(name).To(MatchRegexp("^[a-z0-9-]+$"))
	})

	It("produces names in the format packer-iso-<hash>", func() {
		url := "https://example.com/fedora.iso"
		name := iso.GenerateIsoName(url)

		// Format should be: packer-iso-<16-char-hex-hash>
		// Total length: 11 (packer-iso-) + 16 (hash) = 27 characters
		Expect(name).To(HavePrefix("packer-iso-"))
		Expect(len(name)).To(Equal(27)) // 11 + 16 = 27
	})
})

var _ = Describe("Config.Prepare", func() {
	Describe("ISO source validation", func() {
		It("returns error when both iso_url and iso_volume_name are specified", func() {
			config := iso.Config{
				IsoUrl:        "https://example.com/fedora.iso",
				IsoVolumeName: "existing-iso",
			}
			_, err := config.Prepare(map[string]interface{}{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("mutually exclusive"))
		})

		It("returns error when neither iso_url nor iso_volume_name is specified", func() {
			config := iso.Config{}
			_, err := config.Prepare(map[string]interface{}{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("iso_url or iso_volume_name must be specified"))
		})

		It("succeeds when only iso_url is specified", func() {
			config := iso.Config{
				IsoUrl: "https://example.com/fedora.iso",
			}
			_, err := config.Prepare(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds when only iso_volume_name is specified", func() {
			config := iso.Config{
				IsoVolumeName: "existing-iso",
			}
			_, err := config.Prepare(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Storage class override logic", func() {
		It("populates BuildStorageClass from StorageClass when not explicitly set", func() {
			config := iso.Config{
				IsoVolumeName: "test-iso",
				StorageClass:  "default-storage",
			}
			_, err := config.Prepare(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(config.BuildStorageClass).To(Equal("default-storage"))
		})

		It("populates OutputStorageClass from StorageClass when not explicitly set", func() {
			config := iso.Config{
				IsoVolumeName: "test-iso",
				StorageClass:  "default-storage",
			}
			_, err := config.Prepare(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(config.OutputStorageClass).To(Equal("default-storage"))
		})

		It("does not override BuildStorageClass when explicitly set", func() {
			config := iso.Config{
				IsoVolumeName:     "test-iso",
				StorageClass:      "default-storage",
				BuildStorageClass: "fast-nvme",
			}
			_, err := config.Prepare(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(config.BuildStorageClass).To(Equal("fast-nvme"))
		})

		It("does not override OutputStorageClass when explicitly set", func() {
			config := iso.Config{
				IsoVolumeName:      "test-iso",
				StorageClass:       "default-storage",
				OutputStorageClass: "ceph-rbd",
			}
			_, err := config.Prepare(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(config.OutputStorageClass).To(Equal("ceph-rbd"))
		})

		It("allows different storage classes for build and output", func() {
			config := iso.Config{
				IsoVolumeName:      "test-iso",
				BuildStorageClass:  "fast-nvme",
				OutputStorageClass: "ceph-rbd",
			}
			_, err := config.Prepare(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(config.BuildStorageClass).To(Equal("fast-nvme"))
			Expect(config.OutputStorageClass).To(Equal("ceph-rbd"))
		})
	})
})
