-- Copyright 2022 Google LLC
--
-- Licensed under the Apache License, Version 2.0 (the "License");
-- you may not use this file except in compliance with the License.
-- You may obtain a copy of the License at
--
--      https:--www.apache.org/licenses/LICENSE-2.0
--
-- Unless required by applicable law or agreed to in writing, software
-- distributed under the License is distributed on an "AS IS" BASIS,
-- WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
-- See the License for the specific language governing permissions and
-- limitations under the License.

CREATE TABLE samples (
        sha256 VARCHAR(100)  PRIMARY KEY,
        mimetype text,
        file_output  text,
        size INT
);

CREATE TABLE payloads (
        sha256 VARCHAR(100)  PRIMARY KEY,
        payload bytea
);

CREATE TABLE sources (
        sha256 VARCHAR(100)  PRIMARY KEY,
        sourceID  text[],
        sourcePath  text,
        repoName text,
        repoPath text
);

CREATE TABLE samples_sources (
        sample_sha256 VARCHAR(100) REFERENCES samples(sha256) NOT NULL,
        source_sha256 VARCHAR(100) REFERENCES sources(sha256) NOT NULL,
        sample_paths text[],
        PRIMARY KEY (sample_sha256, source_sha256)
);