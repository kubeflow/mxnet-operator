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

// Package controller provides a Kubernetes controller for a MXJob resource.
package mxnet

import (
	"fmt"
	"reflect"
	"time"

	commonv1 "github.com/kubeflow/common/pkg/apis/common/v1"
	commonutil "github.com/kubeflow/common/pkg/util"
	mxlogger "github.com/kubeflow/common/pkg/util"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	mxv1 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1"
)

const (
	// mxJobCreatedReason is added in a mxjob when it is created.
	mxJobCreatedReason = "MXJobCreated"
	// mxJobSucceededReason is added in a mxjob when it is succeeded.
	mxJobSucceededReason = "MXJobSucceeded"
	// mxJobRunningReason is added in a mxjob when it is running.
	mxJobRunningReason = "MXJobRunning"
	// mxJobFailedReason is added in a mxjob when it is failed.
	mxJobFailedReason = "MXJobFailed"
	// mxJobRestarting is added in a mxjob when it is restarting.
	mxJobRestartingReason = "MXJobRestarting"
)

func (tc *MXController) UpdateJobStatus(job interface{}, replicas map[commonv1.ReplicaType]*commonv1.ReplicaSpec, jobStatus *commonv1.JobStatus) error {
	mxjob, ok := job.(*mxv1.MXJob)
	if !ok {
		return fmt.Errorf("%v is not a type of MXJob", mxjob)
	}

	mxjobKey, err := KeyFunc(mxjob)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for mxjob object %#v: %v", mxjob, err))
		return err
	}

	for rtype, spec := range replicas {
		status := jobStatus.ReplicaStatuses[rtype]

		// Expect to have `replicas - succeeded` pods alive.
		succeeded := status.Succeeded
		expected := *(spec.Replicas) - succeeded
		running := status.Active
		failed := status.Failed

		log.Infof("MXJob=%s, ReplicaType=%s expected=%d, running=%d, succeeded=%d , failed=%d",
			mxjob.Name, rtype, expected, running, succeeded, failed)

		if mxjob.Status.StartTime == nil {
			now := metav1.Now()
			mxjob.Status.StartTime = &now
			// enqueue a sync to check if job past ActiveDeadlineSeconds
			if mxjob.Spec.RunPolicy.ActiveDeadlineSeconds != nil {
				mxlogger.LoggerForJob(mxjob).Infof("Job with ActiveDeadlineSeconds will sync after %d seconds", *mxjob.Spec.RunPolicy.ActiveDeadlineSeconds)
				tc.WorkQueue.AddAfter(mxjobKey, time.Duration(*mxjob.Spec.RunPolicy.ActiveDeadlineSeconds)*time.Second)
			}
		}

		if running > 0 {
			msg := fmt.Sprintf("MXJob %s is running.", mxjob.Name)
			err := commonutil.UpdateJobConditions(jobStatus, commonv1.JobRunning, mxJobRunningReason, msg)
			if err != nil {
				mxlogger.LoggerForJob(mxjob).Infof("Append mxjob condition error: %v", err)
				return err
			}
		}
		if expected == 0 {
			msg := fmt.Sprintf("MXJob %s is successfully completed.", mxjob.Name)
			tc.Recorder.Event(mxjob, corev1.EventTypeNormal, mxJobSucceededReason, msg)
			if mxjob.Status.CompletionTime == nil {
				now := metav1.Now()
				mxjob.Status.CompletionTime = &now
			}
			err := commonutil.UpdateJobConditions(jobStatus, commonv1.JobSucceeded, mxJobSucceededReason, msg)
			if err != nil {
				mxlogger.LoggerForJob(mxjob).Infof("Append mxjob condition error: %v", err)
				return err
			}
		}

		if failed > 0 {
			if spec.RestartPolicy == commonv1.RestartPolicyExitCode {
				msg := fmt.Sprintf("mxjob %s is restarting because %d %s replica(s) failed.", mxjob.Name, failed, rtype)
				tc.Recorder.Event(mxjob, corev1.EventTypeWarning, mxJobRestartingReason, msg)
				err := commonutil.UpdateJobConditions(jobStatus, commonv1.JobRestarting, mxJobRestartingReason, msg)
				if err != nil {
					mxlogger.LoggerForJob(mxjob).Infof("Append job condition error: %v", err)
					return err
				}
			} else {
				msg := fmt.Sprintf("mxjob %s is failed because %d %s replica(s) failed.", mxjob.Name, failed, rtype)
				tc.Recorder.Event(mxjob, corev1.EventTypeNormal, mxJobFailedReason, msg)
				if mxjob.Status.CompletionTime == nil {
					now := metav1.Now()
					mxjob.Status.CompletionTime = &now
				}
				err := commonutil.UpdateJobConditions(jobStatus, commonv1.JobFailed, mxJobFailedReason, msg)
				if err != nil {
					mxlogger.LoggerForJob(mxjob).Infof("Append job condition error: %v", err)
					return err
				}
			}
		}
	}

	return nil
}

// UpdateJobStatusInApiServer updates the status of the given MXJob.
func (tc *MXController) UpdateJobStatusInApiServer(job interface{}, jobStatus *commonv1.JobStatus) error {
	mxJob, ok := job.(*mxv1.MXJob)
	if !ok {
		return fmt.Errorf("%v is not a type of MXJob", mxJob)
	}

	if !reflect.DeepEqual(&mxJob.Status, jobStatus) {
		mxJob = mxJob.DeepCopy()
		mxJob.Status = *jobStatus.DeepCopy()
	}

	if _, err := tc.mxJobClientSet.KubeflowV1().MXJobs(mxJob.Namespace).UpdateStatus(mxJob); err != nil {
		mxlogger.LoggerForJob(mxJob).Error(err, " failed to update MxJob conditions in the API server")
		return err
	}

	return nil
}
