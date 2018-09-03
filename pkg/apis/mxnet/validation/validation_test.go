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
	"testing"

	mxv2 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1alpha2"

	"github.com/gogo/protobuf/proto"
	"k8s.io/api/core/v1"
)

func TestValidateAlphaTwoMXJobSpec(t *testing.T) {
	testCases := []mxv2.MXJobSpec{
		{
			MXReplicaSpecs: nil,
		},
		{
			MXReplicaSpecs: map[mxv2.MXReplicaType]*mxv2.MXReplicaSpec{
				mxv2.MXReplicaTypeWorker: &mxv2.MXReplicaSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{},
						},
					},
				},
			},
		},
		{
			MXReplicaSpecs: map[mxv2.MXReplicaType]*mxv2.MXReplicaSpec{
				mxv2.MXReplicaTypeWorker: &mxv2.MXReplicaSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								v1.Container{
									Image: "",
								},
							},
						},
					},
				},
			},
		},
		{
			MXReplicaSpecs: map[mxv2.MXReplicaType]*mxv2.MXReplicaSpec{
				mxv2.MXReplicaTypeWorker: &mxv2.MXReplicaSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								v1.Container{
									Name:  "",
									Image: "mxjob/mxnet:gpu",
								},
							},
						},
					},
				},
			},
		},
	}
	for _, c := range testCases {
		err := ValidateAlphaTwoMXJobSpec(&c)
		if err.Error() != "MXJobSpec is not valid" {
			t.Error("Failed validate the alpha2.MXJobSpec")
		}
	}
}
