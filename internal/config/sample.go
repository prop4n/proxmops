package config

import _ "embed"

// SampleFile is a commented starter configuration, used by `proxmops init` and
// available as documentation. It is the single source of truth for the example.
//
//go:embed sample.yaml
var SampleFile string
