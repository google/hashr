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

// Package windows implements Windows ISO-13346 repository importer.
package windows

import (
	"context"

	"github.com/google/hashr/core/hashr"
)

const (
	// RepoName contains the repository name.
	RepoName = "windows"
)

type image struct {
	id         string
	localPath  string
	remotePath string
	quickHash  string
}

// Preprocess extracts the contents of Windows ISO file.
func (i *image) Preprocess() (string, error) {
	return "", nil
}

// ID returns non-unique Windows ISO file ID.
func (i *image) ID() string {
	return i.id
}

// RepoName returns repository name.
func (i *image) RepoName() string {
	return RepoName
}

// RepoPath returns repository path.
func (i *image) RepoPath() string {
	return ""
}

// LocalPath returns local path to a Windows ISO file.
func (i *image) LocalPath() string {
	return i.localPath
}

// RemotePath returns remote path to a Windows ISO file.
func (i *image) RemotePath() string {
	return i.remotePath
}

// QuickSHA256Hash calculates sha256 hash of a Windows Update file metadata.
func (i *image) QuickSHA256Hash() (string, error) {
	return i.quickHash, nil
}

// NewRepo returns new instance of a Windows ISO repository.
func NewRepo(ctx context.Context, repositoryPath string) (*Repo, error) {
	return &Repo{path: repositoryPath}, nil
}

// Repo holds data related to a Windows WSUS repository.
type Repo struct {
	path string
}

// RepoName returns repository name.
func (r *Repo) RepoName() string {
	return RepoName
}

// RepoPath returns repository path.
func (r *Repo) RepoPath() string {
	return r.path
}

// DiscoverRepo traverses the repository and looks for files that are related to WSUS packages.
func (r *Repo) DiscoverRepo() ([]hashr.Source, error) {
	var sources []hashr.Source
	return sources, nil
}
