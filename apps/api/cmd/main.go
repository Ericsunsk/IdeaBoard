package main

import (
	"os"

	"ideaboard/api/internal/runtime/bootstrap"
)

func main() {
	bootstrap.Run(os.Args)
}
