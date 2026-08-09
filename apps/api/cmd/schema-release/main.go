package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"ideaboard/api/internal/db/schemarelease"
)

func main() {
	apiRoot := flag.String("api-root", ".", "API project root")
	manifest := flag.String("manifest", "internal/db/schema/manifest.json", "schema manifest path")
	output := flag.String("output", "", "generated schema artifact path (defaults to manifest output)")
	writeArtifact := flag.Bool("write-artifact", false, "publish the deterministic schema artifact after all release checks pass")
	baseSHA := flag.String("base-sha", strings.TrimSpace(os.Getenv("BASE_SHA")), "base commit for migration immutability")
	headSHA := flag.String("head-sha", envOrDefault("HEAD_SHA", "HEAD"), "head commit for migration immutability")
	timeout := flag.Duration("timeout", 5*time.Minute, "Schema Release check timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	if err := schemarelease.Check(ctx, schemarelease.Options{
		APIRoot: *apiRoot, ManifestPath: *manifest, OutputPath: *output,
		WriteArtifact: *writeArtifact,
		BaseSHA:       *baseSHA, HeadSHA: *headSHA, DatabaseDSN: os.Getenv("API_TEST_DATABASE_DSN"),
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("schema release check passed")
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
