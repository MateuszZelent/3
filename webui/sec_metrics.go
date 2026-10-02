package webui

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"

	"github.com/mumax/3/log"
)

type metricsCollector struct {
	mu     sync.Mutex
	busy   bool
	result *MetricsState
}

type MetricsState struct {
	collector *metricsCollector
	ws        *WebSocketManager

	lastGPUUpdate      time.Time
	gpuRefreshInterval time.Duration

	PID             int     `msgpack:"pid"`
	Error           string  `msgpack:"error"`
	CPUPercent      float64 `msgpack:"cpuPercent"`
	CPUPercentTotal float64 `msgpack:"cpuPercentTotal"`
	RAMPercent      float32 `msgpack:"ramPercent"`
	RAMPercentTotal float64 `msgpack:"ramPercentTotal"`

	GPUName               string  `msgpack:"gpuName"`
	GPUUUID               string  `msgpack:"gpuUUID"`
	GPUUtilizationPercent int     `msgpack:"gpuUtilizationPercent"`
	GPUTemperature        int     `msgpack:"gpuTemperature"`
	GPUPowerDraw          float64 `msgpack:"gpuPowerDraw"`
	GPUPowerLimit         float64 `msgpack:"gpuPowerLimit"`
	GPUVramUsed           int     `msgpack:"gpuVramUsed"`
	GPUVramTotal          int     `msgpack:"gpuVramTotal"`
}

func initMetricsAPI(e *echo.Group, ws *WebSocketManager) *MetricsState {
	pid := os.Getpid()
	metricState := &MetricsState{
		PID:                pid,
		collector:          &metricsCollector{},
		ws:                 ws,
		gpuRefreshInterval: 5 * time.Second,
	}
	e.POST("/api/metrics/reset-error", metricState.postMetricsReset)
	return metricState
}

// Poll external processes and OS counters without holding the WebUI state lock.
func (m *MetricsState) Update() {
	// Metrics are UI telemetry, not solver work. Never start a new collector
	// at server startup or after the last main client disconnects.
	if m.ws != nil && m.ws.connections.count() == 0 {
		return
	}
	if m.collector == nil {
		m.collector = &metricsCollector{}
	}
	c := m.collector
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.result != nil {
		ws := m.ws
		*m = *c.result
		m.ws, m.collector = ws, c
		c.result = nil
	}
	if c.busy || m.Error != "" || (!m.lastGPUUpdate.IsZero() && time.Since(m.lastGPUUpdate) < m.gpuRefreshInterval) {
		return
	}
	c.busy = true
	snapshot := *m
	go func() {
		snapshot.getProgramCPUUsage()
		snapshot.getTotalCPUUsage()
		snapshot.getTotalRAMUsage()
		snapshot.getGPUStats1()
		snapshot.getGPUStats2()
		snapshot.lastGPUUpdate = time.Now()
		c.mu.Lock()
		c.result = &snapshot
		c.busy = false
		c.mu.Unlock()
	}()
}

// Function to get GPU stats via `nvidia-smi`
func (m *MetricsState) getGPUStats1() {
	// Get all compute apps, filter by pid, and get the gpu uuid, name, and vram used
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "nvidia-smi",
		"--query-compute-apps=pid,used_gpu_memory,gpu_name,gpu_uuid",
		"--format=csv,nounits,noheader",
	)
	expectedFields := 4
	output, err := cmd.Output()
	if err != nil {
		m.Error = "Error getting gpu stats"
		log.Log.Warn("%s", m.Error)
		return
	}
	var pidFields []string
	// find the line with the pid
	lines := strings.SplitSeq(string(output), "\n")
	for line := range lines {
		fields := strings.Split(line, ", ")
		// the last line is empty
		if line == "" {
			continue
		}
		if len(fields) == expectedFields {
			pid, err1 := strconv.Atoi(fields[0])
			if err1 != nil {
				m.Error = "Error parsing pid"
				log.Log.Warn("%s", m.Error)
				return
			}
			if pid == m.PID {
				pidFields = fields
			}
		} else {
			m.Error = fmt.Sprintf("Expected %d fields in nvidia-smi output, got %v: %s", expectedFields, len(fields), output)
			log.Log.Warn("%s", m.Error)
			return
		}
	}
	if pidFields == nil {
		m.Error = "Couldn't find the process ID in nvidia-smi output, this is normal if running inside a container"
		log.Log.Warn("%s", m.Error)
		return
	}
	m.GPUVramUsed, err = strconv.Atoi(pidFields[1])
	if err != nil {
		m.Error = "Error parsing vram used"
		log.Log.Warn("%s", m.Error)
		return
	}
	m.GPUName = pidFields[2]
	m.GPUUUID = pidFields[3]
}

func (m *MetricsState) getGPUStats2() {
	if m.GPUUUID == "" {
		return
	}
	// filter using the gpu uuid
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu=uuid,temperature.gpu,power.draw,memory.total,utilization.gpu,power.limit",
		"--format=csv,nounits,noheader",
	)
	expectedFields := 6
	output, err := cmd.Output()
	if err != nil {
		m.Error = "Error getting gpu stats"
		log.Log.Warn("%s", m.Error)
		return
	}
	var UUIDFields []string
	// find the line with the uuid
	lines := strings.SplitSeq(string(output), "\n")
	for line := range lines {
		// the last line is empty
		if line == "" {
			continue
		}
		fields := strings.Split(line, ", ")
		if len(fields) == expectedFields {
			if fields[0] == m.GPUUUID {
				UUIDFields = fields
			}
		} else {
			m.Error = fmt.Sprintf("Expected %d fields in nvidia-smi output, got %v: %s", expectedFields, len(fields), output)
			log.Log.Warn("%s", m.Error)
			return
		}
	}
	if UUIDFields == nil {
		m.Error = "No correct UUID found in nvidia-smi output"
		log.Log.Warn("%s", m.Error)
		return
	}
	m.GPUTemperature, err = strconv.Atoi(UUIDFields[1]) // temperature in C
	if err != nil {
		m.Error = fmt.Sprintf("Error parsing GPU temperature: %v", err)
		log.Log.Warn("%s", m.Error)
		return
	}
	m.GPUPowerDraw, err = strconv.ParseFloat(UUIDFields[2], 32) // power draw in W
	if err != nil {
		m.Error = fmt.Sprintf("Error parsing power draw: %v", err)
		log.Log.Warn("%s", m.Error)
		return
	}
	m.GPUVramTotal, err = strconv.Atoi(UUIDFields[3]) // vram total in MiB
	if err != nil {
		m.Error = fmt.Sprintf("Error parsing vram total: %v", err)
		log.Log.Warn("%s", m.Error)
		return
	}
	m.GPUUtilizationPercent, err = strconv.Atoi(UUIDFields[4]) // gpu utilization in %
	if err != nil {
		m.Error = fmt.Sprintf("Error parsing gpu utilization: %v", err)
		log.Log.Warn("%s", m.Error)
		return
	}
	m.GPUPowerLimit, err = strconv.ParseFloat(UUIDFields[5], 32) // power limit in W
	if err != nil {
		m.Error = fmt.Sprintf("Error parsing power limit: %v", err)
		log.Log.Warn("%s", m.Error)
		return
	}
}

// Get total system CPU usage
func (m *MetricsState) getTotalCPUUsage() {
	totalCPUArray, err := cpu.Percent(0, false)
	if err != nil {
		m.Error = fmt.Sprintf("error getting total CPU usage: %v", err)
		log.Log.Warn("%s", m.Error)
		return
	}
	m.CPUPercentTotal = totalCPUArray[0]
}

// Get program-specific CPU usage
func (m *MetricsState) getProgramCPUUsage() {
	proc, err := process.NewProcess(int32(m.PID))
	if err != nil {
		m.Error = fmt.Sprintf("error getting process information: %v", err)
		log.Log.Warn("%s", m.Error)
		return
	}
	numCPU, err := cpu.Counts(true)
	if err != nil {
		m.Error = fmt.Sprintf("error getting CPU count: %v", err)
		log.Log.Warn("%s", m.Error)
		return
	}

	cpuPercent, err := proc.CPUPercent()
	if err != nil {
		m.Error = fmt.Sprintf("error getting program CPU usage: %v", err)
		log.Log.Warn("%s", m.Error)
		return
	}
	m.CPUPercent = cpuPercent / float64(numCPU)

	m.RAMPercent, err = proc.MemoryPercent()
	if err != nil {
		m.Error = fmt.Sprintf("error getting program RAM usage: %v", err)
		log.Log.Warn("%s", m.Error)
		return
	}
}

// Get system RAM usage
func (m *MetricsState) getTotalRAMUsage() {
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		m.Error = fmt.Sprintf("error getting RAM usage: %v", err)
		log.Log.Warn("%s", m.Error)
		return
	}
	m.RAMPercentTotal = vmStat.UsedPercent
}

func (m *MetricsState) postMetricsReset(c echo.Context) error {
	m.ws.stateMu.Lock()
	m.Error = ""
	m.lastGPUUpdate = time.Time{}
	m.ws.stateMu.Unlock()
	m.ws.broadcastEngineStateWithoutPreview() // Use the instance to call the method
	return c.JSON(http.StatusOK, "")
}
