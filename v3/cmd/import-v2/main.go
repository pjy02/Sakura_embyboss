package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pjy02/Sakura_embyboss/v3/internal/legacyimport"
	"github.com/pjy02/Sakura_embyboss/v3/internal/postgres"
)

func main() {
	apply := flag.Bool("apply", false, "write imported records to PostgreSQL")
	flag.Parse()
	source := os.Getenv("SAKURA_V2_DATABASE_DSN")
	if source == "" {
		fatal("SAKURA_V2_DATABASE_DSN is required")
	}
	var target *postgres.Client
	var rawTarget *pgxpool.Pool
	var err error
	if *apply {
		url := os.Getenv("SAKURA_V3_DATABASE_URL")
		if url == "" {
			fatal("SAKURA_V3_DATABASE_URL is required with --apply")
		}
		target, err = postgres.New(context.Background(), url)
		if err != nil {
			fatal(err.Error())
		}
		defer target.Close()
		rawTarget = target.Pool()
	}
	importer, err := legacyimport.New(source, rawTarget, *apply)
	if err != nil {
		fatal(err.Error())
	}
	defer importer.Close()
	report, err := importer.Run(context.Background())
	if err != nil {
		fatal(err.Error())
	}
	body, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(body))
}

func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
