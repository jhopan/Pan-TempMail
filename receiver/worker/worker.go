package worker

import (
	_ "embed"
	"os"
)

//go:embed script.js
var script string

const (
	defaultName       = "tempmailbyjhopanstore"
	defaultCompatDate = "2024-01-01"
)

func Script() string {
	return script
}

func Name() string {
	if name := os.Getenv("CF_WORKER_NAME"); name != "" {
		return name
	}
	return defaultName
}

func CompatDate() string {
	if date := os.Getenv("CF_WORKER_COMPAT_DATE"); date != "" {
		return date
	}
	return defaultCompatDate
}
