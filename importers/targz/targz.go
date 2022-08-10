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

// Package targz implements targz repository importer.
package targz

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/glog"

	"github.com/google/hashr/core/hashr"
	"github.com/google/hashr/importers/common"
)

const (
	// RepoName contains the repository name.
	RepoName  = "targz"
	chunkSize = 1024 * 1024 * 10 // 10MB
)

// TarGzFile holds data related to targz archive.
type TarGzFile struct {
	filename        string
	remotePath      string
	localPath       string
	quickSha256hash string
	repoPath        string
}

// Preprocess extracts the contents of a .tar.gz file.
func (i *TarGzFile) Preprocess() (string, error) {
	var err error
	i.localPath, err = common.CopyToLocal(i.remotePath, i.ID())
	if err != nil {
		return "", fmt.Errorf("error while copying %s to local file system: %v", i.remotePath, err)
	}

	baseDir, _ := filepath.Split(i.localPath)
	extractionDir := filepath.Join(baseDir, "extracted")

	if err := common.ExtractTarGz(i.localPath, extractionDir); err != nil {
		return "", err
	}

	return extractionDir, nil
}

// ID returns non-unique targz TarGzFile ID.
func (i *TarGzFile) ID() string {
	return i.filename
}

// RepoName returns repository name.
func (i *TarGzFile) RepoName() string {
	return RepoName
}

// RepoPath returns repository path.
func (i *TarGzFile) RepoPath() string {
	return i.repoPath
}

// LocalPath returns local path to a targz TarGzFile .tar.gz file.
func (i *TarGzFile) LocalPath() string {
	return i.localPath
}

// RemotePath returns non-local path to a targz TarGzFile .tar.gz file.
func (i *TarGzFile) RemotePath() string {
	return i.remotePath
}

// QuickSHA256Hash calculates sha256 hash of .tar.gz file.
func (i *TarGzFile) QuickSHA256Hash() (string, error) {
	// Check if the quick hash was already calculated.
	if i.quickSha256hash != "" {
		return i.quickSha256hash, nil
	}

	f, err := os.Open(i.remotePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	fileInfo, err := f.Stat()
	if err != nil {
		return "", err
	}

	// Check if the file is smaller than 20MB, if so hash the whole file.
	if fileInfo.Size() < int64(chunkSize*2) {
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return "", err
		}
		i.quickSha256hash = fmt.Sprintf("%x", h.Sum(nil))
		return i.quickSha256hash, nil
	}

	header := make([]byte, chunkSize)
	_, err = f.Read(header)
	if err != nil {
		return "", err
	}

	footer := make([]byte, chunkSize)
	_, err = f.ReadAt(footer, fileInfo.Size()-int64(chunkSize))
	if err != nil {
		return "", err
	}

	i.quickSha256hash = fmt.Sprintf("%x", sha256.Sum256(append(header, footer...)))
	return i.quickSha256hash, nil
}

// NewRepo returns new instance of targz repository.
func NewRepo(path string) *Repo {
	return &Repo{location: path}
}

// Repo holds data related to a targz repository.
type Repo struct {
	name       string
	location   string
	files      []string
	TarGzFiles []*TarGzFile
}

// RepoName returns repository name.
func (r *Repo) RepoName() string {
	return RepoName
}

// RepoPath returns repository path.
func (r *Repo) RepoPath() string {
	return r.location
}

// DiscoverRepo traverses the repository and looks for files that are related to targz base TarGzFiles.
func (r *Repo) DiscoverRepo() ([]hashr.Source, error) {

	if err := filepath.Walk(r.location, walk(&r.files)); err != nil {
		return nil, err
	}

	for _, file := range r.files {
		_, filename := filepath.Split(file)

		if strings.HasSuffix(filename, ".tar.gz") {
			r.TarGzFiles = append(r.TarGzFiles, &TarGzFile{filename: filename, remotePath: file, repoPath: r.location})
		}
	}

	var sources []hashr.Source
	for _, TarGzFile := range r.TarGzFiles {
		sources = append(sources, TarGzFile)
	}

	return sources, nil
}

func walk(files *[]string) filepath.WalkFunc {
	return func(path string, info os.FileInfo, err error) error {
		if err != nil {
			glog.Errorf("Could not open %s: %v", path, err)
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".tar.gz") || strings.HasSuffix(info.Name(), ".tar.gz.sig") {
			*files = append(*files, path)
		}

		return nil
	}
}
