FROM golang:1.13.8

RUN mkdir -p /opt/kubeflow
COPY mxnet-operator.v1beta1 /opt/kubeflow
COPY mxnet-operator.v1 /opt/kubeflow

RUN chmod a+x /opt/kubeflow/mxnet-operator.v1beta1
RUN chmod a+x /opt/kubeflow/mxnet-operator.v1

CMD ["/opt/kubeflow/mxnet-operator.v1"]
