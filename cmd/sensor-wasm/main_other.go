//go:build !(js && wasm)

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "sensor-wasm harus dibangun dengan GOOS=js GOARCH=wasm")
	os.Exit(1)
}
