package engine

import (
	"github.com/mumax/3/util"
	"os/exec"
	"runtime"
)

func init() {
	DeclFunc("RunShell", RunShell, "Run a host shell command; requires the -insecure command-line flag")
}

func RunShell(command string) {
	CheckStop()
	if !*Flag_insecure {
		panic(UserErr("RunShell is disabled; restart mumax3 with -insecure to enable host command execution"))
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = util.CommandContext(SimulationContext(), "cmd.exe", "/C", command)
	} else {
		cmd = util.CommandContext(SimulationContext(), "/bin/sh", "-c", command)
	}
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		LogOut(string(output))
	}
	CheckStop()
	if err != nil {
		panic(UserErr("RunShell: " + err.Error()))
	}
}
