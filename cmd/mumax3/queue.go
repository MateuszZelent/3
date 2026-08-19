package main

// File queue for distributing multiple input files over GPUs.

import (
	"bufio"
	"context"
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
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mumax/3/cuda/cu"
	"github.com/mumax/3/engine"
	"github.com/mumax/3/events"
	"github.com/mumax/3/util"
)

var (
	exitStatus       atom = 0
	numOK, numFailed atom = 0, 0
	queueCUDAOnce    sync.Once
)

var (
	runningMu sync.Mutex
	running   = make(map[*exec.Cmd]runningProcess)
)

type runningProcess struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

func registerRunning(cmd *exec.Cmd, cancel context.CancelFunc) {
	runningMu.Lock()
	running[cmd] = runningProcess{cmd: cmd, cancel: cancel}
	runningMu.Unlock()
}

func unregisterRunning(cmd *exec.Cmd) {
	runningMu.Lock()
	delete(running, cmd)
	runningMu.Unlock()
}

func stopRunning() {
	runningMu.Lock()
	processes := make([]runningProcess, 0, len(running))
	for _, process := range running {
		if process.cmd.Process != nil {
			_ = signalWorkerTree(process.cmd, syscall.SIGTERM)
		}
		processes = append(processes, process)
	}
	runningMu.Unlock()
	if len(processes) == 0 {
		return
	}
	time.AfterFunc(2*time.Second, func() {
		runningMu.Lock()
		defer runningMu.Unlock()
		for _, process := range processes {
			if _, ok := running[process.cmd]; !ok {
				continue
			}
			if process.cmd.Process != nil {
				_ = signalWorkerTree(process.cmd, syscall.SIGKILL)
			}
			if process.cancel != nil {
				process.cancel()
			}
		}
	})
}

func RunQueue(files []string) int {
	exitStatus.set(0)
	numOK.set(0)
	numFailed.set(0)
	queueWeb := queueWebAddress{}
	if *engine.Flag_queueport != "" {
		var err error
		queueWeb, err = newQueueWebAddress(*engine.Flag_queueport)
		if err != nil {
			log.Printf("queue web UI: %v", err)
			exitStatus.set(1)
			return 1
		}
	}
	jobWeb := queueWebAddress{}
	if *engine.Flag_port != "" {
		var err error
		jobWeb, err = newQueueWebAddress(*engine.Flag_port)
		if err != nil {
			log.Printf("worker web UI: %v", err)
			exitStatus.set(1)
			return 1
		}
	}
	s := NewStateTab(files)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	stopSignal := make(chan struct{})
	go func() {
		select {
		case sig := <-signals:
			log.Printf("queue: received %v; stopping active workers", sig)
			exitStatus.set(1)
			s.Stop()
			stopRunning()
		case <-stopSignal:
		}
	}()
	s.PrintTo(os.Stdout)
	if queueWeb.enabled {
		listener, err := s.ListenAndServe(queueWeb.listenAddress())
		if err != nil {
			log.Printf("queue web UI: %v", err)
			exitStatus.set(1)
			signal.Stop(signals)
			close(stopSignal)
			return 1
		}
		path := queueWeb.basePath
		if path == "" {
			path = "/"
		} else if !strings.HasSuffix(path, "/") {
			path += "/"
		}
		fmt.Printf("//Realtime queue overview available at http://%s%s\n", listener.Addr().String(), path)
	}
	s.Run(jobWeb)
	if queueWeb.enabled {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.Shutdown(ctx); err != nil {
			log.Printf("queue web UI shutdown: %v", err)
		}
		cancel()
	}
	fmt.Println(numOK.get(), "OK, ", numFailed.get(), "failed")
	signal.Stop(signals)
	close(stopSignal)
	return int(exitStatus.get())
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
	if port < 1 || port > 65535 {
		return ""
	}
	basePath := a.basePath
	if basePath != "" {
		basePath = strings.TrimSuffix(strings.TrimSuffix(basePath, "/"), "/"+strconv.Itoa(a.port)) + "/" + strconv.Itoa(port)
	}
	return net.JoinHostPort(a.host, strconv.Itoa(port)) + basePath
}

// StateTab holds the queue state (list of jobs + statuses).
// All operations are atomic.
type JobState string

const (
	JobQueued    JobState = "queued"
	JobStarting  JobState = "starting"
	JobRunning   JobState = "running"
	JobReady     JobState = "ready"
	JobSucceeded JobState = "succeeded"
	JobFailed    JobState = "failed"
	JobCancelled JobState = "cancelled"
)

type stateTab struct {
	lock              sync.Mutex
	jobs              []job
	next              int
	stop              bool
	serverMu          sync.Mutex
	server            *http.Server
	publicURLTemplate string
	tunnelRequired    bool
}

type job struct {
	inFile      string
	launchAddr  string
	webAddr     string
	publicURL   string
	errorText   string
	state       JobState
	uid         int
	gpu         int
	started     time.Time
	pid         int
	exitCode    int
	exitCodeSet bool
	tunnelError string
}

func NewStateTab(inFiles []string) *stateTab {
	s := &stateTab{publicURLTemplate: *engine.Flag_webuiPublicURL, tunnelRequired: engine.FlagPassed("tunnel") && strings.TrimSpace(*engine.Flag_tunnel) != ""}
	s.jobs = make([]job, len(inFiles))
	for i, f := range inFiles {
		s.jobs[i] = job{inFile: f, uid: i, state: JobQueued, gpu: -1}
	}
	return s
}

func (s *stateTab) StartNext(webAddr string) (jobCopy job, ok bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.stop || s.next >= len(s.jobs) {
		return job{}, false
	}
	s.jobs[s.next].launchAddr = webAddr
	s.jobs[s.next].state = JobStarting
	s.jobs[s.next].started = time.Now()
	jobCopy = s.jobs[s.next]
	s.next++
	return jobCopy, true
}

func (s *stateTab) SetGPU(uid, gpu int) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if uid >= 0 && uid < len(s.jobs) {
		s.jobs[uid].gpu = gpu
	}
}

func (s *stateTab) SetPID(uid, pid int) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if uid >= 0 && uid < len(s.jobs) && s.jobs[uid].state != JobFailed && s.jobs[uid].state != JobCancelled {
		s.jobs[uid].pid = pid
		if s.jobs[uid].state == JobStarting {
			s.jobs[uid].state = JobRunning
		}
	}
}

func (s *stateTab) SetExitCode(uid, code int) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if uid >= 0 && uid < len(s.jobs) {
		s.jobs[uid].exitCode = code
		s.jobs[uid].exitCodeSet = true
	}
}

func (s *stateTab) SetReady(uid int, addr string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if uid >= 0 && uid < len(s.jobs) && s.jobs[uid].state != JobFailed && s.jobs[uid].state != JobCancelled && s.jobs[uid].state != JobSucceeded {
		s.jobs[uid].webAddr = addr
		s.jobs[uid].state = JobReady
	}
}

func (s *stateTab) SetPublicURL(uid int, addr string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if uid >= 0 && uid < len(s.jobs) {
		s.jobs[uid].publicURL = addr
	}
}

func (s *stateTab) SetTunnelError(uid int, errText string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if uid >= 0 && uid < len(s.jobs) && s.jobs[uid].state != JobCancelled && s.jobs[uid].state != JobSucceeded {
		s.jobs[uid].tunnelError = errText
		s.jobs[uid].state = JobFailed
		s.jobs[uid].errorText = "tunnel: " + errText
	}
}

func (s *stateTab) SetFailed(uid int, errText string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if uid >= 0 && uid < len(s.jobs) && s.jobs[uid].state != JobCancelled && s.jobs[uid].state != JobSucceeded {
		s.jobs[uid].state = JobFailed
		s.jobs[uid].errorText = errText
	}
}

func (s *stateTab) SetCancelled(uid int) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if uid >= 0 && uid < len(s.jobs) && s.jobs[uid].state != JobFailed && s.jobs[uid].state != JobSucceeded {
		s.jobs[uid].state = JobCancelled
	}
}

func (s *stateTab) IsStopped() bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.stop
}

func (s *stateTab) Stop() {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.stop = true
	for i := s.next; i < len(s.jobs); i++ {
		if s.jobs[i].state == JobQueued {
			s.jobs[i].state = JobCancelled
		}
	}
	s.next = len(s.jobs)
}

func (s *stateTab) Finish(j job, outcome runOutcome) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if j.uid < 0 || j.uid >= len(s.jobs) {
		return
	}
	switch outcome {
	case runSucceeded:
		if s.jobs[j.uid].state != JobFailed && s.jobs[j.uid].state != JobCancelled {
			s.jobs[j.uid].state = JobSucceeded
		}
	case runCancelled:
		if s.jobs[j.uid].state != JobFailed {
			s.jobs[j.uid].state = JobCancelled
		}
	}
}

func (s *stateTab) Run(web queueWebAddress) {
	idle, nGPU, err := initGPUs()
	if err != nil {
		log.Printf("queue GPU initialization: %v", err)
		exitStatus.set(1)
		numFailed.inc()
		return
	}
	for {
		gpu := <-idle
		j, ok := s.StartNext(web.jobAddress(gpu))
		if !ok {
			break
		}
		if web.enabled && j.launchAddr == "" {
			s.SetFailed(j.uid, "worker WebUI port is outside 1..65535")
			numFailed.inc()
			exitStatus.set(1)
			if *flag_failfast {
				s.Stop()
				stopRunning()
			}
			idle <- gpu
			continue
		}
		s.SetGPU(j.uid, gpu)
		go func(j job, gpu int) {
			outcome := run(j.inFile, gpu, j.launchAddr,
				func(pid int) { s.SetPID(j.uid, pid) },
				func(addr string) { s.SetReady(j.uid, addr) },
				func(addr string) { s.SetPublicURL(j.uid, addr) },
				func(err error) { s.SetTunnelError(j.uid, err.Error()) },
				func(err error) {
					s.SetFailed(j.uid, err.Error())
					if *flag_failfast {
						s.Stop()
					}
				},
				func(code int) { s.SetExitCode(j.uid, code) },
				func() { s.SetCancelled(j.uid) },
				func() bool { return s.IsStopped() })
			s.Finish(j, outcome)
			idle <- gpu
		}(j, gpu)
	}
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

type runOutcome int

const (
	runSucceeded runOutcome = iota
	runFailed
	runCancelled
)

func run(inFile string, gpu int, webAddr string, onStarted func(int), onReady func(string), onTunnelReady func(string), onFailed func(error), onTunnelFailed func(error), onExited func(int), onCancelled func(), cancelled func() bool) runOutcome {
	gpuFlag := fmt.Sprint("-gpu=", gpu)
	httpFlag := fmt.Sprint("-http=", webAddr)
	flags := []string{gpuFlag, httpFlag}
	seen := map[string]bool{"gpu": true, "http": true}
	var flagErr error
	flag.Visit(func(f *flag.Flag) {
		canonical := engine.CanonicalFlagName(f.Name)
		if canonical != "gpu" && canonical != "http" && canonical != "failfast" && canonical != "max_gpus" && canonical != "webui-queue-addr" && canonical != "webui-public-url" && !seen[canonical] {
			value := f.Value.String()
			if canonical == "tunnel" {
				var err error
				value, err = workerTunnelArgument(value, gpu)
				if err != nil {
					flagErr = err
					return
				}
			}
			seen[canonical] = true
			flags = append(flags, fmt.Sprintf("-%v=%v", canonical, value))
		}
	})
	if flagErr != nil {
		onFailed(flagErr)
		if *flag_failfast {
			stopRunning()
		}
		log.Printf("[%s] %v", inFile, flagErr)
		exitStatus.set(1)
		numFailed.inc()
		return runFailed
	}
	flags = append(flags, inFile)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], flags...)
	configureWorkerProcess(cmd)
	log.Println(os.Args[0], flags)
	fail := func(err error) runOutcome {
		onFailed(err)
		log.Printf("[%s] %v", inFile, err)
		exitStatus.set(1)
		numFailed.inc()
		if *flag_failfast {
			stopRunning()
		}
		return runFailed
	}
	eventReader, eventWriter, err := os.Pipe()
	if err != nil {
		return fail(fmt.Errorf("event pipe: %w", err))
	}
	defer eventReader.Close()
	cmd.ExtraFiles = []*os.File{eventWriter}
	cmd.Env = append(os.Environ(), events.EnvFD+"=3")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		eventWriter.Close()
		return fail(fmt.Errorf("stdout pipe: %w", err))
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		eventWriter.Close()
		return fail(fmt.Errorf("stderr pipe: %w", err))
	}
	if err := cmd.Start(); err != nil {
		eventWriter.Close()
		return fail(fmt.Errorf("start: %w", err))
	}
	onStarted(cmd.Process.Pid)
	eventWriter.Close()
	registerRunning(cmd, cancel)
	defer unregisterRunning(cmd)
	ready := make(chan string, 1)
	protocolErrCh := make(chan error, 1)
	eventDone := make(chan struct{})
	go func() {
		defer close(eventDone)
		err := events.Read(eventReader, func(event events.Event) error {
			switch event.Event {
			case "webui_ready":
				if addr, ok := workerEventAddress(event); ok {
					select {
					case ready <- addr:
					default:
					}
					return nil
				}
				return fmt.Errorf("invalid webui_ready event")
			case "tunnel_ready":
				if publicURL, ok := workerEventPublicURL(event); ok {
					onTunnelReady(publicURL)
					return nil
				}
				return fmt.Errorf("invalid tunnel_ready event")
			case "tunnel_failed":
				message := strings.TrimSpace(event.Error)
				if message == "" {
					message = "worker tunnel failed"
				}
				tunnelErr := errors.New(message)
				onTunnelFailed(tunnelErr)
				return tunnelErr

			case "worker_failed":
				if event.Error == "" {
					return errors.New("worker reported failure")
				}
				return errors.New(event.Error)
			}
			return nil
		})
		if err != nil {
			protocolErrCh <- err
		}
	}()
	streamDone := make(chan struct{}, 2)
	stream := func(r io.Reader) {
		defer func() { streamDone <- struct{}{} }()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if addr, ok := parseWebUIReady(line); ok {
				select {
				case ready <- addr:
				default:
				}
				continue
			}
			if addr, ok := parseTunnelReady(line); ok {
				onTunnelReady(addr)
				continue
			}
			if tunnelErr, ok := parseTunnelFailed(line); ok {
				onTunnelFailed(tunnelErr)
				select {
				case protocolErrCh <- tunnelErr:
				default:
				}
				continue
			}
			log.Printf("[%s] %s", inFile, line)
		}
		if err := scanner.Err(); err != nil {
			log.Printf("[%s] output: %v", inFile, err)
		}
	}
	go stream(stdout)
	go stream(stderr)
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	var waitErr error
	var protocolErr error
	var terminateOnce sync.Once
	var terminateTimer *time.Timer
	terminate := func() {
		terminateOnce.Do(func() {
			_ = signalWorkerTree(cmd, syscall.SIGTERM)
			terminateTimer = time.AfterFunc(2*time.Second, cancel)
		})
	}
	defer func() {
		if terminateTimer != nil {
			terminateTimer.Stop()
		}
	}()
	waitForExit := func() {
		select {
		case waitErr = <-wait:
		case protocolErr = <-protocolErrCh:
			terminate()
			waitErr = <-wait
		}
	}
	readySeen := false
	select {
	case addr := <-ready:
		readySeen = true
		onReady(addr)
		waitForExit()
	case protocolErr = <-protocolErrCh:
		terminate()
		waitErr = <-wait
	case waitErr = <-wait:
	}
	if protocolErr == nil {
		select {
		case protocolErr = <-protocolErrCh:
		default:
		}
	}

	if cmd.ProcessState != nil {
		onExited(cmd.ProcessState.ExitCode())
	}

	eventReader.Close()
	<-eventDone
	<-streamDone
	<-streamDone
	select {
	case protocolErr = <-protocolErrCh:
	default:
	}
	if protocolErr != nil {
		if cancelled() {
			onCancelled()
			return runCancelled
		}
		return fail(protocolErr)
	}
	if waitErr != nil {
		if cancelled() {
			onCancelled()
			return runCancelled
		}
		return fail(waitErr)
	}
	if webAddr != "" && !readySeen {
		return fail(errors.New("worker exited before webui-ready"))
	}
	numOK.inc()
	return runSucceeded
}

func workerTunnelArgument(raw string, gpu int) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || gpu == 0 {
		return raw, nil
	}
	host, port, hasPort, err := splitTunnelEndpoint(raw)
	if err != nil {
		return "", err
	}
	if !hasPort || port == 0 {
		return raw, nil
	}
	if gpu < 0 || port > 65535-gpu {
		return "", fmt.Errorf("tunnel port %d plus GPU offset %d exceeds 65535", port, gpu)
	}
	return net.JoinHostPort(host, strconv.Itoa(port+gpu)), nil
}

func splitTunnelEndpoint(raw string) (string, int, bool, error) {
	if host, portText, err := net.SplitHostPort(raw); err == nil {
		port, parseErr := strconv.Atoi(portText)
		if parseErr != nil || port < 0 || port > 65535 {
			return "", 0, false, fmt.Errorf("invalid tunnel port %q", portText)
		}
		return host, port, true, nil
	}
	if strings.Count(raw, ":") == 1 {
		parts := strings.SplitN(raw, ":", 2)
		port, parseErr := strconv.Atoi(parts[1])
		if parseErr != nil || port < 0 || port > 65535 {
			return "", 0, false, fmt.Errorf("invalid tunnel port %q", parts[1])
		}
		return parts[0], port, true, nil
	}
	if strings.HasPrefix(raw, "[") {
		return "", 0, false, fmt.Errorf("invalid tunnel endpoint %q", raw)
	}
	return raw, 0, false, nil
}

func workerEventAddress(event events.Event) (string, bool) {
	if event.ListenPort < 1 || event.ListenPort > 65535 {
		return "", false
	}
	host := strings.TrimSpace(event.ListenHost)
	if host == "" {
		host = "127.0.0.1"
	}
	addr := net.JoinHostPort(host, strconv.Itoa(event.ListenPort)) + event.BasePath
	if _, _, _, err := parseWebUIAddress(addr); err != nil {
		return "", false
	}
	return addr, true
}

func workerEventPublicURL(event events.Event) (string, bool) {
	if event.PublicURL == "" {
		return "", false
	}
	addr, err := urlpkg.Parse(strings.TrimSpace(event.PublicURL))
	if err != nil || (addr.Scheme != "http" && addr.Scheme != "https") || addr.Host == "" {
		return "", false
	}
	if !strings.HasSuffix(addr.Path, "/") {
		addr.Path += "/"
	}
	return addr.String(), true
}
func parseWebUIReady(line string) (string, bool) {
	const prefix = "//webui-ready "
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	addr := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if _, _, _, err := parseWebUIAddress(addr); err != nil {
		return "", false
	}
	return addr, true
}

func parseTunnelReady(line string) (string, bool) {
	const prefix = "//tunnel-ready "
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	parsed, err := urlpkg.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", false
	}
	if !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}
	return parsed.String(), true
}

func parseTunnelFailed(line string) (error, bool) {
	const prefix = "//tunnel-failed "
	if !strings.HasPrefix(line, prefix) {
		return nil, false
	}
	message := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if message == "" {
		message = "worker tunnel failed"
	}
	return errors.New(message), true
}

// Creates a concurrent channel containing the available GPU IDs for jobs.
// Returns the channel and the number of available GPUs for the queue.
func initGPUs() (idle chan int, nGPU int, err error) {
	var deviceCount int
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("CUDA device enumeration failed: %v", recovered)
			}
		}()
		queueCUDAOnce.Do(func() { cu.Init(0) })
		deviceCount = cu.DeviceGetCount()
	}()
	if err != nil {
		return nil, 0, err
	}
	gpuIDs, err := selectQueueGPUs(deviceCount, engine.FlagPassed("gpu"), *engine.Flag_gpu, *flag_maxGPUs)
	if err != nil {
		return nil, 0, err
	}
	log.Printf("//queue using %d of %d available GPU(s): %v", len(gpuIDs), deviceCount, gpuIDs)
	idle = make(chan int, len(gpuIDs))
	for _, gpu := range gpuIDs {
		idle <- gpu
	}
	return idle, len(gpuIDs), nil
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
		fmt.Fprintf(w, "%3d %-9s %v %v\n", i, j.state, j.inFile, j.webAddr)
	}
}

func (s *stateTab) RenderHTML(w io.Writer, r *http.Request) {
	s.lock.Lock()
	defer s.lock.Unlock()
	fmt.Fprint(w, `
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
		if j.state == JobReady && j.webAddr != "" && (!s.tunnelRequired || j.publicURL != "") {
			link := j.publicURL
			if link == "" {
				if s.publicURLTemplate != "" {
					link = expandPublicURLTemplate(r, s.publicURLTemplate, j.webAddr)
				} else {
					link = queueJobURL(r, j.webAddr)
				}
			}
			if link != "" {
				fmt.Fprint(w, `<b>`, j.uid, ` <a href="`, html.EscapeString(link), `">`, html.EscapeString(j.inFile), " ", html.EscapeString(link), "</a></b>\n")
				continue
			}
		}
		detail := string(j.state)
		if j.errorText != "" {
			detail += ": " + j.errorText
		} else if s.tunnelRequired && j.webAddr != "" && j.publicURL == "" {
			detail += ": waiting for tunnel_ready"
		}
		if j.pid > 0 {
			detail += fmt.Sprintf(" (gpu=%d pid=%d", j.gpu, j.pid)
			if j.exitCodeSet {
				detail += fmt.Sprintf(", exit=%d", j.exitCode)
			}
			detail += ")"
		}
		fmt.Fprint(w, j.uid, " [", html.EscapeString(detail), "] ", html.EscapeString(j.inFile), "\n")
	}

	fmt.Fprint(w, `</pre><hr/></body></html>`)
}

func expandPublicURLTemplate(r *http.Request, template, webAddr string) string {
	_, port, _, err := parseWebUIAddress(webAddr)
	if err != nil {
		return ""
	}
	raw := strings.ReplaceAll(template, "{port}", strconv.Itoa(port))
	if strings.HasPrefix(raw, "/") {
		publicURL := publicRequestURL(r)
		publicURL.Path = raw
		if !strings.HasSuffix(publicURL.Path, "/") {
			publicURL.Path += "/"
		}
		return withPublicURLPathSlash(&publicURL).String()
	}
	publicURL, err := urlpkg.Parse(raw)
	if err != nil || (publicURL.Scheme != "http" && publicURL.Scheme != "https") || publicURL.Host == "" {
		return ""
	}
	if !strings.HasSuffix(publicURL.Path, "/") {
		publicURL.Path += "/"
	}
	return withPublicURLPathSlash(publicURL).String()
}

func (s *stateTab) ListenAndServe(addr string) (net.Listener, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: s}
	s.serverMu.Lock()
	s.server = server
	s.serverMu.Unlock()
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("queue web UI: %v", err)
		}
	}()
	return listener, nil
}

func (s *stateTab) Shutdown(ctx context.Context) error {
	s.serverMu.Lock()
	server := s.server
	s.server = nil
	s.serverMu.Unlock()
	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}
func (s *stateTab) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.RenderHTML(w, r)
}

func withPublicURLPathSlash(publicURL *urlpkg.URL) *urlpkg.URL {
	if publicURL.Path == "" {
		publicURL.Path = "/"
	} else if !strings.HasSuffix(publicURL.Path, "/") {
		publicURL.Path += "/"
	}
	return publicURL
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
		return withPublicURLPathSlash(&publicURL).String()
	}

	if path, ok := replaceTrailingPort(publicRequestPath(r), jobPort); ok {
		publicURL.Path = path
		return withPublicURLPathSlash(&publicURL).String()
	}
	publicURL.Host = hostWithPort(publicURL.Host, jobPort)
	return withPublicURLPathSlash(&publicURL).String()
}

func publicRequestURL(r *http.Request) urlpkg.URL {
	publicURL := urlpkg.URL{Scheme: "http", Host: r.Host}
	if r.TLS != nil {
		publicURL.Scheme = "https"
	}
	if trustedProxyRequest(r) {
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

func trustedProxyRequest(r *http.Request) bool {
	if strings.TrimSpace(*engine.Flag_webuiTrustedProxy) == "" {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return false
	}
	for _, rule := range strings.Split(*engine.Flag_webuiTrustedProxy, ",") {
		rule = strings.TrimSpace(rule)
		if strings.Contains(rule, "/") {
			if _, network, err := net.ParseCIDR(rule); err == nil && network.Contains(ip) {
				return true
			}
		} else if allowed := net.ParseIP(rule); allowed != nil && allowed.Equal(ip) {
			return true
		}
	}
	return false
}

func forwardedPrefix(r *http.Request) string {
	if !trustedProxyRequest(r) {
		return ""
	}
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
