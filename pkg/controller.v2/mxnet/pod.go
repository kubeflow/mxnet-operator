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
	"strconv"
	"strings"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	// policy is set in pod template.
	podTemplateRestartPolicyReason = "SettedPodTemplateRestartPolicy"
	// exitedWithCodeReason is the normal reason when the pod is exited because of the exit code.
	exitedWithCodeReason = "ExitedWithCode"
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
	pods, err := tc.FilterPodsForReplicaType(pods, rt)
	if err != nil {
		return err
	}
	replicas := int(*spec.Replicas)
	restart := false
	schedulerCompleted := false

	initializeMXReplicaStatuses(mxjob, rtype)

	podSlices := tc.GetPodSlices(pods, replicas, logger)
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
			// Get the exit code of the mxnet container.
			var exitCode int32 = 0xbeef // magic number
			for _, status := range pod.Status.ContainerStatuses {
				state := status.State
				if status.Name == mxv1alpha2.DefaultContainerName && state.Terminated != nil {
					exitCode = state.Terminated.ExitCode
					logger.Infof("Pod: %v.%v exited with code %v", pod.Namespace, pod.Name, exitCode)
					tc.Recorder.Eventf(mxjob, v1.EventTypeNormal, exitedWithCodeReason, "Pod: %v.%v exited with code %v", pod.Namespace, pod.Name, exitCode)
				}
			}
			// Check if the pod is retryable.
			if spec.RestartPolicy == mxv1alpha2.RestartPolicyExitCode {
				if pod.Status.Phase == v1.PodFailed && train_util.IsRetryableExitCode(exitCode) {
					logger.Infof("Need to restart the pod: %v.%v", pod.Namespace, pod.Name)
					if err := tc.PodControl.DeletePod(pod.Namespace, pod.Name, mxjob); err != nil {
						return err
					}
					restart = true
				}
			}

			// Check whether scheduler is exited without error.
			if rtype == mxv1alpha2.MXReplicaTypeScheduler && exitCode == 0 {
				schedulerCompleted = true
			}
			updateMXJobReplicaStatuses(mxjob, rtype, pod)
		}
	}

	return updateStatusSingle(mxjob, rtype, replicas, restart, schedulerCompleted)
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

func setClusterSpec(podTemplateSpec *v1.PodTemplateSpec, mxjob *mxv1alpha2.MXJob, rt, index string) error {

	// Add MX_CONFIG environment variable.
	for i := range podTemplateSpec.Spec.Containers {

		c := &podTemplateSpec.Spec.Containers[i]

		for t, r := range mxjob.Spec.MXReplicaSpecs {

			port, err := GetPortFromMXJob(mxjob, t)
			if err != nil {
				return err
			}

			rt := strings.ToLower(string(t))

			switch t {
			case mxv1alpha2.MXReplicaTypeScheduler:
				c.Env = append(c.Env, v1.EnvVar{
					Name:  "DMLC_PS_ROOT_PORT",
					Value: strconv.Itoa(int(port)),
				})
				c.Env = append(c.Env, v1.EnvVar{
					Name:  "DMLC_PS_ROOT_URI",
					Value: fmt.Sprintf("%s", jobcontroller.GenGeneralName(mxjob.Name, rt, fmt.Sprintf("%d", 0))),
				})
			case mxv1alpha2.MXReplicaTypeServer:
				c.Env = append(c.Env, v1.EnvVar{
					Name:  "DMLC_NUM_SERVER",
					Value: strconv.Itoa(int(*r.Replicas)),
				})
			case mxv1alpha2.MXReplicaTypeWorker:
				c.Env = append(c.Env, v1.EnvVar{
					Name:  "DMLC_NUM_WORKER",
					Value: strconv.Itoa(int(*r.Replicas)),
				})
			case mxv1alpha2.MXReplicaTypeTunerTracker:
				c.Env = append(c.Env, v1.EnvVar{
					Name:  "DMLC_TUNER_TRACKER_PORT",
					Value: strconv.Itoa(int(port)),
				})
				c.Env = append(c.Env, v1.EnvVar{
					Name:  "DMLC_TUNER_TRACKER_URI",
					Value: fmt.Sprintf("%s", jobcontroller.GenGeneralName(mxjob.Name, rt, fmt.Sprintf("%d", 0))),
				})
			case mxv1alpha2.MXReplicaTypeTunerRPCServer:
				c.Env = append(c.Env, v1.EnvVar{
					Name:  "DMLC_TUNER_SERVER_KEY",
					Value: r.Label,
				})
			}
		}

		c.Env = append(c.Env, v1.EnvVar{
			Name:  "DMLC_ROLE",
			Value: strings.ToLower(string(rt)),
		})

		c.Env = append(c.Env, v1.EnvVar{
			Name:  "DMLC_USE_KUBERNETES",
			Value: strconv.Itoa(1),
		})
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
