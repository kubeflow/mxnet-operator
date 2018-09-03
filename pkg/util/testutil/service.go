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

package testutil

import (
	"fmt"
	"testing"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	mxv1alpha2 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1alpha2"
)

func NewBaseService(name string, mxJob *mxv1alpha2.MXJob, t *testing.T) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Labels:          GenLabels(mxJob.Name),
			Namespace:       mxJob.Namespace,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(mxJob, controllerKind)},
		},
	}
}

func NewService(mxJob *mxv1alpha2.MXJob, typ string, index int, t *testing.T) *v1.Service {
	service := NewBaseService(fmt.Sprintf("%s-%d", typ, index), mxJob, t)
	service.Labels[mxReplicaTypeLabel] = typ
	service.Labels[mxReplicaIndexLabel] = fmt.Sprintf("%d", index)
	return service
}

// NewServiceList creates count pods with the given phase for the given mxJob
func NewServiceList(count int32, mxJob *mxv1alpha2.MXJob, typ string, t *testing.T) []*v1.Service {
	services := []*v1.Service{}
	for i := int32(0); i < count; i++ {
		newService := NewService(mxJob, typ, int(i), t)
		services = append(services, newService)
	}
	return services
}

func SetServices(serviceIndexer cache.Indexer, mxJob *mxv1alpha2.MXJob, typ string, activeWorkerServices int32, t *testing.T) {
	for _, service := range NewServiceList(activeWorkerServices, mxJob, typ, t) {
		if err := serviceIndexer.Add(service); err != nil {
			t.Errorf("unexpected error when adding service %v", err)
		}
	}
}
