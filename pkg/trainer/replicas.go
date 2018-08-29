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

package trainer

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sErrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"

	"github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/helper"
	mxv1alpha1 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1alpha1"
	"github.com/kubeflow/mxnet-operator/pkg/util"
	"github.com/kubeflow/mxnet-operator/pkg/util/k8sutil"
)

const (
	SuccessfulCreateReason = "SuccessfulCreate"
	FailedCreateReason     = "FailedCreate"
	indexField             = "replica"
)

// MXReplicaSet is a set of MX processes all acting as the same role (e.g. worker
type MXReplicaSet struct {
	ClientSet kubernetes.Interface
	recorder  record.EventRecorder
	// Job is a pointer to the TrainingJob to which this replica belongs.
	Job  *TrainingJob
	Spec mxv1alpha1.MXReplicaSpec

	// contextLogger is a logger to use for logging information about this replica.
	contextLogger *log.Entry
}

// MXReplicaSetInterface is an interface for managing a set of replicas.
type MXReplicaSetInterface interface {
	Create() error
	Delete() error
	GetStatus() (mxv1alpha1.MXReplicaStatus, error)
}

// MXConfig is a struct representing the MXNet config. This struct is turned into an environment
// which is used by MXNet processes to configure themselves.
type MXConfig struct {
	// Cluster represents a MXNet ClusterSpec.
	Cluster ClusterSpec `json:"cluster"`
	Task    TaskSpec    `json:"task"`
}

// NewMXReplicaSet returns MXReplicaSet object for existing replica
func NewMXReplicaSet(clientSet kubernetes.Interface, recorder record.EventRecorder, mxReplicaSpec mxv1alpha1.MXReplicaSpec, job *TrainingJob) (*MXReplicaSet, error) {
	if mxReplicaSpec.MXReplicaType == mxv1alpha1.SCHEDULER && *mxReplicaSpec.Replicas != 1 {
		return nil, errors.New("The SCHEDULER must have Replicas = 1")
	}

	if mxReplicaSpec.MXReplicaType == mxv1alpha1.SCHEDULER {
		if mxReplicaSpec.PsRootPort == nil {
			return nil, errors.New("mxReplicaSpec.PsRootPort can't be nil.")
		}
	}

	if mxReplicaSpec.Template == nil {
		return nil, errors.New("mxReplicaSpec.Template can't be nil.")
	}

	// Make sure the replica type is valid.
	validReplicaTypes := []mxv1alpha1.MXReplicaType{mxv1alpha1.SCHEDULER, mxv1alpha1.SERVER, mxv1alpha1.WORKER}

	isValidReplicaType := false
	for _, t := range validReplicaTypes {
		if t == mxReplicaSpec.MXReplicaType {
			isValidReplicaType = true
			break
		}
	}

	if !isValidReplicaType {
		return nil, fmt.Errorf("mxReplicaSpec.MXReplicaType is %v but must be one of %v", mxReplicaSpec.MXReplicaType, validReplicaTypes)
	}

	return &MXReplicaSet{
		ClientSet: clientSet,
		recorder:  recorder,
		Job:       job,
		Spec:      mxReplicaSpec,
		contextLogger: log.WithFields(log.Fields{
			"job_type":    string(mxReplicaSpec.MXReplicaType),
			"runtime_id":  job.job.Spec.RuntimeId,
			"mx_job_name": job.job.ObjectMeta.Name,
			// We use job to match the key used in controller.go
			// In controller.go we log the key used with the workqueue.
			"job": job.job.ObjectMeta.Namespace + "/" + job.job.ObjectMeta.Name,
		}),
	}, nil
}

// Labels returns the labels for this replica set.
func (s *MXReplicaSet) Labels() KubernetesLabels {
	return KubernetesLabels(map[string]string{
		"kubeflow.org": "",
		"job_type":     string(s.Spec.MXReplicaType),
		// runtime_id is set by Job.setup, which is called after the TFReplicaSet is created.
		// this is why labels aren't a member variable.
		"runtime_id":  s.Job.job.Spec.RuntimeId,
		"mx_job_name": s.Job.job.ObjectMeta.Name})
}

// LabelsByIndex returns the labels for a pod in this replica set.
func (s *MXReplicaSet) LabelsByIndex(index int32) KubernetesLabels {
	labels := s.Labels()
	labels["task_index"] = fmt.Sprintf("%v", index)
	return labels
}

// CreateServiceWithIndex will create a new service with specify index
func (s *MXReplicaSet) CreateServiceWithIndex(index int32) (*v1.Service, error) {
	taskLabels := s.LabelsByIndex(index)

	// Create the service.
	service := &v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:   s.genName(index),
			Labels: taskLabels,
			OwnerReferences: []meta_v1.OwnerReference{
				helper.AsOwner(s.Job.job),
			},
		},
		Spec: v1.ServiceSpec{
			Selector: taskLabels,
			// We use headless services here, because we don't need load balancing
			// since there is a single pod that is the backend for each service.
			ClusterIP: "None",
			Ports: []v1.ServicePort{
				{
					Name: "ps-root-port",
					Port: *s.Spec.PsRootPort,
				},
			},
		},
	}

	s.contextLogger.WithFields(log.Fields{
		indexField: index,
	}).Infof("Creating service: %v", service.ObjectMeta.Name)
	return s.ClientSet.CoreV1().Services(s.Job.job.ObjectMeta.Namespace).Create(service)
}

// CreatePodWithIndex will create a new pod with specify index
func (s *MXReplicaSet) CreatePodWithIndex(index int32) (*v1.Pod, error) {
	taskLabels := s.LabelsByIndex(index)

	pod := &v1.Pod{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:        s.genPodName(index),
			Labels:      taskLabels,
			Annotations: map[string]string{},
			OwnerReferences: []meta_v1.OwnerReference{
				helper.AsOwner(s.Job.job),
			},
		},
		Spec: *s.Spec.Template.Spec.DeepCopy(),
	}

	pod.Spec.SchedulerName = s.Job.SchedulerName()

	// copy labels and annotations to pod from mxjob
	for k, v := range s.Spec.Template.Labels {
		if _, ok := pod.Labels[k]; !ok {
			pod.Labels[k] = v
		}
	}

	for k, v := range s.Spec.Template.Annotations {
		if _, ok := pod.Annotations[k]; !ok {
			pod.Annotations[k] = v
		}
	}

	// Add MX_CONFIG environment variable.
	for i := range pod.Spec.Containers {
		// We can't get c in the loop variable because that would be by value so our modifications
		// wouldn't have any effect.
		c := &pod.Spec.Containers[i]

		//if c.Name != mxv1alpha1.DefaultMXContainer {
		//	continue
		//}
		if len(c.Env) == 0 {
			c.Env = make([]v1.EnvVar, 6)
		}

		for _, r := range s.Job.job.Spec.ReplicaSpecs {
			switch r.MXReplicaType {
			case mxv1alpha1.SCHEDULER:
				c.Env[0].Name = "DMLC_PS_ROOT_PORT"
				c.Env[0].Value = strconv.Itoa(int(*r.PsRootPort))
				c.Env[1].Name = "DMLC_PS_ROOT_URI"
				c.Env[1].Value = fmt.Sprintf("%v-%v-%v-%v", s.Job.job.ObjectMeta.Name, strings.ToLower(string(r.MXReplicaType)), s.Job.job.Spec.RuntimeId, 0)
			case mxv1alpha1.SERVER:
				c.Env[2].Name = "DMLC_NUM_SERVER"
				c.Env[2].Value = strconv.Itoa(int(*r.Replicas))
			case mxv1alpha1.WORKER:
				c.Env[3].Name = "DMLC_NUM_WORKER"
				c.Env[3].Value = strconv.Itoa(int(*r.Replicas))
			}
		}
		c.Env[4].Name = "DMLC_ROLE"
		c.Env[4].Value = strings.ToLower(string(s.Spec.MXReplicaType))
		c.Env[5].Name = "DMLC_USE_KUBERNETES"
		c.Env[5].Value = strconv.Itoa(1)
	}

	s.contextLogger.WithFields(log.Fields{
		indexField: index,
	}).Infof("Creating pod: %v", pod.ObjectMeta.Name)
	return s.ClientSet.CoreV1().Pods(s.Job.job.ObjectMeta.Namespace).Create(pod)
}

// Delete the replicas by cleanup pod policy when the mxjob is complete or failed: CleanupPodAll, CleanupPodNone, CleanupPodRunning, the default is CleanupPodAll
func (s *MXReplicaSet) DeleteResourcesByCleanPolicy(cleanupPodPolicy mxv1alpha1.CleanupPodPolicy) (err error) {
	log.Infof("DeleteResourcesByCleanPolicy for %s with CleanupPodPolicy %v", s.Job.job.ObjectMeta.Name, cleanupPodPolicy)
	switch cleanupPodPolicy {
	case mxv1alpha1.CleanupPodUndefined, mxv1alpha1.CleanupPodAll:
		s.contextLogger.Infof("Apply Clean All Policy for %s", s.Job.job.ObjectMeta.Name)
		err = s.Delete()
	case mxv1alpha1.CleanupPodNone:
		s.contextLogger.Infof("Apply Clean None Policy for %s", s.Job.job.ObjectMeta.Name)
	case mxv1alpha1.CleanupPodRunning:
		s.contextLogger.Infof("Apply Clean Running Pod Policy for %s", s.Job.job.ObjectMeta.Name)
		err = s.DeleteRunningPods()
	default:
		s.contextLogger.Errorf("Unknown cleanupPodPolicy %v", cleanupPodPolicy)
		err = fmt.Errorf("Unknown cleanupPodPolicy %v", cleanupPodPolicy)
	}

	return err
}

// Deletes the running pods
func (s *MXReplicaSet) DeleteRunningPods() error {
	selector, err := s.Labels().ToSelector()
	if err != nil {
		return err
	}

	failures := false
	fieldSelector := fmt.Sprintf("status.phase!=" + string(v1.PodSucceeded) + ",status.phase!=" + string(v1.PodFailed))

	options := meta_v1.ListOptions{
		LabelSelector: selector,
		FieldSelector: fieldSelector,
	}

	s.contextLogger.Infof("Deleting Pods namespace=%v selector=%v fieldSelector=%v",
		s.Job.job.ObjectMeta.Namespace,
		selector,
		fieldSelector)
	err = s.ClientSet.CoreV1().Pods(s.Job.job.ObjectMeta.Namespace).DeleteCollection(&meta_v1.DeleteOptions{}, options)

	if err != nil {
		s.contextLogger.Errorf("There was a problem deleting the pods; %v", err)
		failures = true
	}

	if failures {
		return errors.New("Some of the replicas resources could not be deleted")
	}

	return nil

}

// Delete deletes the replicas
func (s *MXReplicaSet) Delete() error {
	selector, err := s.Labels().ToSelector()
	if err != nil {
		return err
	}

	failures := false

	options := meta_v1.ListOptions{
		LabelSelector: selector,
	}

	s.contextLogger.Infof("Deleting Jobs namespace=%v selector=%v", s.Job.job.ObjectMeta.Namespace, selector)
	err = s.ClientSet.CoreV1().Pods(s.Job.job.ObjectMeta.Namespace).DeleteCollection(&meta_v1.DeleteOptions{}, options)

	if err != nil {
		s.contextLogger.Errorf("There was a problem deleting the jobs; %v", err)
		failures = true
	}

	// We need to delete the completed pods.
	s.contextLogger.Infof("Deleting Pods namespace=%v selector=%v", s.Job.job.ObjectMeta.Namespace, selector)
	err = s.ClientSet.CoreV1().Pods(s.Job.job.ObjectMeta.Namespace).DeleteCollection(&meta_v1.DeleteOptions{}, options)

	if err != nil {
		s.contextLogger.Errorf("There was a problem deleting the pods; %v", err)
		failures = true
	}

	// Services doesn't support DeleteCollection so we delete them individually.
	for index := int32(0); index < *s.Spec.Replicas; index++ {
		s.contextLogger.WithFields(log.Fields{
			indexField: index,
		}).Infof("Deleting Service %v:%v", s.Job.job.ObjectMeta.Namespace, s.genName((index)))
		err = s.ClientSet.CoreV1().Services(s.Job.job.ObjectMeta.Namespace).Delete(s.genName(index), &meta_v1.DeleteOptions{})

		if err != nil {
			s.contextLogger.Errorf("Error deleting service %v; %v", s.genName(index), err)
			failures = true
		}
	}

	// If the ConfigMap for the default parameter server exists, we delete it
	s.contextLogger.Infof("Get ConfigMaps %v:%v", s.Job.job.ObjectMeta.Namespace, s.defaultSERVERConfigMapName())
	_, err = s.ClientSet.CoreV1().ConfigMaps(s.Job.job.ObjectMeta.Namespace).Get(s.defaultSERVERConfigMapName(), meta_v1.GetOptions{})
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			s.contextLogger.Errorf("Error deleting ConfigMap %v; %v", s.defaultSERVERConfigMapName(), err)
			failures = true
		}
	} else {
		s.contextLogger.Infof("Delete ConfigMaps %v:%v", s.Job.job.ObjectMeta.Namespace, s.defaultSERVERConfigMapName())
		err = s.ClientSet.CoreV1().ConfigMaps(s.Job.job.ObjectMeta.Namespace).Delete(s.defaultSERVERConfigMapName(), &meta_v1.DeleteOptions{})
		if err != nil {
			s.contextLogger.Errorf("There was a problem deleting the ConfigMaps; %v", err)
			failures = true
		}
	}

	if failures {
		return errors.New("Some of the replicas resources could not be deleted")
	}
	return nil
}

// replicaStatusFromPodList returns a status from a list of pods for a job.
func replicaStatusFromPodList(l v1.PodList, name string) mxv1alpha1.ReplicaState {
	var latest *v1.Pod
	for _, i := range l.Items {
		if latest == nil {
			latest = &i
			continue
		}
		if latest.Status.StartTime.Before(i.Status.StartTime) {
			latest = &i
		}
	}

	if latest == nil {
		return mxv1alpha1.ReplicaStateRunning
	}

	var mxState v1.ContainerState

	for _, i := range latest.Status.ContainerStatuses {
		if i.Name != name {
			continue
		}

		// We need to decide whether to use the current state or the previous termination state.
		mxState = i.State

		// If the container previously terminated we will look at the termination to decide whether it is a retryable
		// or permanenent error.
		if i.LastTerminationState.Terminated != nil {
			mxState = i.LastTerminationState
		}
	}

	if mxState.Running != nil || mxState.Waiting != nil {
		return mxv1alpha1.ReplicaStateRunning
	}

	if mxState.Terminated != nil {
		if mxState.Terminated.ExitCode == 0 {
			return mxv1alpha1.ReplicaStateSucceeded
		}

		if isRetryableTerminationState(mxState.Terminated) {
			// Since its a retryable error just return RUNNING.
			// We can just let Kubernetes restart the container to retry.
			return mxv1alpha1.ReplicaStateRunning
		}

		return mxv1alpha1.ReplicaStateFailed
	}

	return mxv1alpha1.ReplicaStateUnknown
}

// GetSingleReplicaStatus returns status for a single replica
func (s *MXReplicaSet) GetSingleReplicaStatus(index int32) mxv1alpha1.ReplicaState {
	labels := s.LabelsByIndex(index)
	selector, err := labels.ToSelector()
	if err != nil {
		s.contextLogger.Errorf("labels.ToSelector() error; %v", err)
		return mxv1alpha1.ReplicaStateFailed
	}

	l, err := s.ClientSet.CoreV1().Pods(s.Job.job.ObjectMeta.Namespace).List(meta_v1.ListOptions{
		LabelSelector: selector,
	})

	if err != nil {
		return mxv1alpha1.ReplicaStateFailed
	}

	status := replicaStatusFromPodList(*l, mxv1alpha1.DefaultMXContainer)
	return status
}

// GetStatus returns the status of the replica set.
func (s *MXReplicaSet) GetStatus() (mxv1alpha1.MXReplicaStatus, error) {
	status := mxv1alpha1.MXReplicaStatus{
		MXReplicaType:  s.Spec.MXReplicaType,
		State:          mxv1alpha1.ReplicaStateUnknown,
		ReplicasStates: make(map[mxv1alpha1.ReplicaState]int),
	}

	increment := func(state mxv1alpha1.ReplicaState) {
		v, ok := status.ReplicasStates[state]
		if ok {
			status.ReplicasStates[state] = v + 1
		} else {
			status.ReplicasStates[state] = 1
		}
	}

	for index := int32(0); index < *s.Spec.Replicas; index++ {
		increment(s.GetSingleReplicaStatus(index))
	}

	// Determine the overall status for the replica set based on the status of the individual
	// replicas.
	// If any of the replicas failed mark the set as failed.
	if _, ok := status.ReplicasStates[mxv1alpha1.ReplicaStateFailed]; ok {
		status.State = mxv1alpha1.ReplicaStateFailed
		return status, nil
	}

	// If any replicas are RUNNING mark it as RUNNING.
	if _, ok := status.ReplicasStates[mxv1alpha1.ReplicaStateRunning]; ok {
		status.State = mxv1alpha1.ReplicaStateRunning
		return status, nil
	}

	// If all of the replicas succeeded consider it success.
	if v, ok := status.ReplicasStates[mxv1alpha1.ReplicaStateSucceeded]; ok && int32(v) == *s.Spec.Replicas {
		status.State = mxv1alpha1.ReplicaStateSucceeded
		return status, nil
	}

	return status, nil
}

// SyncPods will try to check current pods for this MXReplicaSet and try to make it as desired.
func (s *MXReplicaSet) SyncPods() error {
	for index := int32(0); index < *s.Spec.Replicas; index++ {

		// Label to get all pods of this MXReplicaType + index
		labels := s.LabelsByIndex(index)

		labelSelector, err := labels.ToSelector()
		if err != nil {
			return err
		}

		// Filter the unactive pods
		fieldSelector := fmt.Sprintf("status.phase!=%s", string(v1.PodFailed))

		options := meta_v1.ListOptions{
			LabelSelector: labelSelector,
			FieldSelector: fieldSelector,
		}

		// List to get pods
		pl, err := s.ClientSet.CoreV1().Pods(s.Job.job.ObjectMeta.Namespace).List(options)
		if err != nil {
			return err
		}

		if len(pl.Items) == 0 {
			s.contextLogger.Infof("Job %v missing pod for replica %v index %v, creating a new one.", s.Job.name(), string(s.Spec.MXReplicaType), index)
			// Create the pod
			createdPod, err := s.CreatePodWithIndex(index)

			// If the pod already exists do nothing.
			if err != nil {
				if k8s_errors.IsAlreadyExists(err) {
					s.contextLogger.Infof("Pod: %v already exists.", createdPod.ObjectMeta.Name)
					continue
				}
				s.recorder.Eventf(s.Job.job, v1.EventTypeWarning, FailedCreateReason, "Error creating: %v", err)
				return k8sErrors.NewAggregate([]error{fmt.Errorf("Creating pod %v returned error.", createdPod.ObjectMeta.Name), err})
			}

			s.recorder.Eventf(s.Job.job, v1.EventTypeNormal, SuccessfulCreateReason, "Created pod: %v", createdPod.Name)
			continue
		}

		if err != nil {
			continue
		}
	}

	return nil
}

// SyncServices will try to check current services for this MXReplicaSet and try to make it as desired.
func (s *MXReplicaSet) SyncServices() error {
	//if s.Spec.MXReplicaType == mxv1alpha1.SCHEDULER{
	for index := int32(0); index < *s.Spec.Replicas; index++ {
		_, err := s.ClientSet.CoreV1().Services(s.Job.job.ObjectMeta.Namespace).Get(s.genName(index), meta_v1.GetOptions{})

		s.contextLogger.Infof("Service: %v check ++++++++++++++++++++!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!", index)
		if err != nil {
			s.contextLogger.Infof("Service: %v check error ++++++++++++++++++++!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!", err)
		}

		if err != nil && k8s_errors.IsNotFound(err) {
			s.contextLogger.Infof("Service: %v not found, create new one.", s.genName(index))
			// Create the service
			createdService, err := s.CreateServiceWithIndex(index)

			// If the service already exists do nothing.
			if err != nil {
				if k8s_errors.IsAlreadyExists(err) {
					s.contextLogger.Infof("Service: %v already exists.", s.genName(index))
					continue
				}
				s.recorder.Eventf(s.Job.job, v1.EventTypeWarning, FailedCreateReason, "Error creating: %v", err)
				return k8sErrors.NewAggregate([]error{fmt.Errorf("Creating Service %v returned error.", createdService.ObjectMeta.Name), err})
			}

			s.recorder.Eventf(s.Job.job, v1.EventTypeNormal, SuccessfulCreateReason, "Created Service: %v", createdService.Name)
			continue
		}

		if err != nil {
			continue
		}
		s.contextLogger.Infof("Service: %v error ++++++++++++++++++++!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!", err)
	}
	//}

	return nil
}

// genName generates the name which is concatenation of jabName, MXReplicaType, job RunId and index
func (s *MXReplicaSet) genName(index int32) string {
	// Truncate mxjob name to 40 characters
	// The whole job name should be compliant with the DNS_LABEL spec, up to a max length of 63 characters
	// Thus genName(40 chars)-replicaType(6 chars)-runtimeId(4 chars)-index(4 chars), also leaving some spaces
	// See https://github.com/kubernetes/community/blob/master/contributors/design-proposals/architecture/identifiers.md
	return fmt.Sprintf("%v-%v-%v-%v", fmt.Sprintf("%.40s", s.Job.job.ObjectMeta.Name), strings.ToLower(string(s.Spec.MXReplicaType)), s.Job.job.Spec.RuntimeId, index)
}

// genPodName generate a new pod name with random string
func (s *MXReplicaSet) genPodName(index int32) string {
	return s.genName(index) + "-" + util.RandString(5)
}

//  defaultSERVERConfigMapName returns the map default PS configuration map name using job's runtimeId
func (s *MXReplicaSet) defaultSERVERConfigMapName() string {
	return fmt.Sprintf("cm-server-%v", s.Job.job.Spec.RuntimeId)
}
