package mxnet

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	metav1unstructured "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	restclientset "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	mxlogger "github.com/kubeflow/common/pkg/util"
	mxv1 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1"
	"github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/validation"
	mxjobinformers "github.com/kubeflow/mxnet-operator/pkg/client/informers/externalversions"
	mxjobinformersv1 "github.com/kubeflow/mxnet-operator/pkg/client/informers/externalversions/mxnet/v1"
	"github.com/kubeflow/mxnet-operator/pkg/common/util/v1/unstructured"
)

const (
	resyncPeriod     = 30 * time.Second
	failedMarshalMsg = "Failed to marshal the object to MXJob: %v"
)

var (
	errGetFromKey    = fmt.Errorf("failed to get MXJob from key")
	errNotExists     = fmt.Errorf("the object is not found")
	errFailedMarshal = fmt.Errorf("failed to marshal the object to MXJob")
	errWrongJobMode  = fmt.Errorf("failed to inspect jobMode, maybe mxReplicaSpecs has a member which is not belong to this jobMode or misses one")
)

func NewUnstructuredMXJobInformer(restConfig *restclientset.Config, namespace string) mxjobinformersv1.MXJobInformer {
	dclient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		panic(err)
	}
	resource := schema.GroupVersionResource{
		Group:    mxv1.GroupName,
		Version:  mxv1.GroupVersion,
		Resource: mxv1.Plural,
	}
	informer := unstructured.NewMXJobInformer(
		resource,
		dclient,
		namespace,
		resyncPeriod,
		cache.Indexers{},
	)
	return informer
}

// NewMXJobInformer returns MXJobInformer from the given factory.
func (tc *MXController) NewMXJobInformer(mxJobInformerFactory mxjobinformers.SharedInformerFactory) mxjobinformersv1.MXJobInformer {
	return mxJobInformerFactory.Kubeflow().V1().MXJobs()
}

func (tc *MXController) getMXJobFromName(namespace, name string) (*mxv1.MXJob, error) {
	key := fmt.Sprintf("%s/%s", namespace, name)
	return tc.getMXJobFromKey(key)
}

func (tc *MXController) getMXJobFromKey(key string) (*mxv1.MXJob, error) {
	// Check if the key exists.
	obj, exists, err := tc.mxJobInformer.GetIndexer().GetByKey(key)
	logger := mxlogger.LoggerForKey(key)
	if err != nil {
		logger.Errorf("Failed to get MXJob '%s' from informer index: %+v", key, err)
		return nil, errGetFromKey
	}
	if !exists {
		// This happens after a mxjob was deleted, but the work queue still had an entry for it.
		return nil, errNotExists
	}

	mxjob, err := mxJobFromUnstructured(obj)
	if err != nil {
		return nil, err
	}
	return mxjob, nil
}

func mxJobFromUnstructured(obj interface{}) (*mxv1.MXJob, error) {
	// Check if the spec is valid.
	un, ok := obj.(*metav1unstructured.Unstructured)
	if !ok {
		log.Errorf("The object in index is not an unstructured; %+v", obj)
		return nil, errGetFromKey
	}
	var mxjob mxv1.MXJob
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(un.Object, &mxjob)
	logger := mxlogger.LoggerForUnstructured(un, mxv1.Kind)
	if err != nil {
		logger.Errorf(failedMarshalMsg, err)
		return nil, errFailedMarshal
	}
	// This is a simple validation for MXJob to close
	// TODO(gaocegege): Add more validation here.
	err = validation.ValidateV1MXJobSpec(&mxjob.Spec)
	if err != nil {
		logger.Errorf(failedMarshalMsg, err)
		return nil, errFailedMarshal
	}
	return &mxjob, nil
}

func unstructuredFromMXJob(obj interface{}, mxJob *mxv1.MXJob) error {
	un, ok := obj.(*metav1unstructured.Unstructured)
	logger := mxlogger.LoggerForJob(mxJob)
	if !ok {
		logger.Warn("The object in index isn't type Unstructured")
		return errGetFromKey
	}

	var err error
	un.Object, err = runtime.DefaultUnstructuredConverter.ToUnstructured(mxJob)
	if err != nil {
		logger.Error("The MXJob convert failed")
		return err
	}
	return nil

}
