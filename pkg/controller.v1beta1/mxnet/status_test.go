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
	"testing"

	"k8s.io/api/core/v1"

	mxv1beta1 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1beta1"
	"github.com/kubeflow/mxnet-operator/pkg/common/util/v1beta1/testutil"
)

func TestFailed(t *testing.T) {
	mxJob := testutil.NewMXJob(3, 0)
	initializeMXReplicaStatuses(mxJob, mxv1beta1.MXReplicaTypeWorker)
	pod := testutil.NewBasePod("pod", mxJob, t)
	pod.Status.Phase = v1.PodFailed
	updateMXJobReplicaStatuses(mxJob, mxv1beta1.MXReplicaTypeWorker, pod)
	if mxJob.Status.MXReplicaStatuses[mxv1beta1.MXReplicaTypeWorker].Failed != 1 {
		t.Errorf("Failed to set the failed to 1")
	}
	err := updateStatusSingle(mxJob, mxv1beta1.MXReplicaTypeWorker, 3, false, false)
	if err != nil {
		t.Errorf("Expected error %v to be nil", err)
	}
	found := false
	for _, condition := range mxJob.Status.Conditions {
		if condition.Type == mxv1beta1.MXJobFailed {
			found = true
		}
	}
	if !found {
		t.Errorf("Failed condition is not found")
	}
}

func TestStatus(t *testing.T) {
	type testCase struct {
		description string
		mxJob       *mxv1beta1.MXJob

		expectedFailedScheduler    int32
		expectedSucceededScheduler int32
		expectedActiveScheduler    int32

		expectedFailedWorker    int32
		expectedSucceededWorker int32
		expectedActiveWorker    int32

		expectedFailedServer    int32
		expectedSucceededServer int32
		expectedActiveServer    int32

		restart            bool
		schedulerCompleted bool

		expectedType mxv1beta1.MXJobConditionType
	}

	testCases := []testCase{
		testCase{
			description:                "Worker is failed",
			mxJob:                      testutil.NewMXJob(1, 0),
			expectedFailedScheduler:    0,
			expectedSucceededScheduler: 0,
			expectedActiveScheduler:    0,
			expectedFailedWorker:       1,
			expectedSucceededWorker:    0,
			expectedActiveWorker:       0,
			expectedFailedServer:       0,
			expectedSucceededServer:    0,
			expectedActiveServer:       0,
			restart:                    false,
			schedulerCompleted:         false,
			expectedType:               mxv1beta1.MXJobFailed,
		},
		testCase{
			description:                "Worker is succeeded",
			mxJob:                      testutil.NewMXJobWithScheduler(1, 0),
			expectedFailedScheduler:    0,
			expectedSucceededScheduler: 1,
			expectedActiveScheduler:    0,
			expectedFailedWorker:       0,
			expectedSucceededWorker:    1,
			expectedActiveWorker:       0,
			expectedFailedServer:       0,
			expectedSucceededServer:    0,
			expectedActiveServer:       0,
			restart:                    false,
			schedulerCompleted:         true,
			expectedType:               mxv1beta1.MXJobSucceeded,
		},
		testCase{
			description:                " Worker is running",
			mxJob:                      testutil.NewMXJobWithScheduler(1, 0),
			expectedFailedScheduler:    0,
			expectedSucceededScheduler: 0,
			expectedActiveScheduler:    1,
			expectedFailedWorker:       0,
			expectedSucceededWorker:    0,
			expectedActiveWorker:       1,
			expectedFailedServer:       0,
			expectedSucceededServer:    0,
			expectedActiveServer:       0,
			restart:                    false,
			schedulerCompleted:         false,
			expectedType:               mxv1beta1.MXJobRunning,
		},
		testCase{
			description:                " 2 workers are succeeded, 2 workers are active",
			mxJob:                      testutil.NewMXJobWithScheduler(4, 0),
			expectedFailedScheduler:    0,
			expectedSucceededScheduler: 0,
			expectedActiveScheduler:    1,
			expectedFailedWorker:       0,
			expectedSucceededWorker:    2,
			expectedActiveWorker:       2,
			expectedFailedServer:       0,
			expectedSucceededServer:    0,
			expectedActiveServer:       0,
			restart:                    false,
			schedulerCompleted:         false,
			expectedType:               mxv1beta1.MXJobRunning,
		},
		testCase{
			description:                " 2 workers are running, 2 workers are failed",
			mxJob:                      testutil.NewMXJobWithScheduler(4, 0),
			expectedFailedScheduler:    0,
			expectedSucceededScheduler: 0,
			expectedActiveScheduler:    1,
			expectedFailedWorker:       2,
			expectedSucceededWorker:    0,
			expectedActiveWorker:       2,
			expectedFailedServer:       0,
			expectedSucceededServer:    0,
			expectedActiveServer:       0,
			restart:                    false,
			schedulerCompleted:         false,
			expectedType:               mxv1beta1.MXJobFailed,
		},
		testCase{
			description:                " 2 workers are succeeded, 2 workers are failed",
			mxJob:                      testutil.NewMXJobWithScheduler(4, 0),
			expectedFailedScheduler:    0,
			expectedSucceededScheduler: 0,
			expectedActiveScheduler:    1,
			expectedFailedWorker:       2,
			expectedSucceededWorker:    2,
			expectedActiveWorker:       0,
			expectedFailedServer:       0,
			expectedSucceededServer:    0,
			expectedActiveServer:       0,
			restart:                    false,
			schedulerCompleted:         false,
			expectedType:               mxv1beta1.MXJobFailed,
		},
	}

	for i, c := range testCases {
		initializeMXReplicaStatuses(c.mxJob, mxv1beta1.MXReplicaTypeScheduler)
		initializeMXReplicaStatuses(c.mxJob, mxv1beta1.MXReplicaTypeServer)
		initializeMXReplicaStatuses(c.mxJob, mxv1beta1.MXReplicaTypeWorker)

		setStatusForTest(c.mxJob, mxv1beta1.MXReplicaTypeScheduler, c.expectedFailedScheduler, c.expectedSucceededScheduler, c.expectedActiveScheduler, t)
		setStatusForTest(c.mxJob, mxv1beta1.MXReplicaTypeServer, c.expectedFailedServer, c.expectedSucceededServer, c.expectedActiveServer, t)
		setStatusForTest(c.mxJob, mxv1beta1.MXReplicaTypeWorker, c.expectedFailedWorker, c.expectedSucceededWorker, c.expectedActiveWorker, t)

		if _, ok := c.mxJob.Spec.MXReplicaSpecs[mxv1beta1.MXReplicaTypeScheduler]; ok {
			err := updateStatusSingle(c.mxJob, mxv1beta1.MXReplicaTypeScheduler, 1, c.restart, c.schedulerCompleted)
			if err != nil {
				t.Errorf("%s: Expected error %v to be nil", c.description, err)
			}
			if c.mxJob.Spec.MXReplicaSpecs[mxv1beta1.MXReplicaTypeWorker] != nil {
				replicas := c.mxJob.Spec.MXReplicaSpecs[mxv1beta1.MXReplicaTypeWorker].Replicas
				err := updateStatusSingle(c.mxJob, mxv1beta1.MXReplicaTypeWorker, int(*replicas), c.restart, c.schedulerCompleted)
				if err != nil {
					t.Errorf("%s: Expected error %v to be nil", c.description, err)
				}
			}
			if c.mxJob.Spec.MXReplicaSpecs[mxv1beta1.MXReplicaTypeServer] != nil {
				replicas := c.mxJob.Spec.MXReplicaSpecs[mxv1beta1.MXReplicaTypeServer].Replicas
				err := updateStatusSingle(c.mxJob, mxv1beta1.MXReplicaTypeServer, int(*replicas), c.restart, c.schedulerCompleted)
				if err != nil {
					t.Errorf("%s: Expected error %v to be nil", c.description, err)
				}
			}
		} else {
			if c.mxJob.Spec.MXReplicaSpecs[mxv1beta1.MXReplicaTypeWorker] != nil {
				replicas := c.mxJob.Spec.MXReplicaSpecs[mxv1beta1.MXReplicaTypeWorker].Replicas
				err := updateStatusSingle(c.mxJob, mxv1beta1.MXReplicaTypeWorker, int(*replicas), c.restart, c.schedulerCompleted)
				if err != nil {
					t.Errorf("%s: Expected error %v to be nil", c.description, err)
				}
			}
			if c.mxJob.Spec.MXReplicaSpecs[mxv1beta1.MXReplicaTypeServer] != nil {
				replicas := c.mxJob.Spec.MXReplicaSpecs[mxv1beta1.MXReplicaTypeServer].Replicas
				err := updateStatusSingle(c.mxJob, mxv1beta1.MXReplicaTypeServer, int(*replicas), c.restart, c.schedulerCompleted)
				if err != nil {
					t.Errorf("%s: Expected error %v to be nil", c.description, err)
				}
			}
		}

		// Test filterOutCondition
		filterOutConditionTest(c.mxJob.Status, t)

		found := false
		for _, condition := range c.mxJob.Status.Conditions {
			if condition.Type == c.expectedType {
				found = true
			}
		}
		if !found {
			t.Errorf("Case[%d]%s: Condition %s is not found", i, c.description, c.expectedType)
		}
	}
}

func setStatusForTest(mxJob *mxv1beta1.MXJob, typ mxv1beta1.MXReplicaType, failed, succeeded, active int32, t *testing.T) {
	pod := testutil.NewBasePod("pod", mxJob, t)
	var i int32
	for i = 0; i < failed; i++ {
		pod.Status.Phase = v1.PodFailed
		updateMXJobReplicaStatuses(mxJob, typ, pod)
	}
	for i = 0; i < succeeded; i++ {
		pod.Status.Phase = v1.PodSucceeded
		updateMXJobReplicaStatuses(mxJob, typ, pod)
	}
	for i = 0; i < active; i++ {
		pod.Status.Phase = v1.PodRunning
		updateMXJobReplicaStatuses(mxJob, typ, pod)
	}
}

func filterOutConditionTest(status mxv1beta1.MXJobStatus, t *testing.T) {
	flag := isFailed(status) || isSucceeded(status)
	for _, condition := range status.Conditions {
		if flag && condition.Type == mxv1beta1.MXJobRunning && condition.Status == v1.ConditionTrue {
			t.Error("Error condition status when succeeded or failed")
		}
	}
}
