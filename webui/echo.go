// Package api provides the web API and web UI for amumax.
package webui

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	runtimedebug "runtime/debug"
	"strconv"
	"strings"
	"syscall"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/mumax/3/engine"
	"github.com/mumax/3/log"
)

func Start(host string, port int, basePath string, tunnel string, debug bool) (int, error) {
	listener, actualPort, err := listenAvailable(host, port)
	if err != nil {
		return 0, err
	}
	effectiveBasePath := RetargetBasePath(basePath, port, actualPort)
	e := echo.New()
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowHeaders: []string{"*"},
	}))

	e.HideBanner = true
	if debug {
		e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
			Format: "method=${method}, uri=${uri}, status=${status}\n",
		}))
	} else {
		e.Logger.SetOutput(io.Discard)
	}

	api := e.Group(effectiveBasePath)

	// redirect "" to "/"
	api.GET("", func(c echo.Context) error {
		return c.Redirect(301, effectiveBasePath+"/")
	})

	// Serve the `index.html` file at the root URL
	api.GET("/", indexFileHandler())

	// Serve the other embedded static files
	api.GET("/*", echo.WrapHandler(staticFileHandler(effectiveBasePath)))

	wsManager := newWebSocketManager()
	api.GET("/ws", wsManager.websocketEntrypoint)
	api.GET("/ws/preview", wsManager.websocketPreviewEntrypoint)
	wsManager.startBroadcastLoop()
	engineState := initEngineStateAPI(api, wsManager)
	wsManager.engineState = engineState

	return startGuiServer(e, effectiveBasePath, actualPort, tunnel, listener)
}

// RetargetBasePath replaces a trailing preferred worker port with the actual
// port selected after automatic port allocation. Other paths are unchanged.
func RetargetBasePath(basePath string, preferredPort, actualPort int) string {
	if basePath == "" || preferredPort < 1 || actualPort < 1 || preferredPort == actualPort {
		return basePath
	}
	trimmed := strings.TrimSuffix(basePath, "/")
	suffix := "/" + strconv.Itoa(preferredPort)
	if !strings.HasSuffix(trimmed, suffix) {
		return basePath
	}
	return strings.TrimSuffix(trimmed, suffix) + "/" + strconv.Itoa(actualPort)
}

func startGuiServer(e *echo.Echo, basePath string, port int, tunnel string, listener net.Listener) (int, error) {
	if listener == nil {
		return 0, fmt.Errorf("nil WebUI listener")
	}
	addr := listener.Addr().String()
	log.Log.Info("Serving the web UI at http://%s%s", addr, basePath)

	engine.WebMetadata.Add("webui", addr)
	engine.WebMetadata.Add("port", port)
	if tunnel != "" {
		go startTunnel(tunnel)
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Log.Warn("WebUI crashed: %v\n%s", r, runtimedebug.Stack())
			}
		}()
		if err := e.Server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Log.Err("WebUI server stopped: %v", err)
		}
	}()

	return port, nil
}

func listenAvailable(host string, startPort int) (net.Listener, int, error) {
	const maxAutoPortAttempts = 100
	if startPort < 1 || startPort > 65535 {
		return nil, 0, fmt.Errorf("invalid start port %d", startPort)
	}
	for offset := 0; offset < maxAutoPortAttempts; offset++ {
		port := startPort + offset
		if port > 65535 {
			break
		}
		address := net.JoinHostPort(host, strconv.Itoa(port))
		listener, err := net.Listen("tcp", address)
		if err == nil {
			return listener, port, nil
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, 0, fmt.Errorf("cannot listen on %s: %w", address, err)
		}
	}
	return nil, 0, fmt.Errorf("no available ports found from %s", net.JoinHostPort(host, strconv.Itoa(startPort)))
}
