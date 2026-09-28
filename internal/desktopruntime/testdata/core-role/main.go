// This inert process exposes real Windows argv and waits for test-owned stdin.
// It performs no runtime, registry, network, task or service operations.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() { fmt.Println("fixture-ready"); _, _ = io.Copy(io.Discard, os.Stdin) }
