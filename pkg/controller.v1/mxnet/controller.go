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
	"time"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"

	commonv1 "github.com/kubeflow/common/pkg/apis/common/v1"
	"github.com/kubeflow/common/pkg/controller.v1/common"
	"github.com/kubeflow/common/pkg/controller.v1/expectation"
	mxlogger "github.com/kubeflow/common/pkg/util"
	"github.com/kubeflow/mxnet-operator/cmd/mxnet-operator.v1/app/options"
	mxv1 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1"
	mxjobclientset "github.com/kubeflow/mxnet-operator/pkg/client/clientset/versioned"
	mxjobscheme "github.com/kubeflow/mxnet-operator/pkg/client/clientset/versioned/scheme"
	mxjobinformers "github.com/kubeflow/mxnet-operator/pkg/client/informers/externalversions"
	mxjobinformersv1 "github.com/kubeflow/mxnet-operator/pkg/client/informers/externalversions/mxnet/v1"
	mxjoblisters "github.com/kubeflow/mxnet-operator/pkg/client/listers/mxnet/v1"
	volcanoclient "volcano.sh/volcano/pkg/client/clientset/versioned"
)

const (
	controllerName = "mxnet-operator"

	// labels for pods and servers.
	mxReplicaTypeLabel  = "mxnet-replica-type"
	mxReplicaIndexLabel = "mxnet-replica-index"
	labelGroupName      = "group-name"
	labelMXJobName      = "mxnet-job-name"
	labelMXJobRole      = "mxnet-job-role"
)

var (
	// KeyFunc is the short name to DeletionHandlingMetaNamespaceKeyFunc.
	// IndexerInformer uses a delta queue, therefore for deletes we have to use this
	// key function but it should be just fine for non delete events.
	KeyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc

	// DefaultMXControllerConfiguration is the suggested mxnet-operator configuration for production.
	DefaultMXControllerConfiguration = common.JobControllerConfiguration{
		ReconcilerSyncLoopPeriod: metav1.Duration{Duration: 15 * time.Second},
		EnableGangScheduling:     false,
	}

	// DefaultCleanPodPolicy is the default clean pod policy controller assign the new Job if not exist
	DefaultCleanPodPolicy = commonv1.CleanPodPolicyNone
)

// MXController is the type for MXJob Controller, which manages
// the lifecycle of MXJobs.
type MXController struct {
	common.JobController

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
	volcanoClientSet volcanoclient.Interface,
	kubeInformerFactory kubeinformers.SharedInformerFactory,
	// This field is not used now but we keep it since it will be used
	// after we support CRD validation.
	mxJobInformerFactory mxjobinformers.SharedInformerFactory,
	option options.ServerOption) *MXController {

	if err := mxjobscheme.AddToScheme(scheme.Scheme); err != nil {
		log.Fatal("Cannot add mxjob scheme")
	}

	log.Info("Creating MXJob controller")
	// Create new MXController.
	tc := &MXController{
		mxJobClientSet: mxJobClientSet,
	}

	// Create base controller
	log.Info("Creating Job controller")
	jc := common.NewJobController(tc, metav1.Duration{Duration: 15 * time.Second},
		option.EnableGangScheduling, kubeClientSet, volcanoClientSet, kubeInformerFactory, mxv1.Plural)
	// Set sync handler.
	tc.syncHandler = tc.syncMXJob
	// TODO: we may not need it
	//tc.updateStatusHandler = tc.UpdateJobStatusInApiServer
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

	jc.PodLister = podInformer.Lister()
	jc.PodInformerSynced = podInformer.Informer().HasSynced

	// Create service informer.
	serviceInformer := kubeInformerFactory.Core().V1().Services()

	// Set up an event handler for when service resources change.
	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    jc.AddService,
		UpdateFunc: jc.UpdateService,
		DeleteFunc: jc.DeleteService,
	})

	jc.ServiceLister = serviceInformer.Lister()
	jc.ServiceInformerSynced = serviceInformer.Informer().HasSynced

	tc.JobController = jc

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

	// TODO(Jeffwan): move to defaulters.
	if mxjob.Spec.RunPolicy.CleanPodPolicy == nil {
		mxjob.Spec.RunPolicy.CleanPodPolicy = &DefaultCleanPodPolicy
	}

	var reconcileMXJobsErr error
	if mxjobNeedsSync && mxjob.DeletionTimestamp == nil {
		reconcileMXJobsErr = tc.ReconcileJobs(mxjob, mxjob.Spec.MXReplicaSpecs, mxjob.Status, &mxjob.Spec.RunPolicy)
	}

	if reconcileMXJobsErr != nil {
		return false, reconcileMXJobsErr
	}

	return true, err
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
		} else if s.Template.Annotations[mxJobTunerServerKey] == "" {
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
		expectationPodsKey := expectation.GenExpectationPodsKey(mxjobKey, string(rtype))
		satisfied = satisfied || tc.Expectations.SatisfiedExpectations(expectationPodsKey)

		// Check the expectations of the services.
		expectationServicesKey := expectation.GenExpectationServicesKey(mxjobKey, string(rtype))
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

func (tc *MXController) GetGroupNameLabelKey() string {
	return labelGroupName
}

func (tc *MXController) GetJobNameLabelKey() string {
	return labelMXJobName
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

func (tc *MXController) GetAPIGroupVersionKind() schema.GroupVersionKind {
	return mxv1.SchemeGroupVersionKind
}

func (tc *MXController) GetAPIGroupVersion() schema.GroupVersion {
	return mxv1.SchemeGroupVersion
}

func (tc *MXController) GetGroupNameLabelValue() string {
	return mxv1.GroupName
}

func (tc *MXController) GetDefaultContainerName() string {
	return "mxnet"
}

func (tc *MXController) GetDefaultContainerPortName() string {
	return ""
}

func (tc *MXController) GetJobRoleKey() string {
	return labelMXJobRole
}

func (tc *MXController) IsMasterRole(replicas map[commonv1.ReplicaType]*commonv1.ReplicaSpec, rtype commonv1.ReplicaType, index int) bool {
	return string(rtype) == string(mxv1.MXReplicaTypeServer)
}
