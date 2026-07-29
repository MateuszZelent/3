package main

// File queue for distributing multiple input files over GPUs.

import (
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	urlpkg "net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/mumax/3/cuda/cu"
	"github.com/mumax/3/engine"
	"github.com/mumax/3/util"
)

var (
	exitStatus       atom = 0
	numOK, numFailed atom = 0, 0
)

func RunQueue(files []string) {
	queueWeb := queueWebAddress{}
	if *engine.Flag_queueport != "" {
		var err error
		queueWeb, err = newQueueWebAddress(*engine.Flag_queueport)
		if err != nil {
			log.Fatal(err)
		}
	}
	jobWeb := queueWebAddress{}
	if *engine.Flag_port != "" {
		var err error
		jobWeb, err = newQueueWebAddress(*engine.Flag_port)
		if err != nil {
			log.Fatal(err)
		}
	}
	s := NewStateTab(files)
	s.PrintTo(os.Stdout)
	if queueWeb.enabled {
		go s.ListenAndServe(queueWeb.listenAddress())
		fmt.Printf("//Realtime queue overview available at http://localhost:%d%s\n", queueWeb.port, queueWeb.basePath)
	}
	s.Run(jobWeb)
	fmt.Println(numOK.get(), "OK, ", numFailed.get(), "failed")
	os.Exit(int(exitStatus))
}

// queueWebAddress describes the queue UI address and the addresses allocated to
// its worker UIs. A path suffix ending in the queue port is a proxy route, for
// example 127.0.0.1:35367/proxy/35367; child UIs use sibling routes such as
// /proxy/35368.
type queueWebAddress struct {
	host     string
	port     int
	basePath string
	enabled  bool
}

func newQueueWebAddress(raw string) (queueWebAddress, error) {
	host, port, basePath, err := parseWebUIAddress(raw)
	if err != nil {
		return queueWebAddress{}, err
	}
	if basePath != "" && !strings.HasSuffix(strings.TrimSuffix(basePath, "/"), "/"+strconv.Itoa(port)) {
		return queueWebAddress{}, fmt.Errorf("queue proxy path %q must end with the queue port %d (for example /proxy/%d)", basePath, port, port)
	}
	return queueWebAddress{host: host, port: port, basePath: basePath, enabled: true}, nil
}

func (a queueWebAddress) listenAddress() string {
	return net.JoinHostPort(a.host, strconv.Itoa(a.port))
}

func (a queueWebAddress) jobAddress(gpu int) string {
	if !a.enabled {
		return ""
	}
	port := a.port + 1 + gpu
	basePath := a.basePath
	if basePath != "" {
		basePath = strings.TrimSuffix(strings.TrimSuffix(basePath, "/"), "/"+strconv.Itoa(a.port)) + "/" + strconv.Itoa(port)
	}
	return net.JoinHostPort(a.host, strconv.Itoa(port)) + basePath
}

// StateTab holds the queue state (list of jobs + statuses).
// All operations are atomic.
type stateTab struct {
	lock sync.Mutex
	jobs []job
	next int
}

// Job info.
type job struct {
	inFile  string // input file to run
	webAddr string // http address for gui of running process
	uid     int
}

// NewStateTab constructs a queue for the given input files.
// After construction, it is accessed atomically.
func NewStateTab(inFiles []string) *stateTab {
	s := new(stateTab)
	s.jobs = make([]job, len(inFiles))
	for i, f := range inFiles {
		s.jobs[i] = job{inFile: f, uid: i}
	}
	return s
}

// StartNext advances the next job and marks it running, setting its webAddr to indicate the GUI url.
// A copy of the job info is returned, the original remains unmodified.
// ok is false if there is no next job.
func (s *stateTab) StartNext(webAddr string) (next job, ok bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.next >= len(s.jobs) {
		return job{}, false
	}
	s.jobs[s.next].webAddr = webAddr
	jobCopy := s.jobs[s.next]
	s.next++
	return jobCopy, true
}

// Finish marks the job with j's uid as finished.
func (s *stateTab) Finish(j job) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.jobs[j.uid].webAddr = ""
}

// Runs all the jobs in stateTab.
func (s *stateTab) Run(web queueWebAddress) {
	idle, nGPU := initGPUs()
	for {
		gpu := <-idle
		j, ok := s.StartNext(web.jobAddress(gpu))
		if !ok {
			break
		}
		go func() {
			run(j.inFile, gpu, j.webAddr)
			s.Finish(j)
			idle <- gpu
		}()
	}
	// drain remaining tasks (one already done)
	for i := 1; i < nGPU; i++ {
		<-idle
	}
}

func atoi(a string) int {
	i, err := strconv.Atoi(a)
	util.PanicErr(err)
	return i
}

type atom int32

func (a *atom) set(v int) { atomic.StoreInt32((*int32)(a), int32(v)) }
func (a *atom) get() int  { return int(atomic.LoadInt32((*int32)(a))) }
func (a *atom) inc()      { atomic.AddInt32((*int32)(a), 1) }

func run(inFile string, gpu int, webAddr string) {
	// overridden flags
	gpuFlag := fmt.Sprint(`-gpu=`, gpu)
	httpFlag := fmt.Sprint(`-http=`, webAddr)

	// pass through flags
	flags := []string{gpuFlag, httpFlag}
	flag.Visit(func(f *flag.Flag) {
		if f.Name != "gpu" && f.Name != "http" && f.Name != "failfast" && f.Name != "max_gpus" {
			flags = append(flags, fmt.Sprintf("-%v=%v", f.Name, f.Value))
		}
	})
	flags = append(flags, inFile)

	cmd := exec.Command(os.Args[0], flags...)
	log.Println(os.Args[0], flags)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Println(inFile, err)
		log.Printf("%s\n", output)
		exitStatus.set(1)
		numFailed.inc()
		if *flag_failfast {
			os.Exit(1)
		}
	} else {
		numOK.inc()
	}
}

// Creates a concurrent channel containing the available GPU IDs for jobs.
// Returns the channel and the number of available GPUs for the queue.
func initGPUs() (chan int, int) {
	deviceCount := cu.DeviceGetCount()
	gpuIDs, err := selectQueueGPUs(deviceCount, engine.FlagPassed("gpu"), *engine.Flag_gpu, *flag_maxGPUs)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("//queue using %d of %d available GPU(s): %v", len(gpuIDs), deviceCount, gpuIDs)
	idle := make(chan int, len(gpuIDs))
	for _, gpu := range gpuIDs {
		idle <- gpu
	}
	return idle, len(gpuIDs)
}

func selectQueueGPUs(deviceCount int, explicitGPU bool, gpu, maxGPUs int) ([]int, error) {
	if deviceCount < 1 {
		return nil, errors.New("no GPUs available")
	}
	if maxGPUs < 0 {
		return nil, fmt.Errorf("max_gpus must be at least 0, got %d", maxGPUs)
	}
	if explicitGPU {
		if gpu < 0 || gpu >= deviceCount {
			return nil, fmt.Errorf("GPU index %d is outside the available range 0..%d", gpu, deviceCount-1)
		}
		return []int{gpu}, nil
	}
	count := deviceCount
	if maxGPUs > 0 && maxGPUs < count {
		count = maxGPUs
	}
	gpuIDs := make([]int, count)
	for i := range gpuIDs {
		gpuIDs[i] = i
	}
	return gpuIDs, nil
}

func (s *stateTab) PrintTo(w io.Writer) {
	s.lock.Lock()
	defer s.lock.Unlock()
	for i, j := range s.jobs {
		fmt.Fprintf(w, "%3d %v %v\n", i, j.inFile, j.webAddr)
	}
}

func (s *stateTab) RenderHTML(w io.Writer, r *http.Request) {
	s.lock.Lock()
	defer s.lock.Unlock()
	fmt.Fprintln(w, ` 
<!DOCTYPE html> <html> <head> 
	<meta http-equiv="Content-Type" content="text/html; charset=utf-8">
	<meta http-equiv="refresh" content="1">
`+engine.CSS+`
	</head><body>
	<span style="color:gray; font-weight:bold; font-size:1.5em"> mumax<sup>3</sup> queue status </span><br/>
	<hr/>
	<pre>
`)

	for _, j := range s.jobs {
		if j.webAddr != "" {
			fmt.Fprint(w, `<b>`, j.uid, ` <a href="`, html.EscapeString(queueJobURL(r, j.webAddr)), `">`, html.EscapeString(j.inFile), " ", html.EscapeString(j.webAddr), "</a></b>\n")
		} else {
			fmt.Fprint(w, j.uid, " ", html.EscapeString(j.inFile), "\n")
		}
	}

	fmt.Fprintln(w, `</pre><hr/></body></html>`)
}

func (s *stateTab) ListenAndServe(addr string) {
	go func() {
		if err := http.ListenAndServe(addr, s); err != nil && err != http.ErrServerClosed {
			log.Printf("queue web UI: %v", err)
		}
	}()
}

func (s *stateTab) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.RenderHTML(w, r)
}

// queueJobURL retains the public origin used by the browser. This makes links
// work for localhost, a LAN address, and reverse proxies that set standard
// Forwarded or X-Forwarded-* headers. A path whose final component is a port
// is treated as a proxy route and updated to the worker's port.
func queueJobURL(r *http.Request, webAddr string) string {
	_, jobPort, jobPath, err := parseWebUIAddress(webAddr)
	if err != nil {
		return ""
	}

	publicURL := publicRequestURL(r)
	if jobPath != "" {
		publicURL.Path = joinPublicPath(forwardedPrefix(r), jobPath)
		return publicURL.String()
	}

	if path, ok := replaceTrailingPort(publicRequestPath(r), jobPort); ok {
		publicURL.Path = path
		return publicURL.String()
	}
	publicURL.Host = hostWithPort(publicURL.Host, jobPort)
	return publicURL.String()
}

func publicRequestURL(r *http.Request) urlpkg.URL {
	publicURL := urlpkg.URL{Scheme: "http", Host: r.Host}
	if r.TLS != nil {
		publicURL.Scheme = "https"
	}

	forwardedHost, forwardedProto := forwardedOrigin(r.Header.Get("Forwarded"))
	if forwardedHost != "" {
		publicURL.Host = forwardedHost
	} else if host := firstHeaderValue(r.Header.Get("X-Forwarded-Host")); validPublicHost(host) {
		publicURL.Host = host
	}
	if forwardedProto != "" {
		publicURL.Scheme = forwardedProto
	} else if proto := firstHeaderValue(r.Header.Get("X-Forwarded-Proto")); proto == "http" || proto == "https" {
		publicURL.Scheme = proto
	}
	if publicURL.Host == "" {
		publicURL.Host = "localhost"
	}
	return publicURL
}

func forwardedOrigin(header string) (host, proto string) {
	first := strings.SplitN(header, ",", 2)[0]
	for _, item := range strings.Split(first, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(item), "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"`)
		switch strings.ToLower(key) {
		case "host":
			if validPublicHost(value) {
				host = value
			}
		case "proto":
			if value == "http" || value == "https" {
				proto = value
			}
		}
	}
	return host, proto
}

func firstHeaderValue(value string) string {
	return strings.TrimSpace(strings.SplitN(value, ",", 2)[0])
}

func validPublicHost(host string) bool {
	return host != "" && !strings.ContainsAny(host, " /\\?#@\t\r\n")
}

func forwardedPrefix(r *http.Request) string {
	prefix := firstHeaderValue(r.Header.Get("X-Forwarded-Prefix"))
	if prefix == "" || !strings.HasPrefix(prefix, "/") || strings.ContainsAny(prefix, "?#\r\n") {
		return ""
	}
	return "/" + strings.Trim(prefix, "/")
}

func publicRequestPath(r *http.Request) string {
	path := r.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	prefix := forwardedPrefix(r)
	if prefix == "" || strings.HasPrefix(path, prefix) {
		return path
	}
	if path == "/" {
		return prefix
	}
	return strings.TrimSuffix(prefix, "/") + "/" + strings.TrimPrefix(path, "/")
}

func joinPublicPath(prefix, path string) string {
	if prefix == "" || strings.HasPrefix(path, prefix+"/") || path == prefix {
		return path
	}
	return strings.TrimSuffix(prefix, "/") + "/" + strings.TrimPrefix(path, "/")
}

func replaceTrailingPort(path string, port int) (string, bool) {
	trimmed := strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(trimmed, "/"), "/")
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return "", false
	}
	if _, err := strconv.Atoi(parts[len(parts)-1]); err != nil {
		return "", false
	}
	parts[len(parts)-1] = strconv.Itoa(port)
	return "/" + strings.Join(parts, "/"), true
}

func hostWithPort(host string, port int) string {
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	return net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port))
}
