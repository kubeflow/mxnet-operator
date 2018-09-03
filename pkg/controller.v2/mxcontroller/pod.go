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
	train_util "github.com/kubeflow/tf-operator/pkg/util/train"
)

const (
	// mxConfig is the environment variable name of MXNet cluster spec.
	mxConfig = "MX_CONFIG"

	// podTemplateRestartPolicyReason is the warning reason when the restart
	// policy is setted in pod template.
	podTemplateRestartPolicyReason = "SettedPodTemplateRestartPolicy"
)

// reconcilePods checks and updates pods for each given MXReplicaSpec.
// It will requeue the mxjob in case of an error while creating/deleting pods.
func (tc *MXController) reconcilePods(
	mxjob *mxv1alpha2.MXJob,
	pods []*v1.Pod,
	rtype mxv1alpha2.MXReplicaType,
	spec *mxv1alpha2.MXReplicaSpec, rstatus map[string]v1.PodPhase) error {

	// Convert MXReplicaType to lower string.
	rt := strings.ToLower(string(rtype))
	logger := mxlogger.LoggerForReplica(mxjob, rt)
	// Get all pods for the type rt.
	pods, err := filterPodsForMXReplicaType(pods, rt)
	if err != nil {
		return err
	}
	replicas := int(*spec.Replicas)
	restart := false

	initializeMXReplicaStatuses(mxjob, rtype)

	podSlices := getPodSlices(pods, replicas, logger)
	for index, podSlice := range podSlices {
		if len(podSlice) > 1 {
			logger.Warningf("We have too many pods for %s %d", rt, index)
			// TODO(gaocegege): Kill some pods.
		} else if len(podSlice) == 0 {
			logger.Infof("Need to create new pod: %s-%d", rt, index)
			err = tc.createNewPod(mxjob, rt, strconv.Itoa(index), spec)
			if err != nil {
				return err
			}
		} else {
			// Check the status of the current pod.
			pod := podSlice[0]
			// Check if the pod is retryable.
			if spec.RestartPolicy == mxv1alpha2.RestartPolicyExitCode {
				var exitCode int32
				for _, status := range pod.Status.ContainerStatuses {
					state := status.State
					// Get the exit code of the mxnet container.
					if status.Name == mxv1alpha2.DefaultContainerName && state.Terminated != nil {
						exitCode = state.Terminated.ExitCode
					}
				}
				if pod.Status.Phase == v1.PodFailed && train_util.IsRetryableExitCode(exitCode) {
					logger.Infof("Need to restart the pod: %s-%d", rt, index)
					if err := tc.PodControl.DeletePod(pod.Namespace, pod.Name, mxjob); err != nil {
						return err
					}
					restart = true
				}
			}
			updateMXJobReplicaStatuses(mxjob, rtype, pod)
		}
	}

	return updateStatusSingle(mxjob, rtype, replicas, restart)
}

// getPodSlices returns a slice, which element is the slice of pod.
func getPodSlices(pods []*v1.Pod, replicas int, logger *log.Entry) [][]*v1.Pod {
	podSlices := make([][]*v1.Pod, replicas)
	for _, pod := range pods {
		if _, ok := pod.Labels[mxReplicaIndexLabel]; !ok {
			logger.Warning("The pod do not have the index label.")
			continue
		}
		index, err := strconv.Atoi(pod.Labels[mxReplicaIndexLabel])
		if err != nil {
			logger.Warningf("Error when strconv.Atoi: %v", err)
			continue
		}
		if index < 0 || index >= replicas {
			logger.Warningf("The label index is not expected: %d", index)
		} else {
			podSlices[index] = append(podSlices[index], pod)
		}
	}
	return podSlices
}

// createNewPod creates a new pod for the given index and type.
func (tc *MXController) createNewPod(mxjob *mxv1alpha2.MXJob, rt, index string, spec *mxv1alpha2.MXReplicaSpec) error {
	mxjobKey, err := KeyFunc(mxjob)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Couldn't get key for mxjob object %#v: %v", mxjob, err))
		return err
	}
	expectationPodsKey := jobcontroller.GenExpectationPodsKey(mxjobKey, rt)
	err = tc.Expectations.ExpectCreations(expectationPodsKey, 1)
	if err != nil {
		return err
	}
	logger := mxlogger.LoggerForReplica(mxjob, rt)
	// Create OwnerReference.
	controllerRef := tc.GenOwnerReference(mxjob)

	// Set type and index for the worker.
	labels := tc.GenLabels(mxjob.Name)
	labels[mxReplicaTypeLabel] = rt
	labels[mxReplicaIndexLabel] = index

	podTemplate := spec.Template.DeepCopy()

	// Set name for the template.
	podTemplate.Name = jobcontroller.GenGeneralName(mxjob.Name, rt, index)

	if podTemplate.Labels == nil {
		podTemplate.Labels = make(map[string]string)
	}

	for key, value := range labels {
		podTemplate.Labels[key] = value
	}

        setSchedulerName(podTemplate, mxjob)
	if err := setClusterSpec(podTemplate, mxjob, rt, index); err != nil {
		return err
	}

	// Submit a warning event if the user specifies restart policy for
	// the pod template. We recommend to set it from the replica level.
	if podTemplate.Spec.RestartPolicy != v1.RestartPolicy("") {
		errMsg := "Restart policy in pod template will be overwritten by restart policy in replica spec"
		logger.Warning(errMsg)
		tc.Recorder.Event(mxjob, v1.EventTypeWarning, podTemplateRestartPolicyReason, errMsg)
	}
	setRestartPolicy(podTemplate, spec)

	err = tc.PodControl.CreatePodsWithControllerRef(mxjob.Namespace, podTemplate, mxjob, controllerRef)
	if err != nil && errors.IsTimeout(err) {
		// Pod is created but its initialization has timed out.
		// If the initialization is successful eventually, the
		// controller will observe the creation via the informer.
		// If the initialization fails, or if the pod keeps
		// uninitialized for a long time, the informer will not
		// receive any update, and the controller will create a new
		// pod when the expectation expires.
		return nil
	} else if err != nil {
		return err
	}
	return nil
}

func setSchedulerName(podTemplateSpec *v1.PodTemplateSpec, mxjob *mxv1alpha2.MXJob) {
	podTemplateSpec.Spec.SchedulerName = mxjob.Spec.SchedulerName
}

func setClusterSpec(podTemplateSpec *v1.PodTemplateSpec, mxjob *mxv1alpha2.MXJob, rt, index string) error {

	// Add MX_CONFIG environment variable.
	for i := range podTemplateSpec.Spec.Containers {

		c := &podTemplateSpec.Spec.Containers[i]

		if len(c.Env) == 0 {
			c.Env = make([]v1.EnvVar, 6)
		}

		for t, r := range mxjob.Spec.MXReplicaSpecs {
		
			port, err := GetPortFromMXJob(mxjob, t)
			if err != nil {
				return err
			}	
                   
                        rt := strings.ToLower(string(t))

			switch t {
			case mxv1alpha2.MXReplicaTypeScheduler:
				c.Env[0].Name = "DMLC_PS_ROOT_PORT"
				c.Env[0].Value = strconv.Itoa(int(port))
				c.Env[1].Name = "DMLC_PS_ROOT_URI"
				c.Env[1].Value = fmt.Sprintf("%s", jobcontroller.GenGeneralName(mxjob.Name, rt, fmt.Sprintf("%d", 0)))	
			case mxv1alpha2.MXReplicaTypeServer:
				c.Env[2].Name = "DMLC_NUM_SERVER"
				c.Env[2].Value = strconv.Itoa(int(*r.Replicas))
			case mxv1alpha2.MXReplicaTypeWorker:
				c.Env[3].Name = "DMLC_NUM_WORKER"
				c.Env[3].Value = strconv.Itoa(int(*r.Replicas))
			}
		}

		c.Env[4].Name = "DMLC_ROLE"
		c.Env[4].Value = strings.ToLower(string(rt))

		c.Env[5].Name = "DMLC_USE_KUBERNETES"
		c.Env[5].Value = strconv.Itoa(1)
	} 
	return nil
}

func setRestartPolicy(podTemplateSpec *v1.PodTemplateSpec, spec *mxv1alpha2.MXReplicaSpec) {
	if spec.RestartPolicy == mxv1alpha2.RestartPolicyExitCode {
		podTemplateSpec.Spec.RestartPolicy = v1.RestartPolicyNever
	} else {
		podTemplateSpec.Spec.RestartPolicy = v1.RestartPolicy(spec.RestartPolicy)
	}
}

// filterPodsForMXReplicaType returns pods belong to a MXReplicaType.
func filterPodsForMXReplicaType(pods []*v1.Pod, mxReplicaType string) ([]*v1.Pod, error) {
	var result []*v1.Pod

	mxReplicaSelector := &metav1.LabelSelector{
		MatchLabels: make(map[string]string),
	}

	mxReplicaSelector.MatchLabels[mxReplicaTypeLabel] = mxReplicaType

	for _, pod := range pods {
		selector, err := metav1.LabelSelectorAsSelector(mxReplicaSelector)
		if err != nil {
			return nil, err
		}
		if !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		result = append(result, pod)
	}
	return result, nil
}
