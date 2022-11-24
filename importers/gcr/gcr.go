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

// Package gcr implements Google Container Repository importer.
package gcr

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	"github.com/google/hashr/core/hashr"
	"github.com/google/hashr/importers/common"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/google"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"golang.org/x/oauth2"
)

const (
	// RepoName contains the repository name.
	RepoName = "gcr"
)

var (
	auth       authn.Authenticator
	opts       google.Option
	remoteOpts []remote.Option
)

// Preprocess extracts the contents of GCR image.
func (i *image) Preprocess() (string, error) {
	imgID := fmt.Sprintf("%s@sha256:%s", i.id, i.quickHash)
	ref, err := name.ParseReference(imgID, name.StrictValidation)
	if err != nil {
		return "", fmt.Errorf("error parsing reference from image %q: %v", imgID, err)
	}

	fmt.Println(remoteOpts)
	// remote.Image(ref, )
	// img, err := remote.Image(ref, remote.WithAuth(auth))
	img, err := remote.Image(ref, remoteOpts...)
	if err != nil {
		return "", fmt.Errorf("error retrieving src image %q: %v", imgID, err)
	}

	layers, err := img.Layers()
	if err != nil {
		return "", fmt.Errorf("error retrieving layers from image %q: %v", imgID, err)
	}

	tmpDir, err := common.LocalTempDir(strings.ReplaceAll(i.id, string(os.PathSeparator), "-"))
	if err != nil {
		return "", fmt.Errorf("error creating temp dir: %v", err)
	}

	i.localPath = filepath.Join(tmpDir, fmt.Sprintf("%s.tar", strings.ReplaceAll(imgID, "/", "_")))

	if err := crane.Save(img, imgID, i.localPath); err != nil {
		return "", fmt.Errorf("error saving src image %q: %v", imgID, err)
	}

	for id, layer := range layers {
		hash, err := layer.Digest()
		if err != nil {
			return "", fmt.Errorf("error retrieving hash layer: %v", err)
		}

		r, err := layer.Compressed()
		if err != nil {
			return "", fmt.Errorf("error downloading layer %d: %v", id, err)
		}

		destFolder := filepath.Join(tmpDir, "extracted", hash.Hex)

		if err := extractTarGz(r, destFolder); err != nil {
			return "", fmt.Errorf("error extracting layer %d: %v", id, err)
		}

		if err := r.Close(); err != nil {
			return "", fmt.Errorf("error closing download for layer %d: %v", id, err)
		}
	}

	return filepath.Join(tmpDir, "extracted"), nil
}

// ID returns non-unique GCR image ID.
func (i *image) ID() string {
	return fmt.Sprintf("%s@sha256:%s", i.id, i.quickHash)
}

// RepoName returns repository name.
func (i *image) RepoName() string {
	return RepoName
}

// RepoPath returns repository path.
func (i *image) RepoPath() string {
	return ""
}

// LocalPath returns local path to a GCR image.
func (i *image) LocalPath() string {
	return i.localPath
}

// RemotePath returns remote path to a GCR image.
func (i *image) RemotePath() string {
	return i.remotePath
}

// QuickSHA256Hash return sha256 hash of a GCR image.
func (i *image) QuickSHA256Hash() (string, error) {
	return i.quickHash, nil
}

// Description provides additional description for GCP image.
func (i *image) Description() string {
	return i.description
}

// NewRepo returns new instance of a GCR repository.
func NewRepo(ctx context.Context, oauth2Token oauth2.TokenSource, repositoryPath string) (*Repo, error) {
	repo, err := name.NewRepository(repositoryPath)
	if err != nil {
		return nil, fmt.Errorf("could not create a new Container Registry repository: %v", err)
	}
	
	auth = google.NewTokenSourceAuthenticator(oauth2Token)
	opts = google.WithAuth(auth)
	remoteOpts = append(remoteOpts, remote.WithAuth(auth))

	return &Repo{path: repositoryPath, gcr: repo}, nil
}

// Repo holds data related to a GCR repository.
type Repo struct {
	path   string
	gcr    name.Repository
	images []*image
}

// RepoName returns repository name.
func (r *Repo) RepoName() string {
	return RepoName
}

// RepoPath returns repository path.
func (r *Repo) RepoPath() string {
	return r.path
}

// DiscoverRepo traverses the GCR repository and return supported images.
func (r *Repo) DiscoverRepo() ([]hashr.Source, error) {
	if err := google.Walk(r.gcr, discoverImages(&r.images), opts); err != nil {
		return nil, fmt.Errorf("error while discovering %s GCR repository: %v", r.path, err)
	}

	var sources []hashr.Source
	for _, image := range r.images {
		sources = append(sources, image)
	}

	return sources, nil
}

type image struct {
	id          string
	localPath   string
	remotePath  string
	quickHash   string
	description string
}

func supportedMedia(mediaType string) bool {
	unsupportedMediaTypes := []string{
		"application/vnd.docker.distribution.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v1+prettyjws",
		"application/vnd.oci.image.manifest.v1+json",
	}

	for _, unsupportedMediaType := range unsupportedMediaTypes {
		if strings.EqualFold(mediaType, unsupportedMediaType) {
			return false
		}
	}

	return true
}

func discoverImages(images *[]*image) google.WalkFunc {
	return func(repo name.Repository, tags *google.Tags, err error) error {
		if err != nil {
			return err
		}

		for digest, manifest := range tags.Manifests {
			if !supportedMedia(manifest.MediaType) {
				continue
			}

			if !strings.Contains(digest, "sha256:") {
				return fmt.Errorf("image digest is not in expected format: %s", digest)
			}

			parts := strings.Split(digest, ":")
			if len(parts[1]) != 64 {
				return fmt.Errorf("image digest is not in expected format: %s", digest)
			}

			*images = append(*images, &image{
				id:          repo.Name(),
				quickHash:   parts[1],
				remotePath:  repo.Name(),
				description: fmt.Sprintf("Tags: %s, Media Type: %s, Created on: %s, Uploaded on: %s", manifest.Tags, manifest.MediaType, manifest.Created.String(), manifest.Uploaded.String()),
			})
		}

		return nil
	}
}

func extractTarGz(r io.Reader, outputFolder string) error {
	if _, err := os.Stat(outputFolder); os.IsNotExist(err) {
		if err2 := os.MkdirAll(outputFolder, 0755); err2 != nil {
			return fmt.Errorf("error while creating target directory: %v", err2)
		}
	}

	gzReader, err := gzip.NewReader(r)
	if err != nil {
		return err
	}

	tarReader := tar.NewReader(gzReader)

	glog.Infof("Extracting to %s", outputFolder)

	for {
		header, err := tarReader.Next()

		switch {
		case err == io.EOF:
			return nil
		case err != nil:
			return err
		}

		if containsDotDot(header.Name) {
			glog.Warningf("not extracting %s, potential path traversal", header.Name)
			continue
		}
		destEntry := filepath.Join(outputFolder, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if _, err := os.Stat(destEntry); os.IsNotExist(err) {
				if err := os.MkdirAll(destEntry, 0755); err != nil {
					return fmt.Errorf("error while creating destination directory: %v", err)
				}
			}
		case tar.TypeReg:
			if _, err := os.Stat(filepath.Dir(destEntry)); os.IsNotExist(err) {
				if err := os.MkdirAll(filepath.Dir(destEntry), 0755); err != nil {
					return fmt.Errorf("error while creating destination directory: %v", err)
				}
			}

			destFile, err := os.Create(destEntry)
			if err != nil {
				return fmt.Errorf("error while creating destination file: %v", err)
			}

			_, err = io.Copy(destFile, tarReader)
			if err != nil {
				return fmt.Errorf("error while extracting destination file: %v", err)
			}
			destFile.Close()
		}
	}
}

func containsDotDot(v string) bool {
	if !strings.Contains(v, "..") {
		return false
	}
	for _, ent := range strings.FieldsFunc(v, isSlashRune) {
		if ent == ".." {
			return true
		}
	}
	return false
}

func isSlashRune(r rune) bool { return r == '/' || r == '\\' }
