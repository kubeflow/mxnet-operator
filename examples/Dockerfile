FROM mxnet/python:gpu

RUN apt-get update \
    && apt-get install -y git \
    && apt-get install -y build-essential git \
    && apt-get install -y libopenblas-dev liblapack-dev \
    && apt-get install -y libopencv-dev 

RUN rm -rf /mxnet \
    && git clone --recursive https://github.com/apache/incubator-mxnet -b v1.2.0

RUN cd /incubator-mxnet \
    && make clean \
    && make -j $(nproc) USE_OPENCV=1 USE_BLAS=openblas USE_DIST_KVSTORE=1 USE_CUDA=1 USE_CUDA_PATH=/usr/local/cuda USE_CUDNN=1

RUN apt-get install -y python-dev python-setuptools python-pip libgfortran3

RUN cd /incubator-mxnet/python \
    && pip install -e .

RUN rm /incubator-mxnet/example/image-classification/train_mnist.py

COPY train_mnist.py /incubator-mxnet/example/image-classification/

RUN apt-get install -y dnsutils 

ENTRYPOINT ["python", "/incubator-mxnet/example/image-classification/train_mnist.py"]

