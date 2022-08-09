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

// Package cache provides functions that are used to interact with local cache.
package cache

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/golang/glog"
	"github.com/google/hashr/common"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	cpb "github.com/google/hashr/cache/proto"
)

func readJSON(extraction *common.Extraction) ([]common.Sample, error) {
	pathJSON := filepath.Join(extraction.Path, "hashes.json")
	var samples []common.Sample

	data, err := ioutil.ReadFile(pathJSON)
	if err != nil {
		return nil, fmt.Errorf("error while reading hashes.json file: %v", err)
	}

	err = json.Unmarshal(data, &samples)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling hashes.json file: %v", err)
	}

	for _, sample := range samples {
		for i := range sample.Paths {
			sample.Paths[i] = filepath.Join(extraction.Path, sample.Paths[i])
		}
	}

	return samples, nil
}

// Save saves the cache to a local file.
func Save(repoName, cacheDir string, cacheMap *sync.Map) error {
	// TODO(mlegin): Compress the file before saving it to disk.
	cachePath := filepath.Join(cacheDir, fmt.Sprintf("hashr-cache-%s", repoName))

	cache := &cpb.Cache{Samples: make(map[string]*cpb.Entries)}
	cacheMap.Range(func(key, value interface{}) bool {
		hash, ok := key.(string)
		if !ok {
			glog.Exitf("Unexpected key type in cache map: %v", key)
		}

		entries, ok := value.(*cpb.Entries)
		if !ok {
			glog.Exitf("Unexpected value type in cache map: %v", key)
		}

		cache.Samples[hash] = entries

		return true
	})

	data, err := proto.Marshal(cache)
	if err != nil {
		return fmt.Errorf("error marshalling %s repo cache: %v", repoName, err)
	}

	cacheFile, err := os.Create(cachePath)
	if err != nil {
		return fmt.Errorf("error opening %s repo cache file for write: %v", repoName, err)
	}

	_, err = cacheFile.Write(data)
	if err != nil {
		return fmt.Errorf("error writing to %s repo cache file: %v", repoName, err)
	}
	glog.Infof("Successfully saved %s repo cache to %s.", repoName, cachePath)

	return nil
}

// Load reads cache entries from a file stored locally. If the file is not present, the cache is
// created in memory.
func Load(repoName, cacheDir string) (*sync.Map, error) {
	var cacheMap sync.Map
	cachePath := filepath.Join(cacheDir, fmt.Sprintf("hashr-cache-%s", repoName))
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		glog.Infof("Cache for %s repo not found at %s. Creating new cache in memory.", repoName, cachePath)
		return &cacheMap, nil
	}

	data, err := ioutil.ReadFile(cachePath)
	if err != nil {
		// If there is an error while reading the file it might be corrupted.
		if err := os.Remove(cachePath); err != nil {
			return nil, fmt.Errorf("error while trying to delete the %s repo cache file: %v", repoName, err)
		}
		return &cacheMap, nil
	}

	cache := &cpb.Cache{}
	if err := proto.Unmarshal(data, cache); err != nil {
		// If there is an error while unmarshalling the file it might be corrupted.
		if err := os.Remove(cachePath); err != nil {
			return nil, fmt.Errorf("error while trying to delete the %s repo cache file: %v", repoName, err)
		}
		return &cacheMap, nil
	}
	glog.Infof("Successfully loaded cache for %s repo from %s.", repoName, cachePath)

	for k, v := range cache.Samples {
		cacheMap.Store(k, v)
	}

	return &cacheMap, nil
}

// Check checks if files present in a given extraction are already in the local cache.
func Check(extraction *common.Extraction, cache *sync.Map) ([]common.Sample, error) {
	samples, err := readJSON(extraction)
	if err != nil {
		return nil, fmt.Errorf("error while reading hashes.json file: %v", err)
	}

	var exports []common.Sample
	for _, sample := range samples {
		newCacheEntry := &cpb.CacheEntry{
			SourceId:   extraction.SourceID,
			SourceHash: extraction.SourceSHA256,
		}
		newExport := common.Sample{
			Sha256: sample.Sha256,
			Paths:  sample.Paths,
		}

		if sampleCache, ok := cache.Load(sample.Sha256); ok {
			// If the sample is already in the cache, add a new entry.
			sampleCache.(*cpb.Entries).Entries = append(sampleCache.(*cpb.Entries).Entries, newCacheEntry)
			sampleCache.(*cpb.Entries).LastUpdated = timestamppb.Now()
		} else {
			// Add a new sample to the cache.
			cache.Store(sample.Sha256, &cpb.Entries{
				LastUpdated: timestamppb.Now(),
				Entries:     []*cpb.CacheEntry{newCacheEntry},
			})
			newExport.Upload = true
		}

		exports = append(exports, newExport)
	}

	return exports, nil
}
