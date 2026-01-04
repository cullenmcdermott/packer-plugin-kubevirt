// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"kubevirt.io/client-go/kubecli"
	"kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

// GetIsoVolumeNameFromState retrieves the ISO volume name from state bag,
// falling back to the provided fallback value if not present in state.
// This is used by steps that need the ISO volume name which may have been
// set by StepCreateIsoDataVolume (when using iso_url) or configured directly
// (when using iso_volume_name).
func GetIsoVolumeNameFromState(state multistep.StateBag, fallback string) string {
	if stateValue, ok := state.Get("iso_volume_name").(string); ok && stateValue != "" {
		return stateValue
	}
	return fallback
}

func WaitUntilDataVolumeSucceeded(ctx context.Context, client kubecli.KubevirtClient, namespace, name string) error {
	pollInterval := 15 * time.Second
	pollTimeout := 3600 * time.Second
	poller := func(ctx context.Context) (bool, error) {
		dataVolume, err := client.CdiClient().CdiV1beta1().DataVolumes(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if dataVolume != nil && dataVolume.Status.Phase == v1beta1.DataVolumePhase(v1beta1.Succeeded) {
			return true, nil
		}
		return false, nil
	}
	return wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true, poller)
}
