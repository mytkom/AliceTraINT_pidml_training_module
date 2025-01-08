FROM ubuntu:22.04

RUN apt update -y && \
    DEBIAN_FRONTEND=noninteractive TZ=Etc/UTC apt install -y \
    curl libcurl4-gnutls-dev build-essential gfortran libmysqlclient-dev xorg-dev \
    libglu1-mesa-dev libfftw3-dev libxml2-dev git unzip autoconf automake autopoint \
    texinfo gettext libtool libtool-bin pkg-config bison flex libperl-dev libbz2-dev \
    swig liblzma-dev libnanomsg-dev rsync lsb-release environment-modules libglfw3-dev \
    libtbb-dev python3-dev python3-venv python3-pip graphviz libncurses-dev \
    software-properties-common gtk-doc-tools sudo bc ca-certificates cmake wget \
    make g++ openssl parallel coreutils && \
    apt clean && \
    add-apt-repository ppa:alisw/ppa && \
    apt update -y && \
    apt install -y python3-alibuild && \
    apt clean

# Versioning and user setup
RUN echo v20250103 > /etc/aliceimageversion && \
    groupadd --gid 1000 alice && \
    useradd --uid 1000 --gid 1000 alice && \
    mkdir -p /wd && chown -R alice:alice /wd

# Copy and configure grid certificate
WORKDIR /wd
USER root
COPY gridCertificate.p12 .
RUN rm -rf /home/alice/.globus && \
    mkdir -p /home/alice/.globus && \
    openssl pkcs12 -clcerts -nokeys -in ./gridCertificate.p12 -out /home/alice/.globus/usercert.pem -password pass: && \
    openssl pkcs12 -nocerts -nodes -in ./gridCertificate.p12 -out /home/alice/.globus/userkey.pem -password pass: && \
    chmod 0400 /home/alice/.globus/userkey.pem && chown -R alice /home/alice

# Initialize O2Physics environment
USER alice
RUN mkdir alice
WORKDIR /wd/alice
RUN aliBuild init O2Physics@master && \
    rm -rf O2Physics && \
    git clone https://github.com/mytkom/O2Physics.git && \
    cd O2Physics && git checkout pidml-training-module

USER root
RUN sed -i 's/GIT_COMMAND_TIMEOUT_SEC = 120/GIT_COMMAND_TIMEOUT_SEC = 600/' /usr/lib/python3/dist-packages/alibuild_helpers/git.py

USER alice
WORKDIR /wd/alice
RUN aliBuild build O2Physics --defaults o2 -j 4

# Install golang
USER root
RUN usermod -aG users alice && \
    wget https://go.dev/dl/go1.22.10.linux-amd64.tar.gz -O ~/go.tar.gz && \
    tar -xzvf ~/go.tar.gz -C /usr/local && rm ~/go.tar.gz
USER alice

# Set up Python virtual environment
RUN mkdir /wd/alice/AliceTraINT_pidml_training_module
WORKDIR /wd/alice/AliceTraINT_pidml_training_module
COPY --chown=default ./pdi ./pdi
USER root
RUN chown alice .
USER alice
RUN python3 -m venv .venv && \
    .venv/bin/pip install -r pdi/requirements.txt && \
    .venv/bin/pip install uproot3
ENV PATH="$HOME/go/bin:/usr/local/go/bin:$PATH"

COPY --chown=alice . .
USER alice
RUN go build -o AliceTraINT_pidml_training_module ./cmd/AliceTraINT_pidml_training_module
CMD [ "./AliceTraINT_pidml_training_module" ]