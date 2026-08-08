package main

import (
	"flag"
	"fmt"
	"os"

	"ideaboard/api/internal/db/schemaassembly"
)

func main() {
	manifest := flag.String("manifest", "internal/db/schema/manifest.json", "schema manifest path")
	output := flag.String("output", "", "generated schema output path (defaults to manifest output)")
	check := flag.Bool("check", false, "fail if output differs from manifest assembly")
	flag.Parse()
	if err := schemaassembly.AssembleFile(*manifest, *output, *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
