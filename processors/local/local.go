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
	"os/exec"
	"path/filepath"
	"regexp"

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

	args := []string{"run", "--rm", "-v", "/tmp/:/tmp", "log2timeline/plaso", "image_export", "--logfile", logFile, "--partitions", "all", "--volumes", "all", "-w", exportDir, sourcePath}
	_, err := shellCommand("docker", args...)
	if err != nil {
		return "", fmt.Errorf("error while running Plaso: %v", err)
	}

	return exportDir, nil
}
