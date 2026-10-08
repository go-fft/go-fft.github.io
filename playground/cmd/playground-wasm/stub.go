// Copyright (c) the go-fft authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build !js || !wasm

// The real entry point is wasm-only (main.go); this stub keeps `go build ./...`
// and `go test ./...` green on every native host.
package main

func main() {}
