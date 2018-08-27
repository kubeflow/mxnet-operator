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

package v1alpha1

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	CRDKind       = "MXJob"
	CRDKindPlural = "mxjobs"
	CRDGroup      = "kubeflow.org"
	CRDVersion    = "v1alpha1"
	// Value of the APP label that gets applied to a lot of entities.
	AppLabel = "mxnet-job"
	// Defaults for the Spec
	Replicas   = 1
	PsRootPort = 9091
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +resource:path=mxjob

// MXJob describes mxjob info
type MXJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MXJobSpec   `json:"spec"`
	Status            MXJobStatus `json:"status"`
}

// MXJobSpec structure for storing the MXJob specifications
type MXJobSpec struct {
	// TODO(jlewi): Can we we get rid of this and use some value from Kubernetes or a random ide.
	RuntimeId string

	// ReplicaSpecs specifies the MX replicas to run.
	ReplicaSpecs []*MXReplicaSpec `json:"replicaSpecs"`

	// MXImage defines the mxnet docker image that should be used for default parameter server
	MXImage string `json:"mxImage,omitempty"`

	// TerminationPolicy specifies the condition that the mxjob should be considered finished.
	TerminationPolicy *TerminationPolicySpec `json:"terminationPolicy,omitempty"`

	// CleanupPodPolicy specifies the operation when mxjob is finished or failed.
	CleanupPodPolicy CleanupPodPolicy `json:"cleanupPodPolicy,omitempty"`

	// SchedulerName specifies the name of scheduler which should handle the MXJob
	SchedulerName string `json:"schedulerName,omitempty"`
}

// CleanupPodPolicy defines all kinds of types of cleanPodPolicy.
type CleanupPodPolicy string

const (
	CleanupPodUndefined CleanupPodPolicy = ""
	CleanupPodAll       CleanupPodPolicy = "All"
	CleanupPodRunning   CleanupPodPolicy = "Running"
	CleanupPodNone      CleanupPodPolicy = "None"
)

// TerminationPolicySpec structure for storing specifications for process termination
type TerminationPolicySpec struct {
	// Chief policy waits for a particular process (which is the chief) to exit.
	Chief *ChiefSpec `json:"chief,omitempty"`
}

// ChiefSpec structure storing the replica name and replica index
type ChiefSpec struct {
	ReplicaName  string `json:"replicaName"`
	ReplicaIndex int    `json:"replicaIndex"`
}

// MXReplicaType determines how a set of MX processes are handled.
type MXReplicaType string

const (
	SCHEDULER MXReplicaType = "SCHEDULER"
	SERVER    MXReplicaType = "SERVER"
	WORKER    MXReplicaType = "WORKER"
)

const (
	DefaultMXContainer string = "mxnet"
	DefaultMXImage     string = "mxjob/mxnet:gpu"
)

// MXReplicaSpec might be useful if you wanted to have a separate set of workers to do eval.
type MXReplicaSpec struct {
	// Replicas is the number of desired replicas.
	// This is a pointer to distinguish between explicit zero and unspecified.
	// Defaults to 1.
	// More info: http://kubernetes.io/docs/user-guide/replication-controller#what-is-a-replication-controller
	// +optional
	Replicas *int32              `json:"replicas,omitempty" protobuf:"varint,1,opt,name=replicas"`
	Template *v1.PodTemplateSpec `json:"template,omitempty" protobuf:"bytes,3,opt,name=template"`
	// PsRootPort is the port to use for MX services.
	MXReplicaType `json:"mxReplicaType"`
	PsRootPort    *int32 `json:"PsRootPort,omitempty" protobuf:"varint,1,opt,name=PsRootPort"`
}

// MXJobPhase is a enum to store the phase of mx job
type MXJobPhase string

const (
	MXJobPhaseNone     MXJobPhase = ""
	MXJobPhaseCreating MXJobPhase = "Creating"
	MXJobPhaseRunning  MXJobPhase = "Running"
	MXJobPhaseCleanUp  MXJobPhase = "CleanUp"
	MXJobPhaseFailed   MXJobPhase = "Failed"
	MXJobPhaseDone     MXJobPhase = "Done"
)

// State is a enum to store the state of mx job
type State string

const (
	StateUnknown   State = "Unknown"
	StateRunning   State = "Running"
	StateSucceeded State = "Succeeded"
	StateFailed    State = "Failed"
)

// MXJobStatus is a structure for storing the status of mx jobs
type MXJobStatus struct {
	// Phase is the MXJob running phase
	Phase  MXJobPhase `json:"phase"`
	Reason string     `json:"reason"`

	// State indicates the state of the job.
	State State `json:"state"`

	// ReplicaStatuses specifies the status of each MX replica.
	ReplicaStatuses []*MXReplicaStatus `json:"replicaStatuses"`
}

// ReplicaState is a enum to store the status of replica
type ReplicaState string

const (
	ReplicaStateUnknown   ReplicaState = "Unknown"
	ReplicaStateRunning   ReplicaState = "Running"
	ReplicaStateFailed    ReplicaState = "Failed"
	ReplicaStateSucceeded ReplicaState = "Succeeded"
)

// MXReplicaStatus  is a structure for storing the status of mx replica
type MXReplicaStatus struct {
	MXReplicaType `json:"mx_replica_type"`

	// State is the overall state of the replica
	State ReplicaState `json:"state"`

	// ReplicasStates provides the number of replicas in each status.
	ReplicasStates map[ReplicaState]int
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +resource:path=mxjobs

// MXJobList is a list of MXJobs clusters.
type MXJobList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a list of MXJobs
	Items []MXJob `json:"items"`
}

// ControllerConfig is a structure for storing the controller configuration
type ControllerConfig struct {
	// Accelerators is a map from the name of the accelerator to the config for that accelerator.
	// This should match the value specified as a container limit.
	// e.g. alpha.kubernetes.io/nvidia-gpu
	Accelerators map[string]AcceleratorConfig

	// Path to the file containing the grpc server source
	GrpcServerFilePath string
}

// AcceleratorVolume represents a host path that must be mounted into
// each container that needs to use GPUs.
type AcceleratorVolume struct {
	Name      string
	HostPath  string
	MountPath string
}

// AcceleratorConfig represents accelerator volumes to be mounted into container along with environment variables.
type AcceleratorConfig struct {
	Volumes []AcceleratorVolume
	EnvVars []EnvironmentVariableConfig
}

// EnvironmentVariableConfig represents the environment variables and their values.
type EnvironmentVariableConfig struct {
	Name  string
	Value string
}
