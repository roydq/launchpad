package main

import (
	"context"
	"fmt"
	"os"

	"github.com/launchpad/launchpad/internal/store"
)

func main() {
	ctx := context.Background()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("LAUNCHPAD_DATABASE_URL")
	}
	if databaseURL == "" {
		databaseURL = "file:launchpad.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	}

	db, driver, err := store.Open(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := store.Migrate(ctx, db, driver); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("migrations applied")
}