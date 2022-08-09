# HashR: Generate your own set of hashes 

<img src="docs/assets/HashR.png" width="800"> <br><br>

## Table of Contents

- [HashR: Generate your own set of hashes](#hashr-generate-your-own-set-of-hashes)
  - [Table of Contents](#table-of-contents)
  - [About](#about)
  - [Requirements](#requirements)
  - [Building HashR binary and running tests](#building-hashr-binary-and-running-tests)
  - [Setting up HashR](#setting-up-hashr)
    - [OS configuration & required 3rd party tooling](#os-configuration--required-3rd-party-tooling)
    - [Setting up storage for processing tasks](#setting-up-storage-for-processing-tasks)
      - [Setting up PostgreSQL storage](#setting-up-postgresql-storage)
      - [Setting up Cloud Spanner](#setting-up-cloud-spanner)
    - [Setting up importers](#setting-up-importers)
      - [TarGz](#targz)
      - [GCP](#gcp)
      - [Windows](#windows)
      - [WSUS](#wsus)
    - [Setting up Postgres exporter](#setting-up-postgres-exporter)
    - [Additional flags](#additional-flags)

## About 

HashR allows you to build your own hash sets based on your data sources. It's a tool that extracts files and hashes out of input sources (e.g. raw disk image, GCE disk image, ISO file, Windows update package, .tar.gz file, etc.).

HashR consists of the following components:

1. Importers, which are responsible for copying the source to local storage and doing any required preprocessing.
1. Core, which takes care of extracting the content from the source using image_export.py (Plaso), caching and repository level deduplication and preparing the extracted files for the exporters.
1. Exporters, which are responsible for exporting files, metadata and hashes to given data sinks. Currently the main exporter is the Postgres exporter.

Currently implemented importers: 

1. TarGz, which extracts files from .tar.gz archives. 
1. GCP, which extracts file from base GCP disk [images](https://cloud.google.com/compute/docs/images)
1. Windows, which extracts files from Windows installation media in ISO-13346 format. 
1. WSUS, which extracts files from Windows Update packages.  

Once files are extracted and hashed results will be passed to the exporter, currently the only available exporter is PostgreSQL. 

You can choose which importers you want to run, each one have different requirements. More about this can be found in sections below. 


## Requirements

HashR requires Linux OS to run, this can be a physical, virtual or cloud machine. Below are optimal hardware requirements: 

1. 8-16 cores 
1. 128GB memory 
1. 2TB fast local storage (SSDs preferred)

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

### OS configuration & required 3rd party tooling 

HashR takes care of the heavy lifting (parsing disk images, volumes, file systems) by using Plaso. You need to pull the Plaso docker container using the following command: 

``` shell
docker pull log2timeline/plaso
```

We also need two additional tools installed on the machine running HashR:

1. mmls, which is part of Sleuthkit, to list volumes on disk image 
1. 7z, which is used by WSUS importer for recursive extraction in Windows Update packages 

You can install both with the following command: 

``` shell
sudo apt install p7zip-full, sleuthkit
```

You need to allow the user, under which HashR will run, to run certain commands via sudo. Assuming that your user is `hashr` create a file `/etc/sudoers.d/hashr` and put in: 

``` shell
hashr ALL = (root) NOPASSWD: /bin/mount,/bin/umount,/sbin/losetup,/bin/rm
```

### Setting up storage for processing tasks

HashR needs to store information about processed sources. It also stores additional telemetry about processing tasks: processing times, number of extracted files, etc. You can choose between using:

1. PostgreSQL 
1. Cloud (GCP) Spanner

#### Setting up PostgreSQL storage 

There are many ways how to you can run and maintain your PostgreSQL instance, one of the simplest way would be to run it in a Docker container. Follow steps below to set up a PostgreSQL Docker container.

Step 1: Pull the PostgreSQL docker image.

``` shell
docker pull postgres
```

Step 2: Initialize and run the PostgreSQL container in the background. Make sure to adjust the password. 

``` shell
docker run -itd -e POSTGRES_DB=hashr -e POSTGRES_USER=hashr -e POSTGRES_PASSWORD=hashr -p 5432:5432 -v /data:/var/lib/postgresql/data --name hashr_postgresql postgres
```

Step 3: Create table that will be used to store processing jobs.

``` shell
cat scripts/CreateJobsTable.sql | docker exec -i hashr_postgresql psql -U hashr -d hashr
```

In order to use PostgreSQL to store information about processing tasks you need to specify the following flags: `-jobsStorage postgres -postgresHost <host> -postgresPort <port> -postgresUser <user> -postgresPassword <pass> -postgresDBName <db_name>`

#### Setting up Cloud Spanner

You can choose the store the data about processing jobs in Cloud Spanner. You'll need a Google Cloud project for that. Main advantage of this setup is that you can easily create dashboard(s) using [Google Data Studio](https://datastudio.google.com/) and directly connect to the Cloud Spanner instance. That allows you monitoring and debugging without running queries against your PostgreSQL instance. 

Assuming that you `gcloud` tool configured with your target hashr GCP project you'll need to follow few steps to enable Cloud Spanner.

Create HashR service account:

``` shell
gcloud iam service-accounts create hashr --description="HashR SA key." --display-name="hashr"
```

Create service account key and store in your home directory. Set *<project_name>* to your project name.

``` shell
gcloud iam service-accounts keys create ~/hashr-sa-private-key.json --iam-account=hashr@<project_name>.iam.gserviceaccount.com
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

Update Spanner database schema: 

``` shell
gcloud spanner databases ddl update hashr --instance=hashr --ddl-file=scripts/CreateJobsTable.ddl
```

Allow the service account to use Spanner database, set *<project_name>* to your project name:

``` shell
gcloud spanner databases add-iam-policy-binding hashr --instance hashr --member="serviceAccount:hashr@<project_name>.iam.gserviceaccount.com" --role="roles/spanner.databaseUser" 
```

In order to use PostgreSQL to store information about processing tasks you need to specify the following flags: `-jobStorage cloudspanner -spannerDBPath <spanner_db_path>`

### Setting up importers 

In order to specify which importer you want to run you should use the `-importers` flag. Possible values: `GCP,targz,windows,wsus`


#### TarGz 

This is a simple importer that traverses repository and looks for `.tar.gz` files. Once found it will hash the first and the last 10MB of the file to check if it was already processed. This is done to prevent hashing the whole file every time the repository is scanned for new sources. To use this importer you need to specify the following flag(s): 

1. `-targz_repo_path` which should point to the path on the local file system that contains `.tar.gz` files

#### GCP 

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
gcloud iam service-accounts create hashr --description="HashR SA key." --display-name="hashr"
```

Step 2: Create service account key and store in your home directory. Make sure to  set *<project_name>* to your project name:

``` shell
gcloud iam service-accounts keys create ~/hashr-sa-private-key.json --iam-account=hashr@<project_name>.iam.gserviceaccount.com
```

Step 3: Point GOOGLE_APPLICATION_CREDENTIALS env variable to your service account key: 

``` shell
export GOOGLE_APPLICATION_CREDENTIALS=/home/hashr/hashr-sa-private-key.json
```

Step 4: Create GCS bucket that will be used to store disk images in .tar.gz format, set *<project_name>* to your project name and  *<gcs_bucket_name>* to your project new GCS bucket name: 

``` shell
gsutil mb -p project_name> gs://<gcs_bucket_name>
```

Step 5: Make the service account admin of this bucket: 
``` shell
gsutil iam ch serviceAccount:hashr@<project_name>.iam.gserviceaccount.com:objectAdmin gs://<gcs_bucket_name>
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
gcloud projects add-iam-policy-binding <project_name> --member="serviceAccount:hashr@<project_name>.iam.gserviceaccount.com" --role="projects/<project_name>/roles/hashr"
```
Step Grant service accounts access required to run Cloud Build, make sure the change the *<project_name>* and *<project_id>* values: 
``` shell
gcloud projects add-iam-policy-binding <project_name> --member='serviceAccount:hashr@<project_name>.iam.gserviceaccount.com' --role='roles/storage.admin'

gcloud projects add-iam-policy-binding <project_name> \
  --member='serviceAccount:hashr@<project_name>.iam.gserviceaccount.com' \
  --role='roles/viewer'

gcloud projects add-iam-policy-binding <project_name> \
  --member='serviceAccount:hashr@<project_name>.iam.gserviceaccount.com' \
  --role='roles/resourcemanager.projectIamAdmin'

gcloud projects add-iam-policy-binding <project_name> \
  --member='serviceAccount:hashr@<project_name>.iam.gserviceaccount.com' \
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

#### Windows 

This importer extracts files from official Windows installation media in ISO-13346 format, e.g. the ones you can download from official Microsoft [website](https://www.microsoft.com/en-gb/software-download/windows10ISO). 
One ISO file can contain multiple WIM images: 

1. Windows10ProEducation
1. Windows10Education
1. Windows10EducationN
1. Windows10ProN
1. etc.. 

This importer will extract files from all images it can find in the `install.wim` file.

#### WSUS

This importer utilizes 7z to recursively extract contents of Windows Update packages. It will look for Windows Update files in the provided GCS bucket, easiest way to automatically update the GCS bucket with new updates would be to do the following: 

1. Set up GCE VM running Windows Server in hashr GCP project. 
1. Configure it with WSUS role, select Windows Update packages that you'd like to process
1. Configure WSUS to automatically approve and download updates to local storage 
1. Set up a Windows task to automatically sync content of the local storage to the GCS bucket: `gsutil -m rsync -r D:/WSUS/WsusContent gs://hashr-wsus/` (remember to adjust the paths)

### Setting up Postgres exporter 

Postgres exporter allows to send hashes, file metadata and the actual content of the file to a PostgreSQL instance. For best performance it's advised to set it up on a separate and dedicated machine. 
If you did set up PostgreSQL while choosing the processing jobs storage you're almost good to go, just run the following command to create the required tables: 
``` shell
cat scripts/CreatePostgresExporterTables.sql | docker exec -i hashr_postgresql psql -U hashr -d hashr
```
If you didn't choose Postgres for processing job storage follow steps 1 & 2 from the [Setting up PostgreSQL storage](####setting-up-postgresql-storage) section. 

This is currently the default exporter, you don't need to explicitly enable it. By default the content of the actual files won't be uploaded to PostgreSQL DB, if you wish to change that use `-upload_payloads true` flag. 

In order for the Postgres exporter to work you need to set the following flags: `-postgresHost <host> -postgresPort <port> -postgresUser <user> -postgresPassword <pass> -postgresDBName <db_name>`

### Additional flags

1. `-processingWorkerCount`: This flag controls number of parallel processing workers. Processing is CPU and I/O heavy, during my testing I found that having 2 workers is the most optimal solution. 
1. `-cacheDir`: Location of local cache used for deduplication, it's advised to change that from `/tmp` to e.g. home directory of the user that will be running hashr.
1. `-export`: When set to false hashr will save the results to disk bypassing the exporter. 
1. `-exportPath`: If export is set to false, this is the folder where samples will be saved.
1. `-reprocess`: Allows to reprocess a given source (in case it e.g. errored out) based on the sha256 value stored in the jobs table. 


This is not an officially supported Google product.