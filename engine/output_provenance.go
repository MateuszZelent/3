package engine

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
)

func recordOutputProvenance() {
	structuredOutput.metadata["engine"] = UNAME
	structuredOutput.metadata["input_file"] = InputFile
	structuredOutput.metadata["command_line"] = append([]string(nil), os.Args...)
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
