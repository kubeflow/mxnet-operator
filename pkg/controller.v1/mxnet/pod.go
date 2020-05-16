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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	commonv1 "github.com/kubeflow/common/pkg/apis/common/v1"
	mxv1 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1"
)

const (
	// gang scheduler name.
	gangSchedulerName = "volcano"

	// mxConfig is the environment variable name of MXNet cluster spec.
	mxConfig = "MX_CONFIG"
)

func (tc *MXController) GetPodsForJob(job interface{}) ([]*corev1.Pod, error) {
	mxJob, ok := job.(*mxv1.MXJob)
	if !ok {
		return nil, fmt.Errorf("%v is not a type of MXJob", mxJob)
	}

	//log := mxlogger.LoggerForJob(mxJob)
	// List all pods to include those that don't match the selector anymore
	// but have a ControllerRef pointing to this controller.

	labelSelector := metav1.LabelSelector{MatchLabels: tc.JobController.GenLabels(mxJob.Name)}
	opts := metav1.ListOptions{
		LabelSelector: labels.Set(labelSelector.MatchLabels).String(),
	}
	podlist, err := tc.KubeClientSet.CoreV1().Pods(mxJob.Namespace).List(opts)
	if err != nil {
		return nil, err
	}
	return convertPodList(podlist.Items), nil
}

// convertPodList convert pod list to pod pointer list
func convertPodList(list []corev1.Pod) []*corev1.Pod {
	if list == nil {
		return nil
	}
	ret := make([]*corev1.Pod, 0, len(list))
	for i := range list {
		ret = append(ret, &list[i])
	}
	return ret
}

func (tc *MXController) SetClusterSpec(job interface{}, podTemplate *corev1.PodTemplateSpec, rtype, index string) error {
	mxJob, ok := job.(*mxv1.MXJob)
	if !ok {
		return fmt.Errorf("%v is not a type of MXJob", mxJob)
	}

	// Generate MX_CONFIG JSON.
	mxConfigData, err := genMXConfig(mxJob, rtype, index)
	if err != nil {
		return err
	}

	// Generate MX_CONFIG JSON Str.
	mxConfigJson, err := json.Marshal(mxConfigData)
	if err != nil {
		return err
	}

	// Add MX_CONFIG environment variable.
	for i := range podTemplate.Spec.Containers {

		c := &podTemplate.Spec.Containers[i]

		// Set environment variable MX_CONFIG
		c.Env = append(c.Env, corev1.EnvVar{
			Name:  mxConfig,
			Value: string(mxConfigJson),
		})

		// Set Mxnet Distributed Training environment variable
		// We get these envs from MX_COFING to make them stay identical
		c.Env = append(c.Env, corev1.EnvVar{
			Name:  "DMLC_PS_ROOT_PORT",
			Value: strconv.Itoa(getConfigAddr(&mxConfigData, mxv1.MXReplicaTypeScheduler, 0).Port),
		})

		c.Env = append(c.Env, corev1.EnvVar{
			Name:  "DMLC_PS_ROOT_URI",
			Value: getConfigAddr(&mxConfigData, mxv1.MXReplicaTypeScheduler, 0).Url,
		})

		c.Env = append(c.Env, corev1.EnvVar{
			Name:  "DMLC_NUM_SERVER",
			Value: strconv.Itoa(getConfigReplica(&mxConfigData, mxv1.MXReplicaTypeServer)),
		})

		c.Env = append(c.Env, corev1.EnvVar{
			Name:  "DMLC_NUM_WORKER",
			Value: strconv.Itoa(getConfigReplica(&mxConfigData, mxv1.MXReplicaTypeWorker)),
		})

		c.Env = append(c.Env, corev1.EnvVar{
			Name:  "DMLC_ROLE",
			Value: mxConfigData.Task.Type,
		})

		c.Env = append(c.Env, corev1.EnvVar{
			Name:  "DMLC_USE_KUBERNETES",
			Value: strconv.Itoa(1),
		})
	}
	return nil
}

func setRestartPolicy(podTemplateSpec *corev1.PodTemplateSpec, spec *commonv1.ReplicaSpec) {
	if spec.RestartPolicy == commonv1.RestartPolicyExitCode {
		podTemplateSpec.Spec.RestartPolicy = corev1.RestartPolicyNever
	} else {
		podTemplateSpec.Spec.RestartPolicy = corev1.RestartPolicy(spec.RestartPolicy)
	}
}

func getConfigAddr(mxConfigData *MXConfig, rtype commonv1.ReplicaType, index int) UrlPort {
	rt := strings.ToLower(string(rtype))
	var url_port UrlPort
	if len(mxConfigData.Cluster[rt]) <= index {
		// index out of range, maybe this url doen't exist
		url_port = UrlPort{
			Url:  "",
			Port: 0,
		}
	} else {
		url_port = mxConfigData.Cluster[rt][index]
	}
	return url_port
}

func getConfigReplica(mxConfigData *MXConfig, rtype commonv1.ReplicaType) int {
	rt := strings.ToLower(string(rtype))
	return len(mxConfigData.Cluster[rt])
}

func isNonGangSchedulerSet(job *mxv1.MXJob) bool {
	for _, spec := range job.Spec.MXReplicaSpecs {
		if spec.Template.Spec.SchedulerName != "" && spec.Template.Spec.SchedulerName != gangSchedulerName {
			return true
		}
	}
	return false
}
