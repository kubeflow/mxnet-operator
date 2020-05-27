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
	commonv1 "github.com/kubeflow/common/pkg/apis/common/v1"

	log "github.com/sirupsen/logrus"

	mxv1 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1"
	mxv1beta1 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1beta1"
)

// ValidateBetaOneMXJobSpec checks that the v1beta1.MXJobSpec is valid.
func ValidateBetaOneMXJobSpec(c *mxv1beta1.MXJobSpec) error {
	return validateBetaOneReplicaSpecs(c.MXReplicaSpecs)
}

func validateBetaOneReplicaSpecs(specs map[mxv1beta1.MXReplicaType]*mxv1beta1.MXReplicaSpec) error {
	if specs == nil {
		return fmt.Errorf("MXJobSpec is not valid")
	}
	foundScheduler := 0
	for rType, value := range specs {
		if value == nil || len(value.Template.Spec.Containers) == 0 {
			return fmt.Errorf("MXJobSpec is not valid")
		}
		if mxv1beta1.IsScheduler(rType) {
			foundScheduler++
		}
		// Make sure the image is defined in the container.
		numNamedMXNet := 0
		for _, container := range value.Template.Spec.Containers {
			if container.Image == "" {
				log.Warn("Image is undefined in the container")
				return fmt.Errorf("MXJobSpec is not valid")
			}
			if container.Name == mxv1beta1.DefaultContainerName {
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
		return fmt.Errorf("more than 1 scheduler found")
	}
	return nil
}

// ValidateV1MXJobSpec checks that the v1.MXJobSpec is valid.
func ValidateV1MXJobSpec(c *mxv1.MXJobSpec) error {
	return validateV1ReplicaSpecs(c.MXReplicaSpecs)
}

func validateV1ReplicaSpecs(specs map[commonv1.ReplicaType]*commonv1.ReplicaSpec) error {
	if specs == nil {
		return fmt.Errorf("MXJobSpec is not valid")
	}
	foundScheduler := 0
	for rType, value := range specs {
		if value == nil || len(value.Template.Spec.Containers) == 0 {
			return fmt.Errorf("MXJobSpec is not valid")
		}
		if mxv1.IsScheduler(rType) {
			foundScheduler++
		}
		// Make sure the image is defined in the container.
		numNamedMXNet := 0
		for _, container := range value.Template.Spec.Containers {
			if container.Image == "" {
				log.Warn("Image is undefined in the container")
				return fmt.Errorf("MXJobSpec is not valid")
			}
			if container.Name == mxv1beta1.DefaultContainerName {
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
		return fmt.Errorf("more than 1 scheduler found")
	}
	return nil
}
