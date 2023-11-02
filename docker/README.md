# HashR in docker

Follow these steps to set-up HashR running in a docker container.

If you want a local installation, check [these steps](https://github.com/google/hashr#setting-up-hashr).

## Table of contents

* [HashR docker image](#hashr-docker-image)
  * [Pull the HashR image](#pull-the-hashr-image)
  * [Build the HashR image](#build-the-hashr-image)
* [Setup a database and importers](#setup-a-database-and-importers)
    * [Database](#database)
    * [Importers](#importers)
    * [Docker networking](#docker-networking)
* [Run HashR](#run-hashr)
    * [Examples](#examples)


## HashR docker image

You can either use our hosted docker image or build it yourself.

### Pull the HashR image

The HashR docker image will provide the HashR binary and tools it needs to
work.

By default the latest tagged release will be pulled if not specified otherwise:

```shell
docker pull us-docker.pkg.dev/osdfir-registry/hashr/release/hashr
```

Pulling a specific release tag:

```shell
docker pull us-docker.pkg.dev/osdfir-registry/hashr/release/hashr:v1.7.1
```

### Build the HashR image

From the repository root folder run the following command:

```shell
docker build -f docker/Dockerfile .
```

## Setup a database and importers

### Database

You still need to provide your own database for HashR to store the results.
Check the [Setting up storage for processing tasks](https://github.com/google/hashr#setting-up-storage-for-processing-tasks) step in the local installation
guide.

### Importers

Follow the [Setting up importers](https://github.com/google/hashr#setting-up-importers)
guide to setup the importers you want to use.

Come back here for running HashR in docker with specific importers.

### Docker networking

Create a docker network that will be used by `hashr_postgresql` and the `hashr`
container.

```shell
docker network create hashr_net
```

```shell
docker network connect hashr_net hashr_postgresql
```

## Run HashR

Get all availalbe HashR flags

```shell
docker run us-docker.pkg.dev/osdfir-registry/hashr/release/hashr -h
```

### Examples

> **NOTE**
Ensure that the host directory mapped into `/data/` in the container is
readable for all!

Run HashR using the `iso9660` importer and export results to PostgreSQL:

```shell
docker run -it \
  --network hashr_net \
  -v ${pwd}/ISO:/data/iso \
  us-docker.pkg.dev/osdfir-registry/hashr/release/hashr \
  -storage postgres \
    -postgres_host hashr_postgresql \
    -postgres_port 5432 \
    -postgres_user hashr \
    -postgres_password hashr \
    -postgres_db hashr \
  -importers iso9660 \
    -iso_repo_path /data/iso/ \
  -exporters postgres
```

Run hashr using the `deb` importer and export results to PostgreSQL:

```shell
docker run -it \
  --network hashr_net \
  -v ${pwd}/DEB:/data/deb \
  us-docker.pkg.dev/osdfir-registry/hashr/release/hashr \
  -storage postgres \
    -postgres_host hashr_postgresql \
    -postgres_port 5432 \
    -postgres_user hashr \
    -postgres_password hashr \
    -postgres_db hashr \
  -importers deb \
    -deb_repo_path /data/deb/ \
  -exporters postgres
```

