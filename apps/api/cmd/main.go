package main

import (
	"os"

	"api/internal/runtime/bootstrap"
)

func main() {
	bootstrap.Run(os.Args)
}
