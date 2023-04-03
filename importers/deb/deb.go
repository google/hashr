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

// Package deb implements deb package importer.
package deb

import (
	"archive/tar"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/glog"

	"github.com/google/hashr/core/hashr"
	"github.com/google/hashr/importers/common"

	"pault.ag/go/debian/deb"
)

const (
	// RepoName contains the repository name.
	RepoName  = "deb"
	chunkSize = 1024 * 1024 * 10 // 10MB
)

// Archive holds data related to deb archive.
type Archive struct {
	filename        string
	remotePath      string
	localPath       string
	quickSha256hash string
	repoPath        string
}

func isSubElem(parent, sub string) (bool, error) {
	up := ".." + string(os.PathSeparator)

	// path-comparisons using filepath.Abs don't work reliably according to docs (no unique representation).
	rel, err := filepath.Rel(parent, sub)
	if err != nil {
		return false, err
	}
	if !strings.HasPrefix(rel, up) && rel != ".." {
		return true, nil
	}
	return false, nil
}

func extractTar(tarfile *tar.Reader, outputFolder string) error {
	for {
		header, err := tarfile.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf("error while unpacking deb package: %v", err)
		}

		name := header.Name

		switch header.Typeflag {
		case tar.TypeSymlink:
			continue

		case tar.TypeDir:
			continue

		case tar.TypeRegA:
		case tar.TypeReg:
			unpackPath := filepath.Join(outputFolder, name)
			unpackFolder := filepath.Dir(unpackPath)
			if _, err := os.Stat(unpackFolder); os.IsNotExist(err) {
				if err2 := os.MkdirAll(unpackFolder, 0755); err2 != nil {
					return fmt.Errorf("error while creating target directory: %v", err2)
				}
			}

			fileIsSubelem, err := isSubElem(outputFolder, unpackPath)
			if err != nil || !fileIsSubelem {
				return fmt.Errorf("error, deb package tried to unpack file above parent")
			}

			unpackFileHandle, err := os.Create(unpackPath)
			if err != nil {
				return fmt.Errorf("error while creating destination file: %v", err)
			}
			defer unpackFileHandle.Close()
			_, err = io.Copy(unpackFileHandle, tarfile)
			if err != nil {
				return fmt.Errorf("error while writing to destination file: %v", err)
			}

		default:
			fmt.Printf("Unknown tar entry type: %c in file %s\n", header.Typeflag, name)
		}
	}

	return nil
}

func extractDeb(debPath, outputFolder string) error {
	if _, err := os.Stat(outputFolder); os.IsNotExist(err) {
		if err2 := os.MkdirAll(outputFolder, 0755); err2 != nil {
			return fmt.Errorf("error while creating target directory: %v", err2)
		}
	}

	fd, err := os.Open(debPath)
	if err != nil {
		return fmt.Errorf("failed to open deb file: %v", err)
	}
	defer fd.Close()

	debFile, err := deb.Load(fd, debPath)
	if err != nil {
		return fmt.Errorf("failed to parse deb file: %v", err)
	}

	err = extractTar(debFile.Data, outputFolder)
	if err != nil {
		return err
	}

	return nil
}

// Preprocess extracts the contents of a .deb file.
func (a *Archive) Preprocess() (string, error) {
	var err error
	a.localPath, err = common.CopyToLocal(a.remotePath, a.ID())
	if err != nil {
		return "", fmt.Errorf("error while copying %s to local file system: %v", a.remotePath, err)
	}

	baseDir, _ := filepath.Split(a.localPath)
	extractionDir := filepath.Join(baseDir, "extracted")

	if err := extractDeb(a.localPath, extractionDir); err != nil {
		return "", err
	}

	return extractionDir, nil
}

// ID returns non-unique deb Archive ID.
func (a *Archive) ID() string {
	return a.filename
}

// RepoName returns repository name.
func (a *Archive) RepoName() string {
	return RepoName
}

// RepoPath returns repository path.
func (a *Archive) RepoPath() string {
	return a.repoPath
}

// LocalPath returns local path to a deb Archive .deb file.
func (a *Archive) LocalPath() string {
	return a.localPath
}

// RemotePath returns non-local path to a deb Archive .deb file.
func (a *Archive) RemotePath() string {
	return a.remotePath
}

// Description provides additional description for a .deb file.
func (a *Archive) Description() string {
	return ""
}

// QuickSHA256Hash calculates sha256 hash of .deb file.
func (a *Archive) QuickSHA256Hash() (string, error) {
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

// NewRepo returns new instance of deb repository.
func NewRepo(path string) *Repo {
	return &Repo{location: path}
}

// Repo holds data related to a deb repository.
type Repo struct {
	location string
	files    []string
	Archives []*Archive
}

// RepoName returns repository name.
func (r *Repo) RepoName() string {
	return RepoName
}

// RepoPath returns repository path.
func (r *Repo) RepoPath() string {
	return r.location
}

// DiscoverRepo traverses the repository and looks for files that are related to deb archives.
func (r *Repo) DiscoverRepo() ([]hashr.Source, error) {

	if err := filepath.Walk(r.location, walk(&r.files)); err != nil {
		return nil, err
	}

	for _, file := range r.files {
		_, filename := filepath.Split(file)

		if strings.HasSuffix(filename, ".deb") {
			r.Archives = append(r.Archives, &Archive{filename: filename, remotePath: file, repoPath: r.location})
		}
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
		if strings.HasSuffix(info.Name(), ".deb") {
			*files = append(*files, path)
		}

		return nil
	}
}
