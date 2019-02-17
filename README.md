# mxnet-operator: a Kubernetes operator for mxnet jobs

## Overview

MXJob provides a Kubernetes custom resource that makes it easy to
run distributed or non-distributed MXNet jobs on Kubernetes.

Using a Custom Resource Definition (CRD) gives users the ability to create and manage MX Jobs just like builtin K8s resources. For example to create a job

```
kubectl create -f examples/mx_job_dist.yaml 
```

To list jobs

```bash
kubectl get mxjobs

NAME          CREATED AT
example-dist-job   3m
```

### Requirements

kubelet : v1.11.1

kubeadm : v1.11.1

Docker： 
```
Client:
 Version:      17.03.2-ce
 API version:  1.27
 Go version:   go1.6.2
 Git commit:   f5ec1e2
 Built:        Thu Jul  5 23:07:48 2018
 OS/Arch:      linux/amd64

Server:
 Version:      17.03.2-ce
 API version:  1.27 (minimum version 1.12)
 Go version:   go1.6.2
 Git commit:   f5ec1e2
 Built:        Thu Jul  5 23:07:48 2018
 OS/Arch:      linux/amd64
 Experimental: false
```

kubectl :

```
Client Version: version.Info{Major:"1", Minor:"11", GitVersion:"v1.11.1", GitCommit:"b1b29978270dc22fecc592ac55d903350454310a", GitTreeState:"clean", BuildDate:"2018-07-17T18:53:20Z", GoVersion:"go1.10.3", Compiler:"gc", Platform:"linux/amd64"}
Server Version: version.Info{Major:"1", Minor:"10", GitVersion:"v1.10.5", GitCommit:"32ac1c9073b132b8ba18aa830f46b77dcceb0723", GitTreeState:"clean", BuildDate:"2018-06-21T11:34:22Z", GoVersion:"go1.9.3", Compiler:"gc", Platform:"linux/amd64"}
```

kubernetes : branch release-1.11

incubator-mxnet : v1.2.0

## Installing the MXJob CRD and operator on your k8s cluster

### Deploy Kubeflow

mxnet-operator has been contributed to kubeflow , please refer to the [kubeflow installation](https://www.kubeflow.org/docs/started/getting-started/) first .

### Verify that MXNet support is included in your Kubeflow deployment

Check that the MXNet custom resource is installed

```
kubectl get crd
```

The output should include `mxjobs.kubeflow.org`

```
NAME                                           AGE
...
mxjobs.kubeflow.org                       4d
...
```

If it is not included you can add it as follows

```
cd ${KSONNET_APP}
ks pkg install kubeflow/mxnet-job
ks generate mxnet-operator mxnet-operator
ks apply ${ENVIRONMENT} -c mxnet-operator
```

### Creating a job

You create a job by defining a MXJob and then creating it with.

```
kubectl create -f examples/mx_job_dist.yaml 
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


For each replica you define a **template** which is a K8s
[PodTemplateSpec](https://kubernetes.io/docs/api-reference/v1.8/#podtemplatespec-v1-core).
The template allows you to specify the containers, volumes, etc... that
should be created for each replica.

### Using GPUs

Mxnet-operator is now supporting the gpu training .

Please verify your image is available for gpu distributed training .

For example ,

```
command: ["python"]
args: ["/incubator-mxnet/example/image-classification/train_mnist.py","--num-epochs","1","--num-layers","2","--kv-store","dist_device_sync","--gpus","0"]
resources:
  limits:
    nvidia.com/gpu: 1
```

Mxnet-operator will arrange the pod to nodes which satisfied the gpu limit .

## Monitoring your job

To get the status of your job

```bash
kubectl get -o yaml mxjobs $JOB
```   

Here is sample output for an example job

```yaml
apiVersion: kubeflow.org/v1alpha1
kind: MXJob
metadata:
  clusterName: ""
  creationTimestamp: 2018-08-10T07:13:39Z
  generation: 1
  name: example-dist-job
  namespace: default
  resourceVersion: "491499"
  selfLink: /apis/kubeflow.org/v1alpha1/namespaces/default/mxjobs/example-dist-job
  uid: e800b1ed-9c6c-11e8-962f-704d7b2c0a63
spec:
  RuntimeId: aycw
  jobMode: dist
  mxImage: mxjob/mxnet:gpu
  replicaSpecs:
  - PsRootPort: 9000
    mxReplicaType: SCHEDULER
    replicas: 1
    template:
      metadata:
        creationTimestamp: null
      spec:
        containers:
        - args:
          - train_mnist.py
          command:
          - python
          image: mxjob/mxnet:gpu
          name: mxnet
          resources: {}
          workingDir: /incubator-mxnet/example/image-classification
        restartPolicy: OnFailure
  - PsRootPort: 9091
    mxReplicaType: SERVER
    replicas: 1
    template:
      metadata:
        creationTimestamp: null
      spec:
        containers:
        - args:
          - train_mnist.py
          command:
          - python
          image: mxjob/mxnet:gpu
          name: mxnet
          resources: {}
          workingDir: /incubator-mxnet/example/image-classification
        restartPolicy: OnFailure
  - PsRootPort: 9091
    mxReplicaType: WORKER
    replicas: 1
    template:
      metadata:
        creationTimestamp: null
      spec:
        containers:
        - args:
          - train_mnist.py
          - --num-epochs=10
          - --num-layers=2
          - --kv-store=dist_device_sync
          command:
          - python
          image: mxjob/mxnet:gpu
          name: mxnet
          resources: {}
          workingDir: /incubator-mxnet/example/image-classification
        restartPolicy: OnFailure
  terminationPolicy:
    chief:
      replicaIndex: 0
      replicaName: SCHEDULER
status:
  phase: Running
  reason: ""
  replicaStatuses:
  - ReplicasStates:
      Running: 1
    mx_replica_type: SCHEDULER
    state: Running
  - ReplicasStates:
      Running: 1
    mx_replica_type: SERVER
    state: Running
  - ReplicasStates:
      Running: 1
    mx_replica_type: WORKER
    state: Running
  state: Running


```

The first thing to note is the **RuntimeId**. This is a random unique
string which is used to give names to all the K8s resouces
(e.g Job controllers & services) that are created by the MXJob.

As with other K8s resources status provides information about the state
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

##Tvm auto tuning
If your mxnet operator version is kubeflow.org/v1alpha2, you can use its auto tuning framework. 
You can find example in examples/mxnet-operator.v2/tune/mx_job_tune_gpu.yaml.
But there is some preparatory work need to be finished first.


To let tvm tune your network, you should create a docker image which has tvm module. You can reference the docker file 
in examples/mxnet-operator.v2/tune which install mxnet and tvm automatically. If you want to know more information about
tvm installation, please reference https://docs.tvm.ai/tutorials/


Next, you need a auto-tuning script to specify which network will be tuned and set the auto-tuning parameters, 
You can reference examples/mxnet-operator.v2/tune/auto-tuning.py to get some help, this script define a mxnet neetwork 
and then run the auto-tuning module. For more details, please see 
https://docs.tvm.ai/tutorials/autotvm/tune_relay_mobile_gpu.html#sphx-glr-tutorials-autotvm-tune-relay-mobile-gpu-py


Last, you need a startup script to start the auto tuning. In fact, mxnet-operator will set all the parameters 
as environment variables and the startup script need to reed these variable and then transmit them to auto-tuning 
script. You can reference examples/mxnet-operator.v2/tune/start-job.py


If you are not familiar to tvm, a good solution is replace the network defined in 
examples/mxnet-operator.v2/tune/auto-tuning.py to yours. Tuning result will be save in a log file like resnet-18.log in 
the example we gave.

```yaml
apiVersion: "kubeflow.org/v1alpha2"
kind: "MXJob"
metadata:
  name: "auto-tuning-job"
spec:
  jobMode: MXTune
  mxReplicaSpecs:
    TunerTracker:
      replicas: 1
      restartPolicy: Never
      template:
        spec:
          containers:
          - name: mxnet
            image: mxjob/auto-tuning:gpu
            command: ["python3"]
            args: ["/home/start-job.py"]
    TunerServer:
      label: 2080ti
      replicas: 1
      restartPolicy: Never
      template:
        spec:
          containers:
          - name: mxnet
            image: mxjob/auto-tuning:gpu
            command: ["python3"]
            args: ["/home/start-job.py"]
            resources:
              limits:
                nvidia.com/gpu: 1
    Tuner:
      replicas: 1
      restartPolicy: Never
      template:
        spec:
          containers:
          - name: mxnet
            image: mxjob/auto-tuning:gpu
            command: ["python3"]
            args: ["/home/start-job.py"]
```


## Contributing

Please refer to the [developer_guide](https://github.com/kubeflow/tf-operator/blob/master/developer_guide.md)

## Community

This is a part of Kubeflow, so please see [readme in kubeflow/kubeflow](https://github.com/kubeflow/kubeflow#get-involved) to get in touch with the community.
