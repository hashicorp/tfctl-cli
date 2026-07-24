// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// package main is a utility used to compute the CRC32 checksum of a given file using hash/crc32 package.
package main

import (
	"context"
	"fmt"
	"hash/crc32"
	"io"
	"os"

	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

func main() {
	os.Exit(realMain(os.Args))
}

func realMain(args []string) int {
	// Create our iostreams
	io, err := iostreams.System(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to configure iostreams: %v\n", err)
		return 1
	}
	defer func() {
		if err := io.RestoreConsole(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to restore console output: %v\n", err)
		}
	}()

	if len(args) != 2 {
		fmt.Fprintln(io.Err(), "tfctl-cksum uses the standard IEEE 802.3 CRC32 checksum algorithm to compute the checksum of the given file.")
		fmt.Fprintln(io.Err(), "Usage: tfctl-cksum <file>")
		return 1
	}

	stat, err := os.Stat(args[1])
	if err != nil {
		fmt.Fprintf(io.Err(), "File %q invalid or not found\n", args[1])
		return 1
	}

	cksum, err := computeChecksum(args[1])
	if err != nil {
		fmt.Fprintf(io.Err(), "Failed to compute checksum: %v\n", err)
		return 1
	}

	fmt.Fprintf(io.Out(), "%d %d\n", cksum, stat.Size())
	return 0
}

func computeChecksum(filePath string) (uint32, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open file %q: %w", filePath, err)
	}
	defer file.Close()

	hasher := crc32.NewIEEE()
	if _, err := io.Copy(hasher, file); err != nil {
		return 0, err
	}
	return hasher.Sum32(), nil
}
