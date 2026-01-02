// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubevirt.io/client-go/kubecli"
)

type StepValidateIsoDataVolume struct {
	Config Config
	Client kubecli.KubevirtClient
}

func (s *StepValidateIsoDataVolume) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)

	// If iso_url was used, the StepCreateIsoDataVolume already validated and created the DataVolume.
	// Skip validation in this case.
	if s.Config.IsoUrl != "" {
		ui.Say("ISO DataVolume was created/validated by previous step, skipping validation...")
		return multistep.ActionContinue
	}

	// Get ISO volume name from state bag (set by StepCreateIsoDataVolume)
	isoVolumeName, ok := state.Get("iso_volume_name").(string)
	if !ok || isoVolumeName == "" {
		// Fall back to config for backward compatibility
		isoVolumeName = s.Config.IsoVolumeName
	}

	isoVolumeNamespace := s.Config.Namespace

	ui.Sayf("Validating the existence of the ISO DataVolume (%s/%s)...", isoVolumeNamespace, isoVolumeName)

	_, err := s.Client.CdiClient().CdiV1beta1().DataVolumes(isoVolumeNamespace).Get(ctx, isoVolumeName, metav1.GetOptions{})
	if err != nil {
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	if err := WaitUntilDataVolumeSucceeded(ctx, s.Client, isoVolumeNamespace, isoVolumeName); err != nil {
		ui.Error(err.Error())
		return multistep.ActionHalt
	}
	return multistep.ActionContinue
}

func (s *StepValidateIsoDataVolume) Cleanup(state multistep.StateBag) {
	// Left blank intentionally
}
