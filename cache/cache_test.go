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

package cache

import (
	"reflect"
	"sync"
	"testing"

	"google.golang.org/protobuf/testing/protocmp"

	"github.com/google/go-cmp/cmp"
	"github.com/google/hashr/common"
	"google.golang.org/protobuf/proto"

	cpb "github.com/google/hashr/cache/proto/cachepb"
	tpb "google.golang.org/protobuf/types/known/timestamppb"
)

var (
	testdataPath     = "testdata"
	wantCacheSamples = map[string]*cpb.Entries{
		"5f29a7d14b95b7b4b5be0b908ef8cd4b3525daf45a7e2ff3b621e4d1402733ac": {
			LastUpdated: &tpb.Timestamp{Seconds: 1586286139, Nanos: 643304385},
			Entries: []*cpb.CacheEntry{
				{
					SourceId:   "20200109.00.00-ubuntu-desktop",
					SourceHash: "5a3b7f7f65cdb332854432f6496b53da27ec6f6c25aa243da2c5822d8991c903",
					Path:       []string{"/tmp/hashr-20200109.00.00-ubuntu-desktop-905482723/export/tmp/hashr-20200109.00.00-ubuntu-desktop-905482723/extracted/file.04"},
				},
				{
					SourceId:   "20200108.00.00-ubuntu-desktop",
					SourceHash: "dcb3a5937e4e609ca8bae24bf79380a06f07d3b0ea08e8cf3b342ae7b1c3f149",
					Path:       []string{"/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/export/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/extracted/file.04"},
				},
			},
		},
		"ca8a605cf72b21b89f9211af1550d7f943a2b844084241f60eddd9d6536c78ec": {
			LastUpdated: &tpb.Timestamp{Seconds: 1586286139, Nanos: 643292219},
			Entries: []*cpb.CacheEntry{
				{
					SourceId:   "20200109.00.00-ubuntu-desktop",
					SourceHash: "5a3b7f7f65cdb332854432f6496b53da27ec6f6c25aa243da2c5822d8991c903",
					Path:       []string{"/tmp/hashr-20200109.00.00-ubuntu-desktop-905482723/export/tmp/hashr-20200109.00.00-ubuntu-desktop-905482723/extracted/file.02"},
				},
				{
					SourceId:   "20200108.00.00-ubuntu-desktop",
					SourceHash: "dcb3a5937e4e609ca8bae24bf79380a06f07d3b0ea08e8cf3b342ae7b1c3f149",
					Path:       []string{"/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/export/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/extracted/file.02"},
				},
			},
		},
		"4741b2746859cbe24f529a4f3108c2d8b4ea5f442f8a3743ff3543c76f369c90": {
			LastUpdated: &tpb.Timestamp{Seconds: 1586286139, Nanos: 643310013},
			Entries: []*cpb.CacheEntry{
				{
					SourceId:   "20200109.00.00-ubuntu-desktop",
					SourceHash: "5a3b7f7f65cdb332854432f6496b53da27ec6f6c25aa243da2c5822d8991c903",
					Path:       []string{"/tmp/hashr-20200109.00.00-ubuntu-desktop-905482723/export/tmp/hashr-20200109.00.00-ubuntu-desktop-905482723/extracted/file.08"},
				},
				{
					SourceId:   "20200108.00.00-ubuntu-desktop",
					SourceHash: "dcb3a5937e4e609ca8bae24bf79380a06f07d3b0ea08e8cf3b342ae7b1c3f149",
					Path:       []string{"/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/export/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/extracted/file.08"},
				},
			},
		},
		"d889bcc21cffc076d6e9cf7e32d0dd801977141e6f71d4c96ae84e5f1765e71a": {
			LastUpdated: &tpb.Timestamp{Seconds: 1586286139, Nanos: 643307983},
			Entries: []*cpb.CacheEntry{
				{
					SourceId:   "20200109.00.00-ubuntu-desktop",
					SourceHash: "5a3b7f7f65cdb332854432f6496b53da27ec6f6c25aa243da2c5822d8991c903",
					Path:       []string{"/tmp/hashr-20200109.00.00-ubuntu-desktop-905482723/export/tmp/hashr-20200109.00.00-ubuntu-desktop-905482723/extracted/file.06"},
				},
				{
					SourceId:   "20200108.00.00-ubuntu-desktop",
					SourceHash: "dcb3a5937e4e609ca8bae24bf79380a06f07d3b0ea08e8cf3b342ae7b1c3f149",
					Path:       []string{"/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/export/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/extracted/file.06"},
				},
			},
		},
		"d88b6267f164354ed3bc3561fc42cc0d117693f0efe8d19379ce5b4c2a12d144": {
			LastUpdated: &tpb.Timestamp{Seconds: 1586286139, Nanos: 643302632},
			Entries: []*cpb.CacheEntry{
				{
					SourceId:   "20200109.00.00-ubuntu-desktop",
					SourceHash: "5a3b7f7f65cdb332854432f6496b53da27ec6f6c25aa243da2c5822d8991c903",
					Path:       []string{"/tmp/hashr-20200109.00.00-ubuntu-desktop-905482723/export/tmp/hashr-20200109.00.00-ubuntu-desktop-905482723/extracted/file.07"},
				},
				{
					SourceId:   "20200108.00.00-ubuntu-desktop",
					SourceHash: "dcb3a5937e4e609ca8bae24bf79380a06f07d3b0ea08e8cf3b342ae7b1c3f149",
					Path:       []string{"/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/export/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/extracted/file.07"},
				},
			},
		},
		"b1f8a81821e18bba696a52b5169524076f77bc588c02ab195f969df4e2650dce": {
			LastUpdated: &tpb.Timestamp{Seconds: 1586286139, Nanos: 643306729},
			Entries: []*cpb.CacheEntry{
				{
					SourceId:   "20200109.00.00-ubuntu-desktop",
					SourceHash: "5a3b7f7f65cdb332854432f6496b53da27ec6f6c25aa243da2c5822d8991c903",
					Path:       []string{"/tmp/hashr-20200109.00.00-ubuntu-desktop-905482723/export/tmp/hashr-20200109.00.00-ubuntu-desktop-905482723/extracted/file.05"},
				},
				{
					SourceId:   "20200108.00.00-ubuntu-desktop",
					SourceHash: "dcb3a5937e4e609ca8bae24bf79380a06f07d3b0ea08e8cf3b342ae7b1c3f149",
					Path:       []string{"/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/export/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/extracted/file.05"},
				},
			},
		},
		"8780622e75a9c1be4b30ae9e15d6d94249926aaa9139b7a563e42ee0eab70eea": {
			LastUpdated: &tpb.Timestamp{Seconds: 1586286139, Nanos: 643305424},
			Entries: []*cpb.CacheEntry{
				{
					SourceId:   "20200109.00.00-ubuntu-desktop",
					SourceHash: "5a3b7f7f65cdb332854432f6496b53da27ec6f6c25aa243da2c5822d8991c903",
					Path:       []string{"/tmp/hashr-20200109.00.00-ubuntu-desktop-905482723/export/tmp/hashr-20200109.00.00-ubuntu-desktop-905482723/extracted/file.03"},
				},
				{
					SourceId:   "20200108.00.00-ubuntu-desktop",
					SourceHash: "dcb3a5937e4e609ca8bae24bf79380a06f07d3b0ea08e8cf3b342ae7b1c3f149",
					Path:       []string{"/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/export/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/extracted/file.03"},
				},
			},
		},
		"e0a98ad618a3cef7f8754a2711322e398879f47e50ca491c75eca6ba476e421a": {
			LastUpdated: &tpb.Timestamp{Seconds: 1586286139, Nanos: 643293792},
			Entries: []*cpb.CacheEntry{
				{
					SourceId:   "20200108.00.00-ubuntu-desktop",
					SourceHash: "dcb3a5937e4e609ca8bae24bf79380a06f07d3b0ea08e8cf3b342ae7b1c3f149",
					Path:       []string{"/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/export/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/extracted/file.10"},
				},
			},
		},
		"8548f0f1cb3a2c2216ddee12cdb32a3a8152b1ac2cffc1464e4950237957151a": {
			LastUpdated: &tpb.Timestamp{Seconds: 1586286139, Nanos: 643290225},
			Entries: []*cpb.CacheEntry{
				{
					SourceId:   "20200109.00.00-ubuntu-desktop",
					SourceHash: "5a3b7f7f65cdb332854432f6496b53da27ec6f6c25aa243da2c5822d8991c903",
					Path:       []string{"/tmp/hashr-20200109.00.00-ubuntu-desktop-905482723/export/tmp/hashr-20200109.00.00-ubuntu-desktop-905482723/extracted/file.09"},
				},
				{
					SourceId:   "20200108.00.00-ubuntu-desktop",
					SourceHash: "dcb3a5937e4e609ca8bae24bf79380a06f07d3b0ea08e8cf3b342ae7b1c3f149",
					Path:       []string{"/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/export/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/extracted/file.09"},
				},
			},
		},
		"bb94f556af1c9ec108c0f1f1d62bbe466ed148f1e72b57e35cfac3e31018b067": {
			LastUpdated: &tpb.Timestamp{Seconds: 1586286139, Nanos: 643301801},
			Entries: []*cpb.CacheEntry{
				{
					SourceId:   "20200109.00.00-ubuntu-desktop",
					SourceHash: "5a3b7f7f65cdb332854432f6496b53da27ec6f6c25aa243da2c5822d8991c903",
					Path:       []string{"/tmp/hashr-20200109.00.00-ubuntu-desktop-905482723/export/tmp/hashr-20200109.00.00-ubuntu-desktop-905482723/extracted/file.01"},
				},
				{
					SourceId:   "20200108.00.00-ubuntu-desktop",
					SourceHash: "dcb3a5937e4e609ca8bae24bf79380a06f07d3b0ea08e8cf3b342ae7b1c3f149",
					Path:       []string{"/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/export/tmp/hashr-20200108.00.00-ubuntu-desktop-818523028/extracted/file.01"},
				},
			},
		},
	}
)

func TestCheck(t *testing.T) {
	extraction := &common.Extraction{
		SourceID:     "20200227.00.00-desktop",
		RepoName:     "gLinux",
		Path:         testdataPath,
		SourceSHA256: "6e0290d62f6db1779d6318df50209de8c9b93adb29b7dd46e7b563f044103b40",
	}

	var cacheMap sync.Map
	for hash, entries := range cloneProtoMap(wantCacheSamples).(map[string]*cpb.Entries) {
		cacheMap.Store(hash, entries)
	}

	gotSamples, err := Check(extraction, &cacheMap)
	if err != nil {
		t.Fatalf("unexpected error while checking cache: %v", err)
	}

	wantSamples := []common.Sample{
		{
			Sha256: "d5d66fe6a4559c59ad103ab40e01c4fc0df7eb8ba901d50e5ceae3909b2e0d61",
			Paths:  []string{"testdata/gLinuxTestRepo/20200227.00.00/export/file.09"},
			Upload: true,
		},
		{
			Sha256: "4878dd6c7af7fecdf89832384d84ed93b78123e69e6a0097efac5320da2ac637",
			Paths:  []string{"testdata/gLinuxTestRepo/20200227.00.00/export/file.02"},
			Upload: true,
		},
		{
			Sha256: "ca8a605cf72b21b89f9211af1550d7f943a2b844084241f60eddd9d6536c78ec",
			Paths:  []string{"testdata/gLinuxTestRepo/20200227.00.00/export/file.10"},
			Upload: false,
		},
		{
			Sha256: "4741b2746859cbe24f529a4f3108c2d8b4ea5f442f8a3743ff3543c76f369c90",
			Paths:  []string{"testdata/gLinuxTestRepo/20200227.00.00/export/file.01"},
			Upload: false,
		},
		{
			Sha256: "d889bcc21cffc076d6e9cf7e32d0dd801977141e6f71d4c96ae84e5f1765e71a",
			Paths:  []string{"testdata/gLinuxTestRepo/20200227.00.00/export/file.07"},
			Upload: false,
		},
		{
			Sha256: "00632850049f80763ada81ec0cacf015dbd67fb1b956ec2acb8aa862e511b3bc",
			Paths:  []string{"testdata/gLinuxTestRepo/20200227.00.00/export/file.04"},
			Upload: true,
		},
		{
			Sha256: "b1f8a81821e18bba696a52b5169524076f77bc588c02ab195f969df4e2650dce",
			Paths:  []string{"testdata/gLinuxTestRepo/20200227.00.00/export/file.03"},
			Upload: false,
		},
		{
			Sha256: "8780622e75a9c1be4b30ae9e15d6d94249926aaa9139b7a563e42ee0eab70eea",
			Paths:  []string{"testdata/gLinuxTestRepo/20200227.00.00/export/file.05"},
			Upload: false,
		},
		{
			Sha256: "99962d9e62c15c73527ca72b4e5e85809d4254326800eb2c65b35339029e02d1",
			Paths:  []string{"testdata/gLinuxTestRepo/20200227.00.00/export/file.06"},
			Upload: true,
		},
		{
			Sha256: "e0a98ad618a3cef7f8754a2711322e398879f47e50ca491c75eca6ba476e421a",
			Paths:  []string{"testdata/gLinuxTestRepo/20200227.00.00/export/file.08"},
			Upload: false,
		},
	}

	if !cmp.Equal(wantSamples, gotSamples) {
		t.Errorf("Check() unexpected diff (-want/+got):\n%s", cmp.Diff(wantSamples, gotSamples))
	}
}

// cloneProtoMap clones a map[K]M where M must be a proto.Message type.
func cloneProtoMap(v interface{}) interface{} {
	src := reflect.ValueOf(v)
	dst := reflect.MakeMap(src.Type())
	for _, k := range src.MapKeys() {
		m := proto.Clone(src.MapIndex(k).Interface().(proto.Message))
		dst.SetMapIndex(k, reflect.ValueOf(m))
	}
	return dst.Interface()
}

func TestLoad(t *testing.T) {
	cacheMap, err := Load("gLinux", testdataPath)
	if err != nil {
		t.Fatalf("unexpected error while loading cache file: %v", err)
	}

	gotCache := &cpb.Cache{Samples: make(map[string]*cpb.Entries)}
	cacheMap.Range(func(key, value interface{}) bool {
		hash, ok := key.(string)
		if !ok {
			t.Fatalf("Unexpected key type in jobs map: %v", key)
		}
		entries, ok := value.(*cpb.Entries)
		if !ok {
			t.Fatalf("Unexpected value type in jobs map: %v", key)
		}

		gotCache.GetSamples()[hash] = entries

		return true
	})

	if diff := cmp.Diff(wantCacheSamples, gotCache.GetSamples(), protocmp.Transform()); diff != "" {
		t.Errorf("Load(\"gLinux\", %s) unexpected diff (-want/+got):\n%s", testdataPath, diff)
	}
}
