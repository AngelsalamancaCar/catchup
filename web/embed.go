// Package web embebe los templates y assets estáticos en el binario.
package web

import "embed"

//go:embed templates
var TemplatesFS embed.FS

//go:embed static
var StaticFS embed.FS
