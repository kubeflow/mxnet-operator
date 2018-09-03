FROM golang:1.8.2

RUN mkdir -p /opt/kubeflow
RUN mkdir -p /opt/kubeflow/test
COPY mxnet-operator /opt/kubeflow
RUN chmod a+x /opt/kubeflow/mxnet-operator

CMD ["/opt/kubeflow/mxnet-operator", "--alsologtostderr", "-v=1"]
