// mumax3 main command
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/mumax/3/cuda"
	"github.com/mumax/3/engine"
	"github.com/mumax/3/script"
	mx3template "github.com/mumax/3/template"
	"github.com/mumax/3/updater"
	"github.com/mumax/3/util"
	"github.com/mumax/3/webui"
)

var (
	flag_failfast = flag.Bool("failfast", false, "If one simulation fails, stop entire batch immediately")
	flag_maxGPUs  = flag.Int("max_gpus", 0, "Maximum number of GPUs used by a batch (0 uses all available GPUs)")
	flag_test     = flag.Bool("test", false, "Cuda test (internal)")
	flag_version  = flag.Bool("v", false, "Print version and exit")
	flag_vet      = flag.Bool("vet", false, "Check input files for errors, but don't run them")
	flag_update   = flag.Bool("update", false, "Update this binary from the latest GitHub release")
	// more flags in engine/gofiles.go
	commitHash string
)

func init() {
	flag.BoolVar(flag_version, "version", false, "Alias for -v")
	flag.BoolVar(flag_update, "u", false, "Alias for -update")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options] [mx3 paths...]\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "       %s template [--flat] [--run] TEMPLATE.mx3\n\nOptions:\n", os.Args[0])
		flag.PrintDefaults()
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "template" {
		runTemplateCommand(os.Args[2:])
		return
	}
	flag.Parse()
	log.SetPrefix("")
	if *engine.Flag_debug {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		*engine.Flag_webdebug = true
	} else {
		log.SetFlags(0)
	}
	if *engine.Flag_webuidisable {
		*engine.Flag_port = ""
		*engine.Flag_queueport = ""
	} else if *engine.Flag_port == "" && !engine.FlagPassed("webui-queue-addr") {
		*engine.Flag_queueport = ""
	}
	util.SetProgressHidden(*engine.Flag_hideprogress)
	if *flag_version {
		printVersion()
		return
	}
	if *flag_update {
		fmt.Println("Updating mumax3 from", updater.DefaultURL)
		if err := updater.Apply(updater.DefaultURL); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Update complete")
		return
	}
	engine.FftEnabled = *engine.Flag_fft
	if *engine.Flag_core {
		engine.EnableCoreTracking()
	}
	switch strings.ToLower(*engine.Flag_storage) {
	case "ovf":
		engine.StorageFormat = engine.StorageFormatOVF
	case "zarr":
		engine.StorageFormat = engine.StorageFormatZarr
	case "h5", "hdf5":
		engine.StorageFormat = engine.StorageFormatHDF5
	default:
		log.Fatalf("invalid -storage-format %q (want ovf, zarr, or h5)", *engine.Flag_storage)
	}

	cuda.Init(*engine.Flag_gpu)
	engine.SetStructuredGPUInfo(cuda.GPUInfo)
	printVersion()

	cuda.Synchronous = *engine.Flag_sync
	// used by bootstrap launcher to test cuda
	// successful exit means cuda was initialized fine
	if *flag_test {
		os.Exit(0)
	}

	defer engine.Close() // flushes pending output, if any

	if *flag_vet {
		vet()
		return
	}

	switch flag.NArg() {
	case 0:
		if *engine.Flag_interactive {
			runInteractive()
		}
	case 1:
		runFileAndServe(flag.Arg(0))
	default:
		RunQueue(flag.Args())
	}
}

func runTemplateCommand(args []string) {
	flags := flag.NewFlagSet("template", flag.ExitOnError)
	flat := flags.Bool("flat", false, "Generate files without nested directories")
	run := flags.Bool("run", false, "Run every generated MX3 file")
	_ = flags.Parse(args)
	if flags.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: mumax3 template [--flat] [--run] TEMPLATE.mx3")
		os.Exit(2)
	}
	files, err := mx3template.Generate(flags.Arg(0), mx3template.Options{Flat: *flat})
	if err != nil {
		log.Fatal(err)
	}
	for _, filename := range files {
		fmt.Println(filename)
	}
	if *run {
		for _, filename := range files {
			command := exec.Command(os.Args[0], "--webui-disable", filename)
			command.Stdout, command.Stderr, command.Stdin = os.Stdout, os.Stderr, os.Stdin
			if err := command.Run(); err != nil {
				log.Fatal(err)
			}
		}
	}
}

func runInteractive() {
	fmt.Println("//no input files: starting interactive session")
	//initEngine()

	// setup output dir
	now := time.Now()
	extension := ".out"
	if engine.StorageFormat != engine.StorageFormatOVF {
		extension = ".zarr"
	}
	outdir := fmt.Sprintf("mumax-%v-%02d-%02d_%02dh%02d%s", now.Year(), int(now.Month()), now.Day(), now.Hour(), now.Minute(), extension)
	engine.InitIO(outdir, outdir, *engine.Flag_forceclean)

	engine.Timeout = 365 * 24 * time.Hour // basically forever

	// set up some sensible start configuration
	engine.Eval(`SetGridSize(128, 64, 1)
		SetCellSize(4e-9, 4e-9, 4e-9)
		Msat = 1e6
		Aex = 10e-12
		alpha = 1
		m = RandomMag()`)
	webURL := goServeGUI()
	openbrowser(webURL)
	engine.RunInteractive()
}

func runFileAndServe(fname string) {
	if path.Ext(fname) == ".go" {
		runGoFile(fname)
	} else {
		runScript(fname)
	}
}

func runScript(fname string) {
	extension := ".out"
	if engine.StorageFormat != engine.StorageFormatOVF {
		extension = ".zarr"
	}
	outDir := util.NoExt(fname) + extension
	if *engine.Flag_od != "" {
		outDir = *engine.Flag_od
	}
	engine.InitIO(fname, outDir, *engine.Flag_forceclean)

	fname = engine.InputFile

	var code *script.BlockStmt
	var err2 error
	if fname != "" {
		// first we compile the entire file into an executable tree
		code, err2 = engine.CompileFile(fname)
		util.FatalErr(err2)
	}

	// now the parser is not used anymore so it can handle web requests
	webURL := goServeGUI()

	if *engine.Flag_interactive {
		openbrowser(webURL)
	}

	// start executing the tree, possibly injecting commands from web gui
	engine.EvalFile(code)

	if *engine.Flag_interactive {
		engine.RunInteractive()
	}
}

func runGoFile(fname string) {
	// pass through flags
	flags := []string{"run", fname}
	flag.Visit(func(f *flag.Flag) {
		if f.Name != "o" {
			flags = append(flags, fmt.Sprintf("-%v=%v", f.Name, f.Value))
		}
	})

	if *engine.Flag_od != "" {
		flags = append(flags, fmt.Sprintf("-o=%v", *engine.Flag_od))
	}

	cmd := exec.Command("go", flags...)
	log.Println("go", flags)
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		os.Exit(1)
	}
}

// start GUI server and return server address
func goServeGUI() string {
	if *engine.Flag_port == "" {
		log.Println(`//not starting GUI (-http="")`)
		return ""
	}
	if *engine.Flag_legacygui {
		addr := engine.GoServe(*engine.Flag_port)
		url := "http://127.0.0.1" + addr
		fmt.Print("//starting legacy GUI at ", url, "\n")
		return url
	}
	host, port, basePath, err := parseWebUIAddress(*engine.Flag_port)
	if err != nil {
		log.Fatal(err)
	}
	actualPort, err := webui.Start(host, port, basePath, *engine.Flag_tunnel, *engine.Flag_webdebug)
	if err != nil {
		log.Fatal(err)
	}
	browserHost := host
	if browserHost == "0.0.0.0" || browserHost == "::" {
		browserHost = "127.0.0.1"
	}
	url := "http://" + net.JoinHostPort(browserHost, strconv.Itoa(actualPort)) + basePath + "/"
	fmt.Print("//starting new web UI at ", url, "\n")
	return url
}

func parseWebUIAddress(raw string) (host string, port int, basePath string, err error) {
	addressAndPath := strings.SplitN(raw, "/", 2)
	address := addressAndPath[0]
	if len(addressAndPath) == 2 && addressAndPath[1] != "" {
		basePath = "/" + strings.Trim(addressAndPath[1], "/")
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, "", fmt.Errorf("invalid -http address %q: %w", raw, err)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	port, err = strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, "", fmt.Errorf("invalid -http port %q", portText)
	}
	return host, port, basePath, nil
}

// print version to stdout
func printVersion() {
	engine.LogOut(engine.UNAME)
	engine.LogOut(fmt.Sprintf("commit hash: %s", commitHash))
	engine.LogOut(getCPUInfo())
	if cuda.GPUInfo != "" {
		engine.LogOut(fmt.Sprintf("GPU info: %s, using cc=%d PTX", cuda.GPUInfo, cuda.UseCC))
	}
	osInfo := fmt.Sprintf("OS  info: %s, Hostname: %s", getOSInfo(), getHostname())
	engine.LogOut(osInfo)
	engine.LogOut(fmt.Sprintf("Timestamp: %s", time.Now().Format("2006-01-02 15:04:05")))
	engine.LogOut("(c) Arne Vansteenkiste, Dynamat LAB, Ghent University, Belgium")
	engine.LogOut("This is free software without any warranty. See license.txt")
	engine.LogOut("********************************************************************//")
	engine.LogOut("  If you use mumax in any work or publication,                      //")
	engine.LogOut("  we kindly ask you to cite the references in references.bib        //")
	engine.LogOut("********************************************************************//")
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "Unknown"
	}
	return hostname
}

func getOSInfo() string {
	// Check the runtime operating system
	switch runtime.GOOS {
	case "windows":
		return "Windows OS"
	case "linux":
		return getLinuxOSInfo()
	// Add more cases for other operating systems if needed
	default:
		return fmt.Sprintf("Unknown OS: %s", runtime.GOOS)
	}
}

func getLinuxOSInfo() string {
	// Check if the file exists
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return fmt.Sprintf("Unknown OS, Error: %s", err.Error())
	}
	defer file.Close()

	// Scan the file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && parts[0] == "PRETTY_NAME" {
			// Remove surrounding quotes and return
			return strings.Trim(parts[1], `"`)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Sprintf("Unknown OS, Error: %s", err.Error())
	}

	return "Unknown OS"
}

func getCPUInfo() string {
	// Check the runtime operating system
	switch runtime.GOOS {
	case "windows":
		return getWindowsCPUInfo()
	case "linux":
		return getLinuxCPUInfo()
	// Add more cases for other operating systems if needed
	default:
		return fmt.Sprintf("CPU info: Unknown OS: %s", runtime.GOOS)
	}
}

func getWindowsCPUInfo() string {
	// Get CPU model name
	cmd := exec.Command("wmic", "cpu", "get", "Name")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return fmt.Sprintf("CPU info: Unknown, Error: %s", err.Error())
	}
	output := strings.Split(out.String(), "\n")
	cpuModel := "Unknown model"
	if len(output) > 1 {
		cpuModel = strings.TrimSpace(output[1])
	}

	// Get CPU number of cores
	cpuCores := runtime.NumCPU()

	// Get CPU speed
	cmd = exec.Command("wmic", "cpu", "get", "MaxClockSpeed")
	out.Reset()
	cmd.Stdout = &out
	cpuMHz := "Unknown clock frequency"
	if err := cmd.Run(); err == nil {
		output = strings.Split(out.String(), "\n")
		if len(output) > 1 {
			cpuMHz = strings.TrimSpace(output[1]) + " MHz"
		}
	}

	return fmt.Sprintf("CPU info: %s, Cores: %d, %s", cpuModel, cpuCores, cpuMHz)
}

func getLinuxCPUInfo() string {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return fmt.Sprintf("CPU info: Unknown, Error: %s", err.Error())
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var cpuDetails []string
	var cpuModel, cpuCores, cpuMHz string
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, ":")
		if len(fields) != 2 {
			continue
		}
		key := strings.TrimSpace(fields[0])
		value := strings.TrimSpace(fields[1])
		switch key {
		case "model name":
			cpuModel = value
		case "cpu cores":
			cpuCores = value
		case "cpu MHz":
			cpuMHz = value
		}
	}
	if cpuModel != "" && cpuCores != "" && cpuMHz != "" {
		cpuDetails = append(cpuDetails, fmt.Sprintf("CPU info: %s, Cores: %s, MHz: %s", cpuModel, cpuCores, cpuMHz))
	}

	return strings.Join(cpuDetails, "; ")
}
