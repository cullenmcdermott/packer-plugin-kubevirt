// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubevirt.io/client-go/kubecli"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

// StepCreateIsoDataVolume creates a DataVolume from an ISO URL using CDI's HTTP source.
// If iso_url is not configured, it stores the existing IsoVolumeName in the state bag.
// If the DataVolume already exists and is in Succeeded phase, creation is skipped.
type StepCreateIsoDataVolume struct {
	Config Config
	Client kubecli.KubevirtClient

	// isoCreated tracks whether this step created the ISO DataVolume (vs. reusing existing)
	isoCreated bool
	// resolvedIsoName stores the name of the ISO DataVolume (generated or from config)
	resolvedIsoName string
}

func (s *StepCreateIsoDataVolume) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	namespace := s.Config.Namespace

	// If iso_url is not configured, validate the existing IsoVolumeName DataVolume
	if s.Config.IsoUrl == "" {
		s.resolvedIsoName = s.Config.IsoVolumeName

		ui.Sayf("Validating ISO DataVolume (%s/%s)...", namespace, s.resolvedIsoName)

		_, err := s.Client.CdiClient().CdiV1beta1().DataVolumes(namespace).Get(ctx, s.resolvedIsoName, metav1.GetOptions{})
		if err != nil {
			ui.Errorf("ISO DataVolume %s/%s not found: %v", namespace, s.resolvedIsoName, err)
			return multistep.ActionHalt
		}

		if err := WaitUntilDataVolumeSucceeded(ctx, s.Client, namespace, s.resolvedIsoName); err != nil {
			ui.Errorf("ISO DataVolume %s/%s failed to reach Succeeded phase: %v", namespace, s.resolvedIsoName, err)
			return multistep.ActionHalt
		}

		ui.Sayf("ISO DataVolume is ready (%s/%s)", namespace, s.resolvedIsoName)
		state.Put("iso_volume_name", s.resolvedIsoName)
		return multistep.ActionContinue
	}

	// Resolve ISO DataVolume name: use IsoDataVolumeName if set, otherwise generate from URL hash
	if s.Config.IsoDataVolumeName != "" {
		s.resolvedIsoName = s.Config.IsoDataVolumeName
	} else {
		s.resolvedIsoName = GenerateIsoName(s.Config.IsoUrl)
	}

	ui.Sayf("Checking for existing ISO DataVolume (%s/%s)...", namespace, s.resolvedIsoName)

	// Check if DataVolume with target name already exists
	existingDV, err := s.Client.CdiClient().CdiV1beta1().DataVolumes(namespace).Get(ctx, s.resolvedIsoName, metav1.GetOptions{})
	if err == nil {
		// DataVolume exists - check if it's already Succeeded
		if existingDV.Status.Phase == cdiv1.Succeeded {
			ui.Sayf("ISO DataVolume already exists and is ready, reusing (%s/%s)", namespace, s.resolvedIsoName)
			state.Put("iso_volume_name", s.resolvedIsoName)
			return multistep.ActionContinue
		}
		// Exists but not Succeeded - wait for it
		ui.Sayf("ISO DataVolume exists but is in phase %s, waiting for it to succeed...", existingDV.Status.Phase)
		if err := WaitUntilDataVolumeSucceeded(ctx, s.Client, namespace, s.resolvedIsoName); err != nil {
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
		state.Put("iso_volume_name", s.resolvedIsoName)
		return multistep.ActionContinue
	}

	// If error is not "not found", it's a real error
	if !k8serrors.IsNotFound(err) {
		ui.Errorf("Failed to check for existing ISO DataVolume: %v", err)
		return multistep.ActionHalt
	}

	// DataVolume does not exist - create it
	ui.Sayf("Creating ISO DataVolume from URL (%s/%s)...", namespace, s.resolvedIsoName)

	isoDV := httpIsoDataVolume(s.resolvedIsoName, namespace, s.Config.IsoUrl, s.Config.IsoStorageClass)

	_, err = s.Client.CdiClient().CdiV1beta1().DataVolumes(namespace).Create(ctx, isoDV, metav1.CreateOptions{})
	if err != nil {
		ui.Errorf("Failed to create ISO DataVolume: %v", err)
		return multistep.ActionHalt
	}

	s.isoCreated = true

	ui.Say("Waiting for ISO DataVolume to be ready...")
	if err := WaitUntilDataVolumeSucceeded(ctx, s.Client, namespace, s.resolvedIsoName); err != nil {
		ui.Errorf("ISO DataVolume failed to reach Succeeded phase: %v", err)
		return multistep.ActionHalt
	}

	ui.Sayf("ISO DataVolume is ready (%s/%s)", namespace, s.resolvedIsoName)
	state.Put("iso_volume_name", s.resolvedIsoName)
	return multistep.ActionContinue
}

func (s *StepCreateIsoDataVolume) Cleanup(state multistep.StateBag) {
	ui := state.Get("ui").(packer.Ui)
	namespace := s.Config.Namespace

	// Only delete if:
	// 1. DeleteIso is true
	// 2. We created the ISO DataVolume (not reusing an existing one)
	// 3. We're using iso_url mode (not iso_volume_name mode)
	if !s.Config.DeleteIso {
		if s.Config.IsoUrl != "" && s.resolvedIsoName != "" {
			ui.Sayf("Retaining ISO DataVolume for caching (%s/%s)", namespace, s.resolvedIsoName)
		}
		return
	}

	if !s.isoCreated {
		ui.Sayf("ISO DataVolume was pre-existing, not deleting (%s/%s)", namespace, s.resolvedIsoName)
		return
	}

	if s.resolvedIsoName == "" {
		return
	}

	ui.Sayf("Deleting ISO DataVolume (%s/%s)...", namespace, s.resolvedIsoName)
	err := s.Client.CdiClient().CdiV1beta1().DataVolumes(namespace).Delete(context.Background(), s.resolvedIsoName, metav1.DeleteOptions{})
	if err != nil {
		ui.Errorf("Failed to delete ISO DataVolume: %v", err)
	}
}
