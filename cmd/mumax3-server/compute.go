package main

/*
	Compute service runs jobs on this node's GPUs, if any.
*/

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mumax/3/cuda/cu"
	"github.com/mumax/3/events"
	"github.com/mumax/3/httpfs"
	"github.com/mumax/3/util"
)

var (
	MumaxVersion string
	GPUs         []string
	Processes    = make(map[string]*Process) // job id -> process
)

// Process is a running simulation process
type Process struct {
	*exec.Cmd
	Start       time.Time
	Out         io.WriteCloser
	Stdout      io.ReadCloser
	EventReader io.ReadCloser
	EventWriter *os.File
	ID          string
	OutputURL   string
	GUI         string
	Killed      bool
}

type synchronizedWriteCloser struct {
	mu sync.Mutex
	w  io.WriteCloser
}

func (w *synchronizedWriteCloser) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(data)
}

func (w *synchronizedWriteCloser) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Close()
}

func (p *Process) Host() string {
	return JobHost(p.OutputURL)
}

// Runs a compute service on this node, if GPUs are available.
// The compute service asks storage nodes for a job, runs it,
// saves results over httpfs and notifies storage when ready.
func RunComputeService() {

	if len(GPUs) == 0 {
		return
	}

	// queue of available GPU numbers
	idle := make(chan int, len(GPUs))
	for i := range GPUs {
		idle <- i
	}

	for {
		jobGPU := <-idle // take an available GPU
		jobGUIAddr := fmt.Sprint(thisHost+":", GUI_PORT+jobGPU)
		jobID := WaitForJob() // take an available job
		go func(gpu int, ID, GUIAddr string) {

			defer func() {
				// remove from "running" list
				WLock()
				delete(Processes, ID)
				WUnlock()
				// add GPU number back to idle stack
				idle <- gpu
			}()

			p := NewProcess(ID, gpu, GUIAddr)
			if p == nil {
				return
			}

			WLock()
			Processes[ID] = p
			WUnlock()

			p.Run()

			_, err := RPCCall(JobHost(ID), "UpdateJob", ID)
			if err != nil {
				log.Println(err)
			}

		}(jobGPU, jobID, jobGUIAddr)
	}
}

func WaitForJob() string {
	ID := FindJob()
	for ID == "" {
		time.Sleep(2 * time.Second) // TODO: don't poll
		ID = FindJob()
	}
	return ID
}

func FindJob() string {

	// quickly list peers first
	RLock()
	p := make([]string, 0, len(peers))
	for addr := range peers {
		p = append(p, addr)
	}
	RUnlock()
	// TODO: pick peers fairly

	// then do slow RPC calls without blocking the rest of the program
	for _, addr := range p {
		ID, _ := RPCCall(addr, "GiveJob", thisAddr)
		if ID != "" {
			return ID
		}
	}
	return ""
}

// RPC-callable function kills job corresponding to given job id.
// The job has to be running on this node.
func Kill(id string) string {
	log.Println("KILL", id)

	WLock() // modifies Cmd state
	defer WUnlock()

	job := Processes[id]
	if job == nil {
		return fmt.Sprintf("kill %v: job not running.", id)
	}
	job.Killed = true
	err := job.Cmd.Process.Kill()
	if err != nil {
		return err.Error()
	}
	return "" // OK
}

// prepare exec.Cmd to run mumax3 compute process
func NewProcess(ID string, gpu int, webAddr string) *Process {
	inputURL := "http://" + ID
	command := *flag_mumax
	gpuFlag := fmt.Sprint("-gpu=", gpu)
	httpFlag := fmt.Sprint("-http=", webAddr)
	cacheFlag := fmt.Sprint("-cache=", *flag_cachedir)
	forceFlag := "-f=0"
	cmd := exec.Command(command, gpuFlag, httpFlag, cacheFlag, forceFlag, inputURL)
	outDir := util.NoExt(inputURL) + ".out"
	if err := httpfs.Mkdir(outDir); err != nil {
		SetJobError(ID, err)
		log.Println("makeProcess", err)
		if j := JobByName(ID); j != nil {
			j.Reque()
		}
		return nil
	}
	out, err := httpfs.Create(outDir + "/stdout.txt")
	if err != nil {
		SetJobError(ID, err)
		log.Println("makeProcess", err)
		if j := JobByName(ID); j != nil {
			j.Reque()
		}
		return nil
	}
	output := &synchronizedWriteCloser{w: out}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		output.Close()
		SetJobError(ID, err)
		return nil
	}
	eventReader, eventWriter, err := os.Pipe()
	if err != nil {
		output.Close()
		SetJobError(ID, err)
		return nil
	}
	cmd.Stderr = output
	cmd.ExtraFiles = []*os.File{eventWriter}
	cmd.Env = append(os.Environ(), events.EnvFD+"=3")
	return &Process{ID: ID, Cmd: cmd, Start: time.Now(), Out: output, Stdout: stdout, EventReader: eventReader, EventWriter: eventWriter, OutputURL: OutputDir(inputURL), GUI: ""}
}
func (p *Process) Run() {
	log.Println("=> exec  ", p.Path, p.Args)
	defer p.Out.Close()
	defer p.EventReader.Close()
	httpfs.Put(p.OutputURL+"host", []byte(thisAddr))
	startTime := AskTime(p.Host())
	httpfs.Put(p.OutputURL+"start", []byte(startTime.Format(time.UnixDate)))
	WLock()
	err1 := p.Cmd.Start()
	WUnlock()
	if p.EventWriter != nil {
		p.EventWriter.Close()
		p.EventWriter = nil
	}
	if err1 != nil {
		SetJobError(p.ID, err1)
	}
	timeOffset := time.Now().Sub(startTime)
	stopHeartbeat := make(chan struct{})
	heartbeatDone := make(chan struct{})
	httpfs.Put(p.OutputURL+"alive", []byte(time.Now().Add(timeOffset).Format(time.UnixDate)))
	go func() {
		ticker := time.NewTicker(KeepaliveInterval)
		defer ticker.Stop()
		defer close(heartbeatDone)
		for {
			select {
			case t := <-ticker.C:
				httpfs.Put(p.OutputURL+"alive", []byte(t.Add(timeOffset).Format(time.UnixDate)))
			case <-stopHeartbeat:
				return
			}
		}
	}()
	outputDone := make(chan struct{})
	eventDone := make(chan struct{})
	if err1 == nil {
		go p.captureOutput(outputDone)
		go p.captureEvents(eventDone)
	} else {
		close(outputDone)
		p.EventReader.Close()
		close(eventDone)
	}
	var err2 error
	if err1 == nil {
		err2 = p.Cmd.Wait()
	} else {
		err2 = err1
	}
	close(stopHeartbeat)
	<-heartbeatDone
	if err1 == nil {
		p.EventReader.Close()
	}
	<-outputDone
	<-eventDone
	if err1 == nil && err2 != nil {
		SetJobError(p.ID, err2)
	}
	status := 1
	if err1 == nil && err2 == nil {
		status = 0
	} else {
		log.Println(p.Path, p.Args, err1, err2)
	}
	if p.Killed {
		httpfs.Put(p.OutputURL+"killed", []byte(time.Now().Format(time.UnixDate)))
	} else {
		httpfs.Put(p.OutputURL+"exitstatus", []byte(fmt.Sprint(status)))
	}
	stopTime := AskTime(p.Host())
	nanos := stopTime.Sub(startTime).Nanoseconds()
	httpfs.Put(p.OutputURL+"duration", []byte(fmt.Sprint(nanos)))
	if status == 0 {
		ret, err := RPCCall(p.Host(), "AddFairShare", JobUser(p.ID)+"/"+fmt.Sprint(nanos/1e9))
		if err != nil || ret != "" {
			log.Println("***ERR: AddFairShare", JobUser(p.ID), ret, err)
		}
	}
}

func (p *Process) Duration() time.Duration { return Since(time.Now(), p.Start) }

func (p *Process) captureOutput(done chan<- struct{}) {
	defer close(done)
	if p.Stdout == nil {
		return
	}
	scanner := bufio.NewScanner(p.Stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		_, _ = fmt.Fprintln(p.Out, line)
		if addr, ok := parseWebUIReadyLine(line); ok {
			WLock()
			p.GUI = addr
			WUnlock()
		}
	}
	if err := scanner.Err(); err != nil {
		log.Println("stdout scanner", p.ID, err)
	}
}

func (p *Process) captureEvents(done chan<- struct{}) {
	defer close(done)
	if p.EventReader == nil {
		return
	}
	err := events.Read(p.EventReader, func(event events.Event) error {
		if event.Event != "webui_ready" {
			return nil
		}
		addr, ok := eventGUIAddress(event)
		if !ok {
			return fmt.Errorf("invalid webui_ready event")
		}
		WLock()
		p.GUI = addr
		WUnlock()
		return nil
	})
	if err != nil {
		log.Println("worker event stream", p.ID, err)
	}
}

func eventGUIAddress(event events.Event) (string, bool) {
	if event.ListenPort < 1 || event.ListenPort > 65535 {
		return "", false
	}
	host := strings.TrimSpace(event.ListenHost)
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, strconv.Itoa(event.ListenPort)) + event.BasePath, true
}

func parseWebUIReadyLine(line string) (string, bool) {
	const prefix = "//webui-ready "
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	raw := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	parts := strings.SplitN(raw, "/", 2)
	host, portText, err := net.SplitHostPort(parts[0])
	if err != nil {
		return "", false
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", false
	}
	basePath := ""
	if len(parts) == 2 && parts[1] != "" {
		basePath = "/" + strings.Trim(parts[1], "/")
	}
	return net.JoinHostPort(host, strconv.Itoa(port)) + basePath, true
}

func DetectGPUs() {
	if GPUs != nil {
		panic("multiple DetectGPUs() calls")
	}
	deviceCount := 0
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Println("CUDA device enumeration failed:", recovered)
			}
		}()
		cu.Init(0)
		deviceCount = cu.DeviceGetCount()
	}()
	for i := 0; i < deviceCount; i++ {
		gpuflag := fmt.Sprint("-gpu=", i)
		out, err := exec.Command(*flag_mumax, "-test", gpuflag).Output()
		if err != nil {
			continue
		}
		info := strings.TrimSuffix(string(out), "\n")
		log.Println("gpu", i, ":", info)
		GPUs = append(GPUs, info)
	}
}
func DetectMumax() {
	out, err := exec.Command(*flag_mumax, "-test", "-v").CombinedOutput()
	info := string(out)
	if err == nil {
		split := strings.SplitN(info, "\n", 2)
		version := split[0]
		log.Println("have", version)
		MumaxVersion = version
	} else {
		MumaxVersion = fmt.Sprint(*flag_mumax, "-test", ": ", err, info)
	}
}

// RPC-callable function, answers by this node's time
func WhatsTheTime(string) string {
	return time.Now().Format(time.UnixDate)
}

func AskTime(host string) time.Time {
	str, _ := RPCCall(host, "WhatsTheTime", "")
	return parseTime(str)
}

func parseTime(str string) time.Time {
	t, _ := time.Parse(time.UnixDate, str)
	return t
}
