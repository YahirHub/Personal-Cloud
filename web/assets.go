package web

import "embed"

// Assets contiene plantillas y recursos estáticos compilados dentro del binario.
//
//go:embed layouts/*.html components/*.html pages/*.html static/*
var Assets embed.FS
