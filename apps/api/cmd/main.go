package main

import (
	"os"

	"ideaboard/api/internal/runtime/composition"
)

func main() {
	composition.Run(os.Args)
}
