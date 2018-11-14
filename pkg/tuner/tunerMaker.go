package tuner

import (
	"fmt"

	mxv1alpha2 "github.com/kubeflow/mxnet-operator/pkg/apis/mxnet/v1alpha2"
	mxlogger "github.com/kubeflow/tf-operator/pkg/logger"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func AutoExtension(mxjob *mxv1alpha2.MXJob) error {

	logger := mxlogger.LoggerForJob(mxjob)
	logger.Infof("AutoExtension: Tune Mode, please make sure tvm has been installed")

	if mxjob.Spec.JobMode == mxv1alpha2.MXTune {
		tunerSpec, ok := mxjob.Spec.MXReplicaSpecs[mxv1alpha2.MXReplicaTypeTuner]
		if !ok {
			return fmt.Errorf("AutoExtension: Invalid JobMode")
		}

		trackerSpec, ok := mxjob.Spec.MXReplicaSpecs[mxv1alpha2.MXReplicaTypeTunerTracker]
		if !ok {
			logger.Infof("AutoExtension: Tuner Tracker hasn't been set, so autoExtension well add new Tracker")
			if len(tunerSpec.Template.Spec.Containers) != 1 {
				return fmt.Errorf("AutoExtension: Unsupport container number (must be 1), please try to define a custom tracker.")
			}

			trackerSpec = tunerSpec.DeepCopy()
			*trackerSpec.Replicas = 1
			trackerSpec.Template.Spec.Containers[0].Command = []string{"python3"}
			// TODO: when tvm finds provided port can't be bound, it will search next usable port, which is potential bug
			trackerSpec.Template.Spec.Containers[0].Args = []string{
				"-m",
				"tvm.exec.rpc_tracker",
				"--host=0.0.0.0",
				fmt.Sprintf("--port=%d", mxv1alpha2.DefaultPort),
			}

			mxjob.Spec.MXReplicaSpecs[mxv1alpha2.MXReplicaTypeTunerTracker] = trackerSpec
		} else {
			logger.Infof("AutoExtension: Tuner Tracker has been set, replicas = %d", *trackerSpec.Replicas)
		}

		serverSpec, ok := mxjob.Spec.MXReplicaSpecs[mxv1alpha2.MXReplicaTypeTunerServer]
		if !ok {
			logger.Infof("AutoExtension: Tuner Server hasn't been set, so autoExtension well add new Server")
			if len(tunerSpec.Template.Spec.Containers) != 1 {
				return fmt.Errorf("AutoExtension: Unsupport container number (must be 1), please try to define a custom server.")
			}

			serverSpec = tunerSpec.DeepCopy()
			*serverSpec.Replicas = *tunerSpec.TunerSpec.TunerServerReplicas

			if serverSpec.Template.Spec.Containers[0].Resources.Limits == nil {
				serverSpec.Template.Spec.Containers[0].Resources.Limits = make(v1.ResourceList)
			}
			if tunerSpec.TunerSpec.TunerServerGPU {
				serverSpec.Template.Spec.Containers[0].Resources.Limits["nvidia.com/gpu"] = *resource.NewQuantity(1, resource.DecimalSI)
			} else {
				serverSpec.Template.Spec.Containers[0].Resources.Limits["nvidia.com/gpu"] = *resource.NewQuantity(0, resource.DecimalSI)
			}

			tunerKey, err := GetTunerServerKey(mxjob)
			if err != nil {
				return err
			}

			serverSpec.Template.Spec.Containers[0].Command = []string{"/bin/sh"}
			serverSpec.Template.Spec.Containers[0].Args = []string{
				"-c",
				fmt.Sprintf("sleep 5; python3 -m tvm.exec.rpc_server --tracker=$DMLC_TUNER_TRACKER_URI:$DMLC_TUNER_TRACKER_PORT --key=%s", tunerKey),
			}

			mxjob.Spec.MXReplicaSpecs[mxv1alpha2.MXReplicaTypeTunerServer] = serverSpec
		} else {
			logger.Infof("AutoExtension: Tuner Server has been set, Tuner Server's replica will be %d. The tunerServerReplicas in Tuner will be ignored", *serverSpec.Replicas)
		}
	}
	return nil
}

func GetTunerServerKey(mxjob *mxv1alpha2.MXJob) (string, error) {
	tunerSpec, ok := mxjob.Spec.MXReplicaSpecs[mxv1alpha2.MXReplicaTypeTuner]
	if !ok {
		return "", fmt.Errorf("GetTunerServerKey: Invalid JobMode")
	}
	var tunerKey string
	if tunerSpec.TunerSpec.TunerServerKey != "" {
		tunerKey = tunerSpec.TunerSpec.TunerServerKey
	} else {
		tunerKey = mxjob.Name + "-tunerServerKey"
	}
	return tunerKey, nil
}
