CREATE TABLE jobs (
  imported_at TIMESTAMP NOT NULL,
  id STRING(500),
  repo STRING(200),
  repo_path STRING(500),
  quick_sha256 STRING(100) NOT NULL,
  location STRING(1000),
  sha256 STRING(100),
  status STRING(50),
  error STRING(10000),
  preprocessing_duration INT64,
  processing_duration INT64,
  export_duration INT64,
  files_extracted INT64,
  files_exported INT64,
) PRIMARY KEY(quick_sha256)
