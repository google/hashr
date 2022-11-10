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

package local

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseMmlsOutput(t *testing.T) {
	cases := []struct {
		output string
		want   []volume
	}{
		{
			output: `DOS Partition Table
Offset Sector: 0
Units are in 512-byte sectors

      Slot      Start        End          Length       Description
000:  Meta      0000000000   0000000000   0000000001   Primary Table (#0)
001:  -------   0000000000   0000002047   0000002048   Unallocated
002:  000:000   0000002048   0020971519   0020969472   Linux (0x83)`,
			want: []volume{
				{id: 0, start: 0, length: 1},
				{id: 1, start: 0, length: 2048},
				{id: 2, start: 2048, length: 20969472},
			},
		},
		{
			output: `GUID Partition Table (EFI)
Offset Sector: 0
Units are in 512-byte sectors

      Slot      Start        End          Length       Description
000:  Meta      0000000000   0000000000   0000000001   Safety Table
001:  -------   0000000000   0000000063   0000000064   Unallocated
002:  Meta      0000000001   0000000001   0000000001   GPT Header
003:  Meta      0000000002   0000000033   0000000032   Partition Table
004:  010       0000000064   0000016447   0000016384   RWFW
005:  005       0000016448   0000016448   0000000001   KERN-C
006:  006       0000016449   0000016449   0000000001   ROOT-C
007:  008       0000016450   0000016450   0000000001   reserved
008:  009       0000016451   0000016451   0000000001   reserved
009:  -------   0000016452   0000020479   0000004028   Unallocated
010:  001       0000020480   0000053247   0000032768   KERN-A
011:  003       0000053248   0000086015   0000032768   KERN-B
012:  007       0000086016   0000118783   0000032768   OEM
013:  -------   0000118784   0000249855   0000131072   Unallocated
014:  011       0000249856   0000315391   0000065536   EFI-SYSTEM
015:  004       0000315392   0004509695   0004194304   ROOT-B
016:  002       0004509696   0008703999   0004194304   ROOT-A
017:  000       0008704000   0018874476   0010170477   STATE
018:  -------   0018874477   0020971519   0002097043   Unallocated`,
			want: []volume{
				{id: 0, start: 0, length: 1},
				{id: 1, start: 0, length: 64},
				{id: 2, start: 1, length: 1},
				{id: 3, start: 2, length: 32},
				{id: 4, start: 64, length: 16384},
				{id: 5, start: 16448, length: 1},
				{id: 6, start: 16449, length: 1},
				{id: 7, start: 16450, length: 1},
				{id: 8, start: 16451, length: 1},
				{id: 9, start: 16452, length: 4028},
				{id: 10, start: 20480, length: 32768},
				{id: 11, start: 53248, length: 32768},
				{id: 12, start: 86016, length: 32768},
				{id: 13, start: 118784, length: 131072},
				{id: 14, start: 249856, length: 65536},
				{id: 15, start: 315392, length: 4194304},
				{id: 16, start: 4509696, length: 4194304},
				{id: 17, start: 8704000, length: 10170477},
				{id: 18, start: 18874477, length: 2097043},
			},
		},
		{
			output: `GUID Partition Table (EFI)
Offset Sector: 0
Units are in 512-byte sectors

      Slot      Start        End          Length       Description
000:  Meta      0000000000   0000000000   0000000001   Safety Table
001:  -------   0000000000   0000002047   0000002048   Unallocated
002:  Meta      0000000001   0000000001   0000000001   GPT Header
003:  Meta      0000000002   0000000033   0000000032   Partition Table
004:  000       0000002048   0000006143   0000004096   p.legacy
005:  001       0000006144   0000047103   0000040960   p.UEFI
006:  002       0000047104   0020971486   0020924383   p.lxroot
007:  -------   0020971487   0020971519   0000000033   Unallocated`,
			want: []volume{
				{id: 0, start: 0, length: 1},
				{id: 1, start: 0, length: 2048},
				{id: 2, start: 1, length: 1},
				{id: 3, start: 2, length: 32},
				{id: 4, start: 2048, length: 4096},
				{id: 5, start: 6144, length: 40960},
				{id: 6, start: 47104, length: 20924383},
				{id: 7, start: 20971487, length: 33},
			},
		},
		{
			output: `GUID Partition Table (EFI)
Offset Sector: 0
Units are in 512-byte sectors

      Slot      Start        End          Length       Description
000:  Meta      0000000000   0000000000   0000000001   Safety Table
001:  -------   0000000000   0000002047   0000002048   Unallocated
002:  Meta      0000000001   0000000001   0000000001   GPT Header
003:  Meta      0000000002   0000000033   0000000032   Partition Table
004:  000       0000002048   0000411647   0000409600   EFI System Partition
005:  001       0000411648   0041940991   0041529344
006:  -------   0041940992   0041943039   0000002048   Unallocated`,
			want: []volume{
				{id: 0, start: 0, length: 1},
				{id: 1, start: 0, length: 2048},
				{id: 2, start: 1, length: 1},
				{id: 3, start: 2, length: 32},
				{id: 4, start: 2048, length: 409600},
				{id: 5, start: 411648, length: 41529344},
				{id: 6, start: 41940992, length: 2048},
			},
		},
	}

	for _, tc := range cases {
		got := parseMmlsOutput(tc.output)

		if !cmp.Equal(tc.want, got, cmp.AllowUnexported(volume{})) {
			t.Errorf("parseMmlsOutput() unexpected diff (-want/+got):\n%s", cmp.Diff(tc.want, got, cmp.AllowUnexported(volume{})))
		}
	}
}

func TestXfsVolumes(t *testing.T) {
	gotVolumes, err := xfsVolumes("testdata/disk_2_xfs_volumes.raw", []volume{{id: 1, start: 2048, length: 6144}, {id: 2, start: 8192, length: 4096}, {id: 3, start: 12288, length: 6144}})
	if err != nil {
		t.Fatalf("unexpected error while running xfsVolumes(): %v", err)
	}

	wantVolumes := []volume{
		{id: 1, start: 2048, length: 6144},
		{id: 3, start: 12288, length: 6144},
	}
	if !cmp.Equal(wantVolumes, gotVolumes, cmp.AllowUnexported(volume{})) {
		t.Errorf("xfsVolumes() unexpected diff (-want/+got):\n%s", cmp.Diff(wantVolumes, gotVolumes, cmp.AllowUnexported(volume{})))
	}
}

func TestExecute(t *testing.T) {
	bytes, err := execute("echo", "test").Output()
	if err != nil {
		t.Fatalf("unexpected error while running test echo cmd: %v", err)
	}
	if got, want := string(bytes), "test\n"; got != want {
		t.Errorf("echo = %s; want = %s", got, want)
	}
}

func TestImageExport(t *testing.T) {
	execute = fakeExecute
	tempDir, err := ioutil.TempDir("", "hashr-test")
	if err != nil {
		t.Fatalf("error while creating temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	sourceFile, err := os.Open("testdata/disk_2_xfs_volumes.raw")
	if err != nil {
		t.Fatalf("unexpected error while opening test WIM file: %v", err)
	}

	xfsTempPath := filepath.Join(tempDir, "disk_2_xfs_volumes.raw")
	destFile, err := os.Create(xfsTempPath)
	if err != nil {
		t.Fatalf("unexpected error creating temp destination file: %v", err)
	}

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		t.Fatalf("unexpected error while copying to temp destination file: %v", err)
	}

	processor := New()
	gotOut, err := processor.ImageExport(xfsTempPath)
	if err != nil {
		t.Fatalf("unexpected error while running ImageExport(): %v", err)
	}

	wantOut := filepath.Join(tempDir, "export")

	if gotOut != wantOut {
		t.Errorf("ImageExport() = %s; want = %s", gotOut, wantOut)
	}

}

var mmlsOut = `DOS Partition Table
Offset Sector: 0
Units are in 512-byte sectors

      Slot      Start        End          Length       Description
000:  Meta      0000000000   0000000000   0000000001   Primary Table (#0)
001:  -------   0000000000   0000002047   0000002048   Unallocated
002:  000:000   0000002048   0000008191   0000006144   Linux (0x83)
003:  000:001   0000008192   0000012287   0000004096   Linux (0x83)
004:  000:002   0000012288   0000018431   0000006144   Linux (0x83)
005:  -------   0000018432   0000020479   0000002048   Unallocated`

func fakeExecute(command string, args ...string) *exec.Cmd {
	var mockStdOut string
	if command == "mmls" {
		mockStdOut = mmlsOut
	}

	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1",
		"STDOUT=" + mockStdOut}
	return cmd
}

// This isn't a real test. It's used as a helper process.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	fmt.Fprint(os.Stdout, os.Getenv("STDOUT"))
	os.Exit(0)
}
