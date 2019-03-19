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

package v1beta1

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +resource:path=mxjob

// MXJob represents the configuration of signal MXJob
type MXJob struct {
	metav1.TypeMeta `json:",inline"`

	// Standard object's metadata.
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of the MXJob.
	Spec MXJobSpec `json:"spec,omitempty"`

	// Most recently observed status of the MXJob.
	// This data may not be up to date.
	// Populated by the system.
	// Read-only.
	Status MXJobStatus `json:"status,omitempty"`
}

// MXJobSpec is a desired state description of the MXJob.
type MXJobSpec struct {
	// CleanPodPolicy defines the policy to kill pods after MXJob is
	// succeeded.
	// Default to Running.
	CleanPodPolicy *CleanPodPolicy `json:"cleanPodPolicy,omitempty"`

	// TTLSecondsAfterFinished is the TTL to clean up mxnet-jobs (temporary
	// before kubernetes adds the cleanup controller).
	// It may take extra ReconcilePeriod seconds for the cleanup, since
	// reconcile gets called periodically.
	// Default to infinite.
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`

	// JobMode specify the kind of MXjob to do. Different mode may have
	// different MXReplicaSpecs request
	JobMode JobModeType `json:"jobMode"`

	// MXReplicaSpecs is map of MXReplicaType and MXReplicaSpec
	// specifies the MX replicas to run.
	// For example,
	//   {
	//     "Scheduler": MXReplicaSpec,
	//     "Server": MXReplicaSpec,
	//     "Worker": MXReplicaSpec,
	//   }
	MXReplicaSpecs map[MXReplicaType]*MXReplicaSpec `json:"mxReplicaSpecs"`
}

// MXReplicaSpec is a description of the MXReplica
type MXReplicaSpec struct {
	// Replicas is the desired number of replicas of the given template.
	// If unspecified, defaults to 1.
	Replicas *int32 `json:"replicas,omitempty"`

	// Label is used as tunerServerKey, it's designed for tvm auto-tuning.
	Label string `json:"label,omitempty"`

	// Template is the object that describes the pod that
	// will be created for this MXReplica. RestartPolicy in PodTemplateSpec
	// will be overide by RestartPolicy in MXReplicaSpec
	Template v1.PodTemplateSpec `json:"template,omitempty"`

	// Restart policy for all MXReplicas within the MXJob.
	// One of Always, OnFailure, Never and ExitCode.
	// Default to Never.
	RestartPolicy RestartPolicy `json:"restartPolicy,omitempty"`
}

// CleanPodPolicy describes how to deal with pods when the MXJob is finished.
type CleanPodPolicy string

const (
	CleanPodPolicyUndefined CleanPodPolicy = ""
	CleanPodPolicyAll       CleanPodPolicy = "All"
	CleanPodPolicyRunning   CleanPodPolicy = "Running"
	CleanPodPolicyNone      CleanPodPolicy = "None"
)

// RestartPolicy describes how the MXReplicas should be restarted.
// Only one of the following restart policies may be specified.
// If none of the following policies is specified, the default one
// is RestartPolicyAlways.
type RestartPolicy string

const (
	RestartPolicyAlways    RestartPolicy = "Always"
	RestartPolicyOnFailure RestartPolicy = "OnFailure"
	RestartPolicyNever     RestartPolicy = "Never"

	// `ExitCode` policy means that user should add exit code by themselves,
	// `mxnet-operator` will check these exit codes to
	// determine the behavior when an error occurs:
	// - 1-127: permanent error, do not restart.
	// - 128-255: retryable error, will restart the pod.
	RestartPolicyExitCode RestartPolicy = "ExitCode"
)

// JobModeType id the type for JobMode
type JobModeType string

const (
	// Train Mode, in this mode requested MXReplicaSpecs need
	// has Server, Scheduler, Worker
	MXTrain JobModeType = "MXTrain"

	// Tune Mode, in this mode requested MXReplicaSpecs need
	// has Tuner
	MXTune JobModeType = "MXTune"
)

// MXReplicaType is the type for MXReplica.
type MXReplicaType string

const (
	// MXReplicaTypeScheduler is the type for scheduler replica in MXNet.
	MXReplicaTypeScheduler MXReplicaType = "Scheduler"

	// MXReplicaTypeServer is the type for parameter servers of distributed MXNet.
	MXReplicaTypeServer MXReplicaType = "Server"

	// MXReplicaTypeWorker is the type for workers of distributed MXNet.
	// This is also used for non-distributed MXNet.
	MXReplicaTypeWorker MXReplicaType = "Worker"

	// MXReplicaTypeTunerTracker
	// This the auto-tuning tracker e.g. autotvm tracker, it will dispatch tuning task to TunerServer
	MXReplicaTypeTunerTracker MXReplicaType = "TunerTracker"

	// MXReplicaTypeTunerServer
	MXReplicaTypeTunerServer MXReplicaType = "TunerServer"

	// MXReplicaTuner is the type for auto-tuning of distributed MXNet.
	// This is also used for non-distributed MXNet.
	MXReplicaTypeTuner MXReplicaType = "Tuner"
)

// MXJobStatus represents the current observed state of the MXJob.
type MXJobStatus struct {
	// Conditions is an array of current observed MXJob conditions.
	Conditions []MXJobCondition `json:"conditions"`

	// MXReplicaStatuses is map of MXReplicaType and MXReplicaStatus,
	// specifies the status of each MXReplica.
	MXReplicaStatuses map[MXReplicaType]*MXReplicaStatus `json:"mxReplicaStatuses"`

	// Represents time when the MXJob was acknowledged by the MXJob controller.
	// It is not guaranteed to be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// Represents time when the MXJob was completed. It is not guaranteed to
	// be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Represents last time when the MXJob was reconciled. It is not guaranteed to
	// be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`
}

// MXReplicaStatus represents the current observed state of the MXReplica.
type MXReplicaStatus struct {
	// The number of actively running pods.
	Active int32 `json:"active,omitempty"`

	// The number of pods which reached phase Succeeded.
	Succeeded int32 `json:"succeeded,omitempty"`

	// The number of pods which reached phase Failed.
	Failed int32 `json:"failed,omitempty"`
}

// MXJobCondition describes the state of the MXJob at a certain point.
type MXJobCondition struct {
	// Type of MXJob condition.
	Type MXJobConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
}

// MXJobConditionType defines all kinds of types of MXJobStatus.
type MXJobConditionType string

const (
	// MXJobCreated means the mxjob has been accepted by the system,
	// but one or more of the pods/services has not been started.
	// This includes time before pods being scheduled and launched.
	MXJobCreated MXJobConditionType = "Created"

	// MXJobRunning means all sub-resources (e.g. services/pods) of this MXJob
	// have been successfully scheduled and launched.
	// The training is running without error.
	MXJobRunning MXJobConditionType = "Running"

	// MXJobRestarting means one or more sub-resources (e.g. services/pods) of this MXJob
	// reached phase failed but maybe restarted according to it's restart policy
	// which specified by user in v1.PodTemplateSpec.
	// The training is freezing/pending.
	MXJobRestarting MXJobConditionType = "Restarting"

	// MXJobSucceeded means all sub-resources (e.g. services/pods) of this MXJob
	// reached phase have terminated in success.
	// The training is complete without error.
	MXJobSucceeded MXJobConditionType = "Succeeded"

	// MXJobFailed means one or more sub-resources (e.g. services/pods) of this MXJob
	// reached phase failed with no restarting.
	// The training has failed its execution.
	MXJobFailed MXJobConditionType = "Failed"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +resource:path=mxjobs

// MXJobList is a list of MXJobs.
type MXJobList struct {
	metav1.TypeMeta `json:",inline"`

	// Standard list metadata.
	metav1.ListMeta `json:"metadata,omitempty"`

	// List of MXJobs.
	Items []MXJob `json:"items"`
}
