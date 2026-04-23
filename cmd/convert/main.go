package main

import (
	"log"

	"backend/pkg/convert"
)

func main() {
	if err := convert.Run(); err != nil {
		log.Fatalf("ERROR: %v", err)
	}
}
