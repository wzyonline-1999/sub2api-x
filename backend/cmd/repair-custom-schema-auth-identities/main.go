package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	_ "github.com/lib/pq"
)

func main() {
	schema := flag.String("schema", "", "required existing non-public PostgreSQL schema")
	execute := flag.Bool(
		"execute",
		false,
		"required confirmation; start the current release once to finish normal migrations before running this repair",
	)
	timeout := flag.Duration("timeout", 10*time.Minute, "overall maintenance timeout")
	flag.Parse()

	*schema = strings.TrimSpace(*schema)
	if *schema == "" || *schema == "public" {
		log.Fatal("--schema must name an existing non-public PostgreSQL schema")
	}
	if !config.IsValidPostgresIdentifier(*schema) {
		log.Fatal("--schema must use lowercase letters, digits, and underscores")
	}
	if !*execute {
		fmt.Fprintln(os.Stderr, "refusing to modify data without --execute")
		os.Exit(2)
	}
	if *timeout <= 0 {
		log.Fatal("--timeout must be positive")
	}

	cfg, err := config.LoadForBootstrap()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	cfg.Database.Schema = *schema

	db, err := sql.Open("postgres", cfg.Database.DSNWithTimezone(cfg.Timezone))
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("connect database: %v", err)
	}

	result, err := repository.RepairCustomSchemaAuthIdentityHistory(ctx, db, cfg.Database.Schema)
	if err != nil {
		log.Fatalf(
			"repair failed (eligible_missing_before=%d eligible_missing_after=%d): %v",
			result.EligibleMissingBefore,
			result.EligibleMissingAfter,
			err,
		)
	}
	fmt.Printf(
		"repair complete schema=%s eligible_missing_before=%d eligible_missing_after=%d\n",
		cfg.Database.Schema,
		result.EligibleMissingBefore,
		result.EligibleMissingAfter,
	)
}
