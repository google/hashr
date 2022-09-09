package main

import (
	"context"

	"cloud.google.com/go/spanner"
	"github.com/golang/glog"
	"google.golang.org/api/iterator"
)

func main() {
	ctx := context.Background()

	spannerClient, err := spanner.NewClient(ctx, "projects/hashdb/instances/hashdb/databases/hashdb")
	if err != nil {
		glog.Exitf("Error initializing Spanner client: %v", err)
	}

	var paths, existingPaths []string

	paths = []string{"ccc", "ddd"}

	sql := spanner.Statement{
		SQL: `SELECT sample_paths FROM samples_sources WHERE sample_sha256 = @sha256`,
		Params: map[string]interface{}{
			"sha256": "sha256sss",
		},
	}

	iter := spannerClient.Single().Query(ctx, sql)
	defer iter.Stop()
	row, err := iter.Next()
	if err != iterator.Done {
		if err := row.Columns(&existingPaths); err != nil {
			glog.Exit(err)
		}
	}
	if err != iterator.Done && err != nil {
		glog.Exit(err)
	}

	_, err = spannerClient.Apply(ctx, []*spanner.Mutation{
		spanner.InsertOrUpdate("samples_sources",
			[]string{
				"sample_sha256",
				"source_sha256",
				"sample_paths"},
			[]interface{}{
				"sha256sss",
				"sourceHash",
				append(existingPaths, paths...),
			})})
	if err != nil {
		glog.Exit(err)
	}

}
