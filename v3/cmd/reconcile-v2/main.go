package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pjy02/Sakura_embyboss/v3/internal/postgres"
	"github.com/pjy02/Sakura_embyboss/v3/internal/reconcile"
)

func main() {
	output := flag.String("output", "", "also write the JSON report to this file")
	fail := flag.Bool("fail-on-difference", false, "exit non-zero when reconciliation has blockers")
	flag.Parse()
	source, targetURL := os.Getenv("SAKURA_V2_DATABASE_DSN"), os.Getenv("SAKURA_V3_DATABASE_URL")
	if source == "" || targetURL == "" {
		fatal("SAKURA_V2_DATABASE_DSN and SAKURA_V3_DATABASE_URL are required")
	}
	ctx := context.Background()
	target, err := postgres.New(ctx, targetURL)
	if err != nil {
		fatal(err.Error())
	}
	defer target.Close()
	checker, err := reconcile.New(source, target.Pool())
	if err != nil {
		fatal(err.Error())
	}
	defer checker.Close()
	report, err := checker.Run(ctx)
	if err != nil {
		fatal(err.Error())
	}
	body, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(body))
	if *output != "" {
		if err = os.MkdirAll(filepath.Dir(*output), 0o750); err != nil {
			fatal(err.Error())
		}
		if err = os.WriteFile(*output, append(body, '\n'), 0o600); err != nil {
			fatal(err.Error())
		}
	}
	if *fail && !report.Pass {
		os.Exit(2)
	}
}

func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
