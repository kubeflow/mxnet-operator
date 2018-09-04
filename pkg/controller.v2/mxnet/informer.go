package mxnet

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1unstructured "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	restclientset "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	mxv1alpha2 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1alpha2"
	"github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/validation"
	mxjobinformers "github.com/kubeflow/mxnet-operator/pkg/client/informers/externalversions"
	mxjobinformersv1alpha2 "github.com/kubeflow/mxnet-operator/pkg/client/informers/externalversions/kubeflow/v1alpha2"
	mxlogger "github.com/kubeflow/tf-operator/pkg/logger"
	"github.com/kubeflow/mxnet-operator/pkg/util/unstructured"
)

const (
	resyncPeriod     = 30 * time.Second
	failedMarshalMsg = "Failed to marshal the object to MXJob: %v"
)

var (
	errGetFromKey    = fmt.Errorf("Failed to get MXJob from key")
	errNotExists     = fmt.Errorf("The object is not found")
	errFailedMarshal = fmt.Errorf("Failed to marshal the object to MXJob")
)

func NewUnstructuredMXJobInformer(restConfig *restclientset.Config, namespace string) mxjobinformersv1alpha2.MXJobInformer {
	dynClientPool := dynamic.NewDynamicClientPool(restConfig)
	dclient, err := dynClientPool.ClientForGroupVersionKind(mxv1alpha2.SchemeGroupVersionKind)
	if err != nil {
		panic(err)
	}
	resource := &metav1.APIResource{
		Name:         mxv1alpha2.Plural,
		SingularName: mxv1alpha2.Singular,
		Namespaced:   true,
		Group:        mxv1alpha2.GroupName,
		Version:      mxv1alpha2.GroupVersion,
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
func (tc *MXController) NewMXJobInformer(mxJobInformerFactory mxjobinformers.SharedInformerFactory) mxjobinformersv1alpha2.MXJobInformer {
	return mxJobInformerFactory.Kubeflow().V1alpha2().MXJobs()
}

func (tc *MXController) getMXJobFromName(namespace, name string) (*mxv1alpha2.MXJob, error) { 	
	key := fmt.Sprintf("%s/%s", namespace, name)
	return tc.getMXJobFromKey(key)
}

func (tc *MXController) getMXJobFromKey(key string) (*mxv1alpha2.MXJob, error) {
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

func mxJobFromUnstructured(obj interface{}) (*mxv1alpha2.MXJob, error) {
	// Check if the spec is valid.
	un, ok := obj.(*metav1unstructured.Unstructured)
	if !ok {
		log.Errorf("The object in index is not an unstructured; %+v", obj)
		return nil, errGetFromKey
	}
	var mxjob mxv1alpha2.MXJob
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(un.Object, &mxjob)
	logger := mxlogger.LoggerForUnstructured(un, mxv1alpha2.Kind)
	if err != nil {
		logger.Errorf(failedMarshalMsg, err)
		return nil, errFailedMarshal
	}
	// This is a simple validation for MXJob to close
	// TODO(gaocegege): Add more validation here.
	err = validation.ValidateAlphaTwoMXJobSpec(&mxjob.Spec)
	if err != nil {
		logger.Errorf(failedMarshalMsg, err)
		return nil, errFailedMarshal
	}
	return &mxjob, nil
}

func unstructuredFromMXJob(obj interface{}, mxJob *mxv1alpha2.MXJob) error {
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
