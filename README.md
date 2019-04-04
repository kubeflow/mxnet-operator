# mxnet-operator: a Kubernetes operator for mxnet jobs

## Overview

MXJob provides a Kubernetes custom resource that makes it easy to
run distributed or non-distributed MXNet jobs (training and tuning) on Kubernetes. 
Using a Custom Resource Definition (CRD) gives users the ability to create 
and manage MX Jobs just like builtin K8S resources. 

### Prerequisites

- Kubernetes >= 1.8
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl)
- MXNet >= v1.2.0

## Installing the MXJob CRD and operator on your k8s cluster

### Deploy Kubeflow

Please refer to the [kubeflow installation](https://www.kubeflow.org/docs/started/getting-started/).

### Verify that MXNet support is included in your Kubeflow deployment

Check that the MXNet custom resource is installed

```
kubectl get crd
```

The output should include `mxjobs.kubeflow.org`

```
NAME                                           AGE
...
mxjobs.kubeflow.org                            4d
...
```

If it is not included you can add it as follows

```
cd ${KSONNET_APP}
ks pkg install kubeflow/mxnet-job
ks generate mxnet-operator mxnet-operator
ks apply default -c mxnet-operator
```

As an alternative solution, you can deploy mxnet-operator bypass ksonnect

```
kubectl create -f manifests/crd-v1beta1.yaml 
kubectl create -f manifests/rbac.yaml 
kubectl create -f manifests/deployment.yaml
```

### Creating a MXNet training job

You create a training job by defining a MXJob with MXTrain mode and then creating it with.

```
kubectl create -f examples/v1beta1/train/mx_job_dist_gpu.yaml
```

Each replicaSpec defines a set of MXNet processes.
The mxReplicaType defines the semantics for the set of processes.
The semantics are as follows

**scheduler**
  * A job must have 1 and only 1 scheduler
  * The pod must contain a container named mxnet
  * The overall status of the MXJob is determined by the exit code of the
    mxnet container
      * 0 = success
      * 1 || 2 || 126 || 127 || 128 || 139 = permanent errors:
          * 1: general errors
          * 2: misuse of shell builtins
          * 126: command invoked cannot execute
          * 127: command not found
          * 128: invalid argument to exit
          * 139: container terminated by SIGSEGV(Invalid memory reference)
      * 130 || 137 || 143 = retryable error for unexpected system signals:
          * 130: container terminated by Control-C
          * 137: container received a SIGKILL
          * 143: container received a SIGTERM
      * 138 = reserved in tf-operator for user specified retryable errors
      * others = undefined and no guarantee

**worker**
  * A job can have 0 to N workers
  * The pod must contain a container named mxnet
  * Workers are automatically restarted if they exit

**server**
  * A job can have 0 to N servers
  * parameter servers are automatically restarted if they exit


For each replica you define a **template** which is a K8S
[PodTemplateSpec](https://kubernetes.io/docs/api-reference/v1.8/#podtemplatespec-v1-core).
The template allows you to specify the containers, volumes, etc... that
should be created for each replica.

### Creating a TVM tuning job (AutoTVM)

[TVM](https://docs.tvm.ai/tutorials/) is a end to end deep learning compiler stack, you can easily run AutoTVM with mxnet-operator. 
You can create a auto tuning job by define a type of MXTune job and then creating it with

```
kubectl create -f examples/v1beta1/tune/mx_job_tune_gpu.yaml
```

Before you use the auto-tuning example, there is some preparatory work need to be finished in advance. To let TVM tune your network, you should create a docker image which has TVM module. Then, you need a auto-tuning script to specify which network will be tuned and set the auto-tuning parameters, For more details, please see https://docs.tvm.ai/tutorials/autotvm/tune_relay_mobile_gpu.html#sphx-glr-tutorials-autotvm-tune-relay-mobile-gpu-py. Finally, you need a startup script to start the auto-tuning program. In fact, mxnet-operator will set all the parameters as environment variables and the startup script need to reed these variable and then transmit them to auto-tuning script. We provide an example under examples/v1beta1/tune/, tuning result will be saved in a log file like resnet-18.log in the example we gave. You can refer it for details.

### Using GPUs

Mxnet-operator is now supporting the GPU training .

Please verify your image is available for GPU distributed training .

For example ,

```
command: ["python"]
args: ["/incubator-mxnet/example/image-classification/train_mnist.py","--num-epochs","1","--num-layers","2","--kv-store","dist_device_sync","--gpus","0"]
resources:
  limits:
    nvidia.com/gpu: 1
```

Mxnet-operator will arrange the pod to nodes which satisfied the GPU limit.

### Monitoring your MXNet job

To get the status of your job

```bash
kubectl get -o yaml mxjobs $JOB
```   

Here is sample output for an example job

```yaml
apiVersion: kubeflow.org/v1beta1
kind: MXJob
metadata:
  creationTimestamp: 2019-03-19T09:24:27Z
  generation: 1
  name: mxnet-job
  namespace: default
  resourceVersion: "3681685"
  selfLink: /apis/kubeflow.org/v1beta1/namespaces/default/mxjobs/mxnet-job
  uid: cb11013b-4a28-11e9-b7f4-704d7bb59f71
spec:
  cleanPodPolicy: All
  jobMode: MXTrain
  mxReplicaSpecs:
    Scheduler:
      replicas: 1
      restartPolicy: Never
      template:
        metadata:
          creationTimestamp: null
        spec:
          containers:
          - image: mxjob/mxnet:gpu
            name: mxnet
            ports:
            - containerPort: 9091
              name: mxjob-port
            resources: {}
    Server:
      replicas: 1
      restartPolicy: Never
      template:
        metadata:
          creationTimestamp: null
        spec:
          containers:
          - image: mxjob/mxnet:gpu
            name: mxnet
            ports:
            - containerPort: 9091
              name: mxjob-port
            resources: {}
    Worker:
      replicas: 1
      restartPolicy: Never
      template:
        metadata:
          creationTimestamp: null
        spec:
          containers:
          - args:
            - /incubator-mxnet/example/image-classification/train_mnist.py
            - --num-epochs
            - "10"
            - --num-layers
            - "2"
            - --kv-store
            - dist_device_sync
            - --gpus
            - "0"
            command:
            - python
            image: mxjob/mxnet:gpu
            name: mxnet
            ports:
            - containerPort: 9091
              name: mxjob-port
            resources:
              limits:
                nvidia.com/gpu: "1"
status:
  completionTime: 2019-03-19T09:25:11Z
  conditions:
  - lastTransitionTime: 2019-03-19T09:24:27Z
    lastUpdateTime: 2019-03-19T09:24:27Z
    message: MXJob mxnet-job is created.
    reason: MXJobCreated
    status: "True"
    type: Created
  - lastTransitionTime: 2019-03-19T09:24:27Z
    lastUpdateTime: 2019-03-19T09:24:29Z
    message: MXJob mxnet-job is running.
    reason: MXJobRunning
    status: "False"
    type: Running
  - lastTransitionTime: 2019-03-19T09:24:27Z
    lastUpdateTime: 2019-03-19T09:25:11Z
    message: MXJob mxnet-job is successfully completed.
    reason: MXJobSucceeded
    status: "True"
    type: Succeeded
  mxReplicaStatuses:
    Scheduler: {}
    Server: {}
    Worker: {}
  startTime: 2019-03-19T09:24:29Z
```

The first thing to note is the **RuntimeId**. This is a random unique
string which is used to give names to all the K8s resouces
(e.g Job controllers & services) that are created by the MXJob.

As with other K8S resources status provides information about the state
of the resource.

**phase** - Indicates the phase of a job and will be one of
 - Creating
 - Running
 - CleanUp
 - Failed
 - Done

**state** - Provides the overall status of the job and will be one of
  - Running
  - Succeeded
  - Failed

For each replica type in the job, there will be a ReplicaStatus that
provides the number of replicas of that type in each state.

For each replica type, the job creates a set of K8s
[Job Controllers](https://kubernetes.io/docs/concepts/workloads/controllers/jobs-run-to-completion/)
named

```
${REPLICA-TYPE}-${RUNTIME_ID}-${INDEX}
```

For example, if you have 2 servers and runtime id 76n0 MXJob
will create the jobs

```
server-76no-0
server-76no-1
```

## Contributing

Please refer to the [developer_guide](https://github.com/kubeflow/tf-operator/blob/master/developer_guide.md)

## Community

This is a part of Kubeflow, so please see [readme in kubeflow/kubeflow](https://github.com/kubeflow/kubeflow#get-involved) to get in touch with the community.
