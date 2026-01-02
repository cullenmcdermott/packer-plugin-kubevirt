// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,Network,NetworkSource,PodNetwork,MultusNetwork

package iso

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
)

// Network represents a network type and a resource that should be connected to the VM.
// Source: https://kubevirt.io/api-reference/v1.6.0/definitions.html#_v1_network
type Network struct {
	// Network name.
	// Must be a DNS_LABEL and unique within the VM.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	Name string `mapstructure:"name"`

	// NetworkSource represents the network type and the source interface that should be connected to the VM.
	// Defaults to Pod, if no type is specified.
	NetworkSource `mapstructure:",squash"`
}

// Represents the source resource that will be connected to the VM.
// Only one of its members may be specified.
type NetworkSource struct {
	Pod    *PodNetwork    `mapstructure:"pod"`
	Multus *MultusNetwork `mapstructure:"multus"`
}

// Represents the stock pod network interface.
// Source: https://kubevirt.io/api-reference/v1.6.0/definitions.html#_v1_podnetwork
type PodNetwork struct {
	// CIDR for VM network.
	// Default 10.0.2.0/24 if not specified.
	VMNetworkCIDR string `mapstructure:"vmNetworkCIDR,omitempty"`

	// IPv6 CIDR for the VM network.
	// Defaults to fd10:0:2::/120 if not specified.
	VMIPv6NetworkCIDR string `mapstructure:"vmIPv6NetworkCIDR,omitempty"`
}

// Represents the multus CNI network.
// Source: https://kubevirt.io/api-reference/v1.6.0/definitions.html#_v1_multusnetwork
type MultusNetwork struct {
	// References to a NetworkAttachmentDefinition CRD object. Format:
	// <networkName>, <namespace>/<networkName>. If namespace is not
	// specified, VMI namespace is assumed.
	NetworkName string `mapstructure:"networkName"`

	// Select the default network and add it to the
	// multus-cni.io/default-network annotation.
	Default bool `mapstructure:"default,omitempty"`
}

type Config struct {
	common.PackerConfig `mapstructure:",squash"`

	// KubeConfig is the path to the kubeconfig file.
	KubeConfig string `mapstructure:"kube_config" required:"true"`
	// Name is the name of the VM image.
	Name string `mapstructure:"name" required:"true"`
	// Namespace is the namespace in which to create the VM image.
	Namespace string `mapstructure:"namespace" required:"true"`
	// IsoVolumeName is the name of an existing DataVolume resource that contains the installation ISO.
	// This DataVolume must already exist in the namespace.
	// Either iso_volume_name or iso_url must be specified, but not both.
	IsoVolumeName string `mapstructure:"iso_volume_name" required:"false"`
	// IsoUrl is the URL to download the installation ISO from. When specified, a DataVolume
	// will be created using CDI's HTTP source to download the ISO automatically.
	// Either iso_url or iso_volume_name must be specified, but not both.
	IsoUrl string `mapstructure:"iso_url" required:"false"`
	// IsoName is an optional override for the auto-generated ISO DataVolume name when using iso_url.
	// If not specified, a deterministic name will be generated from the URL hash.
	// Format when auto-generated: packer-iso-<truncated-hash>
	IsoName string `mapstructure:"iso_name" required:"false"`
	// IsoStorageClass is the storage class to use for the ISO DataVolume when using iso_url.
	// If not specified, the cluster default storage class will be used.
	IsoStorageClass string `mapstructure:"iso_storage_class" required:"false"`
	// DeleteIso indicates whether to delete the ISO DataVolume after the build completes.
	// Only applies when using iso_url. Default is false (ISO is cached for reuse).
	DeleteIso bool `mapstructure:"delete_iso" required:"false"`
	// StorageClass is a shorthand to set both build_storage_class and output_storage_class
	// to the same value. If build_storage_class or output_storage_class are explicitly set,
	// they will override this value for their respective volumes.
	StorageClass string `mapstructure:"storage_class" required:"false"`
	// BuildStorageClass is the storage class to use for the temporary root disk during installation.
	// If not specified, falls back to storage_class, then the cluster default.
	BuildStorageClass string `mapstructure:"build_storage_class" required:"false"`
	// OutputStorageClass is the storage class to use for the final cloned DataVolume artifact.
	// If not specified, falls back to storage_class, then the cluster default.
	OutputStorageClass string `mapstructure:"output_storage_class" required:"false"`
	// DiskSize is the size of the root disk to of the temporary VM.
	DiskSize string `mapstructure:"disk_size" required:"true"`
	// InstanceType is the name of the InstanceType resource to use in the temporary VM.
	InstanceType string `mapstructure:"instance_type" required:"true"`
	// InstanceTypeKind is the kind of the InstanceType resource to use in the temporary VM.
	// Other supported value is "virtualmachineclusterinstancetype".
	InstanceTypeKind string `mapstructure:"instance_type_kind" required:"false"`
	// Preference is the name of the Preference resource to use in the temporary VM.
	Preference string `mapstructure:"preference" required:"true"`
	// PreferenceKind is the kind of the Preference resource to use in the temporary VM.
	// Other supported value is "virtualmachineclusterpreference".
	PreferenceKind string `mapstructure:"preference_kind" required:"false"`
	// OperatingSystemType is the type of operating system to install.
	// Supported values are "linux" and "windows". Default is "linux".
	OperatingSystemType string `mapstructure:"os_type" required:"false"`
	// Networks is a list of networks to attach to the temporary VM.
	// If no networks are specified, a single pod network will be used.
	Networks []Network `mapstructure:"networks" required:"false"`
	// MediaFiles is a path list of files to be copied and used during the ISO installation.
	MediaFiles []string `mapstructure:"media_files" required:"false"`
	// BootCommand is a list of strings that represent the keystrokes to be sent to the VM console
	// to automate the installation via a new VNC connection.
	BootCommand []string `mapstructure:"boot_command" required:"false"`
	// BootWait is the amount of time to wait before sending the boot command.
	// This is useful if the VM takes some time to boot and be ready to accept keystrokes.
	BootWait time.Duration `mapstructure:"boot_wait" required:"false"`
	// InstallationWaitTimeout is the amount of time to wait for the installation to be completed.
	InstallationWaitTimeout time.Duration `mapstructure:"installation_wait_timeout" required:"true"`
	// Communicator is the type of communicator to use to connect to the VM.
	// Supported values are "ssh" and "winrm".
	Communicator string `mapstructure:"communicator" required:"false"`
	// SSHHost is the hostname or IP address to use to connect via SSH.
	SSHHost string `mapstructure:"ssh_host" required:"false"`
	// SSHLocalPort is the local port to use to connect via SSH.
	SSHLocalPort int `mapstructure:"ssh_local_port" required:"false"`
	// SSHRemotePort is the remote port to use to connect via SSH.
	SSHRemotePort int `mapstructure:"ssh_remote_port" required:"false"`
	// SSHUsername is the username to use to connect via SSH.
	SSHUsername string `mapstructure:"ssh_username" required:"false"`
	// SSHPassword is the password to use to connect via SSH.
	SSHPassword string `mapstructure:"ssh_password" required:"false"`
	// SSHWaitTimeout is the amount of time to wait for the SSH service to be available.
	SSHWaitTimeout time.Duration `mapstructure:"ssh_wait_timeout" required:"false"`
	// WinRMHost is the hostname or IP address to use to connect via WinRM.
	WinRMHost string `mapstructure:"winrm_host" required:"false"`
	// WinRMLocalPort is the local port to use to connect via WinRM.
	WinRMLocalPort int `mapstructure:"winrm_local_port" required:"false"`
	// WinRMRemotePort is the remote port to use to connect via WinRM.
	WinRMRemotePort int `mapstructure:"winrm_remote_port" required:"false"`
	// WinRMUsername is the username to use to connect via WinRM.
	WinRMUsername string `mapstructure:"winrm_username" required:"false"`
	// WinRMPassword is the password to use to connect via WinRM.
	WinRMPassword string `mapstructure:"winrm_password" required:"false"`
	// WinRMWaitTimeout is the amount of time to wait for the WinRM service to be available.
	WinRMWaitTimeout time.Duration `mapstructure:"winrm_wait_timeout" required:"false"`

	// KeepVM indicates whether to keep the temporary VM after the image has been created.
	// If false, the VM and all its resources will be deleted after the image is created.
	// If true, only the VM resource will be kept, all other resources will be deleted.
	// Default is false.
	//
	// This can be useful for debugging purposes, to inspect the VM and its disks.
	// However, it is recommended to set this to false in production environments to avoid
	// resource leaks.
	KeepVM bool `mapstructure:"keep_vm" required:"false"`
}

func (c *Config) Prepare(raws ...interface{}) ([]string, error) {
	err := config.Decode(c, &config.DecodeOpts{
		PluginType:  "builder.kubevirt.iso",
		Interpolate: true,
	}, raws...)
	if err != nil {
		return nil, err
	}

	// Validate network configuration
	for _, n := range c.Networks {
		if n.Pod != nil && n.Multus != nil {
			return nil, fmt.Errorf("network %q: only one of pod or multus can be defined", n.Name)
		}
	}

	// Validate ISO source configuration: exactly one of iso_url or iso_volume_name must be set
	if c.IsoUrl != "" && c.IsoVolumeName != "" {
		return nil, fmt.Errorf("iso_url and iso_volume_name are mutually exclusive; specify only one")
	}
	if c.IsoUrl == "" && c.IsoVolumeName == "" {
		return nil, fmt.Errorf("one of iso_url or iso_volume_name must be specified")
	}

	// Apply storage class override logic: populate BuildStorageClass and OutputStorageClass
	// from StorageClass if they are not explicitly set
	if c.BuildStorageClass == "" && c.StorageClass != "" {
		c.BuildStorageClass = c.StorageClass
	}
	if c.OutputStorageClass == "" && c.StorageClass != "" {
		c.OutputStorageClass = c.StorageClass
	}

	// DeleteIso defaults to false (Go zero value for bool is false, so no action needed)

	return nil, nil
}

// GenerateIsoName creates a deterministic, DNS-compliant name from an ISO URL.
// The format is: packer-iso-<first-16-chars-of-sha256-hex-hash>
// This ensures the name is:
// - Deterministic: same URL always produces the same name
// - DNS-compliant: lowercase, alphanumeric with hyphens, max 63 characters
func GenerateIsoName(url string) string {
	hash := sha256.Sum256([]byte(url))
	hexHash := hex.EncodeToString(hash[:])
	// Use first 16 characters of hex hash for uniqueness while keeping name short
	// Total length: 11 (packer-iso-) + 16 (hash) = 27 characters
	return "packer-iso-" + hexHash[:16]
}
