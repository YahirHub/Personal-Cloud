package config

import (
	"bufio"
	"os"
	"strings"
)

// LoadDotEnv reads a .env file and sets environment variables for any keys
// that are not already defined in the process environment. This means
// explicit env vars always win over .env values.
//
// The file path is resolved as-is; if empty it defaults to ".env" in the
// current working directory. A missing file is silently ignored.
func LoadDotEnv(path string) {
	if path == "" {
		path = ".env"
	}

	f, err := os.Open(path)
	if err != nil {
		// File missing or unreadable — nothing to do.
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on first '=' only.
		eq := strings.IndexByte(line, '=')
		if eq < 1 {
			continue
		}

		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])

		// Strip surrounding quotes (single or double) if present.
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		// Do not override an already-set environment variable.
		if os.Getenv(key) != "" {
			continue
		}

		os.Setenv(key, val)
	}
}
