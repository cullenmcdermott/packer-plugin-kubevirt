// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	ptr "k8s.io/utils/ptr"

	"kubevirt.io/client-go/kubecli"
)

type StepCreateVirtualMachine struct {
	Config Config
	Client kubecli.KubevirtClient
}

func (s *StepCreateVirtualMachine) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	name := s.Config.Name
	namespace := s.Config.Namespace
	diskSize := s.Config.DiskSize
	instanceTypeName := s.Config.InstanceType
	instanceTypeKind := s.Config.InstanceTypeKind
	preferenceName := s.Config.Preference
	preferenceKind := s.Config.PreferenceKind
	osType := s.Config.OperatingSystemType
	networks := s.Config.Networks
	buildStorageClass := s.Config.BuildStorageClass

	// Get ISO volume name from state bag (set by StepCreateIsoDataVolume),
	// falling back to config value for backward compatibility
	isoVolumeName := GetIsoVolumeNameFromState(state, s.Config.IsoVolumeName)

	if osType == "" || (osType != "linux" && osType != "windows") {
		ui.Errorf("OS type of '%s' is not supported, set 'linux' or 'windows'.", osType)
		return multistep.ActionHalt
	}

	virtualMachine := VirtualMachine(VirtualMachineOptions{
		Name:              name,
		IsoVolumeName:     isoVolumeName,
		DiskSize:          diskSize,
		InstanceType:      instanceTypeName,
		PreferenceName:    preferenceName,
		InstanceTypeKind:  instanceTypeKind,
		PreferenceKind:    preferenceKind,
		OSType:            osType,
		Networks:          networks,
		BuildStorageClass: buildStorageClass,
	})

	ui.Sayf("Creating a new temporary VirtualMachine (%s/%s)...", namespace, name)

	_, err := s.Client.VirtualMachine(namespace).Create(ctx, virtualMachine, metav1.CreateOptions{})
	if err != nil {
		ui.Errorf("Failed to create VirtualMachine %s/%s: %v", namespace, name, err)
		return multistep.ActionHalt
	}

	if err := s.waitUntilVirtualMachineReady(ctx); err != nil {
		return multistep.ActionHalt
	}
	return multistep.ActionContinue
}

func (s *StepCreateVirtualMachine) Cleanup(state multistep.StateBag) {
	ui := state.Get("ui").(packer.Ui)
	name := s.Config.Name
	namespace := s.Config.Namespace
	keepVM := s.Config.KeepVM

	if keepVM {
		ui.Sayf("Keeping VirtualMachine (%s/%s).", namespace, name)
		return
	}

	ui.Sayf("Deleting VirtualMachine (%s/%s)...", namespace, name)

	_ = s.Client.VirtualMachine(namespace).Delete(context.Background(), name, metav1.DeleteOptions{
		GracePeriodSeconds: ptr.To(int64(0)),
	})
}

func (s *StepCreateVirtualMachine) waitUntilVirtualMachineReady(ctx context.Context) error {
	name := s.Config.Name
	namespace := s.Config.Namespace
	pollInterval := 5 * time.Second
	pollTimeout := 3600 * time.Second
	poller := func(ctx context.Context) (bool, error) {
		vm, err := s.Client.VirtualMachine(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if vm.Status.Ready {
			return true, nil
		}
		return false, nil
	}

	return wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true, poller)
}
