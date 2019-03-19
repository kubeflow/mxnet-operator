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

package mxnet

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	mxv1beta1 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1beta1"
	"github.com/kubeflow/mxnet-operator/pkg/common/util/v1beta1/testutil"
)

func TestGenOwnerReference(t *testing.T) {
	testName := "test-mxjob"
	testUID := types.UID("test-UID")
	mxJob := &mxv1beta1.MXJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: testName,
			UID:  testUID,
		},
	}

	ref := testutil.GenOwnerReference(mxJob)
	if ref.UID != testUID {
		t.Errorf("Expected UID %s, got %s", testUID, ref.UID)
	}
	if ref.Name != testName {
		t.Errorf("Expected Name %s, got %s", testName, ref.Name)
	}
	if ref.APIVersion != mxv1beta1.SchemeGroupVersion.String() {
		t.Errorf("Expected APIVersion %s, got %s", mxv1beta1.SchemeGroupVersion.String(), ref.APIVersion)
	}
}

func TestGenLabels(t *testing.T) {
	testKey := "test/key"
	expctedKey := "test-key"

	labels := testutil.GenLabels(testKey)

	if labels[labelMXJobName] != expctedKey {
		t.Errorf("Expected %s %s, got %s", labelMXJobName, expctedKey, labels[labelMXJobName])
	}
	if labels[labelGroupName] != mxv1beta1.GroupName {
		t.Errorf("Expected %s %s, got %s", labelGroupName, mxv1beta1.GroupName, labels[labelGroupName])
	}
}

func TestConvertMXJobToUnstructured(t *testing.T) {
	testName := "test-mxjob"
	testUID := types.UID("test-UID")
	mxJob := &mxv1beta1.MXJob{
		TypeMeta: metav1.TypeMeta{
			Kind: mxv1beta1.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: testName,
			UID:  testUID,
		},
	}

	_, err := testutil.ConvertMXJobToUnstructured(mxJob)
	if err != nil {
		t.Errorf("Expected error to be nil while got %v", err)
	}
}
