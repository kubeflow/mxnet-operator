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
package mxcontroller

import (
	"fmt"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	mxv1alpha2 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1alpha2"
	"github.com/kubeflow/tf-operator/pkg/controller.v2/jobcontroller"
	mxlogger "github.com/kubeflow/tf-operator/pkg/logger"
)

// reconcileServices checks and updates services for each given MXReplicaSpec.
// It will requeue the mxjob in case of an error while creating/deleting services.
func (tc *MXController) reconcileServices(
	mxjob *mxv1alpha2.MXJob,
	services []*v1.Service,
	rtype mxv1alpha2.MXReplicaType,
	spec *mxv1alpha2.MXReplicaSpec) error {

	// Convert MXReplicaType to lower string.
	rt := strings.ToLower(string(rtype))

	replicas := int(*spec.Replicas)
	// Get all services for the type rt.
	services, err := filterServicesForMXReplicaType(services, rt)
	if err != nil {
		return err
	}

	serviceSlices := getServiceSlices(services, replicas, mxlogger.LoggerForReplica(mxjob, rt))

	for index, serviceSlice := range serviceSlices {
		if len(serviceSlice) > 1 {
			mxlogger.LoggerForReplica(mxjob, rt).Warningf("We have too many services for %s %d", rt, index)
			// TODO(gaocegege): Kill some services.
		} else if len(serviceSlice) == 0 {
			mxlogger.LoggerForReplica(mxjob, rt).Infof("need to create new service: %s-%d", rt, index)
			err = tc.createNewService(mxjob, rtype, strconv.Itoa(index), spec)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// getServiceSlices returns a slice, which element is the slice of service.
// Assume the return object is serviceSlices, then serviceSlices[i] is an
// array of pointers to services corresponding to Services for replica i.
func getServiceSlices(services []*v1.Service, replicas int, logger *log.Entry) [][]*v1.Service {
	serviceSlices := make([][]*v1.Service, replicas)
	for _, service := range services {
		if _, ok := service.Labels[mxReplicaIndexLabel]; !ok {
			logger.Warning("The service do not have the index label.")
			continue
		}
		index, err := strconv.Atoi(service.Labels[mxReplicaIndexLabel])
		if err != nil {
			logger.Warningf("Error when strconv.Atoi: %v", err)
			continue
		}
		if index < 0 || index >= replicas {
			logger.Warningf("The label index is not expected: %d", index)
		} else {
			serviceSlices[index] = append(serviceSlices[index], service)
		}
	}
	return serviceSlices
}

// createNewService creates a new service for the given index and type.
func (tc *MXController) createNewService(mxjob *mxv1alpha2.MXJob, rtype mxv1alpha2.MXReplicaType, index string, spec *mxv1alpha2.MXReplicaSpec) error {
	mxjobKey, err := KeyFunc(mxjob)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Couldn't get key for mxjob object %#v: %v", mxjob, err))
		return err
	}

	// Convert MXReplicaType to lower string.
	rt := strings.ToLower(string(rtype))
	expectationServicesKey := jobcontroller.GenExpectationServicesKey(mxjobKey, rt)
	err = tc.Expectations.ExpectCreations(expectationServicesKey, 1)
	if err != nil {
		return err
	}

	// Create OwnerReference.
	controllerRef := tc.GenOwnerReference(mxjob)

	// Append mxReplicaTypeLabel and mxReplicaIndexLabel labels.
	labels := tc.GenLabels(mxjob.Name)
	labels[mxReplicaTypeLabel] = rt
	labels[mxReplicaIndexLabel] = index

	port, err := GetPortFromMXJob(mxjob, rtype)
	if err != nil {
		return err
	}

	service := &v1.Service{
		Spec: v1.ServiceSpec{
			ClusterIP: "None",
			Selector:  labels,
			Ports: []v1.ServicePort{
				{
					Name: mxv1alpha2.DefaultPortName,
					Port: port,
				},
			},
		},
	}

	service.Name = jobcontroller.GenGeneralName(mxjob.Name, rt, index)
	service.Labels = labels

	err = tc.ServiceControl.CreateServicesWithControllerRef(mxjob.Namespace, service, mxjob, controllerRef)
	if err != nil && errors.IsTimeout(err) {
		// Service is created but its initialization has timed out.
		// If the initialization is successful eventually, the
		// controller will observe the creation via the informer.
		// If the initialization fails, or if the service keeps
		// uninitialized for a long time, the informer will not
		// receive any update, and the controller will create a new
		// service when the expectation expires.
		return nil
	} else if err != nil {
		return err
	}
	return nil
}

// filterServicesForMXReplicaType returns service belong to a MXReplicaType.
func filterServicesForMXReplicaType(services []*v1.Service, mxReplicaType string) ([]*v1.Service, error) {
	var result []*v1.Service

	mxReplicaSelector := &metav1.LabelSelector{
		MatchLabels: make(map[string]string),
	}

	mxReplicaSelector.MatchLabels[mxReplicaTypeLabel] = mxReplicaType

	for _, service := range services {
		selector, err := metav1.LabelSelectorAsSelector(mxReplicaSelector)
		if err != nil {
			return nil, err
		}
		if !selector.Matches(labels.Set(service.Labels)) {
			continue
		}
		result = append(result, service)
	}
	return result, nil
}
