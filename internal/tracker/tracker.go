package tracker

import _ "embed"

//go:embed tracker.js
var Script []byte

//go:embed demo.html
var Demo []byte
