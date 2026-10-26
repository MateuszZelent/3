package engine

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
)

func recordOutputProvenance() {
	var runID [16]byte
	if _, err := rand.Read(runID[:]); err != nil {
		panic(fmt.Errorf("cannot generate output run ID: %w", err))
	}
	structuredOutput.metadata["run_id"] = hex.EncodeToString(runID[:])
	structuredOutput.metadata["field_commit_schema"] = 1
	structuredOutput.metadata["engine"] = UNAME
	structuredOutput.metadata["input_file"] = InputFile
	structuredOutput.metadata["command_line"] = append([]string(nil), os.Args...)
	if input, err := os.Open(InputFile); err == nil {
		hash := sha256.New()
		if _, err := io.Copy(hash, input); err == nil {
			structuredOutput.metadata["input_sha256"] = fmt.Sprintf("%x", hash.Sum(nil))
		}
		_ = input.Close()
	}
	if executable, err := os.Executable(); err == nil {
		structuredOutput.metadata["executable"] = executable
		image := executable
		if runtime.GOOS == "linux" {
			image = "/proc/self/exe"
		}
		if f, err := os.Open(image); err == nil {
			hash := sha256.New()
			if _, err = io.Copy(hash, f); err == nil {
				structuredOutput.metadata["binary_sha256"] = fmt.Sprintf("%x", hash.Sum(nil))
			}
			_ = f.Close()
		}
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		structuredOutput.metadata["build_info"] = info.String()
	}
}
