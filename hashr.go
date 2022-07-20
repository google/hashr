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
	"github.com/golang/glog"
	"github.com/google/hashr/core/hashr"
	"github.com/google/hashr/importers/gcp"
	"github.com/google/hashr/importers/targz"
	"github.com/google/hashr/importers/windows"
	"github.com/google/hashr/importers/wsus"
	"github.com/google/hashr/processors/local"
	"github.com/google/hashr/storage/cloudspanner"
	"github.com/google/hashr/storage/postgres"

	exporter "github.com/google/hashr/exporters/postgres"
	cloudbuild "google.golang.org/api/cloudbuild/v1"
	compute "google.golang.org/api/compute/v1"
	GCPStorage "google.golang.org/api/storage/v1"
)

var (
	processingWorkerCount = flag.Int("processing_worker_count", 2, "Number of processing workers.")
	importersToRun        = flag.String("importers", strings.Join([]string{}, ","), fmt.Sprintf("Importers to be run: %s,%s,%s,%s", gcp.RepoName, targz.RepoName, windows.RepoName, wsus.RepoName))
	jobStorage            = flag.String("storage", "", "Storage that should be used for storing data about processing jobs, can have one of the two values: postgres, cloudspanner")
	cacheDir              = flag.String("cache_dir", "/tmp/", "Path to cache dir used to store local cache.")
	export                = flag.Bool("export", true, "Whether to export samples, otherwise, they'll be saved to disk")
	exportPath            = flag.String("export_path", "/tmp/hashr-uploads", "If export is set to false, this is the folder where samples will be saved.")
	reprocess             = flag.String("reprocess", "", "Sha256 of sources that should be reprocessed")
	spannerDBPath         = flag.String("spanner_db_path", "", "Path to spanner DB.")
	uploadPayloads        = flag.Bool("upload_payloads", false, "If true the content of the files will be uploaded to Postgres storage.")

	// Postgres DB flags
	postgresHost     = flag.String("postgres_host", "localhost", "PostgreSQL instance address.")
	postgresPort     = flag.Int("postgres_port", 5432, "PostgresSQL instance port.")
	postgresUser     = flag.String("postgres_user", "hashr", "PostgresSQL user.")
	postgresPassword = flag.String("postgres_password", "hashr", "PostgresSQL password.")
	postgresDBName   = flag.String("postgres_db", "hashr", "PostgresSQL database.")
	// WSUS importer flags
	wsusGCSbucket = flag.String("wsus_repo_gcs_bucket", "hashr-wsus", "Name of the GCS bucket containing WSUS packages")
	// GCP importer flags
	gcpProjects     = flag.String("gcp_projects", "centos-cloud,cos-cloud,coreos-cloud,debian-cloud,rhel-cloud,suse-cloud,ubuntu-os-cloud,windows-cloud,windows-sql-cloud", "Comma separated list of GCP projects.")
	hashrGCPProject = flag.String("hashr_gcp_project", "", "HashR GCP Project.")
	hashrGCSBucket  = flag.String("hashr_gcs_bucket", "", "HashR GCS bucket used for storing base images.")
	// Windows importer flags
	windowsRepoPath = flag.String("windows_iso_repo_path", "", "Path to Windows ISO repository.")
	// tarGz importer flags
	tarGzRepoPath = flag.String("targz_repo_path", "", "Path to TarGz repository.")
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
			s, err := GCPStorage.NewService(ctx)
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

			storageClient, err := GCPStorage.NewService(ctx)
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
		}
	}

	// Prepare DB connection.
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		*postgresHost, *postgresPort, *postgresUser, *postgresPassword, *postgresDBName)

	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		glog.Exitf("Error initializing Postgres client: %v", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		glog.Exitf("Error connecting to the Postgres DB: %v", err)
	}

	// Initialze Postgres exporter.
	postgresExporter, err := exporter.NewExporter(db, *uploadPayloads)
	if err != nil {
		glog.Exitf("Error initializing Postgres exporter: %v", err)
	}

	// Initialize job storage.
	var s hashr.Storage
	switch *jobStorage {
	case "postgres":
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

	hdb := hashr.New(importers, local.New(), postgresExporter, s)

	hdb.ProcessingWorkerCount = *processingWorkerCount
	hdb.CacheDir = *cacheDir
	hdb.Export = *export
	hdb.ExportPath = *exportPath
	hdb.SourcesForReprocessing = strings.Split(*reprocess, ",")

	if err := hdb.Run(ctx); err != nil {
		glog.Exit(err)
	}
}
