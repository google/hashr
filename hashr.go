// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"strings"

	"cloud.google.com/go/spanner"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/golang/glog"
	"github.com/google/hashr/core/hashr"
	gcpExporter "github.com/google/hashr/exporters/gcp"
	postgresExporter "github.com/google/hashr/exporters/postgres"
	"github.com/google/hashr/importers/deb"
	"github.com/google/hashr/importers/gcp"
	"github.com/google/hashr/importers/gcr"
	"github.com/google/hashr/importers/iso9660"
	"github.com/google/hashr/importers/rpm"
	"github.com/google/hashr/importers/targz"
	"github.com/google/hashr/importers/windows"
	"github.com/google/hashr/importers/wsus"
	"github.com/google/hashr/importers/zip"
	"github.com/google/hashr/processors/local"
	"github.com/google/hashr/storage/cloudspanner"
	"github.com/google/hashr/storage/postgres"
	"golang.org/x/oauth2/google"

	"google.golang.org/api/cloudbuild/v1"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/storage/v1"

	awsImporter "github.com/google/hashr/importers/aws"
)

var (
	processingWorkerCount  = flag.Int("processing_worker_count", 2, "Number of processing workers.")
	importersToRun         = flag.String("importers", strings.Join([]string{}, ","), fmt.Sprintf("Importers to be run: %s,%s,%s,%s,%s,%s,%s,%s,%s", gcp.RepoName, targz.RepoName, windows.RepoName, wsus.RepoName, deb.RepoName, rpm.RepoName, zip.RepoName, gcr.RepoName, iso9660.RepoName))
	exportersToRun         = flag.String("exporters", strings.Join([]string{}, ","), fmt.Sprintf("Exporters to be run: %s,%s", gcpExporter.Name, postgresExporter.Name))
	jobStorage             = flag.String("storage", "", "Storage that should be used for storing data about processing jobs, can have one of the two values: postgres, cloudspanner")
	cacheDir               = flag.String("cache_dir", "/tmp/", "Path to cache dir used to store local cache.")
	export                 = flag.Bool("export", true, "Whether to export samples, otherwise, they'll be saved to disk")
	exportPath             = flag.String("export_path", "/tmp/hashr-uploads", "If export is set to false, this is the folder where samples will be saved.")
	reprocess              = flag.String("reprocess", "", "Sha256 of sources that should be reprocessed")
	spannerDBPath          = flag.String("spanner_db_path", "", "Path to spanner DB.")
	uploadPayloads         = flag.Bool("upload_payloads", false, "If true the content of the files will be uploaded using defined exporters.")
	gcpExporterWorkerCount = flag.Int("gcp_exporter_worker_count", 100, "Number of workers/goroutines that will be used to upload data to Cloud Spanner.")
	gcpExporterGCSbucket   = flag.String("gcp_exporter_gcs_bucket", "", "Name of the GCS bucket which will be used by GCP exporter to store exported samples.")

	// Postgres DB flags
	postgresHost     = flag.String("postgres_host", "localhost", "PostgreSQL instance address.")
	postgresPort     = flag.Int("postgres_port", 5432, "PostgresSQL instance port.")
	postgresUser     = flag.String("postgres_user", "hashr", "PostgresSQL user.")
	postgresPassword = flag.String("postgres_password", "hashr", "PostgresSQL password.")
	postgresDBName   = flag.String("postgres_db", "hashr", "PostgresSQL database.")
	// WSUS importer flags
	wsusGCSbucket = flag.String("wsus_repo_gcs_bucket", "", "Name of the GCS bucket containing WSUS packages")
	// GCP importer flags
	gcpProjects     = flag.String("gcp_projects", "centos-cloud,cos-cloud,coreos-cloud,debian-cloud,rhel-cloud,suse-cloud,ubuntu-os-cloud,windows-cloud,windows-sql-cloud", "Comma separated list of GCP projects.")
	hashrGCPProject = flag.String("hashr_gcp_project", "", "HashR GCP Project.")
	hashrGCSBucket  = flag.String("hashr_gcs_bucket", "", "HashR GCS bucket used for storing base images.")
	// Windows importer flags
	windowsRepoPath = flag.String("windows_iso_repo_path", "", "Path to Windows ISO repository.")
	// tarGz importer flags
	tarGzRepoPath = flag.String("targz_repo_path", "", "Path to TarGz repository.")
	// deb importer flags
	debRepoPath = flag.String("deb_repo_path", "", "Path to Deb repository.")
	// rpm importer flags
	rpmRepoPath = flag.String("rpm_repo_path", "", "Path to RPM repository.")
	// zip importer flags
	zipRepoPath       = flag.String("zip_repo_path", "", "Path to Zip repository.")
	zipFileExtensions = flag.String("zip_file_exts", "zip", "Comma-separated list of files to treat as Zip files")
	// GCR importer flags
	gcrRepos = flag.String("gcr_repos", "", "Comma separated list of GCR (Google Container Registry) repos.")
	// iso importer flags
	isoRepoPath = flag.String("iso_repo_path", "", "Path to ISO9660 repository.")

	// AWS importer flags
	awsBucket   = flag.String("aws_bucket", "", "HashR S3 bucket")
	awsSSHUser  = flag.String("aws_ssh_user", "ec2-user", "EC2 SSH user")
	awsOsFilter = flag.String("aws_os_filter", "debian,ubuntu", "Comma-separated list of OS filter keywords")
	awsOsArch   = flag.String("aws_os_arch", "x86_64", "Comma-separated list of OS architecture x86_64, arm64, x86_64_mac")
)

func main() {
	ctx := context.Background()
	flag.Parse()
	var importers []hashr.Importer

	if !(*jobStorage == "postgres" || *jobStorage == "cloudspanner") {
		glog.Exit("storage flag needs to have one of the two values: postgres, cloudspanner")
	}

	// Initialize importers.
	for _, importerName := range strings.Split(*importersToRun, ",") {
		switch importerName {
		case windows.RepoName:
			r, err := windows.NewRepo(ctx, *windowsRepoPath)
			if err != nil {
				glog.Exitf("Could not initialize Windows ISO repository: %v", err)
			}
			importers = append(importers, r)
		case wsus.RepoName:
			s, err := storage.NewService(ctx)
			if err != nil {
				glog.Exitf("Could not initialize GCP Storage client: %v", err)
			}
			r, err := wsus.NewRepo(ctx, s, *wsusGCSbucket)
			if err != nil {
				glog.Exitf("Could not initialize WSUS importer: %v", err)
			}
			importers = append(importers, r)
		case gcp.RepoName:
			computeClient, err := compute.NewService(ctx)
			if err != nil {
				glog.Exitf("Could not initialize GCP Compute client: %v", err)
			}

			storageClient, err := storage.NewService(ctx)
			if err != nil {
				glog.Exitf("Could not initialize GCP Storage client: %v", err)
			}

			cloudBuildClient, err := cloudbuild.NewService(ctx)
			if err != nil {
				glog.Exitf("Could not initialize GCP Cloud Build client: %v", err)
			}
			for _, gcpProject := range strings.Split(*gcpProjects, ",") {
				r, err := gcp.NewRepo(ctx, computeClient, storageClient, cloudBuildClient, gcpProject, *hashrGCPProject, *hashrGCSBucket)
				if err != nil {
					glog.Exit(err)
				}
				importers = append(importers, r)
			}
		case targz.RepoName:
			importers = append(importers, targz.NewRepo(*tarGzRepoPath))
		case iso9660.RepoName:
			importers = append(importers, iso9660.NewRepo(*isoRepoPath))
		case deb.RepoName:
			importers = append(importers, deb.NewRepo(*debRepoPath))
		case rpm.RepoName:
			importers = append(importers, rpm.NewRepo(*rpmRepoPath))
		case zip.RepoName:
			importers = append(importers, zip.NewRepo(*zipRepoPath, *zipFileExtensions))
		case gcr.RepoName:
			tokenSource, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
			if err != nil {
				glog.Exit(err)
			}
			for _, gcrRepo := range strings.Split(*gcrRepos, ",") {
				r, err := gcr.NewRepo(ctx, tokenSource, gcrRepo)
				if err != nil {
					glog.Exit(err)
				}
				importers = append(importers, r)
			}
		case awsImporter.RepoName, strings.ToLower(awsImporter.RepoName):
			awsConfig, err := config.LoadDefaultConfig(context.TODO())
			if err != nil {
				glog.Exit(err)
			}

			ec2Client := ec2.NewFromConfig(awsConfig)
			s3Client := s3.NewFromConfig(awsConfig)

			osarchs := strings.Split(*awsOsArch, ",")
			for _, osfilter := range strings.Split(*awsOsFilter, ",") {
				r, err := awsImporter.NewRepo(ctx, ec2Client, s3Client, *awsBucket, *awsSSHUser, osfilter, osarchs)
				if err != nil {
					glog.Exit(err)
				}
				importers = append(importers, r)
			}
		}
	}

	var exporters []hashr.Exporter
	// Initialize exporters.
	for _, exporterName := range strings.Split(*exportersToRun, ",") {
		switch exporterName {
		case postgresExporter.Name:
			psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
				*postgresHost, *postgresPort, *postgresUser, *postgresPassword, *postgresDBName)

			db, err := sql.Open("postgres", psqlInfo)
			if err != nil {
				glog.Exitf("Error initializing Postgres client: %v", err)
			}
			defer db.Close()

			postgresExporter, err := postgresExporter.NewExporter(db, *uploadPayloads)
			if err != nil {
				glog.Exitf("Error initializing Postgres exporter: %v", err)
			}
			exporters = append(exporters, postgresExporter)
		case gcpExporter.Name:
			spannerClient, err := spanner.NewClient(ctx, *spannerDBPath)
			if err != nil {
				glog.Exitf("Error initializing Spanner client: %v", err)
			}

			storageClient, err := storage.NewService(ctx)
			if err != nil {
				glog.Exitf("Could not initialize GCP Storage client: %v", err)
			}

			gceExporter, err := gcpExporter.NewExporter(spannerClient, storageClient, *gcpExporterGCSbucket, *uploadPayloads, *gcpExporterWorkerCount)
			if err != nil {
				glog.Exitf("Error initializing Postgres exporter: %v", err)
			}
			exporters = append(exporters, gceExporter)
		}
	}

	if len(exporters) == 0 && *export {
		glog.Exit("You need to specify at least one exporter.")
	}

	// Initialize job storage.
	var s hashr.Storage
	switch *jobStorage {
	case "postgres":
		psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			*postgresHost, *postgresPort, *postgresUser, *postgresPassword, *postgresDBName)

		db, err := sql.Open("postgres", psqlInfo)
		if err != nil {
			glog.Exitf("Error initializing Postgres client: %v", err)
		}
		defer db.Close()

		s, err = postgres.NewStorage(db)
		if err != nil {
			glog.Exitf("Error initializing Postgres storage: %v", err)
		}
	case "cloudspanner":
		spannerClient, err := spanner.NewClient(ctx, *spannerDBPath)
		if err != nil {
			glog.Exitf("Error initializing Spanner client: %v", err)
		}

		s, err = cloudspanner.NewStorage(ctx, spannerClient)
		if err != nil {
			glog.Exitf("Error initializing Postgres storage: %v", err)
		}
	default:
		glog.Exit("storage flag needs to have one of the two values: postgres, cloudspanner")
	}

	hdb := hashr.New(importers, local.New(), exporters, s)

	hdb.ProcessingWorkerCount = *processingWorkerCount
	hdb.CacheDir = *cacheDir
	hdb.Export = *export
	hdb.ExportPath = *exportPath
	hdb.SourcesForReprocessing = strings.Split(*reprocess, ",")

	if err := hdb.Run(ctx); err != nil {
		glog.Exit(err)
	}
}
