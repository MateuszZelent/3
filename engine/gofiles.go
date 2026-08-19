package engine

// support for running Go files as if they were mx3 files.

import (
	"flag"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/mumax/3/cuda"
	"github.com/mumax/3/util"
)

var (
	// These flags are shared between cmd/mumax3 and Go input files.
	Flag_cachedir           = flag.String("cache", filepath.Join(os.TempDir(), "amumax_kernels"), "Kernel cache directory (empty disables caching)")
	Flag_gpu                = flag.Int("gpu", 0, "Specify a single GPU (use CUDA_AVAILABLE_DEVICES environment variable for advanced selection)")
	Flag_interactive        = flag.Bool("i", false, "Open interactive browser session")
	Flag_od                 = flag.String("o", "", "Override output directory")
	Flag_port               = flag.String("http", ":35367", "Port to serve web gui")
	Flag_selftest           = flag.Bool("paranoid", false, "Enable convolution self-test for cuFFT sanity.")
	Flag_silent             = flag.Bool("s", false, "Silent") // provided for backwards compatibility
	Flag_sync               = flag.Bool("sync", false, "Synchronize all CUDA calls (debug)")
	Flag_forceclean         = flag.Bool("f", false, "Force start, clean existing output directory")
	Flag_skipexist          = flag.Bool("skip-exist", false, "Skip the simulation when its output directory already exists")
	Flag_hideprogress       = flag.Bool("hide-progress-bar", false, "Hide terminal progress bars")
	Flag_debug              = flag.Bool("debug", false, "Enable debug logging")
	Flag_fft                = flag.Bool("fft", false, "Enable real-time FFT data for the web interface")
	Flag_core               = flag.Bool("core", false, "Track vortex core position and enable its web UI plot")
	Flag_storage            = flag.String("storage-format", "zarr", "Storage backend used by Save/AutoSave: ovf, zarr, or h5")
	Flag_legacygui          = flag.Bool("legacy-gui", false, "Use the original mumax3 web GUI")
	Flag_tunnel             = flag.String("tunnel", "", "SSH host used for an optional reverse web-UI tunnel")
	Flag_webdebug           = flag.Bool("webui-debug", false, "Enable web API request logging")
	Flag_webuidisable       = flag.Bool("webui-disable", false, "Disable the web interface")
	Flag_queueport          = flag.String("webui-queue-addr", "localhost:35366", "Address used by the queue web interface")
	Flag_interactiveTimeout = flag.Duration("interactive-disconnect-timeout", 3*time.Second, "Grace period before an interactive simulation exits after its last WebUI client disconnects")
	Flag_webuiPublicURL     = flag.String("webui-public-url", "", "Explicit public WebUI URL template; use {port} for the actual worker port")
	Flag_webuiTrustedProxy  = flag.String("webui-trusted-proxy", "127.0.0.0/8,::1/128", "Comma-separated proxy IPs/CIDRs allowed to supply Forwarded headers")
	Flag_insecure           = flag.Bool("insecure", false, "Allow unsafe mx3 operations such as RunShell")
)

func init() {
	flag.StringVar(Flag_cachedir, "c", *Flag_cachedir, "Alias for -cache")
	flag.IntVar(Flag_gpu, "g", *Flag_gpu, "Alias for -gpu")
	flag.BoolVar(Flag_interactive, "interactive", *Flag_interactive, "Alias for -i")
	flag.StringVar(Flag_od, "output-dir", *Flag_od, "Alias for -o")
	flag.BoolVar(Flag_selftest, "p", *Flag_selftest, "Alias for -paranoid")
	flag.BoolVar(Flag_silent, "silent", *Flag_silent, "Alias for -s")
	flag.BoolVar(Flag_forceclean, "force-clean", *Flag_forceclean, "Alias for -f")
	flag.StringVar(Flag_tunnel, "t", *Flag_tunnel, "Alias for -tunnel")
	flag.StringVar(Flag_port, "webui-addr", *Flag_port, "Alias for -http")
	flag.BoolVar(Flag_debug, "d", *Flag_debug, "Alias for -debug")
}

var flagAliases = map[string]string{
	"c": "cache", "g": "gpu", "interactive": "i", "output-dir": "o",
	"p": "paranoid", "silent": "s", "force-clean": "f", "t": "tunnel",
	"webui-addr": "http", "d": "debug", "u": "update", "version": "v",
}

// CanonicalFlagName returns the primary name for a CLI flag or alias.
func CanonicalFlagName(name string) string {
	if canonical, ok := flagAliases[name]; ok {
		return canonical
	}
	return name
}

func FlagPassed(name string) bool {
	canonical := CanonicalFlagName(name)
	passed := false
	flag.Visit(func(f *flag.Flag) {
		if CanonicalFlagName(f.Name) == canonical {
			passed = true
		}
	})
	return passed
}

// Usage: in every Go input file, write:
//
//	func main(){
//		defer InitAndClose()()
//		// ...
//	}
//
// This initialises the GPU, output directory, etc,
// and makes sure pending output will get flushed.
func InitAndClose() func() {

	flag.Parse()

	cuda.Init(*Flag_gpu)
	cuda.Synchronous = *Flag_sync

	od := *Flag_od
	extension := ".out"
	if StorageFormat != StorageFormatOVF {
		extension = ".zarr"
	}
	if od == "" {
		od = path.Base(os.Args[0]) + extension
	}
	inFile := util.NoExt(od)
	InitIO(inFile, od, *Flag_forceclean)

	if *Flag_port != "" {
		GoServe(*Flag_port)
	}

	return func() {
		Close()
	}
}
