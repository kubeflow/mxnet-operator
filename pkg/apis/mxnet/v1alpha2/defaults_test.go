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

package v1alpha2

import (
	"reflect"
	"testing"

	"k8s.io/api/core/v1"

	"github.com/kubeflow/mxnet-operator/pkg/util"
)

const (
	testImage = "test-image:latest"
)

func expectedMXJob(cleanPodPolicy CleanPodPolicy, restartPolicy RestartPolicy, portName string, port int32) *MXJob {
	ports := []v1.ContainerPort{}

	// port not set
	if portName != "" {
		ports = append(ports,
			v1.ContainerPort{
				Name:          portName,
				ContainerPort: port,
			},
		)
	}

	// port set with custom name
	if portName != DefaultPortName {
		ports = append(ports,
			v1.ContainerPort{
				Name:          DefaultPortName,
				ContainerPort: DefaultPort,
			},
		)
	}

	return &MXJob{
		Spec: MXJobSpec{
			CleanPodPolicy: &cleanPodPolicy,
			MXReplicaSpecs: map[MXReplicaType]*MXReplicaSpec{
				MXReplicaTypeWorker: &MXReplicaSpec{
					Replicas:      Int32(1),
					RestartPolicy: restartPolicy,
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								v1.Container{
									Name:  DefaultContainerName,
									Image: testImage,
									Ports: ports,
								},
							},
						},
					},
				},
			},
		},
	}
}
func TestSetTypeNames(t *testing.T) {
	spec := &MXReplicaSpec{
		RestartPolicy: RestartPolicyAlways,
		Template: v1.PodTemplateSpec{
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					v1.Container{
						Name:  DefaultContainerName,
						Image: testImage,
						Ports: []v1.ContainerPort{
							v1.ContainerPort{
								Name:          DefaultPortName,
								ContainerPort: DefaultPort,
							},
						},
					},
				},
			},
		},
	}

	workerUpperCase := MXReplicaType("WORKER")
	original := &MXJob{
		Spec: MXJobSpec{
			MXReplicaSpecs: map[MXReplicaType]*MXReplicaSpec{
				workerUpperCase: spec,
			},
		},
	}

	setTypeNamesToCamelCase(original)
	if _, ok := original.Spec.MXReplicaSpecs[workerUpperCase]; ok {
		t.Errorf("Failed to delete key %s", workerUpperCase)
	}
	if _, ok := original.Spec.MXReplicaSpecs[MXReplicaTypeWorker]; !ok {
		t.Errorf("Failed to set key %s", MXReplicaTypeWorker)
	}
}

func TestSetDefaultMXJob(t *testing.T) {
	customPortName := "customPort"
	var customPort int32 = 1234
	customRestartPolicy := RestartPolicyNever

	testCases := map[string]struct {
		original *MXJob
		expected *MXJob
	}{
		"set replicas": {
			original: &MXJob{
				Spec: MXJobSpec{
					MXReplicaSpecs: map[MXReplicaType]*MXReplicaSpec{
						MXReplicaTypeWorker: &MXReplicaSpec{
							RestartPolicy: customRestartPolicy,
							Template: v1.PodTemplateSpec{
								Spec: v1.PodSpec{
									Containers: []v1.Container{
										v1.Container{
											Name:  DefaultContainerName,
											Image: testImage,
											Ports: []v1.ContainerPort{
												v1.ContainerPort{
													Name:          DefaultPortName,
													ContainerPort: DefaultPort,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: expectedMXJob(CleanPodPolicyAll, customRestartPolicy, DefaultPortName, DefaultPort),
		},
		"set replicas with default restartpolicy": {
			original: &MXJob{
				Spec: MXJobSpec{
					MXReplicaSpecs: map[MXReplicaType]*MXReplicaSpec{
						MXReplicaTypeWorker: &MXReplicaSpec{
							Template: v1.PodTemplateSpec{
								Spec: v1.PodSpec{
									Containers: []v1.Container{
										v1.Container{
											Name:  DefaultContainerName,
											Image: testImage,
											Ports: []v1.ContainerPort{
												v1.ContainerPort{
													Name:          DefaultPortName,
													ContainerPort: DefaultPort,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: expectedMXJob(CleanPodPolicyAll, DefaultRestartPolicy, DefaultPortName, DefaultPort),
		},
		"set replicas with default port": {
			original: &MXJob{
				Spec: MXJobSpec{
					MXReplicaSpecs: map[MXReplicaType]*MXReplicaSpec{
						MXReplicaTypeWorker: &MXReplicaSpec{
							Replicas:      Int32(1),
							RestartPolicy: customRestartPolicy,
							Template: v1.PodTemplateSpec{
								Spec: v1.PodSpec{
									Containers: []v1.Container{
										v1.Container{
											Name:  DefaultContainerName,
											Image: testImage,
										},
									},
								},
							},
						},
					},
				},
			},
			expected: expectedMXJob(CleanPodPolicyAll, customRestartPolicy, "", 0),
		},
		"set replicas adding default port": {
			original: &MXJob{
				Spec: MXJobSpec{
					MXReplicaSpecs: map[MXReplicaType]*MXReplicaSpec{
						MXReplicaTypeWorker: &MXReplicaSpec{
							Replicas:      Int32(1),
							RestartPolicy: customRestartPolicy,
							Template: v1.PodTemplateSpec{
								Spec: v1.PodSpec{
									Containers: []v1.Container{
										v1.Container{
											Name:  DefaultContainerName,
											Image: testImage,
											Ports: []v1.ContainerPort{
												v1.ContainerPort{
													Name:          customPortName,
													ContainerPort: customPort,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: expectedMXJob(CleanPodPolicyAll, customRestartPolicy, customPortName, customPort),
		},
		"set custom cleanpod policy": {
			original: &MXJob{
				Spec: MXJobSpec{
					CleanPodPolicy: cleanPodPolicyPointer(CleanPodPolicyRunning),
					MXReplicaSpecs: map[MXReplicaType]*MXReplicaSpec{
						MXReplicaTypeWorker: &MXReplicaSpec{
							Replicas:      Int32(1),
							RestartPolicy: customRestartPolicy,
							Template: v1.PodTemplateSpec{
								Spec: v1.PodSpec{
									Containers: []v1.Container{
										v1.Container{
											Name:  DefaultContainerName,
											Image: testImage,
											Ports: []v1.ContainerPort{
												v1.ContainerPort{
													Name:          customPortName,
													ContainerPort: customPort,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: expectedMXJob(CleanPodPolicyRunning, customRestartPolicy, customPortName, customPort),
		},
	}

	for name, tc := range testCases {
		SetDefaults_MXJob(tc.original)
		if !reflect.DeepEqual(tc.original, tc.expected) {
			t.Errorf("%s: Want\n%v; Got\n %v", name, util.Pformat(tc.expected), util.Pformat(tc.original))
		}
	}
}

func cleanPodPolicyPointer(cleanPodPolicy CleanPodPolicy) *CleanPodPolicy {
	c := cleanPodPolicy
	return &c
}
