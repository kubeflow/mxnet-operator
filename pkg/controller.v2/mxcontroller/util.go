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

package mxcontroller

import (
	"fmt"

	mxv1alpha2 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1alpha2"
)

var (
	errPortNotFound = fmt.Errorf("Failed to found the port")
)

// GetPortFromMXJob gets the port of mxnet container.
func GetPortFromMXJob(mxJob *mxv1alpha2.MXJob, rtype mxv1alpha2.MXReplicaType) (int32, error) {
	containers := mxJob.Spec.MXReplicaSpecs[rtype].Template.Spec.Containers
	for _, container := range containers {
		if container.Name == mxv1alpha2.DefaultContainerName {
			ports := container.Ports
			for _, port := range ports {
				if port.Name == mxv1alpha2.DefaultPortName {
					return port.ContainerPort, nil
				}
			}
		}
	}
	return -1, errPortNotFound
}

func ContainSchedulerSpec(mxJob *mxv1alpha2.MXJob) bool {
	if _, ok := mxJob.Spec.MXReplicaSpecs[mxv1alpha2.MXReplicaTypeScheduler]; ok {
		return true
	}
	return false
}
