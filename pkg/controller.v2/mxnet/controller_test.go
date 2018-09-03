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
	"reflect"
	"testing"
	"time"

	"k8s.io/api/core/v1"
	kubeinformers "k8s.io/client-go/informers"
	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/golang/protobuf/proto"
	"github.com/kubeflow/mxnet-operator/cmd/mxnet-operator.v2/app/options"
	mxv1alpha2 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1alpha2"
	mxjobclientset "github.com/kubeflow/mxnet-operator/pkg/client/clientset/versioned"
	mxjobinformers "github.com/kubeflow/mxnet-operator/pkg/client/informers/externalversions"
	"github.com/kubeflow/mxnet-operator/pkg/control"
	"github.com/kubeflow/mxnet-operator/pkg/util/testutil"
	"k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

var (
	mxJobRunning   = mxv1alpha2.MXJobRunning
	mxJobSucceeded = mxv1alpha2.MXJobSucceeded
)

func newMXController(
	config *rest.Config,
	kubeClientSet kubeclientset.Interface,
	mxJobClientSet mxjobclientset.Interface,
	resyncPeriod controller.ResyncPeriodFunc,
	option options.ServerOption,
) (
	*MXController,
	kubeinformers.SharedInformerFactory, mxjobinformers.SharedInformerFactory,
) {
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClientSet, resyncPeriod())
	mxJobInformerFactory := mxjobinformers.NewSharedInformerFactory(mxJobClientSet, resyncPeriod())

	mxJobInformer := NewUnstructuredMXJobInformer(config, metav1.NamespaceAll)

	ctr := NewMXController(mxJobInformer, kubeClientSet, mxJobClientSet, kubeInformerFactory, mxJobInformerFactory, option)
	ctr.PodControl = &controller.FakePodControl{}
	ctr.ServiceControl = &control.FakeServiceControl{}
	return ctr, kubeInformerFactory, mxJobInformerFactory
}

func TestNormalPath(t *testing.T) {
	testCases := map[string]struct {
		scheduler int
		server int
                worker int

		// pod setup
		ControllerError error
		jobKeyForget    bool

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

		// expectations
		expectedPodCreations     int32
		expectedPodDeletions     int32
		expectedServiceCreations int32

		expectedActiveSchedulerPods    int32
		expectedSucceededSchedulerPods int32
		expectedFailedSchedulerPods    int32

		expectedActiveWorkerPods    int32
		expectedSucceededWorkerPods int32
		expectedFailedWorkerPods    int32

		expectedActiveServerPods    int32
		expectedSucceededServerPods int32
		expectedFailedServerPods    int32

		expectedCondition       *mxv1alpha2.MXJobConditionType
		expectedConditionReason string

		// There are some cases that should not check start time since the field should be set in the previous sync loop.
		needCheckStartTime bool
	}{
		/*"Local MXJob is created": {
			1, 0,
			nil, true,
			0, 0, 0, 0,
			0, 0, 0, 0,
			0, 0,
			1, 0, 1,
			0, 0, 0,
			0, 0, 0,
			// We can not check if it is created since the condition is set in addTFJob.
			nil, "",
			false,
		},
		"Distributed TFJob (4 workers, 2 PS) is created": {
			4, 2,
			nil, true,
			0, 0, 0, 0,
			0, 0, 0, 0,
			0, 0,
			6, 0, 6,
			0, 0, 0,
			0, 0, 0,
			nil, "",
			false,
		},
		"Distributed TFJob (4 workers, 2 PS) is created and all replicas are pending": {
			4, 2,
			nil, true,
			4, 0, 0, 0,
			2, 0, 0, 0,
			4, 2,
			0, 0, 0,
			0, 0, 0,
			0, 0, 0,
			nil, "",
			false,
		},
		"Distributed TFJob (4 workers, 2 PS) is created and all replicas are running": {
			4, 2,
			nil, true,
			0, 4, 0, 0,
			0, 2, 0, 0,
			4, 2,
			0, 0, 0,
			4, 0, 0,
			2, 0, 0,
			&tfJobRunning, tfJobRunningReason,
			true,
		},
		"Distributed TFJob (4 workers, 2 PS) is created, 2 workers, 1 PS are pending": {
			4, 2,
			nil, true,
			2, 0, 0, 0,
			1, 0, 0, 0,
			2, 1,
			3, 0, 3,
			0, 0, 0,
			0, 0, 0,
			nil, "",
			false,
		},
		"Distributed TFJob (4 workers, 2 PS) is created, 2 workers, 1 PS are pending, 1 worker is running": {
			4, 2,
			nil, true,
			2, 1, 0, 0,
			1, 0, 0, 0,
			3, 1,
			2, 0, 2,
			1, 0, 0,
			0, 0, 0,
			&tfJobRunning, tfJobRunningReason,
			false,
		},
		"Distributed TFJob (4 workers, 2 PS) is created, 2 workers, 1 PS are pending, 1 worker is succeeded": {
			4, 2,
			nil, true,
			2, 0, 1, 0,
			1, 0, 0, 0,
			3, 1,
			2, 0, 2,
			0, 1, 0,
			0, 0, 0,
			nil, "",
			false,
		},
                */
		"Distributed MXJob (4 workers, 2 Server, 1 Scheduler) is succeeded": {
			1, 4, 2,
			nil, true,
                        0, 1, 0, 0,
			0, 0, 4, 0,
			0, 0, 2, 0,
			1, 4, 2,
			0, 0, 0,
                        1, 0, 0,
			0, 4, 0,
			2, 0, 0,
			&mxJobSucceeded, mxJobSucceededReason,
			false,
		},
	}

	for name, tc := range testCases {
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
				GroupVersion: &mxv1alpha2.SchemeGroupVersion,
			},
		}
		option := options.ServerOption{}
		mxJobClientSet := mxjobclientset.NewForConfigOrDie(config)
		ctr, kubeInformerFactory, _ := newMXController(config, kubeClientSet, mxJobClientSet, controller.NoResyncPeriodFunc, option)
		ctr.mxJobInformerSynced = testutil.AlwaysReady
		ctr.PodInformerSynced = testutil.AlwaysReady
		ctr.ServiceInformerSynced = testutil.AlwaysReady
		mxJobIndexer := ctr.mxJobInformer.GetIndexer()

		var actual *mxv1alpha2.MXJob
		ctr.updateStatusHandler = func(mxJob *mxv1alpha2.MXJob) error {
			actual = mxJob
			return nil
		}

		// Run the test logic.
		mxJob := testutil.NewMXJob(tc.worker, tc.ps)
		unstructured, err := testutil.ConvertMXJobToUnstructured(mxJob)
		if err != nil {
			t.Errorf("Failed to convert the MXJob to Unstructured: %v", err)
		}

		if err := mxJobIndexer.Add(unstructured); err != nil {
			t.Errorf("Failed to add mxjob to mxJobIndexer: %v", err)
		}

		podIndexer := kubeInformerFactory.Core().V1().Pods().Informer().GetIndexer()
                testutil.SetPodsStatuses(podIndexer, mxJob, testutil.LabelScheduler, tc.pendingSchedulerPods, tc.activeSchedulerPods, tc.succeededSchedulerPods, tc.failedSchedulerPods, t)
		testutil.SetPodsStatuses(podIndexer, mxJob, testutil.LabelWorker, tc.pendingWorkerPods, tc.activeWorkerPods, tc.succeededWorkerPods, tc.failedWorkerPods, t)
		testutil.SetPodsStatuses(podIndexer, mxJob, testutil.LabelServer, tc.pendingServerPods, tc.activeServerPods, tc.succeededServerPods, tc.failedServerPods, t)

		serviceIndexer := kubeInformerFactory.Core().V1().Services().Informer().GetIndexer()
		testutil.SetServices(serviceIndexer, mxJob, testutil.LabelScheduler, tc.activeSchedulerServices, t)
		testutil.SetServices(serviceIndexer, mxJob, testutil.LabelWorker, tc.activeWorkerServices, t)
		testutil.SetServices(serviceIndexer, mxJob, testutil.LabelServer, tc.activeServerServices, t)

		forget, err := ctr.syncMXJob(testutil.GetKey(mxJob, t))
		// We need requeue syncJob task if podController error
		if tc.ControllerError != nil {
			if err == nil {
				t.Errorf("%s: Syncing jobs would return error when podController exception", name)
			}
		} else {
			if err != nil {
				t.Errorf("%s: unexpected error when syncing jobs %v", name, err)
			}
		}
		if forget != tc.jobKeyForget {
			t.Errorf("%s: unexpected forget value. Expected %v, saw %v\n", name, tc.jobKeyForget, forget)
		}

		fakePodControl := ctr.PodControl.(*controller.FakePodControl)
		fakeServiceControl := ctr.ServiceControl.(*control.FakeServiceControl)
		if int32(len(fakePodControl.Templates)) != tc.expectedPodCreations {
			t.Errorf("%s: unexpected number of pod creates.  Expected %d, saw %d\n", name, tc.expectedPodCreations, len(fakePodControl.Templates))
		}
		if int32(len(fakeServiceControl.Templates)) != tc.expectedServiceCreations {
			t.Errorf("%s: unexpected number of service creates.  Expected %d, saw %d\n", name, tc.expectedServiceCreations, len(fakeServiceControl.Templates))
		}
		if int32(len(fakePodControl.DeletePodName)) != tc.expectedPodDeletions {
			t.Errorf("%s: unexpected number of pod deletes.  Expected %d, saw %d\n", name, tc.expectedPodDeletions, len(fakePodControl.DeletePodName))
		}
		// Each create should have an accompanying ControllerRef.
		if len(fakePodControl.ControllerRefs) != int(tc.expectedPodCreations) {
			t.Errorf("%s: unexpected number of ControllerRefs.  Expected %d, saw %d\n", name, tc.expectedPodCreations, len(fakePodControl.ControllerRefs))
		}
		// Make sure the ControllerRefs are correct.
		for _, controllerRef := range fakePodControl.ControllerRefs {
			if got, want := controllerRef.APIVersion, mxv1alpha2.SchemeGroupVersion.String(); got != want {
				t.Errorf("controllerRef.APIVersion = %q, want %q", got, want)
			}
			if got, want := controllerRef.Kind, mxv1alpha2.Kind; got != want {
				t.Errorf("controllerRef.Kind = %q, want %q", got, want)
			}
			if got, want := controllerRef.Name, mxJob.Name; got != want {
				t.Errorf("controllerRef.Name = %q, want %q", got, want)
			}
			if got, want := controllerRef.UID, mxJob.UID; got != want {
				t.Errorf("controllerRef.UID = %q, want %q", got, want)
			}
			if controllerRef.Controller == nil || !*controllerRef.Controller {
				t.Errorf("controllerRef.Controller is not set to true")
			}
		}
                // Validate scheduler status.
		if actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeScheduler] != nil {
			if actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeScheduler].Active != tc.expectedActiveSchedulerPods {
				t.Errorf("%s: unexpected number of active pods.  Expected %d, saw %d\n", name, tc.expectedActiveSchedulerPods, actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeScheduler].Active)
			}
			if actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeScheduler].Succeeded != tc.expectedSucceededSchedulerPods {
				t.Errorf("%s: unexpected number of succeeded pods.  Expected %d, saw %d\n", name, tc.expectedSucceededSchedulerPods, actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeScheduler].Succeeded)
			}
			if actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeScheduler].Failed != tc.expectedFailedSchedulerPods {
				t.Errorf("%s: unexpected number of failed pods.  Expected %d, saw %d\n", name, tc.expectedFailedSchedulerPods, actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeScheduler].Failed)
			}
		}
		// Validate worker status.
		if actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeWorker] != nil {
			if actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeWorker].Active != tc.expectedActiveWorkerPods {
				t.Errorf("%s: unexpected number of active pods.  Expected %d, saw %d\n", name, tc.expectedActiveWorkerPods, actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeWorker].Active)
			}
			if actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeWorker].Succeeded != tc.expectedSucceededWorkerPods {
				t.Errorf("%s: unexpected number of succeeded pods.  Expected %d, saw %d\n", name, tc.expectedSucceededWorkerPods, actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeWorker].Succeeded)
			}
			if actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeWorker].Failed != tc.expectedFailedWorkerPods {
				t.Errorf("%s: unexpected number of failed pods.  Expected %d, saw %d\n", name, tc.expectedFailedWorkerPods, actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeWorker].Failed)
			}
		}
		// Validate Server status.
		if actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeServer] != nil {
			if actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeServer].Active != tc.expectedActiveServerPods {
				t.Errorf("%s: unexpected number of active pods.  Expected %d, saw %d\n", name, tc.expectedActiveServerPods, actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeServer].Active)
			}
			if actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeServer].Succeeded != tc.expectedSucceededServerPods {
				t.Errorf("%s: unexpected number of succeeded pods.  Expected %d, saw %d\n", name, tc.expectedSucceededServerPods, actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeServer].Succeeded)
			}
			if actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeServer].Failed != tc.expectedFailedServerPods {
				t.Errorf("%s: unexpected number of failed pods.  Expected %d, saw %d\n", name, tc.expectedFailedServerPods, actual.Status.MXReplicaStatuses[mxv1alpha2.MXReplicaTypeServer].Failed)
			}
		}
		// Validate StartTime.
		if tc.needCheckStartTime && actual.Status.StartTime == nil {
			t.Errorf("%s: StartTime was not set", name)
		}
		// Validate conditions.
		if tc.expectedCondition != nil && !testutil.CheckCondition(actual, *tc.expectedCondition, tc.expectedConditionReason) {
			t.Errorf("%s: expected condition %#v, got %#v", name, *tc.expectedCondition, actual.Status.Conditions)
		}
	}
}

func TestRun(t *testing.T) {
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
			GroupVersion: &mxv1alpha2.SchemeGroupVersion,
		},
	}
	mxJobClientSet := mxjobclientset.NewForConfigOrDie(config)
	ctr, _, _ := newMXController(config, kubeClientSet, mxJobClientSet, controller.NoResyncPeriodFunc, options.ServerOption{})
	ctr.mxJobInformerSynced = testutil.AlwaysReady
	ctr.PodInformerSynced = testutil.AlwaysReady
	ctr.ServiceInformerSynced = testutil.AlwaysReady

	stopCh := make(chan struct{})
	go func() {
		// It is a hack to let the controller stop to run without errors.
		// We can not just send a struct to stopCh because there are multiple
		// receivers in controller.Run.
		time.Sleep(testutil.SleepInterval)
		stopCh <- struct{}{}
	}()
	err := ctr.Run(testutil.ThreadCount, stopCh)
	if err != nil {
		t.Errorf("Failed to run: %v", err)
	}
}

func TestSyncPdb(t *testing.T) {
	config := &rest.Config{
		Host: "",
		ContentConfig: rest.ContentConfig{
			GroupVersion: &mxv1alpha2.SchemeGroupVersion,
		},
	}
	mxJobClientSet := mxjobclientset.NewForConfigOrDie(config)
	kubeClientSet := fake.NewSimpleClientset()
	option := options.ServerOption{
		EnableGangScheduling: true,
	}
	ctr, _, _ := newMXController(config, kubeClientSet, mxJobClientSet, controller.NoResyncPeriodFunc, option)

	type testCase struct {
		mxJob     *mxv1alpha2.MXJob
		expectPdb *v1beta1.PodDisruptionBudget
	}

	minAvailable2 := intstr.FromInt(2)
	testCases := []testCase{
		{
			mxJob: &mxv1alpha2.MXJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sync-pdb",
				},
				Spec: mxv1alpha2.MXJobSpec{
					MXReplicaSpecs: map[mxv1alpha2.MXReplicaType]*mxv1alpha2.MXReplicaSpec{
						mxv1alpha2.MxReplicaTypeWorker: &mxv1alpha2.MXReplicaSpec{
							Replicas: proto.Int32(1),
						},
					},
				},
			},
			expectPdb: nil,
		},
		{
			mxJob: &mxv1alpha2.MXJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sync-pdb",
				},
				Spec: mxv1alpha2.MXJobSpec{
					MXReplicaSpecs: map[mxv1alpha2.MXReplicaType]*mxv1alpha2.MXReplicaSpec{
						mxv1alpha2.MXReplicaTypeWorker: &mxv1alpha2.MXReplicaSpec{
							Replicas: proto.Int32(2),
						},
					},
				},
			},
			expectPdb: &v1beta1.PodDisruptionBudget{
				Spec: v1beta1.PodDisruptionBudgetSpec{
					MinAvailable: &minAvailable2,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"mx_job_name": "test-sync-pdb",
						},
					},
				},
			},
		},
	}
	for _, c := range testCases {
		pdb, _ := ctr.SyncPdb(c.mxJob, getTotalReplicas(c.mxJob))
		if pdb == nil && c.expectPdb != nil {
			t.Errorf("Got nil, want %v", c.expectPdb.Spec)
		}

		if pdb != nil && !reflect.DeepEqual(c.expectPdb.Spec, pdb.Spec) {
			t.Errorf("Got %+v, want %+v", pdb.Spec, c.expectPdb.Spec)
		}
	}
}
