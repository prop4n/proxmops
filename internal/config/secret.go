package config

import (
	"fmt"
	"os"
	"strings"
)

// resolveSecret returns a secret value from, in precedence order:
//
//  1. the environment variable named envKey,
//  2. the contents of the file at filePath (whitespace-trimmed),
//  3. the inline value.
//
// The returned bool reports whether the inline (least secure) source supplied
// the value, so the caller can warn about clear-text secrets in the config file.
func resolveSecret(envKey, filePath, inline string) (value string, fromInline bool, err error) {
	if v := os.Getenv(envKey); v != "" {
		return v, false, nil
	}
	if filePath != "" {
		b, err := os.ReadFile(filePath)
		if err != nil {
			return "", false, fmt.Errorf("read secret file %s: %w", filePath, err)
		}
		return strings.TrimSpace(string(b)), false, nil
	}
	if inline != "" {
		return inline, true, nil
	}
	return "", false, nil
}
