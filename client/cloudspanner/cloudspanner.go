package cloudspanner

import (
	"context"
	"strconv"

	"cloud.google.com/go/spanner"

	"google.golang.org/api/iterator"
)

// Storage allows to interact with cloud spanner.
type Storage struct {
	spannerClient *spanner.Client
}

// NewStorage creates new Storage struct that allows to interact with cloud spanner.
func NewStorage(ctx context.Context, spannerClient *spanner.Client) (*Storage, error) {
	return &Storage{spannerClient: spannerClient}, nil
}

// GetSamples fetches processing samples from cloud spanner.
func (s *Storage) GetSamples(ctx context.Context) (map[string]map[string]string, error) {
	samples := make(map[string]map[string]string)
	iter := s.spannerClient.Single().Read(ctx, "samples",
		spanner.AllKeys(), []string{"sha256", "mimetype", "file_output", "size"})
	defer iter.Stop()
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var sha256, mimetype, fileOutput string
		var size int64
		err = row.ColumnByName("sha256", &sha256)
		if err != nil {
			return nil, err
		}
		err = row.ColumnByName("mimetype", &mimetype)
		if err != nil {
			return nil, err
		}
		err = row.ColumnByName("file_output", &fileOutput)
		if err != nil {
			return nil, err
		}
		err = row.ColumnByName("size", &size)
		if err != nil {
			return nil, err
		}
		samples[sha256] = make(map[string]string)

		// Assign values to the nested map
		samples[sha256]["sha256"] = sha256
		samples[sha256]["mimetype"] = mimetype
		samples[sha256]["file_output"] = fileOutput
		samples[sha256]["size"] = strconv.FormatInt(size, 10)

	}
	return samples, nil
}
