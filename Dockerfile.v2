FROM golang:1.8.2

RUN mkdir -p /opt/mlkube
RUN mkdir -p /opt/mlkube/test
COPY mxnet-operator.v2 /opt/mlkube
RUN chmod a+x /opt/mlkube/mxnet-operator.v2

CMD ["/opt/mlkube/mxnet-operator.v2", "--alsologtostderr", "-v=1"]
