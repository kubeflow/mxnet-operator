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

package testutil

import (
	"encoding/json"
	"strings"
	"testing"

	mxv1beta1 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1beta1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"
)

const (
	LabelGroupName = "group_name"
	LabelMXJobName = "mxnet_job_name"
)

var (
	// KeyFunc is the short name to DeletionHandlingMetaNamespaceKeyFunc.
	// IndexerInformer uses a delta queue, therefore for deletes we have to use this
	// key function but it should be just fine for non delete events.
	KeyFunc   = cache.DeletionHandlingMetaNamespaceKeyFunc
	GroupName = mxv1beta1.GroupName
)

func GenLabels(jobName string) map[string]string {
	return map[string]string{
		LabelGroupName: GroupName,
		LabelMXJobName: strings.Replace(jobName, "/", "-", -1),
	}
}

func GenOwnerReference(mxjob *mxv1beta1.MXJob) *metav1.OwnerReference {
	boolPtr := func(b bool) *bool { return &b }
	controllerRef := &metav1.OwnerReference{
		APIVersion:         mxv1beta1.SchemeGroupVersion.String(),
		Kind:               mxv1beta1.Kind,
		Name:               mxjob.Name,
		UID:                mxjob.UID,
		BlockOwnerDeletion: boolPtr(true),
		Controller:         boolPtr(true),
	}

	return controllerRef
}

// ConvertMXJobToUnstructured uses JSON to convert MXJob to Unstructured.
func ConvertMXJobToUnstructured(mxJob *mxv1beta1.MXJob) (*unstructured.Unstructured, error) {
	var unstructured unstructured.Unstructured
	b, err := json.Marshal(mxJob)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(b, &unstructured); err != nil {
		return nil, err
	}
	return &unstructured, nil
}

func GetKey(mxJob *mxv1beta1.MXJob, t *testing.T) string {
	key, err := KeyFunc(mxJob)
	if err != nil {
		t.Errorf("Unexpected error getting key for job %v: %v", mxJob.Name, err)
		return ""
	}
	return key
}

func CheckCondition(mxJob *mxv1beta1.MXJob, condition mxv1beta1.MXJobConditionType, reason string) bool {
	for _, v := range mxJob.Status.Conditions {
		if v.Type == condition && v.Status == v1.ConditionTrue && v.Reason == reason {
			return true
		}
	}
	return false
}
