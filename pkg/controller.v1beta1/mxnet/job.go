package mxnet

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1unstructured "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"

	mxv1beta1 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1beta1"
	"github.com/kubeflow/mxnet-operator/pkg/util/k8sutil"
	mxlogger "github.com/kubeflow/tf-operator/pkg/logger"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	failedMarshalMXJobReason = "FailedInvalidMXJobSpec"
	inspectFailMXJobReason   = "InspectFailedInvalidMXReplicaSpec"
)

// When a pod is added, set the defaults and enqueue the current mxjob.
func (tc *MXController) addMXJob(obj interface{}) {
	// Convert from unstructured object.
	mxJob, err := mxJobFromUnstructured(obj)
	if err != nil {
		un, ok := obj.(*metav1unstructured.Unstructured)
		logger := &log.Entry{}
		if ok {
			logger = mxlogger.LoggerForUnstructured(un, mxv1beta1.Kind)
		}
		logger.Errorf("Failed to convert the MXJob: %v", err)
		// Log the failure to conditions.
		if err == errFailedMarshal {
			errMsg := fmt.Sprintf("Failed to marshal the object to MXJob; the spec is invalid: %v", err)
			logger.Warn(errMsg)
			// TODO(jlewi): v1 doesn't appear to define an error type.
			tc.Recorder.Event(un, v1.EventTypeWarning, failedMarshalMXJobReason, errMsg)

			status := mxv1beta1.MXJobStatus{
				Conditions: []mxv1beta1.MXJobCondition{
					mxv1beta1.MXJobCondition{
						Type:               mxv1beta1.MXJobFailed,
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
			client, err := k8sutil.NewCRDRestClient(&mxv1beta1.SchemeGroupVersion)
			if err == nil {
				metav1unstructured.SetNestedField(un.Object, statusMap, "status")
				logger.Infof("Updating the job to; %+v", un.Object)
				err = client.Update(un, mxv1beta1.Plural)
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
	err = updateMXJobConditions(mxJob, mxv1beta1.MXJobCreated, mxJobCreatedReason, msg)
	if err != nil {
		logger.Errorf("Append mxJob condition error: %v", err)
		return
	}

	// Convert from mxjob object
	err = unstructuredFromMXJob(obj, mxJob)
	if err != nil {
		logger.Error("Failed to convert the obj: %v", err)
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
	log.Infof("Updating mxjob: %s", oldMXJob.Name)
	tc.enqueueMXJob(cur)
}

func (tc *MXController) deletePodsAndServices(mxJob *mxv1beta1.MXJob, pods []*v1.Pod) error {
	if len(pods) == 0 {
		return nil
	}

	// Delete nothing when the cleanPodPolicy is None.
	if *mxJob.Spec.CleanPodPolicy == mxv1beta1.CleanPodPolicyNone {
		return nil
	}

	for _, pod := range pods {
		if *mxJob.Spec.CleanPodPolicy == mxv1beta1.CleanPodPolicyRunning && pod.Status.Phase != v1.PodRunning {
			continue
		}

		if err := tc.PodControl.DeletePod(pod.Namespace, pod.Name, mxJob); err != nil {
			return err
		}
		// Pod and service have the same name, thus the service could be deleted using pod's name.
		if err := tc.ServiceControl.DeleteService(pod.Namespace, pod.Name, mxJob); err != nil {
			return err
		}
	}
	return nil
}

func (tc *MXController) cleanupMXJob(mxJob *mxv1beta1.MXJob) error {
	currentTime := time.Now()
	ttl := mxJob.Spec.TTLSecondsAfterFinished
	if ttl == nil {
		// do nothing if the cleanup delay is not set
		return nil
	}
	duration := time.Second * time.Duration(*ttl)
	if currentTime.After(mxJob.Status.CompletionTime.Add(duration)) {
		err := tc.deleteMXJobHandler(mxJob)
		if err != nil {
			mxlogger.LoggerForJob(mxJob).Warnf("Cleanup MXJob error: %v.", err)
			return err
		}
		return nil
	}
	key, err := KeyFunc(mxJob)
	if err != nil {
		mxlogger.LoggerForJob(mxJob).Warnf("Couldn't get key for mxjob object: %v", err)
		return err
	}
	tc.WorkQueue.AddRateLimited(key)
	return nil
}

// deleteMXJob deletes the given MXJob.
func (tc *MXController) deleteMXJob(mxJob *mxv1beta1.MXJob) error {
	return tc.mxJobClientSet.KubeflowV1beta1().MXJobs(mxJob.Namespace).Delete(mxJob.Name, &metav1.DeleteOptions{})
}
