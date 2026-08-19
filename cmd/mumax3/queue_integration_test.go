package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mumax/3/events"
	"github.com/mumax/3/webui"
)

func TestQueueWorkerHelperProcess(t *testing.T) {
	if os.Getenv("MUMAX_QUEUE_HELPER") != "1" {
		return
	}
	preferred, err := strconv.Atoi(os.Getenv("MUMAX_HELPER_PORT"))
	if err != nil {
		t.Fatal(err)
	}
	basePath := os.Getenv("MUMAX_HELPER_BASE")
	var listener net.Listener
	var actual int
	for offset := 0; offset < 100; offset++ {
		port := preferred + offset
		listener, err = net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			actual = port
			break
		}
	}
	if listener == nil {
		t.Fatalf("could not bind helper from %d: %v", preferred, err)
	}
	defer listener.Close()
	effectiveBase := webui.RetargetBasePath(basePath, preferred, actual)
	if err := events.Emit(events.Event{Event: "webui_ready", ListenHost: "127.0.0.1", ListenPort: actual, BasePath: effectiveBase}); err != nil {
		t.Fatal(err)
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc(effectiveBase+"/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "helper worker ready")
	})
	mux.HandleFunc(effectiveBase+"/api/ping", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		_, _ = io.WriteString(w, "pong")
	})
	wsHandler := func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte("ready"))
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}
	mux.HandleFunc(effectiveBase+"/ws", wsHandler)
	mux.HandleFunc(effectiveBase+"/ws/preview", wsHandler)
	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	signals := make(chan os.Signal, 1)
	signalNotify(signals)
	<-signals
	_ = server.Close()
}

// signalNotify is a variable so the helper remains easy to run in the test
// binary without changing production signal handling.
var signalNotify = func(ch chan<- os.Signal) { signal.Notify(ch, syscall.SIGTERM, os.Interrupt) }

type queueHelperProcess struct {
	cmd    *exec.Cmd
	reader *os.File
	events chan events.Event
}

func startQueueHelper(t *testing.T, preferred int, basePath string) queueHelperProcess {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestQueueWorkerHelperProcess$")
	cmd.Env = append(os.Environ(),
		"MUMAX_QUEUE_HELPER=1",
		"MUMAX_HELPER_PORT="+strconv.Itoa(preferred),
		"MUMAX_HELPER_BASE="+basePath,
		events.EnvFD+"=3",
	)
	cmd.ExtraFiles = []*os.File{writer}
	if err := cmd.Start(); err != nil {
		reader.Close()
		writer.Close()
		t.Fatal(err)
	}
	_ = writer.Close()
	got := make(chan events.Event, 1)
	go func() {
		_ = events.Read(reader, func(event events.Event) error {
			got <- event
			return nil
		})
	}()
	return queueHelperProcess{cmd: cmd, reader: reader, events: got}
}

func (p queueHelperProcess) stop(t *testing.T) {
	t.Helper()
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Signal(syscall.SIGTERM)
	}
	_ = p.cmd.Wait()
	_ = p.reader.Close()
}

func reservePortPair(t *testing.T) (int, net.Listener, net.Listener) {
	t.Helper()
	for port := 39000; port < 45000; port++ {
		first, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			continue
		}
		second, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port+1)))
		if err == nil {
			return port, first, second
		}
		_ = first.Close()
	}
	t.Fatal("could not reserve a port pair")
	return 0, nil, nil
}

func waitQueueHelperEvent(t *testing.T, p queueHelperProcess) events.Event {
	t.Helper()
	select {
	case event := <-p.events:
		if event.Event != "webui_ready" {
			t.Fatalf("event = %+v", event)
		}
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for webui_ready")
		return events.Event{}
	}
}

func TestQueueWorkersUseActualPortsAndProxyPaths(t *testing.T) {
	preferred, occupied1, occupied2 := reservePortPair(t)
	defer occupied1.Close()
	defer occupied2.Close()
	first := startQueueHelper(t, preferred, "/proxy/"+strconv.Itoa(preferred))
	second := startQueueHelper(t, preferred+1, "/proxy/"+strconv.Itoa(preferred+1))
	defer first.stop(t)
	defer second.stop(t)
	firstEvent := waitQueueHelperEvent(t, first)
	secondEvent := waitQueueHelperEvent(t, second)
	if firstEvent.ListenPort == preferred || secondEvent.ListenPort == preferred+1 {
		t.Fatalf("helper kept an occupied preferred port: %+v %+v", firstEvent, secondEvent)
	}
	if firstEvent.ListenPort == secondEvent.ListenPort {
		t.Fatalf("helpers collided on actual port %d", firstEvent.ListenPort)
	}
	if firstEvent.BasePath != "/proxy/"+strconv.Itoa(firstEvent.ListenPort) || secondEvent.BasePath != "/proxy/"+strconv.Itoa(secondEvent.ListenPort) {
		t.Fatalf("base paths were not retargeted: %+v %+v", firstEvent, secondEvent)
	}

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:35366/proxy/35366/", nil)
	firstAddr := net.JoinHostPort(firstEvent.ListenHost, strconv.Itoa(firstEvent.ListenPort)) + firstEvent.BasePath
	secondAddr := net.JoinHostPort(secondEvent.ListenHost, strconv.Itoa(secondEvent.ListenPort)) + secondEvent.BasePath
	firstLink := queueJobURL(request, firstAddr)
	secondLink := queueJobURL(request, secondAddr)
	if !strings.HasSuffix(firstLink, firstEvent.BasePath+"/") || !strings.HasSuffix(secondLink, secondEvent.BasePath+"/") {
		t.Fatalf("queue links do not use actual routes: %q %q", firstLink, secondLink)
	}

	for _, event := range []events.Event{firstEvent, secondEvent} {
		base := "http://127.0.0.1:" + strconv.Itoa(event.ListenPort) + event.BasePath
		response, err := http.Get(base + "/")
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET status = %d", response.StatusCode)
		}
		_ = response.Body.Close()
		request, err := http.NewRequest(http.MethodPost, base+"/api/ping", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		response, err = http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("POST status = %d", response.StatusCode)
		}
		_ = response.Body.Close()
		for _, path := range []string{"/ws", "/ws/preview"} {
			wsURL := "ws://127.0.0.1:" + strconv.Itoa(event.ListenPort) + event.BasePath + path
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, message, err := conn.ReadMessage()
			if err != nil || string(message) != "ready" {
				t.Fatalf("WebSocket %s message = %q, err = %v", path, message, err)
			}
			_ = conn.Close()
		}
	}
}
