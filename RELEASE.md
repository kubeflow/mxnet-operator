# MXNet Operator Releases

## Release v1.0.0

* Remove Travis buddy webhook ([#77](https://github.com/kubeflow/mxnet-operator/pull/77), [@terrytangyuan](https://github.com/terrytangyuan))
* Fix the reconcile flow ([#74](https://github.com/kubeflow/mxnet-operator/pull/74), [@ChanYiLin](https://github.com/ChanYiLin))
* Remove go.mod suffix when grabbing code-generator version ([#73](https://github.com/kubeflow/mxnet-operator/pull/73), [@terrytangyuan](https://github.com/terrytangyuan))
* Add Kustomize package deployable on its own ([#71](https://github.com/kubeflow/mxnet-operator/pull/71), [@Jeffwan](https://github.com/Jeffwan)
* Create separate clusterrole for mxnet-operator ([#70](https://github.com/kubeflow/mxnet-operator/pull/70), [@Jeffwan](https://github.com/Jeffwan))
* Fix Python script name and add links ([#69](https://github.com/kubeflow/mxnet-operator/pull/69), [@terrytangyuan](https://github.com/terrytangyuan))
* Add mxnet-operator v1 examples ([#68](https://github.com/kubeflow/mxnet-operator/pull/68), [@Jeffwan](https://github.com/Jeffwan))
* Add initial list of adopters ([#63](https://github.com/kubeflow/mxnet-operator/pull/63), [@terrytangyuan](https://github.com/terrytangyuan))
* Enhancements on README.md ([#61](https://github.com/kubeflow/mxnet-operator/pull/61), [@terrytangyuan](https://github.com/terrytangyuan))
* Update Contributing docs for recent go changes ([#58](https://github.com/kubeflow/mxnet-operator/pull/58), [@Jeffwan](https://github.com/Jeffwan))
* Remove vendor directory ([#57](https://github.com/kubeflow/mxnet-operator/pull/57), [@Jeffwan](https://github.com/Jeffwan))
* Upgrade Golang version to 1.13.8 and Kubernetes version to 1.15.9 ([#53](https://github.com/kubeflow/mxnet-operator/pull/53), [@Jeffwan](https://github.com/Jeffwan))
* Deprecate gometalinter and use golangcli-lint instead ([#56](https://github.com/kubeflow/mxnet-operator/pull/56), [@Jeffwan](https://github.com/Jeffwan))

## Release v0.7.0

* Implementation of mxnet operator API v1 ([#39](https://github.com/kubeflow/mxnet-operator/pull/39), [@wackxu](https://github.com/wackxu))
* Implement ActiveDeadlineSeconds and BackoffLimit ([#40](https://github.com/kubeflow/mxnet-operator/pull/40), [@wackxu](https://github.com/wackxu))
* Sync with tf-operator ([#41](https://github.com/kubeflow/mxnet-operator/pull/41), [@wackxu](https://github.com/wackxu))
* Add wackxu to OWNERS ([#42](https://github.com/kubeflow/mxnet-operator/pull/42), [@wackxu](https://github.com/wackxu))
* Add uuid to id for leader election ([#43](https://github.com/kubeflow/mxnet-operator/pull/43), [@fisherxu](https://github.com/fisherxu))
* Skip condition update when succeeded and bump tf-operator to v0.5.3 version ([#44](https://github.com/kubeflow/mxnet-operator/pull/44), [@wackxu](https://github.com/wackxu))
* Fix bug for check PodPending ([#45](https://github.com/kubeflow/mxnet-operator/pull/45), [@wackxu](https://github.com/wackxu))
* Fix wrong api version when delete mxjob ([#46](https://github.com/kubeflow/mxnet-operator/pull/46), [@wackxu](https://github.com/wackxu))
* Renaming labels to consistent format ([#47](https://github.com/kubeflow/mxnet-operator/pull/47), [@wackxu](https://github.com/wackxu))
* Set annotation automatically when EnableGangScheduling is set to true ([#48](https://github.com/kubeflow/mxnet-operator/pull/48), [@wackxu](https://github.com/wackxu))
* Add kubeconfig flag ([#49](https://github.com/kubeflow/mxnet-operator/pull/49), [@yeya24](https://github.com/yeya24))

## Release v0.1.0

Initial release of the MXNet Operator.
