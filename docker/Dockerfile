# Use: docker build --no-cache --build-arg PPA_TRACK="[staging|stable]"

# Stage 0 - hashr-builder
FROM golang:alpine AS hashr-builder
RUN apk add --no-cache  build-base

# Compile hashr statically
RUN mkdir -p /opt/hashr/
COPY . /opt/hashr/
RUN cd /opt/hashr/ && GOOS=linux GOARCH=amd64 go build -v -ldflags="-linkmode=external -extldflags=-static" -tags osusergo,netgo -o hashr hashr.go

# Stage 1 - hashr
FROM ubuntu:22.04

ARG PPA_TRACK=stable
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get -y upgrade && apt-get -y install \
    p7zip-full \
    apt-transport-https \
    apt-utils \
    sudo \
    curl \
    file \
    gpg \
    software-properties-common \
    && apt-get clean && rm -rf /var/cache/apt/* /var/lib/apt/lists/*

RUN add-apt-repository -y ppa:gift/$PPA_TRACK
RUN apt-get update && apt-get -y install plaso-tools

RUN useradd -G disk,sudo -u 999 hashr
RUN echo "hashr ALL = (root) NOPASSWD: /bin/mount,/bin/umount,/sbin/losetup,/bin/rm" > /etc/sudoers.d/hashr

USER root

WORKDIR /usr/local/bin
COPY --from=hashr-builder /opt/hashr/hashr .

VOLUME ["/data"]

WORKDIR /home/hashr/
RUN chmod +x /usr/local/bin/hashr
USER hashr
ENTRYPOINT ["/usr/local/bin/hashr"]
