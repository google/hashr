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

// Package gcp implements GCP repository importer.
package gcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/google/hashr/core/hashr"
	"github.com/google/hashr/importers/common"

	"google.golang.org/api/cloudbuild/v1"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/storage/v1"
)

const (
	// RepoName contains the repository name.
	RepoName     = "GCP"
	buildTimeout = "21600s"
)

var (
	computeClient    = &compute.Service{}
	storageClient    = &storage.Service{}
	cloudBuildClient = &cloudbuild.Service{}
	gcpProject       string
	gcsBucket        string
)

// Image holds data related to GCP image.
type Image struct {
	id              string
	name            string
	project         string
	localTarGzPath  string
	remoteTarGzPath string
	quickSha256hash string
	description     string
}

// Preprocess creates tar.gz file from an image, copies to local storage and extracts it.
func (i *Image) Preprocess() (string, error) {
	if err := i.copy(); err != nil {
		return "", fmt.Errorf("error while copying image %s to %s GCP project: %v", i.name, gcpProject, err)
	}

	if err := i.export(); err != nil {
		return "", fmt.Errorf("error while exporting image %s to %s GCS bucket: %v", i.name, gcsBucket, err)
	}

	if err := i.cleanup(); err != nil {
		glog.Warningf("error while deleting image %s: %v", i.name, err)
	}

	if err := i.download(); err != nil {
		return "", fmt.Errorf("error while downloading image %s to local storage: %v", i.name, err)
	}

	baseDir, _ := filepath.Split(i.localTarGzPath)
	extractionDir := filepath.Join(baseDir, "extracted")

	if err := common.ExtractTarGz(i.localTarGzPath, extractionDir); err != nil {
		return "", fmt.Errorf("error while downloading image %s to local storage: %v", i.name, err)
	}

	return filepath.Join(extractionDir, "disk.raw"), nil
}

// ID returns non-unique GCP image ID.
func (i *Image) ID() string {
	return i.id
}

// RepoName returns repository name.
func (i *Image) RepoName() string {
	return RepoName
}

// RepoPath returns repository path.
func (i *Image) RepoPath() string {
	return i.project
}

// LocalPath returns local path to a GCP image .tar.gz raw disk file.
func (i *Image) LocalPath() string {
	return i.localTarGzPath
}

// RemotePath returns remote path to a GCP image .tar.gz raw disk file.
func (i *Image) RemotePath() string {
	return fmt.Sprintf("gs://%s/%s-%s.tar.gz", gcsBucket, i.project, i.name)
}

// Description provides additional description for GCP image.
func (i *Image) Description() string {
	return i.description
}

// QuickSHA256Hash returns sha256 of custom properties of a GCP image.
func (i *Image) QuickSHA256Hash() (string, error) {
	// Check if the quick hash was already calculated.
	if i.quickSha256hash != "" {
		return i.quickSha256hash, nil
	}

	image, err := computeClient.Images.Get(i.project, i.name).Do()
	if err != nil {
		return "", err
	}

	data := [][]byte{
		[]byte(strconv.Itoa(int(image.Id))),
		[]byte(image.Name),
		[]byte(strconv.Itoa(int(image.ArchiveSizeBytes))),
		[]byte(image.CreationTimestamp),
		[]byte(strconv.Itoa(int(image.DiskSizeGb))),
	}

	var hashBytes []byte

	for _, bytes := range data {
		hashBytes = append(hashBytes, bytes...)
	}

	i.quickSha256hash = fmt.Sprintf("%x", sha256.Sum256(hashBytes))

	return i.quickSha256hash, nil
}

// Repo holds data related to a GCP repository.
type Repo struct {
	projectName string
	images      []*Image
}

// NewRepo returns new instance of GCP repository.
func NewRepo(ctx context.Context, computeService *compute.Service, storageService *storage.Service, cloudBuildService *cloudbuild.Service, projectName, hashrGCPProject, hashrGCSBucket string) (*Repo, error) {
	gcpProject = hashrGCPProject
	gcsBucket = hashrGCSBucket

	// Rename/move below
	computeClient = computeService
	storageClient = storageService
	cloudBuildClient = cloudBuildService

	return &Repo{projectName: projectName}, nil
}

// RepoName returns repository name.
func (r *Repo) RepoName() string {
	return RepoName
}

// RepoPath returns repository path.
func (r *Repo) RepoPath() string {
	return r.projectName
}

// DiscoverRepo traverses GCP project and looks for images.
func (r *Repo) DiscoverRepo() ([]hashr.Source, error) {
	req := computeClient.Images.List(r.projectName)
	if err := req.Pages(context.Background(), func(page *compute.ImageList) error {
		for _, image := range page.Items {
			if image.Deprecated != nil {
				glog.Infof("Image %s is deprecated, skipping.", image.Name)
				continue
			}
			r.images = append(r.images, &Image{id: fmt.Sprintf("%s-%s", r.projectName, image.Name), name: image.Name, project: r.projectName, description: image.Description})
		}
		return nil
	}); err != nil {
		return nil, err
	}

	var sources []hashr.Source
	for _, image := range r.images {
		sources = append(sources, image)
	}

	return sources, nil
}

func (i *Image) copy() error {
	sourceURL := fmt.Sprintf("projects/%s/global/images/%s", i.project, i.name)
	targetURL := fmt.Sprintf("projects/%s/global/images/%s", gcpProject, i.name)
	image := &compute.Image{
		SourceImage: sourceURL,
		Name:        i.name,
	}

	glog.Infof("Copying %s to %s", sourceURL, targetURL)
	op, err := computeClient.Images.Insert(gcpProject, image).Do()
	if err != nil {
		return err
	}

	for {
		time.Sleep(10 * time.Second)
		o, err := computeClient.GlobalOperations.Get(gcpProject, op.Name).Do()
		if err != nil {
			return err
		}
		if o.Status == "DONE" {
			if o.Error != nil {
				var errorMsgs []string
				for _, e := range o.Error.Errors {
					errorMsgs = append(errorMsgs, fmt.Sprintf("code: %v, message: %v", e.Code, e.Message))
				}
				return fmt.Errorf("error(s) while copying image: \n%s", strings.Join(errorMsgs, "\n"))
			}
			glog.Infof("Done copying %s to %s", sourceURL, targetURL)
			return nil
		}
		glog.Infof("Operation %s status: %s", op.Name, o.Status)
	}
}

func (i *Image) cleanup() error {
	glog.Infof("Deleting image %s from %s", i.name, gcpProject)
	_, err := computeClient.Images.Delete(gcpProject, i.name).Do()
	if err != nil {
		return err
	}

	return nil
}

func (i *Image) export() error {
	return RunImageExportBuild(cloudBuildClient, i.project, i.name, gcpProject, gcsBucket)
}

// RunImageExportBuild runs Cloud build to create a .tar.gz file containing disk image from a cloud image.
func RunImageExportBuild(cloudBuildClient *cloudbuild.Service, sourceProjectName, sourceImageName, buildProjectName, targetGCSbucket string) error {
	build := &cloudbuild.Build{
		Timeout:   buildTimeout,
		Id:        fmt.Sprintf("%s-%s", sourceProjectName, sourceImageName),
		ProjectId: buildProjectName,
		Steps: []*cloudbuild.BuildStep{
			{
				Args: []string{
					"-client_id=api",
					fmt.Sprintf("-timeout=%s", buildTimeout),
					fmt.Sprintf("-source_image=%s", sourceImageName),
					fmt.Sprintf("-destination_uri=gs://%s/%s-%s.tar.gz", targetGCSbucket, sourceProjectName, sourceImageName),
				},
				Name: "gcr.io/compute-image-tools/gce_vm_image_export:release",
				Env:  []string{"BUILD_ID=$BUILD_ID"},
			},
		},
		Tags: []string{"gce-daisy", "gce-daisy-image-export"},
	}

	glog.Infof("Exporting %s", sourceImageName)
	op, err := cloudBuildClient.Projects.Builds.Create(buildProjectName, build).Do()
	if err != nil {
		return err
	}

	if op.Error != nil {
		return fmt.Errorf("error while creating build: %v", op.Error)
	}

	var metadata cloudbuild.BuildOperationMetadata
	if err := json.Unmarshal(op.Metadata, &metadata); err != nil {
		return fmt.Errorf("error unmarshalling cloudbuild metadata: %v", op.Metadata)
	}

	glog.Infof("Started Cloud Build. ID: %s", metadata.Build.Id)
	glog.Infof("Cloud Console Logs: %s", metadata.Build.LogUrl)

	for {
		time.Sleep(10 * time.Second)
		o, err := cloudBuildClient.Operations.Get(op.Name).Do()
		if err != nil {
			return err
		}
		if o.Done {
			var m cloudbuild.BuildOperationMetadata
			if err := json.Unmarshal(o.Metadata, &m); err != nil {
				return fmt.Errorf("error unmarshalling cloudbuild metadata: %v", op.Metadata)
			}

			bo, err := cloudBuildClient.Projects.Builds.Get(buildProjectName, m.Build.Id).Do()
			if err != nil {
				return err
			}
			if bo.Status != "SUCCESS" {
				return fmt.Errorf("build terminated in status %s, logs: %s", bo.Status, bo.LogUrl)
			}
			glog.Infof("Build completed. Id:%v, status:%v, logs:%v", m.Build.Id, bo.Status, bo.LogUrl)
			break
		}
		glog.Infof("Build %s in progress.", op.Name)
	}

	return nil
}

func (i *Image) download() error {
	imageFile := fmt.Sprintf("%s-%s.tar.gz", i.project, i.name)
	i.remoteTarGzPath = filepath.Join()

	resp, err := storageClient.Objects.Get(gcsBucket, imageFile).Download()
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	tempDir, err := common.LocalTempDir(i.ID())
	if err != nil {
		return err
	}

	i.localTarGzPath = filepath.Join(tempDir, imageFile)
	out, err := os.Create(i.localTarGzPath)
	if err != nil {
		return fmt.Errorf("error while creating %s: %v", i.localTarGzPath, err)
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("error while writing to %s: %v", i.localTarGzPath, err)
	}

	glog.Infof("Image %s successfully written to %s", i.name, i.localTarGzPath)

	return nil
}
