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
	"time"

	"github.com/kubeflow/common/pkg/controller.v1/control"
	batchv1beta1 "volcano.sh/volcano/pkg/apis/scheduling/v1beta1"
	volcanoclient "volcano.sh/volcano/pkg/client/clientset/versioned"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/kubeflow/mxnet-operator/cmd/mxnet-operator.v1/app/options"
	mxv1 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1"
	mxjobclientset "github.com/kubeflow/mxnet-operator/pkg/client/clientset/versioned"
	mxjobinformers "github.com/kubeflow/mxnet-operator/pkg/client/informers/externalversions"
	"github.com/kubeflow/mxnet-operator/pkg/common/util/v1/testutil"
)

func newMXController(
	config *rest.Config,
	kubeClientSet kubeclientset.Interface,
	mxJobClientSet mxjobclientset.Interface,
	volcanoClientSet volcanoclient.Interface,
	resyncPeriod controller.ResyncPeriodFunc,
	option options.ServerOption,
) (
	*MXController,
	kubeinformers.SharedInformerFactory, mxjobinformers.SharedInformerFactory,
) {
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClientSet, resyncPeriod())
	mxJobInformerFactory := mxjobinformers.NewSharedInformerFactory(mxJobClientSet, resyncPeriod())

	mxJobInformer := NewUnstructuredMXJobInformer(config, metav1.NamespaceAll)

	ctr := NewMXController(mxJobInformer, kubeClientSet, mxJobClientSet, volcanoClientSet, kubeInformerFactory, mxJobInformerFactory, option)
	ctr.PodControl = &control.FakePodControl{}
	ctr.ServiceControl = &control.FakeServiceControl{}
	return ctr, kubeInformerFactory, mxJobInformerFactory
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
