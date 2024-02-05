# HashR: Generate your own set of hashes

<img src="docs/assets/HashR.png" width="800"> <br><br>

## Table of Contents

- [HashR: Generate your own set of hashes](#hashr-generate-your-own-set-of-hashes)
  - [Table of Contents](#table-of-contents)
  - [About](#about)
  - [Requirements](#requirements)
  - [Building HashR binary and running tests](#building-hashr-binary-and-running-tests)
  - [Setting up HashR](#setting-up-hashr)
    - [HashR using docker](#hashr-using-docker)
    - [OS configuration \& required 3rd party tooling](#os-configuration--required-3rd-party-tooling)
    - [Setting up storage for processing tasks](#setting-up-storage-for-processing-tasks)
      - [Setting up PostgreSQL storage](#setting-up-postgresql-storage)
      - [Setting up Cloud Spanner](#setting-up-cloud-spanner)
    - [Setting up importers](#setting-up-importers)
      - [GCP (Google Cloud Platform)](#gcp-google-cloud-platform)
      - [AWS (Amazon Web Services)](#aws)
      - [GCR (Google Container Registry)](#gcr-google-container-registry)
      - [Windows](#windows)
      - [WSUS](#wsus)
      - [TarGz](#targz)
      - [Deb](#deb)
      - [RPM](#rpm)
      - [Zip (and other zip-like formats)](#zip-and-other-zip-like-formats)
      - [ISO 9660](#iso-9660)
    - [Setting up exporters](#setting-up-exporters)
      - [Setting up Postgres exporter](#setting-up-postgres-exporter)
      - [Setting up GCP exporter](#setting-up-gcp-exporter)
    - [Additional flags](#additional-flags)

## About

HashR allows you to build your own hash sets based on your data sources. It's a tool that extracts files and hashes out of input sources (e.g. raw disk image, GCE disk image, ISO file, Windows update package, .tar.gz file, etc.).

HashR consists of the following components:

1. Importers, which are responsible for copying the source to local storage and doing any required preprocessing.
1. Core, which takes care of extracting the content from the source using image_export.py (Plaso), caching and repository level deduplication and preparing the extracted files for the exporters.
1. Exporters, which are responsible for exporting files, metadata and hashes to given data sinks.

Currently implemented importers:

1. GCP, which extracts file from base GCP disk [images](https://cloud.google.com/compute/docs/images)
1. Windows, which extracts files from Windows installation media in ISO-13346 format.
1. WSUS, which extracts files from Windows Update packages.
1. GCR, which extracts file from container images stored in Google Container Registry.
1. TarGz, which extracts files from .tar.gz archives.
1. Deb, which extracts files Debian software packages.
1. RPM, which extracts files from RPM software packages.
1. Zip, which extracts files from .zip (and zip-like) archives.

Once files are extracted and hashed results will be passed to the exporters, currently implemented exporters:

1. PostgreSQL, which upload the data to PostgreSQL instance.
1. Cloud Spanner, which uploads the data to GCP Spanner instance.

You can choose which importers you want to run, each one have different requirements. More about this can be found in sections below.


## Requirements

HashR requires Linux OS to run, this can be a physical, virtual or cloud machine. Below are optimal hardware requirements:

1. 8-16 cores
1. 128GB memory
1. 2TB fast local storage (SSDs preferred)

HashR can likely run how machines with lower specifications, however this was not thoroughly tested.

## Building HashR binary and running tests

In order to build a hashr binary run the following command:

``` shell
env GOOS=linux GOARCH=amd64 go build hashr.go
```

In order to run tests for the core hashR package you need to run Spanner emulator:

``` shell
gcloud emulators spanner start
```

Then to execute all tests run the following command:

``` shell
go test -timeout 2m ./...
```

## Setting up HashR

### HashR using docker

To run HashR in a docker container visit the [docker specific guide](docker/README.md)

### OS configuration & required 3rd party tooling

HashR takes care of the heavy lifting (parsing disk images, volumes, file systems) by using Plaso. You need to pull the Plaso docker container using the following command:

``` shell
docker pull log2timeline/plaso
```

We also need 7z, which is used by WSUS importer for recursive extraction of Windows Update packages, to be installed on the machine running HashR:

``` shell
sudo apt install p7zip-full
```

You need to allow the user, under which HashR will run, to run certain commands via sudo. Assuming that your user is `hashr` create a file `/etc/sudoers.d/hashr` and put in:

``` shell
hashr ALL = (root) NOPASSWD: /bin/mount,/bin/umount,/sbin/losetup,/bin/rm
```

The user under which HashR will run will also need to be able to run docker. Assuming that your user is `hashr`, add them to the docker group like this:

``` shell
sudo usermod -aG docker hashr
```


### Setting up storage for processing tasks

HashR needs to store information about processed sources. It also stores additional telemetry about processing tasks: processing times, number of extracted files, etc. You can choose between using:

1. PostgreSQL
1. Cloud (GCP) Spanner

#### Setting up PostgreSQL storage

There are many ways you can run and maintain your PostgreSQL instance, one of the simplest ways would be to run it in a Docker container. Follow the steps below to set up a PostgreSQL Docker container.

Step 1: Pull the PostgreSQL docker image.

``` shell
docker pull postgres
```

Step 2: Initialize and run the PostgreSQL container in the background. Make sure to adjust the password.

``` shell
docker run -itd -e POSTGRES_DB=hashr -e POSTGRES_USER=hashr -e POSTGRES_PASSWORD=hashr -p 5432:5432 -v /data:/var/lib/postgresql/data --name hashr_postgresql postgres
```

Step 3: Create a table that will be used to store processing jobs.

``` shell
cat scripts/CreateJobsTable.sql | docker exec -i hashr_postgresql psql -U hashr -d hashr
```

In order to use PostgreSQL to store information about processing tasks you need to specify the following flags: `-storage postgres -postgres_host <host> -postgres_port <port> -postgres_user <user> -postgres_password <pass> -postgres_db <db_name>`

#### Setting up Cloud Spanner

You can choose the store the data about processing jobs in Cloud Spanner. You'll need a Google Cloud project for that. The main advantage of this setup is that you can easily create dashboard(s) using [Google Data Studio](https://datastudio.google.com/) and directly connect to the Cloud Spanner instance that allows monitoring and debugging without running queries against your PostgreSQL instance.

Assuming that your `gcloud` tool is configured with your target hashr GCP project, you'll need to follow the steps below to enable Cloud Spanner.

Create HashR service account:

``` shell
gcloud iam service-accounts create hashr --description="HashR SA key." --display-name="hashr"
```

Create service account key and store in your home directory. Set *<project_name>* to your project name.

``` shell
gcloud iam service-accounts keys create ~/hashr-sa-private-key.json --iam-account=hashr-sa@<project_name>.iam.gserviceaccount.com
```

Point GOOGLE_APPLICATION_CREDENTIALS env variable to your service account key:

``` shell
export GOOGLE_APPLICATION_CREDENTIALS=/home/hashr/hashr-sa-private-key.json
```

Create Spanner instance, adjust the config and processing-units value if needed:

``` shell
gcloud spanner instances create hashr --config=regional-us-central1 --description="hashr" --processing-units=100
```

Create Spanner database:

``` shell
gcloud spanner databases create hashr --instance=hashr
```

Allow the service account to use Spanner database, set *<project_name>* to your project name:

``` shell
gcloud spanner databases add-iam-policy-binding hashr --instance hashr --member="serviceAccount:hashr-sa@<project_name>.iam.gserviceaccount.com" --role="roles/spanner.databaseUser"
```

Update Spanner database schema:

``` shell
gcloud spanner databases ddl update hashr --instance=hashr --ddl-file=scripts/CreateJobsTable.ddl
```

In order to use Cloud Spanner to store information about processing tasks you need to specify the following flags: `-jobStorage cloudspanner -spannerDBPath <spanner_db_path>`

### Setting up importers

In order to specify which importer you want to run you should use the `-importers` flag. Possible values: `GCP,targz,windows,wsus,deb,rpm,zip,gcr,iso9660`

#### GCP (Google Cloud Platform)

This importer can extract files from GCP disk [images](https://cloud.google.com/compute/docs/images). This is done in few steps:

1. Check for new images in the target project (e.g. ubuntu-os-cloud)
1. Copy new/unprocessed image to the hashR GCP project
1. Run Cloud Build, which creates a temporary VM, runs dd on the copied image and saves the output to a .tar.gz file.
1. Export raw_disk.tar.gz to the GCS bucket in hashR GCP project
1. Copy raw_disk.tar.gz from GCS to local hashR storage
1. Extract raw_disk.tar.gz and pass the disk image to Plaso

List of GCP projects containing public GCP images can be found [here](https://cloud.google.com/compute/docs/images/os-details#general-info). In order to use this importer you need to have a GCP project and follow these steps:

Step 1: Create HashR service account, if this was done while setting up Cloud Spanner please go to step 4.

``` shell
gcloud iam service-accounts create hashr-sa --description="HashR SA key." --display-name="hashr"
```

Step 2: Create service account key and store in your home directory. Make sure to  set *<project_name>* to your project name:

``` shell
gcloud iam service-accounts keys create ~/hashr-sa-private-key.json --iam-account=hashr-sa@<project_name>.iam.gserviceaccount.com
```

Step 3: Point GOOGLE_APPLICATION_CREDENTIALS env variable to your service account key:

``` shell
export GOOGLE_APPLICATION_CREDENTIALS=~/hashr-sa-private-key.json
```

Step 4: Create GCS bucket that will be used to store disk images in .tar.gz format, set *<project_name>* to your project name and  *<gcs_bucket_name>* to your project new GCS bucket name:

``` shell
gsutil mb -p project_name> gs://<gcs_bucket_name>
```

Step 5: Make the service account admin of this bucket:
``` shell
gsutil iam ch serviceAccount:hashr-sa@<project_name>.iam.gserviceaccount.com:objectAdmin gs://<gcs_bucket_name>
```
Step 6: Enable Compute API:
``` shell
gcloud services enable compute.googleapis.com cloudbuild.googleapis.com
```
Step 7: Create IAM role and assign it required permissions:
``` shell
gcloud iam roles create hashr --project=<project_name> --title=hashr --description="Permissions required to run hashR" --permissions compute.images.create compute.images.delete compute.globalOperations.ge
```
Step 8: Bind IAM role to the service account:
``` shell
gcloud projects add-iam-policy-binding <project_name> --member="serviceAccount:hashr-sa@<project_name>.iam.gserviceaccount.com" --role="projects/<project_name>/roles/hashr"
```
Step Grant service accounts access required to run Cloud Build, make sure the change the *<project_name>* and *<project_id>* values:
``` shell
gcloud projects add-iam-policy-binding <project_name> --member='serviceAccount:hashr-sa@<project_name>.iam.gserviceaccount.com' --role='roles/storage.admin'

gcloud projects add-iam-policy-binding <project_name> \
  --member='serviceAccount:hashr-sa@<project_name>.iam.gserviceaccount.com' \
  --role='roles/viewer'

gcloud projects add-iam-policy-binding <project_name> \
  --member='serviceAccount:hashr-sa@<project_name>.iam.gserviceaccount.com' \
  --role='roles/resourcemanager.projectIamAdmin'

gcloud projects add-iam-policy-binding <project_name> \
  --member='serviceAccount:hashr-sa@<project_name>.iam.gserviceaccount.com' \
  --role='roles/cloudbuild.builds.editor'


gcloud projects add-iam-policy-binding <project_name> \
   --member='serviceAccount:<project_id>@cloudbuild.gserviceaccount.com' \
   --role='roles/compute.admin'

gcloud projects add-iam-policy-binding <project_name> \
   --member='serviceAccount:<project_id>@cloudbuild.gserviceaccount.com' \
   --role='roles/iam.serviceAccountUser'

gcloud projects add-iam-policy-binding <project_name> \
   --member='serviceAccount:<project_id>@cloudbuild.gserviceaccount.com' \
   --role='roles/iam.serviceAccountTokenCreator'

gcloud projects add-iam-policy-binding <project_name> \
   --member='serviceAccount:<project_id>@cloudbuild.gserviceaccount.com' \
   --role='roles/compute.networkUser'

gcloud projects add-iam-policy-binding <project_name> \
  --member='serviceAccount:<project_id>-compute@developer.gserviceaccount.com' \
  --role='roles/compute.storageAdmin'

gcloud projects add-iam-policy-binding <project_name> \
  --member='serviceAccount:<project_id>-compute@developer.gserviceaccount.com' \
  --role='roles/storage.objectViewer'

gcloud projects add-iam-policy-binding <project_name> \
  --member='serviceAccount:<project_id>-compute@developer.gserviceaccount.com' \
  --role='roles/storage.objectAdmin'
```

To use this importer you need to specify the following flag(s):

1. `-gcpProjects` which is a comma separated list of cloud projects containing disk images. If you'd like to import public images take a look [here](https://cloud.google.com/compute/docs/images/os-details#general-info)
1. `-hashrGCPProject` GCP project that will be used to store copy of disk images for processing and also run Cloud Build
1. `-hashrGCSBucket` GCS bucket that will be used to store output of Cloud Build (disk images in .tar.gz format)

#### AWS

This importer processes Amazon owned AMIs and generates hashes. The importer requires at least one HashR worker (an EC2 instance).

##### AWS HashR Workers

AWS HashR worker is an EC2 instance where AMI’s volume is attached, disk archive is created, and then uploaded to S3 bucket. It is recommended to have at least two AWS HashR workers. If your setup uses a single AWS worker use `-processing_worker_count 1`.

An AWS HashR worker needs to have to meet the following requirements:

- EC2 instances must have the tag `InUse: false`. If the value is `true`, the worker is not used for processing.

```shell
aws ec2 describe-instances --instance-id INSTANCE_ID | jq -r ‘.Reservations[].Instances[0].Tags’
```

- The system running `hashr` must be able to SSH to the EC2 instance using:
  - SSH key as described in `Keyname`.

  ```shell
  aws ec2 describe-instances --instance-id INSTANCE_ID | jq -r ‘.Reservations[].Instances[0].Keyname’
  ```

  - To FQDN as described in `PublicDnsName`.

  ```shell
  aws ec2 describe-instances --instance-id INSTANCE_ID | jq -r ‘.Reservations[].Instances[0].PublicDnsName’
  ```

- `scripts/hashr-archive` must be copied to AWS HashR worker to `/usr/local/sbin/hashr-archive`

- An AWS account with permission to upload files to HashR bucket. AWS configuration and credential should be stored in `$HOME/.aws/` directory.

```shell
aws configure
```

##### HashR Application

On the system that runs the `hashr` the following is required.

- An AWS account with permissions to call followings APIs:
  - EC2
    - AttachVolume
    - CopyImage
    - CreateTags
    - CreateVolume
    - DeleteVolume
    - DescribeAvailabilityZones
    - DescribeImages
    - DescribeInstances
    - DescribeSnapshots
    - DescribeVolumes
    - DetachVolume
  - S3
    - DeleteObject
- AWS account configuration and credential file must be located at `$HOME/.aws/` directory.
- The SSH private key used for AWS HashR must be located in the `$HOME/.ssh/` directory. It must match the value of `Keyname` as described in `aws ec2 describe-instances --instance-id INSTANCE_ID | jq -r ‘.Reservations[].Instances[0].Keyname’

##### Setting up AWS EC2 Instance

This section describes how to create EC2 instances to use with HashR. Ideally we want two AWS accounts `hashr.uploader` and `hashr.worker`.

`hashr.uploader` is used on EC2 instances and needs permissions to upload archived disk images to S3 bucket. `scripts/aws/AwsHashrUploaderPolicy.json` contains sample policy for the S3 bucket `hashr-bucket`.

`hashr.worker` is used on the computer running HashR commands. The account needs EC2 and S3 permissions. `scripts/aws/AwsHashrWorkerPolicy.json` contains sample policy for the `hashr.worker` account.

The `hashr_setup.sh` is a script that helps create EC2 instances. Edit `hashr_setup.sh` and review and update the following fields as required:
- `AWS_PROFILE`
- `AWS_REGION`
- `SECURITY_SOURCE_CIDR`
- `WORKER_AWS_CONFIG_FILE`

**Note**: The file specified `WORKER_AWS_CONFIG_FILE` must exist in the directory with `hashr_setup.sh`.

**Note**: The `hashr_setup.sh` must be executed from the same directory as `hashr_setup.sh`.

Run the following commands to create and set up the EC2 instances.

```shell
$ git clone https://github.com/google/hashr
$ cd hashr/scripts/aws
$ aws configure
$ cp -r ~/.aws ./
$ tar -zcf hashr.uploader.tar.gz .aws
$ hash_setup.sh setup
```

##### HashR AWS Importer Workflow

AWS importer takes the following high level steps:

1. Copies a new/unprocessed Amazon owned AMI to HashR project
2. Creates a volume based on the copied AMI
3. Attaches the volume to an available AWS HashR worker
4. On an AWS HashR worker
  a. Creates disk archive (tar.gz) on the AWS HashR worker
  b. Uploads the disk archive to HashR S3 bucket
5. Downloads the disk archive from HashR S3 bucket
6. Unarchives the disk image
7. Processes the raw disk using Plaso

##### HashR AWS Importer Command

The command below processes `debian-12` images and stores them in a PostgreSQL database.

```shell
hashr -storage postgres -exporters postgres -importers aws -aws_bucket aws-hashr-bucket -aws_os_filter debian-12
```

**Note**: Amazon Linux (al2023-*) was used as a worker while developing the importer. Thus, the default value for `-aws_ssh_user` is set to `ec2-user`. A different distro may have a different default SSH user, use `-aws_ssh_user` to set the appropriate SSH user.

#### GCR (Google Container Registry)
This importer extracts files from container images stored in GCR repositories. In order to set ip up follow these steps:

Step 1: Create HashR service account, skip to step 4 if this was done while setting up other GCP dependent components.

``` shell
gcloud iam service-accounts create hashr-sa --description="HashR SA key." --display-name="hashr"
```

Step 2: Create service account key and store in your home directory. Make sure to  set *<project_name>* to your project name:

``` shell
gcloud iam service-accounts keys create ~/hashr-sa-private-key.json --iam-account=hashr-sa@<project_name>.iam.gserviceaccount.com
```

Step 3: Point GOOGLE_APPLICATION_CREDENTIALS env variable to your service account key:

``` shell
export GOOGLE_APPLICATION_CREDENTIALS=~/hashr-sa-private-key.json
```

Step 4: Grant hashR service account key required permissions to access given GCR repository.

``` shell
gsutil iam ch serviceAccount:hashr-sa@<project_name>.iam.gserviceaccount.com:objectViewer gs://artifacts.<project_name_hosting_gcr_repo>.appspot.com
```

To use this importer you need to specify the following flag(s):

1. `-gcr_repos` which should contain comma separated list of GCR repositories from which you want to import the container images.

#### Windows

This importer extracts files from official Windows installation media in ISO-13346 format, e.g. the ones you can download from official Microsoft [website](https://www.microsoft.com/en-gb/software-download/windows10ISO).
One ISO file can contain multiple WIM images:

1. Windows10ProEducation
1. Windows10Education
1. Windows10EducationN
1. Windows10ProN
1. etc.

This importer will extract files from all images it can find in the `install.wim` file.

#### WSUS

This importer utilizes 7z to recursively extract contents of Windows Update packages. It will look for Windows Update files in the provided GCS bucket, easiest way to automatically update the GCS bucket with new updates would be to do the following:

1. Set up GCE VM running Windows Server in hashr GCP project.
1. Configure it with WSUS role, select Windows Update packages that you'd like to process
1. Configure WSUS to automatically approve and download updates to local storage
1. Set up a Windows task to automatically sync content of the local storage to the GCS bucket: `gsutil -m rsync -r D:/WSUS/WsusContent gs://hashr-wsus/` (remember to adjust the paths)
1. If you'd like to have the filename of the update package (which usually contains KB number) as the ID (by default it's sha1, that's how MS stores WSUS updates) and its description this is something that can be dumped from the internal WID WSUS database. You can use the following Power Shell script and run it as a task:

```
#SQL Query
$delimiter = ";"
$SqlQuery = 'select DISTINCT CONVERT([varchar](512), tbfile.FileDigest, 2) as sha1, tbfile.[FileName], vu.[KnowledgebaseArticle], vu.[DefaultTitle]  from [SUSDB].[dbo].[tbFile] tbfile
  left join [SUSDB].[dbo].[tbFileForRevision] ffrev
  on tbfile.FileDigest = ffrev.FileDigest
  left join [SUSDB].[dbo].[tbRevision] rev
  on ffrev.RevisionID = rev.RevisionID
  left join [SUSDB].[dbo].[tbUpdate] u
  on rev.LocalUpdateID = u.LocalUpdateID
  left join [SUSDB].[PUBLIC_VIEWS].[vUpdate] vu
  on u.UpdateID = vu.UpdateId'
$SqlConnection = New-Object System.Data.SqlClient.SqlConnection
$SqlConnection.ConnectionString = 'server=\\.\pipe\MICROSOFT##WID\tsql\query;database=SUSDB;trusted_connection=true;'
$SqlCmd = New-Object System.Data.SqlClient.SqlCommand
$SqlCmd.CommandText = $SqlQuery
$SqlCmd.Connection = $SqlConnection
$SqlCmd.CommandTimeout = 0
$SqlAdapter = New-Object System.Data.SqlClient.SqlDataAdapter
$SqlAdapter.SelectCommand = $SqlCmd
#Creating Dataset
$DataSet = New-Object System.Data.DataSet
$SqlAdapter.Fill($DataSet)
$DataSet.Tables[0] | export-csv -Delimiter $delimiter -Path "D:\WSUS\WsusContent\export.csv" -NoTypeInformation

gsutil -m rsync -r D:/WSUS/WsusContent gs://hashr-wsus/
```

This will dump the relevant information from WSUS DB, store it in the `export.csv` file and sync the contents of the WSUS folder with GCS bucket. WSUS importer will check if `export.csv` file is present in the root of the WSUS repo, if so it will use it.

#### TarGz

This is a simple importer that traverses repositories and looks for `.tar.gz` files. Once found it will hash the first and the last 10MB of the file to check if it was already processed. This is done to prevent hashing the whole file every time the repository is scanned for new sources. To use this importer you need to specify the following flag(s):

1. `-targz_repo_path` which should point to the path on the local file system that contains `.tar.gz` files

#### Deb

This is very similar to the TarGz importer except that it looks for `.deb` packages. Once found it will hash the first and the last 10MB of the file to check if it was already processed. This is done to prevent hashing the whole file every time the repository is scanned for new sources. To use this importer you need to specify the following flag(s):

1. `-deb_repo_path` which should point to the path on the local file system that contains `.deb` files

#### RPM

This is very similar to the TarGz importer except that it looks for `.rpm` packages. Once found it will hash the first and the last 10MB of the file to check if it was already processed. This is done to prevent hashing the whole file every time the repository is scanned for new sources. To use this importer you need to specify the following flag(s):

1. `-rpm_repo_path` which should point to the path on the local file system that contains `.rpm` files

#### Zip (and other zip-like formats)

This is very similar to the TarGz importer except that it looks for `.zip` archives. Once found it will hash the first and the last 10MB of the file to check if it was already processed. This is done to prevent hashing the whole file every time the repository is scanned for new sources. To use this importer you need to specify the following flag(s):

1. `-zip_repo_path` which should point to the path on the local file system that contains `.zip` files

Optionally, you can also set the following flag(s):

1. `-zip_file_exts` comma-separated list of file extensions to treat as zip files, eg. "zip,whl,jar". Default: "zip"

#### ISO 9660

This is very similar to the TarGz importer except that it looks for `.iso` file. Once found it will hash the first and the last 10MB of the file to check if it was already processed. This is done to prevent hashing the whole file every time the repository is scanned for new sources. To use this importer you need to specify the following flag(s):

1. `-iso_repo_path` which should point to the path on the local file system that contains `.iso` files

### Setting up exporters

#### Setting up Postgres exporter

Postgres exporter allows sending of hashes, file metadata and the actual content of the file to a PostgreSQL instance. For best performance it's advised to set it up on a separate and dedicated machine.
If you did set up PostgreSQL while choosing the processing jobs storage you're almost good to go, just run the following command to create the required tables:
``` shell
cat scripts/CreatePostgresExporterTables.sql | docker exec -i hashr_postgresql psql -U hashr -d hashr
```
If you didn't choose Postgres for processing job storage follow steps 1 & 2 from the [Setting up PostgreSQL storage](####setting-up-postgresql-storage) section.

This is currently the default exporter, you don't need to explicitly enable it. By default the content of the actual files won't be uploaded to PostgreSQL DB, if you wish to change that use `-upload_payloads true` flag.

In order for the Postgres exporter to work you need to set the following flags: `-exporters postgres -postgresHost <host> -postgresPort <port> -postgresUser <user> -postgresPassword <pass> -postgresDBName <db_name>`

#### Setting up GCP exporter

GCP exporter allows sending of hashes, file metadata to GCP Spanner instance. Optionally you can upload the extracted files to GCS bucket. If you haven't set up Cloud Spanner for storing processing jobs, follow the steps in [Setting up Cloud Spanner](####setting-up-cloud-spanner) and instead of the last step run the following command to create necessary tables:

``` shell
gcloud spanner databases ddl update hashr --instance=hashr --ddl-file=scripts/CreateCloudSpannerExporterTables.ddl
```

If you have already set up Cloud Spanner for storing jobs data you just need to the run the command above and you're ready to go.

If you'd like to upload the extracted files to GCS you need to create the GCS bucket:

Step 1: Make the service account admin of this bucket:
``` shell
gsutil mb -p project_name> gs://<gcs_bucket_name>
```

Step 2: Make the service account admin of this bucket:
``` shell
gsutil iam ch serviceAccount:hashr-sa@<project_name>.iam.gserviceaccount.com:objectAdmin gs://<gcs_bucket_name>
```

To use this exporter you need to provide the following flags: `-exporters GCP -gcp_exporter_gcs_bucket <gcs_bucket_name>`

### Additional flags

1. `-processing_worker_count`: This flag controls number of parallel processing workers. Processing is CPU and I/O heavy, during my testing I found that having 2 workers is the most optimal solution.
1. `-cache_dir`: Location of local cache used for deduplication, it's advised to change that from `/tmp` to e.g. home directory of the user that will be running hashr.
1. `-export`: When set to false hashr will save the results to disk bypassing the exporter.
1. `-export_path`: If export is set to false, this is the folder where samples will be saved.
1. `-reprocess`: Allows to reprocess a given source (in case it e.g. errored out) based on the sha256 value stored in the jobs table.
1. `-upload_payloads`: Controls if the actual content of the file will be uploaded by defined exporters.
2. `-gcp_exporter_worker_count`: Number of workers/goroutines that the GCP exporter will use to upload the data.


This is not an officially supported Google product.
