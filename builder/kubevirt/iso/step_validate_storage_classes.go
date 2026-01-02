// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// StepValidateStorageClasses validates that any specified storage classes exist in the cluster.
// This provides early feedback before the build starts, rather than failing later when
// CDI tries to create DataVolumes with invalid storage classes.
type StepValidateStorageClasses struct {
	Config Config
	Client kubernetes.Interface
}

func (s *StepValidateStorageClasses) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)

	// Collect all storage classes that need validation
	storageClasses := make(map[string]string) // map[storageClass]configField for error messages

	if s.Config.IsoStorageClass != "" {
		storageClasses[s.Config.IsoStorageClass] = "iso_storage_class"
	}
	if s.Config.BuildStorageClass != "" {
		storageClasses[s.Config.BuildStorageClass] = "build_storage_class"
	}
	if s.Config.OutputStorageClass != "" {
		storageClasses[s.Config.OutputStorageClass] = "output_storage_class"
	}

	if len(storageClasses) == 0 {
		ui.Say("No custom storage classes specified, using cluster defaults")
		return multistep.ActionContinue
	}

	ui.Say("Validating storage classes...")

	for sc, field := range storageClasses {
		_, err := s.Client.StorageV1().StorageClasses().Get(ctx, sc, metav1.GetOptions{})
		if err != nil {
			err := fmt.Errorf("storage class %q specified in %s does not exist: %w", sc, field, err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
		ui.Say(fmt.Sprintf("  - %s: %s (valid)", field, sc))
	}

	ui.Say("All storage classes validated successfully")
	return multistep.ActionContinue
}

func (s *StepValidateStorageClasses) Cleanup(state multistep.StateBag) {
	// Nothing to clean up
}
