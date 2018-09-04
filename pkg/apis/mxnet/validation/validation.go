// Copyright 2018 The Kubeflow Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package validation

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	mxv1 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1alpha1"
	mxv2 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1alpha2"
	"github.com/kubeflow/mxnet-operator/pkg/util"
)

// ValidateMXJobSpec checks that the MXJobSpec is valid.
func ValidateMXJobSpec(c *mxv1.MXJobSpec) error {
	if c.TerminationPolicy == nil || c.TerminationPolicy.Chief == nil {
		return fmt.Errorf("invalid termination policy: %v", c.TerminationPolicy)
	}

	chiefExists := false

	// Check that each replica has a MXNet container and a chief.
	for _, r := range c.ReplicaSpecs {
		found := false
		if r.Template == nil {
			return fmt.Errorf("Replica is missing Template; %v", util.Pformat(r))
		}

		if r.MXReplicaType == mxv1.MXReplicaType(c.TerminationPolicy.Chief.ReplicaName) {
			chiefExists = true
		}

		// Make sure the replica type is valid.
		validReplicaTypes := []mxv1.MXReplicaType{mxv1.SCHEDULER, mxv1.SERVER, mxv1.WORKER}

		isValidReplicaType := false
		for _, t := range validReplicaTypes {
			if t == r.MXReplicaType {
				isValidReplicaType = true
				break
			}
		}

		if !isValidReplicaType {
			return fmt.Errorf("mxReplicaSpec.MXReplicaType is %v but must be one of %v", r.MXReplicaType, validReplicaTypes)
		}

		for _, c := range r.Template.Spec.Containers {
			if c.Name == mxv1.DefaultMXContainer {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("Replica type %v is missing a container named %s", r.MXReplicaType, mxv1.DefaultMXContainer)
		}
	}

	if !chiefExists {
		return fmt.Errorf("Missing ReplicaSpec for chief: %v", c.TerminationPolicy.Chief.ReplicaName)
	}

	return nil
}

// ValidateAlphaTwoMXJobSpec checks that the v1alpha2.MXJobSpec is valid.
func ValidateAlphaTwoMXJobSpec(c *mxv2.MXJobSpec) error {
	return validateAlphaTwoReplicaSpecs(c.MXReplicaSpecs)
}

func validateAlphaTwoReplicaSpecs(specs map[mxv2.MXReplicaType]*mxv2.MXReplicaSpec) error {
	if specs == nil {
		return fmt.Errorf("MXJobSpec is not valid")
	}
	foundScheduler := 0
	for rType, value := range specs {
		if value == nil || len(value.Template.Spec.Containers) == 0 {
			return fmt.Errorf("MXJobSpec is not valid")
		}
		if mxv2.IsScheduler(rType) {
			foundScheduler++
		}
		// Make sure the image is defined in the container.
		numNamedMXNet := 0
		for _, container := range value.Template.Spec.Containers {
			if container.Image == "" {
				log.Warn("Image is undefined in the container")
				return fmt.Errorf("MXJobSpec is not valid")
			}
			if container.Name == mxv2.DefaultContainerName {
				numNamedMXNet++
			}
		}
		// Make sure there has at least one container named "mxnet".
		if numNamedMXNet == 0 {
			log.Warnf("There is no container named mxnet in %v", rType)
			return fmt.Errorf("MXJobSpec is not valid")
		}
	}
	if foundScheduler > 1 {
		return fmt.Errorf("More than 1 scheduler found")
	}
	return nil
}
