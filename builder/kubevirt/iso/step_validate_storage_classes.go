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

// storageClassValidation holds a storage class name and its config field name for error messages.
type storageClassValidation struct {
	storageClass string
	configField  string
}

func (s *StepValidateStorageClasses) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)

	// Collect all storage classes that need validation in deterministic order
	var storageClasses []storageClassValidation

	if s.Config.IsoStorageClass != "" {
		storageClasses = append(storageClasses, storageClassValidation{
			storageClass: s.Config.IsoStorageClass,
			configField:  "iso_storage_class",
		})
	}
	if s.Config.BuildStorageClass != "" {
		storageClasses = append(storageClasses, storageClassValidation{
			storageClass: s.Config.BuildStorageClass,
			configField:  "build_storage_class",
		})
	}
	if s.Config.OutputStorageClass != "" {
		storageClasses = append(storageClasses, storageClassValidation{
			storageClass: s.Config.OutputStorageClass,
			configField:  "output_storage_class",
		})
	}

	if len(storageClasses) == 0 {
		ui.Say("No custom storage classes specified, using cluster defaults")
		return multistep.ActionContinue
	}

	ui.Say("Validating storage classes...")

	for _, sc := range storageClasses {
		_, err := s.Client.StorageV1().StorageClasses().Get(ctx, sc.storageClass, metav1.GetOptions{})
		if err != nil {
			err := fmt.Errorf("storage class %q specified in %s does not exist: %w", sc.storageClass, sc.configField, err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
		ui.Say(fmt.Sprintf("  - %s: %s (valid)", sc.configField, sc.storageClass))
	}

	ui.Say("All storage classes validated successfully")
	return multistep.ActionContinue
}

func (s *StepValidateStorageClasses) Cleanup(state multistep.StateBag) {
	// Nothing to clean up
}
