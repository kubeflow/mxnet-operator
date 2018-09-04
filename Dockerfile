FROM golang:1.8.2

RUN mkdir -p /opt/kubeflow
COPY mxnet-operator /opt/kubeflow
RUN chmod a+x /opt/kubeflow/mxnet-operator
COPY mxnet-operator.v2 /opt/kubeflow
RUN chmod a+x /opt/kubeflow/mxnet-operator.v2

CMD ["/opt/kubeflow/mxnet-operator", "--alsologtostderr", "-v=1"]
