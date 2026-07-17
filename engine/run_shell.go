package engine

import (
	"os/exec"
	"runtime"
)

func init() {
	DeclFunc("RunShell", RunShell, "Run a host shell command; requires the -insecure command-line flag")
}

func RunShell(command string) {
	if !*Flag_insecure {
		panic(UserErr("RunShell is disabled; restart mumax3 with -insecure to enable host command execution"))
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd.exe", "/C", command)
	} else {
		cmd = exec.Command("/bin/sh", "-c", command)
	}
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		LogOut(string(output))
	}
	if err != nil {
		panic(UserErr("RunShell: " + err.Error()))
	}
}
