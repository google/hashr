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

// Package iso9660 implements iso9660 repository importer.
package iso9660

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/hooklift/iso9660"

	"github.com/golang/glog"

	"github.com/google/hashr/core/hashr"
	"github.com/google/hashr/importers/common"
)

const (
	// RepoName contains the repository name.
	RepoName  = "iso9660"
	chunkSize = 1024 * 1024 * 10 // 10MB
)

// Archive holds data related to the ISO file.
type ISO9660 struct {
	filename        string
	remotePath      string
	localPath       string
	quickSha256hash string
	repoPath        string
}

// Preprocess extracts the contents of a .tar.gz file.
func (a *ISO9660) Preprocess() (string, error) {
	var err error
	a.localPath, err = common.CopyToLocal(a.remotePath, a.ID())
	if err != nil {
		return "", fmt.Errorf("error while copying %s to local file system: %v", a.remotePath, err)
	}

	baseDir, _ := filepath.Split(a.localPath)
	extractionDir := filepath.Join(baseDir, "extracted")

	if err := extractIso(a.localPath, extractionDir); err != nil {
		return "", err
	}

	return extractionDir, nil
}

func extractIso(isoPath, outputFolder string) error {
	if _, err := os.Stat(outputFolder); os.IsNotExist(err) {
		if err2 := os.MkdirAll(outputFolder, 0755); err2 != nil {
			return fmt.Errorf("error while creating target directory: %v", err2)
		}
	}

	// Step 1: Open ISO reader
	file, err := os.Open(isoPath)
	if err != nil {
		return fmt.Errorf("error opening ISO file: %v", err)
	}

	r, err := iso9660.NewReader(file)
	if err != nil {
		return fmt.Errorf("error parsing ISO file: %v", err)
	}

	// 2. Get the absolute destination path
	outputFolder, err = filepath.Abs(outputFolder)
	if err != nil {
		return err
	}

	// Step 3: Iterate over files
	for {
		f, err := r.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf("error retrieving next file from ISO: %v", err)
		}

		err = unpackFile(f, outputFolder)
		if err != nil {
			return err
		}
	}

	return nil
}

func unpackFile(f fs.FileInfo, destination string) error {
	// Step 4: Create output path
	fp := filepath.Join(destination, f.Name())
	if f.IsDir() {
		if err := os.MkdirAll(fp, f.Mode()); err != nil {
			return fmt.Errorf("error creating destination directory: %v", err)
		}
		return nil
	}

	parentDir, _ := filepath.Split(fp)
	if err := os.MkdirAll(parentDir, f.Mode()); err != nil {
		return fmt.Errorf("error while creating target directory: %v", err)
	}

	// Step 5: Create destination file
	freader := f.Sys().(io.Reader)
	ff, err := os.Create(fp)
	if err != nil {
		fmt.Errorf("error while creating destination file: %v", err)
	}
	defer func() {
		if err := ff.Close(); err != nil {
			fmt.Errorf("error while closing file: %v", err)
		}
	}()

	if err := ff.Chmod(f.Mode()); err != nil {
		fmt.Errorf("error while chmod: %v", err)
	}

	// Step 6: Extract file contents
	if _, err := io.Copy(ff, freader); err != nil {
		fmt.Errorf("error while extracting file data: %v", err)
	}
	return nil
}

// ID returns non-unique ISO file Archive ID.
func (a *ISO9660) ID() string {
	return a.filename
}

// RepoName returns repository name.
func (a *ISO9660) RepoName() string {
	return RepoName
}

// RepoPath returns repository path.
func (a *ISO9660) RepoPath() string {
	return a.repoPath
}

// LocalPath returns local path to a ISO file Archive .iso file.
func (a *ISO9660) LocalPath() string {
	return a.localPath
}

// RemotePath returns non-local path to a ISO file Archive .iso file.
func (a *ISO9660) RemotePath() string {
	return a.remotePath
}

// Description provides additional description for a .iso file.
func (a *ISO9660) Description() string {
	return ""
}

// QuickSHA256Hash calculates sha256 hash of .iso file.
func (a *ISO9660) QuickSHA256Hash() (string, error) {
	// Check if the quick hash was already calculated.
	if a.quickSha256hash != "" {
		return a.quickSha256hash, nil
	}

	f, err := os.Open(a.remotePath)
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
		a.quickSha256hash = fmt.Sprintf("%x", h.Sum(nil))
		return a.quickSha256hash, nil
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

	a.quickSha256hash = fmt.Sprintf("%x", sha256.Sum256(append(header, footer...)))
	return a.quickSha256hash, nil
}

// NewRepo returns new instance of an ISO file repository.
func NewRepo(path string) *Repo {
	return &Repo{location: path}
}

// Repo holds data related to an ISO file repository.
type Repo struct {
	location string
	files    []string
	Archives []*ISO9660
}

// RepoName returns repository name.
func (r *Repo) RepoName() string {
	return RepoName
}

// RepoPath returns repository path.
func (r *Repo) RepoPath() string {
	return r.location
}

// DiscoverRepo traverses the repository and looks for files that are related to ISO file base Archives.
func (r *Repo) DiscoverRepo() ([]hashr.Source, error) {
	if err := filepath.Walk(r.location, walk(&r.files)); err != nil {
		return nil, err
	}

	for _, file := range r.files {
		_, filename := filepath.Split(file)

		r.Archives = append(r.Archives, &ISO9660{filename: filename, remotePath: file, repoPath: r.location})
	}

	var sources []hashr.Source
	for _, Archive := range r.Archives {
		sources = append(sources, Archive)
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

		if strings.HasSuffix(info.Name(), ".iso") {
			*files = append(*files, path)
		}

		return nil
	}
}
