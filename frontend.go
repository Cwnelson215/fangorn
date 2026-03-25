package fangorn

import "embed"

//go:embed all:frontend/build
var FrontendAssets embed.FS
