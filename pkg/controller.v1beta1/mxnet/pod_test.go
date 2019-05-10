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
	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/kubeflow/mxnet-operator/cmd/mxnet-operator.v1beta1/app/options"
	mxv1beta1 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1beta1"
	mxjobclientset "github.com/kubeflow/mxnet-operator/pkg/client/clientset/versioned"
	"github.com/kubeflow/mxnet-operator/pkg/common/util/v1beta1/testutil"
	batchv1alpha1 "github.com/kubernetes-sigs/kube-batch/pkg/apis/scheduling/v1alpha1"
	kubebatchclient "github.com/kubernetes-sigs/kube-batch/pkg/client/clientset/versioned"
)

func TestAddPod(t *testing.T) {
	// Prepare the clientset and controller for the test.
	kubeClientSet := kubeclientset.NewForConfigOrDie(&rest.Config{
		Host: "",
		ContentConfig: rest.ContentConfig{
			GroupVersion: &v1.SchemeGroupVersion,
		},
	},
	)
	// Prepare the kube-batch clientset and controller for the test.
	kubeBatchClientSet := kubebatchclient.NewForConfigOrDie(&rest.Config{
		Host: "",
		ContentConfig: rest.ContentConfig{
			GroupVersion: &batchv1alpha1.SchemeGroupVersion,
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
	ctr, _, _ := newMXController(config, kubeClientSet, mxJobClientSet, kubeBatchClientSet, controller.NoResyncPeriodFunc, options.ServerOption{})
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

	mxJob := testutil.NewMXJob(1, 0)
	unstructured, err := testutil.ConvertMXJobToUnstructured(mxJob)
	if err != nil {
		t.Errorf("Failed to convert the MXJob to Unstructured: %v", err)
	}

	if err := mxJobIndexer.Add(unstructured); err != nil {
		t.Errorf("Failed to add mxjob to mxJobIndexer: %v", err)
	}
	pod := testutil.NewPod(mxJob, testutil.LabelWorker, 0, t)
	ctr.AddPod(pod)

	syncChan <- "sync"
	if key != testutil.GetKey(mxJob, t) {
		t.Errorf("Failed to enqueue the MXJob %s: expected %s, got %s", mxJob.Name, testutil.GetKey(mxJob, t), key)
	}
	close(stopCh)
}

func TestRestartPolicy(t *testing.T) {
	type tc struct {
		mxJob                 *mxv1beta1.MXJob
		expectedRestartPolicy v1.RestartPolicy
		expectedType          mxv1beta1.MXReplicaType
	}
	testCase := []tc{
		func() tc {
			mxJob := testutil.NewMXJob(1, 0)
			specRestartPolicy := mxv1beta1.RestartPolicyExitCode
			mxJob.Spec.MXReplicaSpecs[mxv1beta1.MXReplicaTypeWorker].RestartPolicy = specRestartPolicy
			return tc{
				mxJob:                 mxJob,
				expectedRestartPolicy: v1.RestartPolicyNever,
				expectedType:          mxv1beta1.MXReplicaTypeWorker,
			}
		}(),
		func() tc {
			mxJob := testutil.NewMXJob(1, 0)
			specRestartPolicy := mxv1beta1.RestartPolicyNever
			mxJob.Spec.MXReplicaSpecs[mxv1beta1.MXReplicaTypeWorker].RestartPolicy = specRestartPolicy
			return tc{
				mxJob:                 mxJob,
				expectedRestartPolicy: v1.RestartPolicyNever,
				expectedType:          mxv1beta1.MXReplicaTypeWorker,
			}
		}(),
		func() tc {
			mxJob := testutil.NewMXJob(1, 0)
			specRestartPolicy := mxv1beta1.RestartPolicyAlways
			mxJob.Spec.MXReplicaSpecs[mxv1beta1.MXReplicaTypeWorker].RestartPolicy = specRestartPolicy
			return tc{
				mxJob:                 mxJob,
				expectedRestartPolicy: v1.RestartPolicyAlways,
				expectedType:          mxv1beta1.MXReplicaTypeWorker,
			}
		}(),
		func() tc {
			mxJob := testutil.NewMXJob(1, 0)
			specRestartPolicy := mxv1beta1.RestartPolicyOnFailure
			mxJob.Spec.MXReplicaSpecs[mxv1beta1.MXReplicaTypeWorker].RestartPolicy = specRestartPolicy
			return tc{
				mxJob:                 mxJob,
				expectedRestartPolicy: v1.RestartPolicyOnFailure,
				expectedType:          mxv1beta1.MXReplicaTypeWorker,
			}
		}(),
	}
	for _, c := range testCase {
		spec := c.mxJob.Spec.MXReplicaSpecs[c.expectedType]
		podTemplate := spec.Template
		setRestartPolicy(&podTemplate, spec)
		if podTemplate.Spec.RestartPolicy != c.expectedRestartPolicy {
			t.Errorf("Expected %s, got %s", c.expectedRestartPolicy, podTemplate.Spec.RestartPolicy)
		}
	}
}

func TestExitCode(t *testing.T) {
	// Prepare the clientset and controller for the test.
	kubeClientSet := kubeclientset.NewForConfigOrDie(&rest.Config{
		Host: "",
		ContentConfig: rest.ContentConfig{
			GroupVersion: &v1.SchemeGroupVersion,
		},
	},
	)
	// Prepare the kube-batch clientset and controller for the test.
	kubeBatchClientSet := kubebatchclient.NewForConfigOrDie(&rest.Config{
		Host: "",
		ContentConfig: rest.ContentConfig{
			GroupVersion: &batchv1alpha1.SchemeGroupVersion,
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
	ctr, kubeInformerFactory, _ := newMXController(config, kubeClientSet, mxJobClientSet, kubeBatchClientSet, controller.NoResyncPeriodFunc, options.ServerOption{})
	fakePodControl := &controller.FakePodControl{}
	ctr.PodControl = fakePodControl
	ctr.mxJobInformerSynced = testutil.AlwaysReady
	ctr.PodInformerSynced = testutil.AlwaysReady
	ctr.ServiceInformerSynced = testutil.AlwaysReady
	mxJobIndexer := ctr.mxJobInformer.GetIndexer()
	podIndexer := kubeInformerFactory.Core().V1().Pods().Informer().GetIndexer()

	stopCh := make(chan struct{})
	run := func(<-chan struct{}) {
		ctr.Run(testutil.ThreadCount, stopCh)
	}
	go run(stopCh)

	ctr.updateStatusHandler = func(mxJob *mxv1beta1.MXJob) error {
		return nil
	}

	mxJob := testutil.NewMXJob(1, 0)
	mxJob.Spec.MXReplicaSpecs[mxv1beta1.MXReplicaTypeWorker].RestartPolicy = mxv1beta1.RestartPolicyExitCode
	unstructured, err := testutil.ConvertMXJobToUnstructured(mxJob)
	if err != nil {
		t.Errorf("Failed to convert the MXJob to Unstructured: %v", err)
	}

	if err := mxJobIndexer.Add(unstructured); err != nil {
		t.Errorf("Failed to add mxjob to mxJobIndexer: %v", err)
	}
	pod := testutil.NewPod(mxJob, testutil.LabelWorker, 0, t)
	pod.Status.Phase = v1.PodFailed
	pod.Spec.Containers = append(pod.Spec.Containers, v1.Container{})
	pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, v1.ContainerStatus{
		Name: mxv1beta1.DefaultContainerName,
		State: v1.ContainerState{
			Terminated: &v1.ContainerStateTerminated{
				ExitCode: 130,
			},
		},
	})

	if err := podIndexer.Add(pod); err != nil {
		t.Errorf("%s: unexpected error when adding pod %v", mxJob.Name, err)
	}
	_, err = ctr.syncMXJob(testutil.GetKey(mxJob, t))
	if err != nil {
		t.Errorf("%s: unexpected error when syncing jobs %v", mxJob.Name, err)
	}

	found := false
	for _, deletedPodName := range fakePodControl.DeletePodName {
		if deletedPodName == pod.Name {
			found = true
		}
	}
	if !found {
		t.Errorf("Failed to delete pod %s", pod.Name)
	}
	close(stopCh)
}
