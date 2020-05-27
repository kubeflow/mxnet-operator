package mxnet

import (
	"fmt"
	"time"

	commonv1 "github.com/kubeflow/common/pkg/apis/common/v1"
	commonutil "github.com/kubeflow/common/pkg/util"
	log "github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1unstructured "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"

	mxlogger "github.com/kubeflow/common/pkg/util"
	mxv1 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1"
	"github.com/kubeflow/mxnet-operator/pkg/util/k8sutil"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	failedMarshalMXJobReason  = "FailedInvalidMXJobSpec"
	inspectFailMXJobReason    = "InspectFailedInvalidMXReplicaSpec"
	FailedDeleteJobReason     = "FailedDeleteJob"
	SuccessfulDeleteJobReason = "SuccessfulDeleteJob"
)

func (tc *MXController) DeleteJob(job interface{}) error {
	mxJob, ok := job.(*mxv1.MXJob)
	if !ok {
		return fmt.Errorf("%v is not a type of MXJob", mxJob)
	}

	log := mxlogger.LoggerForJob(mxJob)
	if err := tc.mxJobClientSet.KubeflowV1().MXJobs(mxJob.Namespace).Delete(mxJob.Name, &metav1.DeleteOptions{}); err != nil {
		tc.JobController.Recorder.Eventf(mxJob, v1.EventTypeWarning, FailedDeleteJobReason, "Error deleting: %v", err)
		log.Errorf("failed to delete job %s/%s, %v", mxJob.Namespace, mxJob.Name, err)
		return err
	}

	tc.JobController.Recorder.Eventf(mxJob, v1.EventTypeNormal, SuccessfulDeleteJobReason, "Deleted job: %v", mxJob.Name)
	log.Infof("job %s/%s has been deleted", mxJob.Namespace, mxJob.Name)
	return nil
}

// When a pod is added, set the defaults and enqueue the current mxjob.
func (tc *MXController) addMXJob(obj interface{}) {
	// Convert from unstructured object.
	mxJob, err := mxJobFromUnstructured(obj)
	if err != nil {
		un, ok := obj.(*metav1unstructured.Unstructured)
		logger := &log.Entry{}
		if ok {
			logger = mxlogger.LoggerForUnstructured(un, mxv1.Kind)
		}
		logger.Errorf("Failed to convert the MXJob: %v", err)
		// Log the failure to conditions.
		if err == errFailedMarshal {
			errMsg := fmt.Sprintf("Failed to marshal the object to MXJob; the spec is invalid: %v", err)
			logger.Warn(errMsg)
			// TODO(jlewi): v1 doesn't appear to define an error type.
			tc.Recorder.Event(un, v1.EventTypeWarning, failedMarshalMXJobReason, errMsg)

			status := commonv1.JobStatus{
				Conditions: []commonv1.JobCondition{
					{
						Type:               commonv1.JobFailed,
						Status:             v1.ConditionTrue,
						LastUpdateTime:     metav1.Now(),
						LastTransitionTime: metav1.Now(),
						Reason:             failedMarshalMXJobReason,
						Message:            errMsg,
					},
				},
			}
			statusMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&status)
			if err != nil {
				logger.Errorf("Could not covert the MXJobStatus to unstructured; %v", err)
				return
			}
			client, err := k8sutil.NewCRDRestClient(&mxv1.SchemeGroupVersion)
			if err == nil {
				if err1 := metav1unstructured.SetNestedField(un.Object, statusMap, "status"); err1 != nil {
					logger.Errorf("Could not set nested field: %v", err1)
				}
				logger.Infof("Updating the job to; %+v", un.Object)
				err = client.UpdateStatus(un, mxv1.Plural)
				if err != nil {
					logger.Errorf("Could not update the MXJob; %v", err)
				}
			} else {
				logger.Errorf("Could not create a REST client to update the MXJob")
			}
		}
		return
	}

	// Set default for the new mxjob.
	scheme.Scheme.Default(mxJob)

	msg := fmt.Sprintf("MXJob %s is created.", mxJob.Name)
	logger := mxlogger.LoggerForJob(mxJob)
	logger.Info(msg)

	// Add a created condition.
	err = commonutil.UpdateJobConditions(&mxJob.Status, commonv1.JobCreated, mxJobCreatedReason, msg)
	if err != nil {
		logger.Errorf("Append mxJob condition error: %v", err)
		return
	}

	// Convert from mxjob object
	err = unstructuredFromMXJob(obj, mxJob)
	if err != nil {
		logger.Errorf("Failed to convert the obj: %v", err)
		return
	}
	tc.enqueueMXJob(obj)
}

// When a pod is updated, enqueue the current mxjob.
func (tc *MXController) updateMXJob(old, cur interface{}) {
	oldMXJob, err := mxJobFromUnstructured(old)
	if err != nil {
		return
	}
	curMXJob, err := mxJobFromUnstructured(cur)
	if err != nil {
		return
	}

	// never return error
	key, err := KeyFunc(curMXJob)
	if err != nil {
		return
	}

	log.Infof("Updating mxjob: %s", oldMXJob.Name)
	tc.enqueueMXJob(cur)

	// check if need to add a new rsync for ActiveDeadlineSeconds
	if curMXJob.Status.StartTime != nil {
		curMXJobADS := curMXJob.Spec.RunPolicy.ActiveDeadlineSeconds
		if curMXJobADS == nil {
			return
		}
		oldMXJobADS := oldMXJob.Spec.RunPolicy.ActiveDeadlineSeconds
		if oldMXJobADS == nil || *oldMXJobADS != *curMXJobADS {
			now := metav1.Now()
			start := curMXJob.Status.StartTime.Time
			passed := now.Time.Sub(start)
			total := time.Duration(*curMXJobADS) * time.Second
			// AddAfter will handle total < passed
			tc.WorkQueue.AddAfter(key, total-passed)
			log.Infof("job ActiveDeadlineSeconds updated, will rsync after %d seconds", total-passed)
		}
	}
}

// deleteMXJob deletes the given MXJob.
func (tc *MXController) deleteMXJob(mxJob *mxv1.MXJob) error {
	return tc.mxJobClientSet.KubeflowV1().MXJobs(mxJob.Namespace).Delete(mxJob.Name, &metav1.DeleteOptions{})
}
