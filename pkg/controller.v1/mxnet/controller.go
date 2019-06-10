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
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"

	"github.com/kubeflow/mxnet-operator/cmd/mxnet-operator.v1/app/options"
	mxv1 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1"
	mxjobclientset "github.com/kubeflow/mxnet-operator/pkg/client/clientset/versioned"
	mxjobscheme "github.com/kubeflow/mxnet-operator/pkg/client/clientset/versioned/scheme"
	mxjobinformers "github.com/kubeflow/mxnet-operator/pkg/client/informers/externalversions"
	mxjobinformersv1 "github.com/kubeflow/mxnet-operator/pkg/client/informers/externalversions/mxnet/v1"
	mxjoblisters "github.com/kubeflow/mxnet-operator/pkg/client/listers/mxnet/v1"
	"github.com/kubeflow/mxnet-operator/pkg/util/k8sutil"
	"github.com/kubeflow/tf-operator/pkg/common/jobcontroller"
	mxlogger "github.com/kubeflow/tf-operator/pkg/logger"
	kubebatchclient "github.com/kubernetes-sigs/kube-batch/pkg/client/clientset/versioned"
)

const (
	controllerName = "mxnet-operator"

	// labels for pods and servers.
	mxReplicaTypeLabel  = "mxnet-replica-type"
	mxReplicaIndexLabel = "mxnet-replica-index"
	labelGroupName      = "group_name"
	labelMXJobName      = "mxnet_job_name"
	labelMXJobRole      = "mxnet-job-role"
)

var (
	// KeyFunc is the short name to DeletionHandlingMetaNamespaceKeyFunc.
	// IndexerInformer uses a delta queue, therefore for deletes we have to use this
	// key function but it should be just fine for non delete events.
	KeyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc

	// DefaultMXControllerConfiguration is the suggested mxnet-operator configuration for production.
	DefaultMXControllerConfiguration = jobcontroller.JobControllerConfiguration{
		ReconcilerSyncLoopPeriod: metav1.Duration{Duration: 15 * time.Second},
		EnableGangScheduling:     false,
	}
)

// MXController is the type for MXJob Controller, which manages
// the lifecycle of MXJobs.
type MXController struct {
	jobcontroller.JobController

	// mxJobClientSet is a clientset for CRD MXJob.
	mxJobClientSet mxjobclientset.Interface

	// To allow injection of sync functions for testing.
	syncHandler func(string) (bool, error)

	// To allow injection of updateStatus for testing.
	updateStatusHandler func(mxjob *mxv1.MXJob) error

	// To allow injection of deleteMXJob for testing.
	deleteMXJobHandler func(mxjob *mxv1.MXJob) error

	// mxJobInformer is a temporary field for unstructured informer support.
	mxJobInformer cache.SharedIndexInformer

	// Listers for MXJob, Pod and Service
	// mxJobLister can list/get mxjobs from the shared informer's store.
	mxJobLister mxjoblisters.MXJobLister

	// mxJobInformerSynced returns true if the mxjob store has been synced at least once.
	mxJobInformerSynced cache.InformerSynced
}

// NewMXController returns a new MXJob controller.
func NewMXController(
	// This variable is for unstructured informer.
	mxJobInformer mxjobinformersv1.MXJobInformer,
	kubeClientSet kubeclientset.Interface,
	mxJobClientSet mxjobclientset.Interface,
	kubeBatchClientSet kubebatchclient.Interface,
	kubeInformerFactory kubeinformers.SharedInformerFactory,
	// This field is not used now but we keep it since it will be used
	// after we support CRD validation.
	mxJobInformerFactory mxjobinformers.SharedInformerFactory,
	option options.ServerOption) *MXController {

	mxjobscheme.AddToScheme(scheme.Scheme)

	log.Info("Creating MXJob controller")
	// Create new MXController.
	tc := &MXController{
		mxJobClientSet: mxJobClientSet,
	}

	// Create base controller
	log.Info("Creating Job controller")
	jc := jobcontroller.NewJobController(tc, metav1.Duration{Duration: 15 * time.Second},
		option.EnableGangScheduling, kubeClientSet, kubeBatchClientSet, kubeInformerFactory, mxv1.Plural)
	tc.JobController = jc
	// Set sync handler.
	tc.syncHandler = tc.syncMXJob
	tc.updateStatusHandler = tc.updateMXJobStatus
	// set delete handler.
	tc.deleteMXJobHandler = tc.deleteMXJob
	// Set up an event handler for when mxjob resources change.
	mxJobInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    tc.addMXJob,
		UpdateFunc: tc.updateMXJob,
		// This will enter the sync loop and no-op,
		// because the mxjob has been deleted from the store.
		DeleteFunc: tc.enqueueMXJob,
	})

	tc.mxJobInformer = mxJobInformer.Informer()
	tc.mxJobLister = mxJobInformer.Lister()
	tc.mxJobInformerSynced = mxJobInformer.Informer().HasSynced

	// Create pod informer.
	podInformer := kubeInformerFactory.Core().V1().Pods()

	// Set up an event handler for when pod resources change
	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    jc.AddPod,
		UpdateFunc: jc.UpdatePod,
		DeleteFunc: jc.DeletePod,
	})

	tc.PodLister = podInformer.Lister()
	tc.PodInformerSynced = podInformer.Informer().HasSynced

	// Create service informer.
	serviceInformer := kubeInformerFactory.Core().V1().Services()

	// Set up an event handler for when service resources change.
	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    jc.AddService,
		UpdateFunc: jc.UpdateService,
		DeleteFunc: jc.DeleteService,
	})

	tc.ServiceLister = serviceInformer.Lister()
	tc.ServiceInformerSynced = serviceInformer.Informer().HasSynced

	return tc
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (tc *MXController) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer tc.WorkQueue.ShutDown()

	// Start the informer factories to begin populating the informer caches.
	log.Info("Starting MXJob controller")

	// Wait for the caches to be synced before starting workers.
	log.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, tc.mxJobInformerSynced); !ok {
		return fmt.Errorf("failed to wait for mxjob caches to sync")
	}

	if ok := cache.WaitForCacheSync(stopCh, tc.PodInformerSynced); !ok {
		return fmt.Errorf("failed to wait for pod caches to sync")
	}

	if ok := cache.WaitForCacheSync(stopCh, tc.ServiceInformerSynced); !ok {
		return fmt.Errorf("failed to wait for service caches to sync")
	}

	log.Infof("Starting %v workers", threadiness)
	// Launch workers to process MXJob resources.
	for i := 0; i < threadiness; i++ {
		go wait.Until(tc.runWorker, time.Second, stopCh)
	}

	log.Info("Started workers")
	<-stopCh
	log.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (tc *MXController) runWorker() {
	for tc.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (tc *MXController) processNextWorkItem() bool {
	key, quit := tc.WorkQueue.Get()
	if quit {
		return false
	}
	defer tc.WorkQueue.Done(key)

	logger := mxlogger.LoggerForKey(key.(string))

	mxJob, err := tc.getMXJobFromKey(key.(string))
	if err != nil {
		if err == errNotExists {
			logger.Infof("MXJob has been deleted: %v", key)
			return true
		}

		// Log the failure to conditions.
		logger.Errorf("Failed to get MXJob from key %s: %v", key, err)
		if err == errFailedMarshal {
			errMsg := fmt.Sprintf("Failed to unmarshal the object to MXJob object: %v", err)
			mxlogger.LoggerForJob(mxJob).Warn(errMsg)
			tc.Recorder.Event(mxJob, v1.EventTypeWarning, failedMarshalMXJobReason, errMsg)
		}

		return true
	}

	// Verify
	err = tc.inspectMXjob(mxJob)
	if err != nil {
		errMsg := fmt.Sprintf("Inspect Fail: %v", err)
		mxlogger.LoggerForJob(mxJob).Warn(errMsg)
		tc.Recorder.Event(mxJob, v1.EventTypeWarning, inspectFailMXJobReason, errMsg)
		return true
	}

	// Sync MXJob to match the actual state to this desired state.
	forget, err := tc.syncHandler(key.(string))
	if err == nil {
		if forget {
			tc.WorkQueue.Forget(key)
		}
		return true
	}

	utilruntime.HandleError(fmt.Errorf("error syncing mxjob: %v", err))
	tc.WorkQueue.AddRateLimited(key)

	return true
}

func (tc *MXController) enqueueMXJob(mxjob interface{}) {
	key, err := KeyFunc(mxjob)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for mxjob object %#v: %v", mxjob, err))
		return
	}

	// TODO: we may need add backoff here
	tc.WorkQueue.Add(key)
}

// syncMXJob syncs the mxjob with the given key if it has had its expectations fulfilled, meaning
// it did not expect to see any more of its pods/services created or deleted.
// This function is not meant to be invoked concurrently with the same key.
func (tc *MXController) syncMXJob(key string) (bool, error) {
	startTime := time.Now()
	logger := mxlogger.LoggerForKey(key)
	defer func() {
		logger.Infof("Finished syncing mxjob %q (%v)", key, time.Since(startTime))
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return false, err
	}
	if len(namespace) == 0 || len(name) == 0 {
		return false, fmt.Errorf("invalid mxjob key %q: either namespace or name is missing", key)
	}

	sharedMXJob, err := tc.getMXJobFromName(namespace, name)
	if err != nil {
		if err == errNotExists {
			logger.Infof("MXJob has been deleted: %v", key)
			// jm.expectations.DeleteExpectations(key)
			return true, nil
		}
		return false, err
	}

	mxjob := sharedMXJob.DeepCopy()
	mxjobNeedsSync := tc.satisfiedExpectations(mxjob)

	// Set default for the new mxjob.
	scheme.Scheme.Default(mxjob)

	var reconcileMXJobsErr error
	if mxjobNeedsSync && mxjob.DeletionTimestamp == nil {
		reconcileMXJobsErr = tc.reconcileMXJobs(mxjob)
	}

	if reconcileMXJobsErr != nil {
		return false, reconcileMXJobsErr
	}

	return true, err
}

// reconcileMXJobs checks and updates replicas for each given MXReplicaSpec.
// It will requeue the mxjob in case of an error while creating/deleting pods/services.
func (tc *MXController) reconcileMXJobs(mxjob *mxv1.MXJob) error {
	mxjobKey, err := KeyFunc(mxjob)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for mxjob object %#v: %v", mxjob, err))
		return err
	}

	logger := mxlogger.LoggerForJob(mxjob)
	logger.Infof("Reconcile MXJobs %s", mxjob.Name)

	pods, err := tc.GetPodsForJob(mxjob)
	if err != nil {
		logger.Warnf("getPodsForMXJob error %v", err)
		return err
	}

	services, err := tc.GetServicesForJob(mxjob)
	if err != nil {
		logger.Warnf("getServicesForMXJob error %v", err)
		return err
	}

	// retrieve the previous number of retry
	previousRetry := tc.WorkQueue.NumRequeues(mxjobKey)

	activePods := k8sutil.FilterActivePods(pods)
	active := int32(len(activePods))
	failed := k8sutil.FilterPodCount(pods, v1.PodFailed)
	totalReplicas := getTotalReplicas(mxjob)
	prevReplicasFailedNum := getTotalFailedReplicas(mxjob)

	var failureMessage string
	mxJobExceedsLimit := false
	exceedsBackoffLimit := false
	pastBackoffLimit := false

	if mxjob.Spec.BackoffLimit != nil {
		jobHasNewFailure := failed > prevReplicasFailedNum
		// new failures happen when status does not reflect the failures and active
		// is different than parallelism, otherwise the previous controller loop
		// failed updating status so even if we pick up failure it is not a new one
		exceedsBackoffLimit = jobHasNewFailure && (active != totalReplicas) &&
			(int32(previousRetry)+1 > *mxjob.Spec.BackoffLimit)

		pastBackoffLimit, err = tc.pastBackoffLimit(mxjob, pods)
		if err != nil {
			return err
		}
	}

	if exceedsBackoffLimit || pastBackoffLimit {
		// check if the number of pod restart exceeds backoff (for restart OnFailure only)
		// OR if the number of failed jobs increased since the last syncJob
		mxJobExceedsLimit = true
		failureMessage = fmt.Sprintf("MXJob %s has failed because it has reached the specified backoff limit", mxjob.Name)
	} else if tc.pastActiveDeadline(mxjob) {
		failureMessage = fmt.Sprintf("MXJob %s has failed because it was active longer than specified deadline", mxjob.Name)
		mxJobExceedsLimit = true
	}

	// If the MXJob is terminated, delete all pods and services.
	if isSucceeded(mxjob.Status) || isFailed(mxjob.Status) || mxJobExceedsLimit {
		if err := tc.deletePodsAndServices(mxjob, pods); err != nil {
			return err
		}

		if mxJobExceedsLimit {
			tc.Recorder.Event(mxjob, v1.EventTypeNormal, mxJobFailedReason, failureMessage)
			if mxjob.Status.CompletionTime == nil {
				now := metav1.Now()
				mxjob.Status.CompletionTime = &now
			}
			err := updateMXJobConditions(mxjob, mxv1.MXJobFailed, mxJobFailedReason, failureMessage)
			if err != nil {
				mxlogger.LoggerForJob(mxjob).Infof("Append mxjob condition error: %v", err)
				return err
			}
		}

		if err := tc.cleanupMXJob(mxjob); err != nil {
			return err
		}

		if tc.Config.EnableGangScheduling {
			if err := tc.DeletePodGroup(mxjob); err != nil {
				return err
			}
		}

		// Initialize the status.
		initializeMXReplicaStatuses(mxjob, mxv1.MXReplicaTypeScheduler)
		initializeMXReplicaStatuses(mxjob, mxv1.MXReplicaTypeWorker)
		initializeMXReplicaStatuses(mxjob, mxv1.MXReplicaTypeServer)
		return tc.updateStatusHandler(mxjob)
	}

	if tc.Config.EnableGangScheduling {
		minAvailableReplicas := getTotalReplicas(mxjob)
		_, err := tc.SyncPodGroup(mxjob, minAvailableReplicas)
		if err != nil {
			logger.Warnf("Sync PodGroup %v: %v", mxjob.Name, err)
		}
	}

	// Save the current state of the replicas
	replicasStatus := make(map[string]v1.PodPhase)

	// Diff current active pods/services with replicas.
	for rtype, spec := range mxjob.Spec.MXReplicaSpecs {
		err = tc.reconcilePods(mxjob, pods, rtype, spec, replicasStatus)
		if err != nil {
			logger.Warnf("reconcilePods error %v", err)
			return err
		}

		err = tc.reconcileServices(mxjob, services, rtype, spec)

		if err != nil {
			logger.Warnf("reconcileServices error %v", err)
			return err
		}
	}

	// TODO(CPH): Add check here, no need to update the mxjob if the status hasn't changed since last time.
	return tc.updateStatusHandler(mxjob)
}

// pastBackoffLimit checks if container restartCounts sum exceeds BackoffLimit
// this method applies only to pods with restartPolicy == OnFailure or Always
func (tc *MXController) pastBackoffLimit(mxjob *mxv1.MXJob, pods []*v1.Pod) (bool, error) {
	if mxjob.Spec.BackoffLimit == nil {
		return false, nil
	}
	logger := mxlogger.LoggerForJob(mxjob)
	result := int32(0)
	for rtype, spec := range mxjob.Spec.MXReplicaSpecs {
		if spec.RestartPolicy != mxv1.RestartPolicyOnFailure && spec.RestartPolicy != mxv1.RestartPolicyAlways {
			logger.Warnf("The restart policy of replica %v of the job %v is not OnFailure or Always. Not counted in backoff limit.", rtype, mxjob.Name)
			continue
		}
		// Convert TFReplicaType to lower string.
		rt := strings.ToLower(string(rtype))
		pods, err := tc.FilterPodsForReplicaType(pods, rt)
		if err != nil {
			return false, err
		}
		for i := range pods {
			po := pods[i]
			if po.Status.Phase == v1.PodRunning || po.Status.Phase == v1.PodPending {
				for j := range po.Status.InitContainerStatuses {
					stat := po.Status.InitContainerStatuses[j]
					result += stat.RestartCount
				}
				for j := range po.Status.ContainerStatuses {
					stat := po.Status.ContainerStatuses[j]
					result += stat.RestartCount
				}
			}
		}
	}

	if *mxjob.Spec.BackoffLimit == 0 {
		return result > 0, nil
	}
	return result >= *mxjob.Spec.BackoffLimit, nil
}

// pastActiveDeadline checks if job has ActiveDeadlineSeconds field set and if it is exceeded.
func (tc *MXController) pastActiveDeadline(mxjob *mxv1.MXJob) bool {
	if mxjob.Spec.ActiveDeadlineSeconds == nil || mxjob.Status.StartTime == nil {
		return false
	}
	now := metav1.Now()
	start := mxjob.Status.StartTime.Time
	duration := now.Time.Sub(start)
	allowedDuration := time.Duration(*mxjob.Spec.ActiveDeadlineSeconds) * time.Second
	return duration >= allowedDuration
}

// inspectMXjob make sure a MXjob has all the necessary MXReplicaSpecs members for a special jobMode.
// if not it return err
func (tc *MXController) inspectMXjob(mxjob *mxv1.MXJob) error {

	logger := mxlogger.LoggerForJob(mxjob)

	if mxjob.Spec.JobMode == mxv1.MXTrain {
		// Must have MXReplicaTypeScheduler, MXReplicaTypeServer, MXReplicaTypeWorker, shouldn't have
		// MXReplicaTypeTuner
		if _, ok := mxjob.Spec.MXReplicaSpecs[mxv1.MXReplicaTypeScheduler]; !ok {
			return errWrongJobMode
		}
		if _, ok := mxjob.Spec.MXReplicaSpecs[mxv1.MXReplicaTypeServer]; !ok {
			return errWrongJobMode
		}
		if _, ok := mxjob.Spec.MXReplicaSpecs[mxv1.MXReplicaTypeWorker]; !ok {
			return errWrongJobMode
		}
	} else if mxjob.Spec.JobMode == mxv1.MXTune {
		// Must have MXReplicaTypeTuner, shouldn't have MXReplicaTypeScheduler, MXReplicaTypeServer,
		// MXReplicaTypeWorker
		if _, ok := mxjob.Spec.MXReplicaSpecs[mxv1.MXReplicaTypeTunerTracker]; !ok {
			return errWrongJobMode
		}
		if s, ok := mxjob.Spec.MXReplicaSpecs[mxv1.MXReplicaTypeTunerServer]; !ok {
			return errWrongJobMode
		} else if s.Label == "" {
			logger.Warnf("MXReplicaTypeTunerRPCServer may need label to set tvm rpc-server key")
		}
		if _, ok := mxjob.Spec.MXReplicaSpecs[mxv1.MXReplicaTypeTuner]; !ok {
			return errWrongJobMode
		}
	}
	return nil
}

// satisfiedExpectations returns true if the required adds/dels for the given mxjob have been observed.
// Add/del counts are established by the controller at sync time, and updated as controllees are observed by the controller
// manager.
func (tc *MXController) satisfiedExpectations(mxjob *mxv1.MXJob) bool {
	satisfied := false
	mxjobKey, err := KeyFunc(mxjob)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for mxjob object %#v: %v", mxjob, err))
		return false
	}

	for rtype := range mxjob.Spec.MXReplicaSpecs {
		// Check the expectations of the pods.
		expectationPodsKey := jobcontroller.GenExpectationPodsKey(mxjobKey, string(rtype))
		satisfied = satisfied || tc.Expectations.SatisfiedExpectations(expectationPodsKey)

		// Check the expectations of the services.
		expectationServicesKey := jobcontroller.GenExpectationServicesKey(mxjobKey, string(rtype))
		satisfied = satisfied || tc.Expectations.SatisfiedExpectations(expectationServicesKey)
	}

	return satisfied
}

func (tc *MXController) GetJobFromInformerCache(namespace, name string) (metav1.Object, error) {
	return tc.getMXJobFromName(namespace, name)
}

func (tc *MXController) GetJobFromAPIClient(namespace, name string) (metav1.Object, error) {
	return tc.mxJobClientSet.KubeflowV1beta1().MXJobs(namespace).Get(name, metav1.GetOptions{})
}

func (tc *MXController) GetAPIGroupVersionKind() schema.GroupVersionKind {
	return mxv1.SchemeGroupVersionKind
}

func (tc *MXController) GetAPIGroupVersion() schema.GroupVersion {
	return mxv1.SchemeGroupVersion
}

func (tc *MXController) GetGroupNameLabelKey() string {
	return labelGroupName
}

func (tc *MXController) GetJobNameLabelKey() string {
	return labelMXJobName
}

func (tc *MXController) GetGroupNameLabelValue() string {
	return mxv1.GroupName
}

func (tc *MXController) GetReplicaTypeLabelKey() string {
	return mxReplicaTypeLabel
}

func (tc *MXController) GetReplicaIndexLabelKey() string {
	return mxReplicaIndexLabel
}

func (tc *MXController) ControllerName() string {
	return controllerName
}

func (tc *MXController) GetJobRoleKey() string {
	return labelMXJobRole
}
