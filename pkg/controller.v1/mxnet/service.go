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

	mxv1 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (tc *MXController) GetServicesForJob(job interface{}) ([]*corev1.Service, error) {
	mxJob, ok := job.(*mxv1.MXJob)
	if !ok {
		return nil, fmt.Errorf("%v is not a type of MXJob", mxJob)
	}

	//log := mxlogger.LoggerForJob(mxJob)
	// List all services to include those that don't match the selector anymore
	// but have a ControllerRef pointing to this controller.

	labelSelector := metav1.LabelSelector{MatchLabels: tc.JobController.GenLabels(mxJob.Name)}
	opts := metav1.ListOptions{
		LabelSelector: labels.Set(labelSelector.MatchLabels).String(),
	}
	svclist, err := tc.KubeClientSet.CoreV1().Services(mxJob.Namespace).List(opts)
	if err != nil {
		return nil, err
	}
	return convertServiceList(svclist.Items), nil
}

// convertServiceList convert service list to service point list
func convertServiceList(list []corev1.Service) []*corev1.Service {
	if list == nil {
		return nil
	}
	ret := make([]*corev1.Service, 0, len(list))
	for i := range list {
		ret = append(ret, &list[i])
	}
	return ret
}
