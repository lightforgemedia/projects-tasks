package main

import (
	"os"
	"strings"
)

func useEngineV2() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("PT_ENGINE")), "v2")
}

