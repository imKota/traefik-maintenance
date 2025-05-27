// Package traefik_maintenance_warden provides a Traefik plugin to redirect traffic to a maintenance page
// while allowing requests with a specific header to bypass the redirection
package traefik_maintenance_warden

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// LogLevel defines the level of logging
type LogLevel int

const (
	// LogLevelNone disables logging
	LogLevelNone LogLevel = iota
	// LogLevelError logs only errors
	LogLevelError
	// LogLevelInfo logs info and errors
	LogLevelInfo
	// LogLevelDebug logs debug, info and errors
	LogLevelDebug
)

// Config holds the plugin configuration.
type Config struct {
	// MaintenanceService is the URL of the maintenance service to redirect to
	MaintenanceService string `json:"maintenanceService,omitempty"`

	// MaintenanceFilePath is the path to a static HTML file to serve instead of redirecting
	MaintenanceFilePath string `json:"maintenanceFilePath,omitempty"`

	// MaintenanceContent is the direct HTML content to serve instead of a file or service
	MaintenanceContent string `json:"maintenanceContent,omitempty"`

	// BypassHeader is the header name that allows bypassing maintenance mode
	BypassHeader string `json:"bypassHeader,omitempty"`

	// BypassHeaderValue is the expected value of the bypass header
	BypassHeaderValue string `json:"bypassHeaderValue,omitempty"`

	// Enabled controls whether the maintenance mode is active
	Enabled bool `json:"enabled,omitempty"`

	// StatusCode is the HTTP status code to return when in maintenance mode
	StatusCode int `json:"statusCode,omitempty"`

	// BypassPaths are paths that should bypass maintenance mode
	BypassPaths []string `json:"bypassPaths,omitempty"`

	// BypassFavicon controls whether favicon.ico requests bypass maintenance mode
	BypassFavicon bool `json:"bypassFavicon,omitempty"`

	// LogLevel controls the verbosity of logging (0=none, 1=error, 2=info, 3=debug)
	LogLevel int `json:"logLevel,omitempty"`

	// MaintenanceTimeout is the timeout for requests to the maintenance service in seconds
	MaintenanceTimeout int `json:"maintenanceTimeout,omitempty"`

	// ContentType is the content type header to set when serving the maintenance file
	ContentType string `json:"contentType,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		MaintenanceService:  "",
		MaintenanceFilePath: "",
		MaintenanceContent:  "",
		BypassHeader:        "X-Maintenance-Bypass",
		BypassHeaderValue:   "true",
		Enabled:             true,
		StatusCode:          503,
		BypassPaths:         []string{},
		BypassFavicon:       true,
		LogLevel:            int(LogLevelError),
		MaintenanceTimeout:  10,
		ContentType:         "text/html; charset=utf-8",
	}
}

// MaintenanceBypass is a middleware that redirects all traffic to a maintenance page
// unless the request has a specific bypass header.
type MaintenanceBypass struct {
	next                   http.Handler
	maintenanceService     *url.URL
	maintenanceFilePath    string
	maintenanceFileContent []byte
	maintenanceContent     string
	maintenanceFileLastMod time.Time
	fileMutex              sync.RWMutex
	bypassHeader           string
	bypassHeaderValue      string
	enabled                bool
	statusCode             int
	bypassPaths            []string
	bypassFavicon          bool
	name                   string
	logger                 *log.Logger
	logLevel               LogLevel
	timeout                time.Duration
	contentType            string
}

// New creates a new MaintenanceBypass middleware.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	// Default to 503 Service Unavailable if not specified
	statusCode := config.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusServiceUnavailable
	}

	// Default content type if not specified
	contentType := config.ContentType
	if contentType == "" {
		contentType = "text/html; charset=utf-8"
	}

	// Create logger
	logger := log.New(os.Stdout, "[maintenance-warden] ", log.LstdFlags)

	// Create the middleware instance
	m := &MaintenanceBypass{
		next:                next,
		maintenanceFilePath: config.MaintenanceFilePath,
		maintenanceContent:  config.MaintenanceContent,
		bypassHeader:        config.BypassHeader,
		bypassHeaderValue:   config.BypassHeaderValue,
		enabled:             config.Enabled,
		statusCode:          statusCode,
		bypassPaths:         config.BypassPaths,
		bypassFavicon:       config.BypassFavicon,
		name:                name,
		logger:              logger,
		logLevel:            LogLevel(config.LogLevel),
		contentType:         contentType,
	}

	// If maintenance file path is specified, try to read it initially
	if config.MaintenanceFilePath != "" {
		err := m.loadMaintenanceFile()
		if err != nil {
			return nil, fmt.Errorf("failed to load maintenance file: %w", err)
		}
	} else if config.MaintenanceContent != "" {
		// If direct content is provided, use that
		m.log(LogLevelInfo, "Using provided maintenance content (%d bytes)", len(config.MaintenanceContent))
	} else if config.MaintenanceService != "" {
		// Validate maintenance service URL
		maintenanceURL, err := url.Parse(config.MaintenanceService)
		if err != nil {
			return nil, fmt.Errorf("invalid maintenance service URL: %w", err)
		}

		if maintenanceURL.Scheme == "" || maintenanceURL.Host == "" {
			return nil, fmt.Errorf("maintenance service URL must include scheme and host")
		}

		// Set default timeout if not specified
		timeout := time.Duration(config.MaintenanceTimeout) * time.Second
		if timeout == 0 {
			timeout = 10 * time.Second
		}

		m.maintenanceService = maintenanceURL
		m.timeout = timeout
	} else {
		return nil, fmt.Errorf("either maintenanceService, maintenanceFilePath, or maintenanceContent must be specified")
	}

	return m, nil
}

// loadMaintenanceFile reads the maintenance HTML file from disk
func (m *MaintenanceBypass) loadMaintenanceFile() error {
	m.fileMutex.Lock()
	defer m.fileMutex.Unlock()

	fileInfo, err := os.Stat(m.maintenanceFilePath)
	if err != nil {
		return fmt.Errorf("error accessing maintenance file: %w", err)
	}

	// Only reload if file is newer than our last modification time
	if m.maintenanceFileContent != nil && !fileInfo.ModTime().After(m.maintenanceFileLastMod) {
		return nil
	}

	content, err := ioutil.ReadFile(m.maintenanceFilePath)
	if err != nil {
		return fmt.Errorf("error reading maintenance file: %w", err)
	}

	// Check if the file is empty
	if len(content) == 0 {
		return fmt.Errorf("maintenance file is empty: %s", m.maintenanceFilePath)
	}

	m.maintenanceFileContent = content
	m.maintenanceFileLastMod = fileInfo.ModTime()
	m.log(LogLevelInfo, "Loaded maintenance file: %s (%d bytes)", m.maintenanceFilePath, len(content))

	return nil
}

// log logs a message at the specified level
func (m *MaintenanceBypass) log(level LogLevel, format string, v ...interface{}) {
	if level <= m.logLevel {
		m.logger.Printf(format, v...)
	}
}

// ServeHTTP implements the http.Handler interface.
func (m *MaintenanceBypass) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// If maintenance mode is disabled, simply pass to the next handler
	if !m.enabled {
		m.log(LogLevelDebug, "Maintenance mode is disabled, passing request through: %s", req.URL.String())
		m.next.ServeHTTP(rw, req)
		return
	}

	// Check if the request is for favicon.ico and should bypass
	if m.bypassFavicon && strings.HasSuffix(req.URL.Path, "/favicon.ico") {
		m.log(LogLevelDebug, "Request is for favicon.ico, bypassing maintenance mode: %s", req.URL.String())
		m.next.ServeHTTP(rw, req)
		return
	}

	// Check if the request path is in the bypass paths list
	for _, path := range m.bypassPaths {
		if strings.HasPrefix(req.URL.Path, path) {
			m.log(LogLevelDebug, "Request path %s matches bypass path %s, passing through", req.URL.Path, path)
			m.next.ServeHTTP(rw, req)
			return
		}
	}

	// Check if the request has the bypass header with the correct value
	headerValue := req.Header.Get(m.bypassHeader)
	if headerValue == m.bypassHeaderValue {
		// If the bypass header is present with the correct value, pass the request to the next handler
		m.log(LogLevelDebug, "Bypass header found with value %s, passing to next handler", headerValue)
		m.next.ServeHTTP(rw, req)
		return
	}

	m.log(LogLevelInfo, "No bypass condition met for %s, serving maintenance page", req.URL.String())

	// Set appropriate response headers for maintenance mode
	rw.Header().Set("Retry-After", "3600") // Suggest client retry after 1 hour
	rw.Header().Set("X-Maintenance-Mode", "true")

	// If we have a maintenance file configured, serve that
	if m.maintenanceFilePath != "" {
		m.serveMaintenanceFile(rw, req)
		return
	}

	// If we have direct content configured, serve that
	if m.maintenanceContent != "" {
		m.serveMaintenanceContent(rw, req)
		return
	}

	// Otherwise, proxy to the maintenance service
	m.proxyToMaintenanceService(rw, req)
}

// serveMaintenanceFile serves the static maintenance file
func (m *MaintenanceBypass) serveMaintenanceFile(rw http.ResponseWriter, req *http.Request) {
	// Try to reload the file if it's changed (check file modification time)
	err := m.loadMaintenanceFile()
	if err != nil {
		m.log(LogLevelError, "Failed to load maintenance file: %v", err)
		rw.Header().Set("X-Maintenance-Mode", "true")
		http.Error(rw, "Service Temporarily Unavailable", m.statusCode)
		return
	}

	// Read the content from our cache
	m.fileMutex.RLock()
	content := m.maintenanceFileContent
	m.fileMutex.RUnlock()

	// Set content type and other headers
	rw.Header().Set("Content-Type", m.contentType)
	rw.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	rw.Header().Set("X-Maintenance-Mode", "true")

	// Write the status code and content
	rw.WriteHeader(m.statusCode)
	rw.Write(content)
}

// serveMaintenanceContent serves the direct maintenance content from configuration
func (m *MaintenanceBypass) serveMaintenanceContent(rw http.ResponseWriter, req *http.Request) {
	// Set content type and other headers
	rw.Header().Set("Content-Type", m.contentType)
	rw.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	rw.Header().Set("X-Maintenance-Mode", "true")

	// Write the status code and content
	rw.WriteHeader(m.statusCode)
	rw.Write([]byte(m.maintenanceContent))
}

// proxyToMaintenanceService proxies the request to the maintenance service
func (m *MaintenanceBypass) proxyToMaintenanceService(rw http.ResponseWriter, req *http.Request) {
	// Create a custom response writer that will set our status code
	maintenanceWriter := &maintenanceResponseWriter{
		ResponseWriter: rw,
		statusCode:     m.statusCode,
	}

	// Create a reverse proxy to the maintenance service
	proxy := httputil.NewSingleHostReverseProxy(m.maintenanceService)

	// Set a timeout for the proxy
	proxy.Transport = &http.Transport{
		ResponseHeaderTimeout: m.timeout,
	}

	// Handle errors from the maintenance service
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		m.log(LogLevelError, "Error proxying to maintenance service: %v", err)
		rw.Header().Set("X-Maintenance-Mode", "true")
		rw.WriteHeader(m.statusCode)
		rw.Write([]byte("Service temporarily unavailable"))
	}

	// Clone the request to avoid modifying the original
	proxyReq := req.Clone(req.Context())

	// Update the cloned request Host to match the maintenance service
	proxyReq.URL.Host = m.maintenanceService.Host
	proxyReq.URL.Scheme = m.maintenanceService.Scheme
	proxyReq.Host = m.maintenanceService.Host

	// Proxy the request to the maintenance service with our custom writer
	proxy.ServeHTTP(maintenanceWriter, proxyReq)
}

// maintenanceResponseWriter is a simple custom response writer that just sets our status code
type maintenanceResponseWriter struct {
	http.ResponseWriter
	statusCode int
	headerSet  bool
}

// WriteHeader overrides the original WriteHeader to set our status code
func (w *maintenanceResponseWriter) WriteHeader(statusCode int) {
	if !w.headerSet {
		w.ResponseWriter.WriteHeader(w.statusCode)
		w.headerSet = true
	}
}

// Write ensures headers are set before writing the body
func (w *maintenanceResponseWriter) Write(b []byte) (int, error) {
	if !w.headerSet {
		w.WriteHeader(w.statusCode)
	}
	return w.ResponseWriter.Write(b)
}
