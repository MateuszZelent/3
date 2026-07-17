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

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/mumax/3/engine"
	"github.com/mumax/3/log"
)

func Start(host string, port int, basePath string, tunnel string, debug bool) (int, error) {
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

	api := e.Group(basePath)

	// redirect "" to "/"
	api.GET("", func(c echo.Context) error {
		return c.Redirect(301, basePath+"/")
	})

	// Serve the `index.html` file at the root URL
	api.GET("/", indexFileHandler())

	// Serve the other embedded static files
	api.GET("/*", echo.WrapHandler(staticFileHandler(basePath)))

	wsManager := newWebSocketManager()
	api.GET("/ws", wsManager.websocketEntrypoint)
	api.GET("/ws/preview", wsManager.websocketPreviewEntrypoint)
	wsManager.startBroadcastLoop()
	engineState := initEngineStateAPI(api, wsManager)
	wsManager.engineState = engineState

	return startGuiServer(e, host, basePath, port, tunnel)
}

func startGuiServer(e *echo.Echo, host string, basePath string, startPort int, tunnel string) (int, error) {
	listener, port, err := listenAvailable(host, startPort)
	if err != nil {
		return 0, err
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
	for port := startPort; port <= 65535; port++ {
		address := net.JoinHostPort(host, strconv.Itoa(port))
		listener, err := net.Listen("tcp", address)
		if err == nil {
			return listener, port, nil
		}
	}
	return nil, 0, fmt.Errorf("no available ports found from %s", net.JoinHostPort(host, strconv.Itoa(startPort)))
}
