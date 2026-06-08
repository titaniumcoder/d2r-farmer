package main

import (
	"log"
	"os"
	"strings"

	"github.com/titaniumcoder/d2r-farmer/internal/d2r"
)

func main() {
	addr := strings.TrimSpace(os.Getenv("ADDR"))
	if addr == "" {
		addr = ":8080"
	}

	if err := d2r.RunWeb(addr); err != nil {
		log.Fatal(err)
	}
}
