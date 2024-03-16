package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"

	"cloud.google.com/go/spanner"
	"github.com/google/hashr/client/cloudspanner"
	"github.com/google/hashr/client/postgres"
	_ "github.com/lib/pq"

	"github.com/golang/glog"
)

var (
	hashStorage   = flag.String("hashStorage", "", "Storage used for computed hashes, can have one of the two values: postgres, cloudspanner")
	spannerDBPath = flag.String("spanner_db_path", "", "Path to spanner DB.")

	// Postgres DB flags
	postgresHost     = flag.String("postgres_host", "localhost", "PostgreSQL instance address.")
	postgresPort     = flag.Int("postgres_port", 5432, "PostgresSQL instance port.")
	postgresUser     = flag.String("postgres_user", "hashr", "PostgresSQL user.")
	postgresPassword = flag.String("postgres_password", "hashr", "PostgresSQL password.")
	postgresDBName   = flag.String("postgres_db", "hashr", "PostgresSQL database.")
)

// Storage represents  storage that is used to store data about processed sources.
type Storage interface {
	GetSamples(ctx context.Context) (map[string]map[string]string, error)
}

func main() {
	ctx := context.Background()
	flag.Parse()

	var storage Storage
	switch *hashStorage {
	case "postgres":
		psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			*postgresHost, *postgresPort, *postgresUser, *postgresPassword, *postgresDBName)

		db, err := sql.Open("postgres", psqlInfo)
		if err != nil {
			glog.Exitf("Error initializing Postgres client: %v", err)
		}
		defer db.Close()

		storage, err = postgres.NewStorage(db)
		if err != nil {
			glog.Exitf("Error initializing Postgres storage: %v", err)
		}
	case "cloudspanner":
		spannerClient, err := spanner.NewClient(ctx, *spannerDBPath)
		if err != nil {
			glog.Exitf("Error initializing Spanner client: %v", err)
		}

		storage, err = cloudspanner.NewStorage(ctx, spannerClient)
		if err != nil {
			glog.Exitf("Error initializing Postgres storage: %v", err)
		}
	default:
		glog.Exit("hashStorage flag needs to have one of the two values: postgres, cloudspanner")

	}
	samples, err := storage.GetSamples(ctx)
	if err != nil {
		glog.Exitf("Error retriving samples: %v", err)
	}

	jsonData, err := json.Marshal(samples)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println(string(jsonData))
}
