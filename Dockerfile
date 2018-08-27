FROM golang:1.8.2

RUN mkdir -p /opt/mlkube
RUN mkdir -p /opt/mlkube/test
COPY mxnet-operator /opt/mlkube
RUN chmod a+x /opt/mlkube/mxnet-operator

CMD ["/opt/mlkube/mxnet-operator", "--alsologtostderr", "-v=1"]
