package web

import (
	"embed"
)

//go:embed dist/*
var DistFiles embed.FS

//go:embed sample.env
var SampleEnv []byte