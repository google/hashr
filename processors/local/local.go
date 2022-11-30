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

// Package local provides functions to process data locally.
package local

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/golang/glog"
)

var execute = func(name string, args ...string) *exec.Cmd {
	glog.Infof("name: %v, args: %v", name, args)
	return exec.Command(name, args...)
}

type volume struct {
	id     int
	start  int
	length int
}

// Processor is an instance of local processor.
type Processor struct {
}

var (
	reRow   = regexp.MustCompile(`(?m)^\d{3}:.*`)
	reSpace = regexp.MustCompile(`\s+`)
)

// New returns new local processor instance.
func New() *Processor {
	return &Processor{}
}

func shellCommand(binary string, args ...string) (string, error) {
	cmd := execute(binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("error while executing %s: %v\nStdout: %v\nStderr: %v", binary, err, stdout.String(), stderr.String())
	}

	return stdout.String(), nil
}

// ImageExport runs image_export.py binary locally.
func (p *Processor) ImageExport(sourcePath string) (string, error) {
	// TODO(mlegin): check if image_export.py is present on the local machine.
	baseDir := filepath.Dir(sourcePath)
	exportDir := filepath.Join(baseDir, "export")
	logFile := filepath.Join(baseDir, "image_export.log")

	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", err
	}

	var xfsMountPoints []string

	// If the sourcePath is a file, check for XFS volumes and try to mount them.
	if info.Mode().IsRegular() {
		mmlsOut, err := shellCommand("mmls", sourcePath)
		if err != nil {
			glog.Infof("mmls error (%v), probably not a disk image", err)
		} else {
			xfsv, err := xfsVolumes(sourcePath, parseMmlsOutput(mmlsOut))
			if err != nil {
				glog.Warningf("error reading source image: %v", err)
			}
			xfsMountPoints = mountXfs(sourcePath, baseDir, xfsv)
		}
	}

	// If there is at least one XFS volume that was mounted successfully, we'll pass the mount
	// directory to Plaso.
	if len(xfsMountPoints) > 0 {
		glog.Info("Disk image contains XFS volume(s)")
		defer unmountXfs(xfsMountPoints)
		sourcePath = filepath.Join(baseDir, "mnt")
	}

	args := []string{"run", "-v", "/tmp/:/tmp", "log2timeline/plaso", "image_export", "--logfile", logFile, "--partitions", "all", "--volumes", "all", "-w", exportDir, sourcePath}
	_, err = shellCommand("docker", args...)
	if err != nil {
		return "", fmt.Errorf("error while running Plaso: %v", err)
	}

	return exportDir, nil
}

func unmountXfs(mountPoints []string) {
	for _, mountPoint := range mountPoints {
		_, err := shellCommand("sudo", "umount", mountPoint)
		if err != nil {
			glog.Errorf("error while unmounting volume: %v", err)
		}
	}
}

func mountXfs(imagePath, baseDir string, xfsVolumes []volume) []string {
	var mountPoints []string

	for _, volume := range xfsVolumes {
		mountSubdir := filepath.Join(baseDir, "mnt", fmt.Sprintf("p%d", volume.id))
		if err := os.MkdirAll(mountSubdir, 0755); err != nil {
			glog.Errorf("could not create mount directory: %v", err)
			continue
		}

		_, err := shellCommand("sudo", "mount", "-t", "xfs", "-o", fmt.Sprintf("loop,offset=%d", volume.start*512), imagePath, mountSubdir)
		if err != nil {
			glog.Errorf("error while executing mount cmd: %v", err)
			continue
		}
		mountPoints = append(mountPoints, mountSubdir)
	}

	return mountPoints
}

func xfsVolumes(imagePath string, volumes []volume) ([]volume, error) {
	file, err := os.Open(imagePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var xfsPartitions []volume
	for _, volume := range volumes {
		data := make([]byte, 4)
		_, err := file.ReadAt(data, int64(volume.start*512))
		if err != nil {
			glog.Errorf("Could not read image file: %v", err)
			continue
		}
		// Check if first 4 bytes of the volume matches XFS signature.
		if bytes.Equal(data, []byte{88, 70, 83, 66}) {
			xfsPartitions = append(xfsPartitions, volume)
		}
	}

	return xfsPartitions, nil
}

func parseMmlsOutput(output string) []volume {
	matches := reRow.FindAllString(output, -1)
	var volumes []volume

	for _, match := range matches {
		// Replace multiple, consecutive spaces with one.
		s := reSpace.ReplaceAllString(match, " ")
		// Split the row.
		row := strings.Split(s, " ")

		if len(row) < 5 {
			glog.Warningf("Skipping row %q due to wrong format", row)
			continue
		}

		var start, length, id int
		var err error

		if row[0] == "000:" {
			id = 0
		} else {
			id, err = strconv.Atoi(strings.TrimRight(strings.TrimLeft(row[0], "0"), ":"))
			if err != nil {
				glog.Warningf("Skipping row %q due to error: %v", row, err)
				continue
			}
		}

		if row[2] == "0000000000" {
			start = 0
		} else {
			start, err = strconv.Atoi(strings.TrimLeft(row[2], "0"))
			if err != nil {
				glog.Warningf("Skipping row %q due to error: %v", row, err)
				continue
			}
		}

		if row[4] == "0000000000" {
			length = 0
		} else {
			length, err = strconv.Atoi(strings.TrimLeft(row[4], "0"))
			if err != nil {
				glog.Warningf("Skipping row %q due to error: %v", row, err)
				continue
			}
		}

		volumes = append(volumes, volume{id: id, start: start, length: length})
	}

	return volumes
}
