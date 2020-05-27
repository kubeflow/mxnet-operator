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
	"context"
	"testing"

	batchv1beta1 "volcano.sh/volcano/pkg/apis/scheduling/v1beta1"
	volcanoclient "volcano.sh/volcano/pkg/client/clientset/versioned"

	v1 "k8s.io/api/core/v1"
	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/kubeflow/mxnet-operator/cmd/mxnet-operator.v1/app/options"
	mxv1 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1"
	mxjobclientset "github.com/kubeflow/mxnet-operator/pkg/client/clientset/versioned"
	"github.com/kubeflow/mxnet-operator/pkg/common/util/v1/testutil"
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
	// Prepare the volcano clientset and controller for the test.
	volcanoClientSet := volcanoclient.NewForConfigOrDie(&rest.Config{
		Host: "",
		ContentConfig: rest.ContentConfig{
			GroupVersion: &batchv1beta1.SchemeGroupVersion,
		},
	},
	)
	config := &rest.Config{
		Host: "",
		ContentConfig: rest.ContentConfig{
			GroupVersion: &mxv1.SchemeGroupVersion,
		},
	}
	mxJobClientSet := mxjobclientset.NewForConfigOrDie(config)
	ctr, _, _ := newMXController(config, kubeClientSet, mxJobClientSet, volcanoClientSet, controller.NoResyncPeriodFunc, options.ServerOption{})
	ctr.mxJobInformerSynced = testutil.AlwaysReady
	ctr.PodInformerSynced = testutil.AlwaysReady
	ctr.ServiceInformerSynced = testutil.AlwaysReady
	mxJobIndexer := ctr.mxJobInformer.GetIndexer()

	stopCh := make(chan struct{})
	run := func(ctx context.Context) {
		if err := ctr.Run(testutil.ThreadCount, stopCh); err != nil {
			t.Errorf("Failed to run MXNet Controller!")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	go run(ctx)

	var key string
	syncChan := make(chan string)
	ctr.syncHandler = func(mxJobKey string) (bool, error) {
		key = mxJobKey
		<-syncChan
		return true, nil
	}
	ctr.updateStatusHandler = func(mxjob *mxv1.MXJob) error {
		return nil
	}
	ctr.deleteMXJobHandler = func(mxjob *mxv1.MXJob) error {
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
	cancel()
}
