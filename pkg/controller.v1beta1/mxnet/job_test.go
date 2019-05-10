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

package mxnet

import (
	"testing"
	"time"

	"k8s.io/api/core/v1"
	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/kubeflow/mxnet-operator/cmd/mxnet-operator.v1beta1/app/options"
	mxv1beta1 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1beta1"
	mxjobclientset "github.com/kubeflow/mxnet-operator/pkg/client/clientset/versioned"
	"github.com/kubeflow/mxnet-operator/pkg/common/util/v1beta1/testutil"
	"github.com/kubeflow/tf-operator/pkg/control"
)

func TestAddMXJob(t *testing.T) {
	// Prepare the clientset and controller for the test.
	kubeClientSet := kubeclientset.NewForConfigOrDie(&rest.Config{
		Host: "",
		ContentConfig: rest.ContentConfig{
			GroupVersion: &v1.SchemeGroupVersion,
		},
	},
	)
	config := &rest.Config{
		Host: "",
		ContentConfig: rest.ContentConfig{
			GroupVersion: &mxv1beta1.SchemeGroupVersion,
		},
	}
	mxJobClientSet := mxjobclientset.NewForConfigOrDie(config)
	ctr, _, _ := newMXController(config, kubeClientSet, mxJobClientSet, controller.NoResyncPeriodFunc, options.ServerOption{})
	ctr.mxJobInformerSynced = testutil.AlwaysReady
	ctr.PodInformerSynced = testutil.AlwaysReady
	ctr.ServiceInformerSynced = testutil.AlwaysReady
	mxJobIndexer := ctr.mxJobInformer.GetIndexer()

	stopCh := make(chan struct{})
	run := func(<-chan struct{}) {
		ctr.Run(testutil.ThreadCount, stopCh)
	}
	go run(stopCh)

	var key string
	syncChan := make(chan string)
	ctr.syncHandler = func(mxJobKey string) (bool, error) {
		key = mxJobKey
		<-syncChan
		return true, nil
	}
	ctr.updateStatusHandler = func(mxjob *mxv1beta1.MXJob) error {
		return nil
	}
	ctr.deleteMXJobHandler = func(mxjob *mxv1beta1.MXJob) error {
		return nil
	}

	mxJob := testutil.NewMXJob(1, 0)
	unstructured, err := testutil.ConvertMXJobToUnstructured(mxJob)
	if err != nil {
		t.Errorf("Failed to convert the MXJob to Unstructured: %v", err)
	}
	if err := mxJobIndexer.Add(unstructured); err != nil {
		t.Errorf("Failed to add mxjob to mxJobIndexer: %v", err)
	}
	ctr.addMXJob(unstructured)

	syncChan <- "sync"
	if key != testutil.GetKey(mxJob, t) {
		t.Errorf("Failed to enqueue the MXJob %s: expected %s, got %s", mxJob.Name, testutil.GetKey(mxJob, t), key)
	}
	close(stopCh)
}

func TestCopyLabelsAndAnnotation(t *testing.T) {
	// Prepare the clientset and controller for the test.
	kubeClientSet := kubeclientset.NewForConfigOrDie(&rest.Config{
		Host: "",
		ContentConfig: rest.ContentConfig{
			GroupVersion: &v1.SchemeGroupVersion,
		},
	},
	)
	config := &rest.Config{
		Host: "",
		ContentConfig: rest.ContentConfig{
			GroupVersion: &mxv1beta1.SchemeGroupVersion,
		},
	}
	mxJobClientSet := mxjobclientset.NewForConfigOrDie(config)
	ctr, _, _ := newMXController(config, kubeClientSet, mxJobClientSet, controller.NoResyncPeriodFunc, options.ServerOption{})
	fakePodControl := &controller.FakePodControl{}
	ctr.PodControl = fakePodControl
	ctr.mxJobInformerSynced = testutil.AlwaysReady
	ctr.PodInformerSynced = testutil.AlwaysReady
	ctr.ServiceInformerSynced = testutil.AlwaysReady
	mxJobIndexer := ctr.mxJobInformer.GetIndexer()

	stopCh := make(chan struct{})
	run := func(<-chan struct{}) {
		ctr.Run(testutil.ThreadCount, stopCh)
	}
	go run(stopCh)

	ctr.updateStatusHandler = func(mxJob *mxv1beta1.MXJob) error {
		return nil
	}

	mxJob := testutil.NewMXJob(1, 0)
	annotations := map[string]string{
		"annotation1": "1",
	}
	labels := map[string]string{
		"label1": "1",
	}
	mxJob.Spec.MXReplicaSpecs[mxv1beta1.MXReplicaTypeWorker].Template.Labels = labels
	mxJob.Spec.MXReplicaSpecs[mxv1beta1.MXReplicaTypeWorker].Template.Annotations = annotations
	unstructured, err := testutil.ConvertMXJobToUnstructured(mxJob)
	if err != nil {
		t.Errorf("Failed to convert the MXJob to Unstructured: %v", err)
	}

	if err := mxJobIndexer.Add(unstructured); err != nil {
		t.Errorf("Failed to add mxjob to mxJobIndexer: %v", err)
	}

	_, err = ctr.syncMXJob(testutil.GetKey(mxJob, t))
	if err != nil {
		t.Errorf("%s: unexpected error when syncing jobs %v", mxJob.Name, err)
	}

	if len(fakePodControl.Templates) != 1 {
		t.Errorf("Expected to create 1 pod while got %d", len(fakePodControl.Templates))
	}
	actual := fakePodControl.Templates[0]
	v, exist := actual.Labels["label1"]
	if !exist {
		t.Errorf("Labels does not exist")
	}
	if v != "1" {
		t.Errorf("Labels value do not equal")
	}

	v, exist = actual.Annotations["annotation1"]
	if !exist {
		t.Errorf("Annotations does not exist")
	}
	if v != "1" {
		t.Errorf("Annotations value does not equal")
	}

	close(stopCh)
}

func TestDeletePodsAndServices(t *testing.T) {
	type testCase struct {
		description string
		mxJob       *mxv1beta1.MXJob

		pendingSchedulerPods   int32
		activeSchedulerPods    int32
		succeededSchedulerPods int32
		failedSchedulerPods    int32

		pendingWorkerPods   int32
		activeWorkerPods    int32
		succeededWorkerPods int32
		failedWorkerPods    int32

		pendingServerPods   int32
		activeServerPods    int32
		succeededServerPods int32
		failedServerPods    int32

		activeSchedulerServices int32
		activeWorkerServices    int32
		activeServerServices    int32

		expectedPodDeletions int
	}

	testCases := []testCase{
		testCase{
			description: "1 scheduler , 4 workers and 2 server is running, policy is all",
			mxJob:       testutil.NewMXJobWithCleanPolicy(1, 4, 2, mxv1beta1.CleanPodPolicyAll),

			pendingSchedulerPods:   0,
			activeSchedulerPods:    1,
			succeededSchedulerPods: 0,
			failedSchedulerPods:    0,

			pendingWorkerPods:   0,
			activeWorkerPods:    4,
			succeededWorkerPods: 0,
			failedWorkerPods:    0,

			pendingServerPods:   0,
			activeServerPods:    2,
			succeededServerPods: 0,
			failedServerPods:    0,

			activeSchedulerServices: 1,
			activeWorkerServices:    4,
			activeServerServices:    2,

			expectedPodDeletions: 7,
		},
		testCase{
			description: "1 scheduler, 4 workers and 2 servers is running, policy is running",
			mxJob:       testutil.NewMXJobWithCleanPolicy(1, 4, 2, mxv1beta1.CleanPodPolicyRunning),

			pendingSchedulerPods:   0,
			activeSchedulerPods:    1,
			succeededSchedulerPods: 0,
			failedSchedulerPods:    0,

			pendingWorkerPods:   0,
			activeWorkerPods:    4,
			succeededWorkerPods: 0,
			failedWorkerPods:    0,

			pendingServerPods:   0,
			activeServerPods:    2,
			succeededServerPods: 0,
			failedServerPods:    0,

			activeSchedulerServices: 1,
			activeWorkerServices:    4,
			activeServerServices:    2,

			expectedPodDeletions: 7,
		},
		testCase{
			description: "1 scheduler, 4 workers and 2 servers is succeeded, policy is running",
			mxJob:       testutil.NewMXJobWithCleanPolicy(1, 4, 2, mxv1beta1.CleanPodPolicyRunning),

			pendingSchedulerPods:   0,
			activeSchedulerPods:    0,
			succeededSchedulerPods: 1,
			failedSchedulerPods:    0,

			pendingWorkerPods:   0,
			activeWorkerPods:    0,
			succeededWorkerPods: 4,
			failedWorkerPods:    0,

			pendingServerPods:   0,
			activeServerPods:    0,
			succeededServerPods: 2,
			failedServerPods:    0,

			activeSchedulerServices: 1,
			activeWorkerServices:    4,
			activeServerServices:    2,

			expectedPodDeletions: 0,
		},
		testCase{
			description: "1 scheduler, 4 workers and 2 servers is succeeded, policy is None",
			mxJob:       testutil.NewMXJobWithCleanPolicy(1, 4, 2, mxv1beta1.CleanPodPolicyNone),

			pendingSchedulerPods:   0,
			activeSchedulerPods:    0,
			succeededSchedulerPods: 1,
			failedSchedulerPods:    0,

			pendingWorkerPods:   0,
			activeWorkerPods:    0,
			succeededWorkerPods: 4,
			failedWorkerPods:    0,

			pendingServerPods:   0,
			activeServerPods:    0,
			succeededServerPods: 2,
			failedServerPods:    0,

			activeSchedulerServices: 1,
			activeWorkerServices:    4,
			activeServerServices:    2,

			expectedPodDeletions: 0,
		},
	}
	for _, tc := range testCases {
		// Prepare the clientset and controller for the test.
		kubeClientSet := kubeclientset.NewForConfigOrDie(&rest.Config{
			Host: "",
			ContentConfig: rest.ContentConfig{
				GroupVersion: &v1.SchemeGroupVersion,
			},
		},
		)
		config := &rest.Config{
			Host: "",
			ContentConfig: rest.ContentConfig{
				GroupVersion: &mxv1beta1.SchemeGroupVersion,
			},
		}
		mxJobClientSet := mxjobclientset.NewForConfigOrDie(config)
		ctr, kubeInformerFactory, _ := newMXController(config, kubeClientSet, mxJobClientSet, controller.NoResyncPeriodFunc, options.ServerOption{})
		fakePodControl := &controller.FakePodControl{}
		ctr.PodControl = fakePodControl
		fakeServiceControl := &control.FakeServiceControl{}
		ctr.ServiceControl = fakeServiceControl
		ctr.Recorder = &record.FakeRecorder{}
		ctr.mxJobInformerSynced = testutil.AlwaysReady
		ctr.PodInformerSynced = testutil.AlwaysReady
		ctr.ServiceInformerSynced = testutil.AlwaysReady
		mxJobIndexer := ctr.mxJobInformer.GetIndexer()
		ctr.updateStatusHandler = func(mxJob *mxv1beta1.MXJob) error {
			return nil
		}

		// Set succeeded to run the logic about deleting.
		err := updateMXJobConditions(tc.mxJob, mxv1beta1.MXJobSucceeded, mxJobSucceededReason, "")
		if err != nil {
			t.Errorf("Append mxjob condition error: %v", err)
		}

		unstructured, err := testutil.ConvertMXJobToUnstructured(tc.mxJob)
		if err != nil {
			t.Errorf("Failed to convert the MXJob to Unstructured: %v", err)
		}

		if err := mxJobIndexer.Add(unstructured); err != nil {
			t.Errorf("Failed to add mxjob to mxJobIndexer: %v", err)
		}

		podIndexer := kubeInformerFactory.Core().V1().Pods().Informer().GetIndexer()
		testutil.SetPodsStatuses(podIndexer, tc.mxJob, testutil.LabelScheduler, tc.pendingSchedulerPods, tc.activeSchedulerPods, tc.succeededSchedulerPods, tc.failedSchedulerPods, t)
		testutil.SetPodsStatuses(podIndexer, tc.mxJob, testutil.LabelWorker, tc.pendingWorkerPods, tc.activeWorkerPods, tc.succeededWorkerPods, tc.failedWorkerPods, t)
		testutil.SetPodsStatuses(podIndexer, tc.mxJob, testutil.LabelServer, tc.pendingServerPods, tc.activeServerPods, tc.succeededServerPods, tc.failedServerPods, t)

		serviceIndexer := kubeInformerFactory.Core().V1().Services().Informer().GetIndexer()
		testutil.SetServices(serviceIndexer, tc.mxJob, testutil.LabelScheduler, tc.activeSchedulerServices, t)
		testutil.SetServices(serviceIndexer, tc.mxJob, testutil.LabelWorker, tc.activeWorkerServices, t)
		testutil.SetServices(serviceIndexer, tc.mxJob, testutil.LabelServer, tc.activeServerServices, t)

		forget, err := ctr.syncMXJob(testutil.GetKey(tc.mxJob, t))
		if err != nil {
			t.Errorf("%s: unexpected error when syncing jobs %v", tc.description, err)
		}
		if !forget {
			t.Errorf("%s: unexpected forget value. Expected true, saw %v\n", tc.description, forget)
		}

		if len(fakePodControl.DeletePodName) != tc.expectedPodDeletions {
			t.Errorf("%s: unexpected number of pod deletes.  Expected %d, saw %d\n", tc.description, tc.expectedPodDeletions, len(fakePodControl.DeletePodName))
		}
		if len(fakeServiceControl.DeleteServiceName) != tc.expectedPodDeletions {
			t.Errorf("%s: unexpected number of service deletes.  Expected %d, saw %d\n", tc.description, tc.expectedPodDeletions, len(fakeServiceControl.DeleteServiceName))
		}
	}
}

func TestCleanupMXJob(t *testing.T) {
	type testCase struct {
		description string
		mxJob       *mxv1beta1.MXJob

		pendingSchedulerPods   int32
		activeSchedulerPods    int32
		succeededSchedulerPods int32
		failedSchedulerPods    int32

		pendingWorkerPods   int32
		activeWorkerPods    int32
		succeededWorkerPods int32
		failedWorkerPods    int32

		pendingServerPods   int32
		activeServerPods    int32
		succeededServerPods int32
		failedServerPods    int32

		activeSchedulerServices int32
		activeWorkerServices    int32
		activeServerServices    int32

		expectedDeleteFinished bool
	}

	ttlaf0 := int32(0)
	ttl0 := &ttlaf0
	ttlaf2s := int32(2)
	ttl2s := &ttlaf2s
	testCases := []testCase{
		testCase{
			description: "1 scheduler , 4 workers and 2 server is running, TTLSecondsAfterFinished unset",
			mxJob:       testutil.NewMXJobWithCleanupJobDelay(1, 4, 2, nil),

			pendingSchedulerPods:   0,
			activeSchedulerPods:    1,
			succeededSchedulerPods: 0,
			failedSchedulerPods:    0,

			pendingWorkerPods:   0,
			activeWorkerPods:    4,
			succeededWorkerPods: 0,
			failedWorkerPods:    0,

			pendingServerPods:   0,
			activeServerPods:    2,
			succeededServerPods: 0,
			failedServerPods:    0,

			activeSchedulerServices: 1,
			activeWorkerServices:    4,
			activeServerServices:    2,

			expectedDeleteFinished: false,
		},
		testCase{
			description: "1 scheduler, 4 workers and 2 servers is running, TTLSecondsAfterFinished is 0",
			mxJob:       testutil.NewMXJobWithCleanupJobDelay(1, 4, 2, ttl0),

			pendingSchedulerPods:   0,
			activeSchedulerPods:    1,
			succeededSchedulerPods: 0,
			failedSchedulerPods:    0,

			pendingWorkerPods:   0,
			activeWorkerPods:    4,
			succeededWorkerPods: 0,
			failedWorkerPods:    0,

			pendingServerPods:   0,
			activeServerPods:    2,
			succeededServerPods: 0,
			failedServerPods:    0,

			activeSchedulerServices: 1,
			activeWorkerServices:    4,
			activeServerServices:    2,

			expectedDeleteFinished: true,
		},
		testCase{
			description: "1 scheduler, 4 workers and 2 servers is succeeded, TTLSecondsAfterFinished is 2",
			mxJob:       testutil.NewMXJobWithCleanupJobDelay(1, 4, 2, ttl2s),

			pendingSchedulerPods:   0,
			activeSchedulerPods:    0,
			succeededSchedulerPods: 1,
			failedSchedulerPods:    0,

			pendingWorkerPods:   0,
			activeWorkerPods:    0,
			succeededWorkerPods: 4,
			failedWorkerPods:    0,

			pendingServerPods:   0,
			activeServerPods:    0,
			succeededServerPods: 2,
			failedServerPods:    0,

			activeSchedulerServices: 1,
			activeWorkerServices:    4,
			activeServerServices:    2,

			expectedDeleteFinished: true,
		},
	}
	for _, tc := range testCases {
		// Prepare the clientset and controller for the test.
		kubeClientSet := kubeclientset.NewForConfigOrDie(&rest.Config{
			Host: "",
			ContentConfig: rest.ContentConfig{
				GroupVersion: &v1.SchemeGroupVersion,
			},
		},
		)
		config := &rest.Config{
			Host: "",
			ContentConfig: rest.ContentConfig{
				GroupVersion: &mxv1beta1.SchemeGroupVersion,
			},
		}
		mxJobClientSet := mxjobclientset.NewForConfigOrDie(config)
		ctr, kubeInformerFactory, _ := newMXController(config, kubeClientSet, mxJobClientSet, controller.NoResyncPeriodFunc, options.ServerOption{})
		fakePodControl := &controller.FakePodControl{}
		ctr.PodControl = fakePodControl
		fakeServiceControl := &control.FakeServiceControl{}
		ctr.ServiceControl = fakeServiceControl
		ctr.Recorder = &record.FakeRecorder{}
		ctr.mxJobInformerSynced = testutil.AlwaysReady
		ctr.PodInformerSynced = testutil.AlwaysReady
		ctr.ServiceInformerSynced = testutil.AlwaysReady
		mxJobIndexer := ctr.mxJobInformer.GetIndexer()
		ctr.updateStatusHandler = func(mxJob *mxv1beta1.MXJob) error {
			return nil
		}
		deleteFinished := false
		ctr.deleteMXJobHandler = func(mxJob *mxv1beta1.MXJob) error {
			deleteFinished = true
			return nil
		}

		// Set succeeded to run the logic about deleting.
		testutil.SetMXJobCompletionTime(tc.mxJob)

		err := updateMXJobConditions(tc.mxJob, mxv1beta1.MXJobSucceeded, mxJobSucceededReason, "")
		if err != nil {
			t.Errorf("Append mxjob condition error: %v", err)
		}

		unstructured, err := testutil.ConvertMXJobToUnstructured(tc.mxJob)
		if err != nil {
			t.Errorf("Failed to convert the MXJob to Unstructured: %v", err)
		}

		if err := mxJobIndexer.Add(unstructured); err != nil {
			t.Errorf("Failed to add mxjob to mxJobIndexer: %v", err)
		}

		podIndexer := kubeInformerFactory.Core().V1().Pods().Informer().GetIndexer()
		testutil.SetPodsStatuses(podIndexer, tc.mxJob, testutil.LabelScheduler, tc.pendingSchedulerPods, tc.activeSchedulerPods, tc.succeededSchedulerPods, tc.failedSchedulerPods, t)
		testutil.SetPodsStatuses(podIndexer, tc.mxJob, testutil.LabelWorker, tc.pendingWorkerPods, tc.activeWorkerPods, tc.succeededWorkerPods, tc.failedWorkerPods, t)
		testutil.SetPodsStatuses(podIndexer, tc.mxJob, testutil.LabelServer, tc.pendingServerPods, tc.activeServerPods, tc.succeededServerPods, tc.failedServerPods, t)

		serviceIndexer := kubeInformerFactory.Core().V1().Services().Informer().GetIndexer()
		testutil.SetServices(serviceIndexer, tc.mxJob, testutil.LabelScheduler, tc.activeSchedulerServices, t)
		testutil.SetServices(serviceIndexer, tc.mxJob, testutil.LabelWorker, tc.activeWorkerServices, t)
		testutil.SetServices(serviceIndexer, tc.mxJob, testutil.LabelServer, tc.activeServerServices, t)

		ttl := tc.mxJob.Spec.TTLSecondsAfterFinished
		if ttl != nil {
			dur := time.Second * time.Duration(*ttl)
			time.Sleep(dur)
		}

		forget, err := ctr.syncMXJob(testutil.GetKey(tc.mxJob, t))
		if err != nil {
			t.Errorf("%s: unexpected error when syncing jobs %v", tc.description, err)
		}
		if !forget {
			t.Errorf("%s: unexpected forget value. Expected true, saw %v\n", tc.description, forget)
		}

		if deleteFinished != tc.expectedDeleteFinished {
			t.Errorf("%s: unexpected status. Expected %v, saw %v", tc.description, tc.expectedDeleteFinished, deleteFinished)
		}
	}
}
