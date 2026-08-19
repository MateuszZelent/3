// mumax3 main command
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"log"
	"net"
	urlpkg "net/url"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/mumax/3/cuda"
	"github.com/mumax/3/engine"
	"github.com/mumax/3/events"
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
	commitHash   string
	buildVersion = "development"
	buildDate    = "unknown"
)

func init() {
	flag.BoolVar(flag_version, "version", false, "Alias for -v")
	flag.BoolVar(flag_update, "u", false, "Alias for -update")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), buildSummary())
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
	if err := validateCLIConfiguration(); err != nil {
		fmt.Fprintln(os.Stderr, "invalid configuration:", err)
		os.Exit(2)
	}
	engine.Timeout = *engine.Flag_interactiveTimeout
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

	// The parent of a queued batch is a scheduler, not a simulation worker.
	// RunQueue enumerates devices without creating a CUDA context; only the
	// single-file path initializes the assigned device here.
	if flag.NArg() > 1 {
		if status := RunQueue(flag.Args()); status != 0 {
			os.Exit(status)
		}
		return
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
	}
}

func validateCLIConfiguration() error {
	if *engine.Flag_interactive && *engine.Flag_port == "" {
		return fmt.Errorf("-i/--interactive requires an enabled WebUI; remove -i or set -http")
	}
	if *engine.Flag_interactiveTimeout <= 0 {
		return fmt.Errorf("-interactive-disconnect-timeout must be positive, got %s", *engine.Flag_interactiveTimeout)
	}
	if flag.NArg() > 1 && *engine.Flag_od != "" {
		return fmt.Errorf("-o/--output-dir cannot be shared by multiple queued input files")
	}
	if *flag_maxGPUs < 0 {
		return fmt.Errorf("-max_gpus must be at least 0, got %d", *flag_maxGPUs)
	}
	if engine.FlagPassed("gpu") && *engine.Flag_gpu < 0 {
		return fmt.Errorf("-gpu must be non-negative, got %d", *engine.Flag_gpu)
	}
	if *engine.Flag_webuiPublicURL != "" {
		if err := validatePublicURLTemplate(*engine.Flag_webuiPublicURL); err != nil {
			return err
		}
	}
	if *engine.Flag_webuiTrustedProxy != "" {
		if err := validateTrustedProxyList(*engine.Flag_webuiTrustedProxy); err != nil {
			return err
		}
	}
	if *engine.Flag_port != "" {
		host, port, _, err := parseWebUIAddress(*engine.Flag_port)
		if err != nil {
			return fmt.Errorf("invalid -http address: %w", err)
		}
		if flag.NArg() > 1 {
			if engine.FlagPassed("gpu") && port+1+*engine.Flag_gpu > 65535 {
				return fmt.Errorf("worker WebUI port for GPU %d exceeds 65535 from %s:%d", *engine.Flag_gpu, host, port)
			}
			if !engine.FlagPassed("gpu") && *flag_maxGPUs > 0 && port+*flag_maxGPUs > 65535 {
				return fmt.Errorf("worker WebUI ports exceed 65535 from %s:%d", host, port)
			}
			if !engine.FlagPassed("gpu") && *flag_maxGPUs == 0 && port == 65535 {
				return fmt.Errorf("queue worker WebUI needs a port above %d", port)
			}
		}
	}
	if *engine.Flag_queueport != "" {
		if _, err := newQueueWebAddress(*engine.Flag_queueport); err != nil {
			return fmt.Errorf("invalid queue WebUI address: %w", err)
		}
	}
	return nil
}

func validatePublicURLTemplate(template string) error {
	if !strings.Contains(template, "{port}") {
		return fmt.Errorf("-webui-public-url must contain the {port} placeholder")
	}
	if strings.Contains(strings.ReplaceAll(template, "{port}", ""), "{") || strings.Contains(strings.ReplaceAll(template, "{port}", ""), "}") {
		return fmt.Errorf("-webui-public-url contains an unsupported placeholder")
	}
	u, err := urlpkg.Parse(template)
	if err != nil {
		return fmt.Errorf("invalid -webui-public-url: %w", err)
	}
	if strings.HasPrefix(template, "/") {
		if u.IsAbs() || u.Host != "" || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("relative -webui-public-url must be a path without query or fragment")
		}
		return nil
	}
	if u.Scheme != "http" && u.Scheme != "https" || u.Host == "" || u.User != nil {
		return fmt.Errorf("absolute -webui-public-url must use http(s) and a host")
	}
	return nil
}

func validateTrustedProxyList(value string) error {
	for _, raw := range strings.Split(value, ",") {
		rule := strings.TrimSpace(raw)
		if rule == "" {
			continue
		}
		if strings.Contains(rule, "/") {
			if _, _, err := net.ParseCIDR(rule); err != nil {
				return fmt.Errorf("invalid trusted proxy CIDR %q: %w", rule, err)
			}
			continue
		}
		if net.ParseIP(rule) == nil {
			return fmt.Errorf("invalid trusted proxy IP %q", rule)
		}
	}
	return nil
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
	seen := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		canonical := engine.CanonicalFlagName(f.Name)
		if canonical != "o" && !seen[canonical] {
			seen[canonical] = true
			flags = append(flags, fmt.Sprintf("-%v=%v", canonical, f.Value))
		}
	})

	if *engine.Flag_od != "" {
		flags = append(flags, fmt.Sprintf("-o=%v", *engine.Flag_od))
	}

	cmd := exec.Command("go", flags...)
	if fdText := os.Getenv(events.EnvFD); fdText != "" {
		if fd, err := strconv.Atoi(fdText); err == nil && fd >= 0 {
			if eventFile := os.NewFile(uintptr(fd), "mumax-event"); eventFile != nil {
				cmd.ExtraFiles = []*os.File{eventFile}
				cmd.Env = append(os.Environ(), events.EnvFD+"=3")
			}
		}
	}
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
		host, actualPort, basePath, err := parseWebUIAddress(addr)
		if err != nil {
			log.Fatal(err)
		}
		browserHost := host
		if browserHost == "0.0.0.0" || browserHost == "::" {
			browserHost = "127.0.0.1"
		}
		url := "http://" + net.JoinHostPort(browserHost, strconv.Itoa(actualPort)) + basePath + "/"
		fmt.Printf("//starting legacy GUI at %s\n", url)
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
	basePath = webui.RetargetBasePath(basePath, port, actualPort)
	browserHost := host
	if browserHost == "0.0.0.0" || browserHost == "::" {
		browserHost = "127.0.0.1"
	}
	url := "http://" + net.JoinHostPort(browserHost, strconv.Itoa(actualPort)) + basePath + "/"
	fmt.Print("//starting new web UI at ", url, "\n")
	ensureWebUIReadyEvent(host, actualPort, basePath)
	return url
}

func ensureWebUIReadyEvent(host string, port int, basePath string) {
	if err := events.Emit(events.Event{Event: "webui_ready", ListenHost: host, ListenPort: port, BasePath: basePath}); err != nil {
		log.Printf("webui event: %v", err)
	}
	fmt.Printf("//webui-ready %s%s\n", net.JoinHostPort(host, strconv.Itoa(port)), basePath)
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
	engine.LogOut(buildSummary())
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

func buildSummary() string {
	commit := commitHash
	if commit == "" {
		commit = "unknown"
	}
	return fmt.Sprintf("mumax3 build: version=%s commit=%s built=%s", buildVersion, commit, buildDate)
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
