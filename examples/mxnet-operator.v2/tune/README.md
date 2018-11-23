mx_job_tune_gpu.yaml will pull sample image and run it.

In the sample image, tvm and mxnet are pre-installed, you can see Dockerfile to get some information.

There two customed scripts in the sample image, startMXJob.py and auto-tuning.py.
startMXJob.py is a script tell you how to read the environment variable MX_CONFIG.
auto-tuning.py is a sample script of autotvm, which will tune a resnet-18 network.