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

// Package common provides common functions used by hashR importers.
package common

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
)

// ExtractTarGz extracts tar.gz file to given output folder. If directory does not exist, it will
// be created.
func ExtractTarGz(tarGzPath, outputFolder string) error {
	if _, err := os.Stat(outputFolder); os.IsNotExist(err) {
		if err2 := os.MkdirAll(outputFolder, 0755); err2 != nil {
			return fmt.Errorf("error while creating target directory: %v", err2)
		}
	}

	gzFile, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}
	defer gzFile.Close()

	gzReader, err := gzip.NewReader(gzFile)
	if err != nil {
		return err
	}

	tarReader := tar.NewReader(gzReader)

	glog.Infof("Extracting %s to %s", tarGzPath, outputFolder)

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

// LocalTempDir creates local temporary directory.
func LocalTempDir(sourceID string) (string, error) {
	tempDir, err := ioutil.TempDir("", fmt.Sprintf("hashr-%s-", sourceID))
	if err != nil {
		return "", err
	}

	return tempDir, nil
}

// CopyToLocal copies a source to a local file system.
func CopyToLocal(remotePath, sourceID string) (string, error) {
	tempDir, err := LocalTempDir(sourceID)
	if err != nil {
		return "", err
	}

	sourceFile, err := os.Open(remotePath)
	if err != nil {
		return "", err
	}

	destPath := path.Join(tempDir, filepath.Base(remotePath))
	destFile, err := os.Create(destPath)
	if err != nil {
		return destPath, err
	}

	glog.Infof("Copying %s to %s", sourceID, destPath)

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return destPath, err
	}

	glog.Infof("Done copying %s", sourceID)
	return destPath, nil
}
