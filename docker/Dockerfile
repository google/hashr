FROM ubuntu:22.04

# Use: docker build --no-cache --build-arg PPA_TRACK="[staging|stable]"
ARG PPA_TRACK=stable

ENV DEBIAN_FRONTEND=noninteractive
#RUN apt-get update && apt-get -y upgrade
RUN apt-get update
RUN apt-get -y install \
    p7zip-full \
    apt-transport-https \
    apt-utils \
    sudo \
    curl \
    file \
    gpg \
    software-properties-common

RUN rm -rf /var/lib/apt/lists/*

RUN curl -sS https://keyserver.ubuntu.com/pks/lookup?op=get\&search=0x3ed1eaece81894b171d7da5b5e80511b10c598b8 | gpg --dearmor | sudo tee /etc/apt/trusted.gpg.d/gift.gpg
RUN add-apt-repository -y ppa:gift/$PPA_TRACK
RUN apt-get update && apt-get -y install \
    plaso-tools \
    --option Acquire::ForceIPv4=true --option Acquire::Retries=100 --option Acquire::http::Timeout=60

RUN useradd -G disk,sudo -u 999 hashr
RUN echo "hashr ALL = (root) NOPASSWD: /bin/mount,/bin/umount,/sbin/losetup,/bin/rm" > /etc/sudoers.d/hashr

USER root

WORKDIR /usr/local/bin
COPY hashr .

VOLUME ["/data"]

WORKDIR /home/hashr/
RUN chmod +x /usr/local/bin/hashr
USER hashr
ENTRYPOINT ["/usr/local/bin/hashr"]
