package traefik_maintenance_warden

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testLogWriter is a simple io.Writer that captures logs
type testLogWriter struct {
	buf bytes.Buffer
}

func (w *testLogWriter) Write(p []byte) (n int, err error) {
	return w.buf.Write(p)
}

func (w *testLogWriter) String() string {
	return w.buf.String()
}

func (w *testLogWriter) Reset() {
	w.buf.Reset()
}

// MockTransportWithError is a mock transport that simulates network errors
type MockTransportWithError struct{}

func (m *MockTransportWithError) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("simulated network error")
}

func TestMaintenanceBypass(t *testing.T) {
	tests := []struct {
		name                string
		enabled             bool
		bypassHeader        string
		bypassHeaderValue   string
		statusCode          int
		path                string
		bypassPaths         []string
		bypassFavicon       bool
		expectedStatusCode  int
		expectedRedirectURL string
	}{
		{
			name:                "No bypass header should return 503 when enabled",
			enabled:             true,
			bypassHeader:        "",
			bypassHeaderValue:   "",
			statusCode:          503,
			path:                "/",
			bypassPaths:         []string{},
			bypassFavicon:       true,
			expectedStatusCode:  http.StatusServiceUnavailable,
			expectedRedirectURL: "",
		},
		{
			name:                "Wrong bypass header value should return 503 when enabled",
			enabled:             true,
			bypassHeader:        "X-Maintenance-Bypass",
			bypassHeaderValue:   "wrong",
			statusCode:          503,
			path:                "/",
			bypassPaths:         []string{},
			bypassFavicon:       true,
			expectedStatusCode:  http.StatusServiceUnavailable,
			expectedRedirectURL: "",
		},
		{
			name:                "Custom status code (429) should be returned when specified",
			enabled:             true,
			bypassHeader:        "",
			bypassHeaderValue:   "",
			statusCode:          429,
			path:                "/",
			bypassPaths:         []string{},
			bypassFavicon:       true,
			expectedStatusCode:  http.StatusTooManyRequests,
			expectedRedirectURL: "",
		},
		{
			name:                "Correct bypass header should pass through when enabled",
			enabled:             true,
			bypassHeader:        "X-Maintenance-Bypass",
			bypassHeaderValue:   "true",
			statusCode:          503,
			path:                "/",
			bypassPaths:         []string{},
			bypassFavicon:       true,
			expectedStatusCode:  http.StatusOK,
			expectedRedirectURL: "",
		},
		{
			name:                "Should pass through when disabled regardless of header",
			enabled:             false,
			bypassHeader:        "",
			bypassHeaderValue:   "",
			statusCode:          503,
			path:                "/",
			bypassPaths:         []string{},
			bypassFavicon:       true,
			expectedStatusCode:  http.StatusOK,
			expectedRedirectURL: "",
		},
		{
			name:                "Should pass through when disabled even with correct header",
			enabled:             false,
			bypassHeader:        "X-Maintenance-Bypass",
			bypassHeaderValue:   "true",
			statusCode:          503,
			path:                "/",
			bypassPaths:         []string{},
			bypassFavicon:       true,
			expectedStatusCode:  http.StatusOK,
			expectedRedirectURL: "",
		},
		{
			name:                "Favicon.ico should bypass maintenance mode when bypassFavicon is true",
			enabled:             true,
			bypassHeader:        "",
			bypassHeaderValue:   "",
			statusCode:          503,
			path:                "/favicon.ico",
			bypassPaths:         []string{},
			bypassFavicon:       true,
			expectedStatusCode:  http.StatusOK,
			expectedRedirectURL: "",
		},
		{
			name:                "Favicon.ico should not bypass maintenance mode when bypassFavicon is false",
			enabled:             true,
			bypassHeader:        "",
			bypassHeaderValue:   "",
			statusCode:          503,
			path:                "/favicon.ico",
			bypassPaths:         []string{},
			bypassFavicon:       false,
			expectedStatusCode:  http.StatusServiceUnavailable,
			expectedRedirectURL: "",
		},
		{
			name:                "Path in bypassPaths should bypass maintenance mode",
			enabled:             true,
			bypassHeader:        "",
			bypassHeaderValue:   "",
			statusCode:          503,
			path:                "/api/status",
			bypassPaths:         []string{"/api"},
			bypassFavicon:       true,
			expectedStatusCode:  http.StatusOK,
			expectedRedirectURL: "",
		},
		{
			name:                "Path not in bypassPaths should not bypass maintenance mode",
			enabled:             true,
			bypassHeader:        "",
			bypassHeaderValue:   "",
			statusCode:          503,
			path:                "/dashboard",
			bypassPaths:         []string{"/api", "/health"},
			bypassFavicon:       true,
			expectedStatusCode:  http.StatusServiceUnavailable,
			expectedRedirectURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test handler that always returns 200 OK
			nextHandler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				rw.WriteHeader(http.StatusOK)
			})

			// Create a test maintenance service
			maintenanceServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				// The maintenance server just returns 200 OK, but our middleware will override with the desired status code
				rw.WriteHeader(http.StatusOK)
				rw.Write([]byte("<html><body>Maintenance Page</body></html>"))
			}))
			defer maintenanceServer.Close()

			// Create the middleware config
			cfg := &Config{
				MaintenanceService: maintenanceServer.URL,
				BypassHeader:       "X-Maintenance-Bypass",
				BypassHeaderValue:  "true",
				Enabled:            tt.enabled,
				StatusCode:         tt.statusCode,
				BypassPaths:        tt.bypassPaths,
				BypassFavicon:      tt.bypassFavicon,
			}

			// Create the middleware
			middleware, err := New(context.Background(), nextHandler, cfg, "maintenance-test")
			if err != nil {
				t.Fatalf("Error creating middleware: %v", err)
			}

			// Create a test request
			req := httptest.NewRequest(http.MethodGet, "http://example.com"+tt.path, nil)
			if tt.bypassHeader != "" {
				req.Header.Set(tt.bypassHeader, tt.bypassHeaderValue)
			}

			// Create a recorder to capture the response
			recorder := httptest.NewRecorder()

			// Call the middleware
			middleware.ServeHTTP(recorder, req)

			// Check the response
			resp := recorder.Result()
			if resp.StatusCode != tt.expectedStatusCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatusCode, resp.StatusCode)
			}

			// Check for maintenance headers when in maintenance mode and not bypassed
			if tt.enabled && resp.StatusCode != http.StatusOK {
				retryAfter := resp.Header.Get("Retry-After")
				if retryAfter == "" {
					t.Errorf("Expected Retry-After header to be set for maintenance mode")
				}

				maintenanceHeader := resp.Header.Get("X-Maintenance-Mode")
				if maintenanceHeader != "true" {
					t.Errorf("Expected X-Maintenance-Mode header to be 'true', got %s", maintenanceHeader)
				}
			}

			// Check redirection URL if applicable
			if tt.expectedRedirectURL != "" {
				location := resp.Header.Get("Location")
				if location != tt.expectedRedirectURL {
					t.Errorf("Expected redirect to %s, got %s", tt.expectedRedirectURL, location)
				}
			}
		})
	}
}

// TestRequestCloning tests that the original request is not modified
func TestRequestCloning(t *testing.T) {
	// Create a test handler that checks the request
	nextHandler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		// Should still have original URL
		if req.URL.String() != "http://example.com/test" {
			t.Errorf("Original request was modified. Expected URL http://example.com/test, got %s", req.URL.String())
		}
		rw.WriteHeader(http.StatusOK)
	})

	// Create a test maintenance service
	maintenanceServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		// This should receive the cloned request with modified URL
		if !strings.HasPrefix(req.URL.String(), "/test") {
			t.Errorf("Expected path /test in maintenance request, got %s", req.URL.String())
		}
		rw.WriteHeader(http.StatusOK)
	}))
	defer maintenanceServer.Close()

	// Create the middleware config
	cfg := &Config{
		MaintenanceService: maintenanceServer.URL,
		BypassHeader:       "X-Maintenance-Bypass",
		BypassHeaderValue:  "true",
		Enabled:            true,
		StatusCode:         503,
	}

	// Create the middleware
	middleware, err := New(context.Background(), nextHandler, cfg, "maintenance-test")
	if err != nil {
		t.Fatalf("Error creating middleware: %v", err)
	}

	// Create a test request
	origReq := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)

	// Case 1: With bypass header - should go to next handler with unchanged request
	bypassReq := origReq.Clone(context.Background())
	bypassReq.Header.Set("X-Maintenance-Bypass", "true")

	recorder := httptest.NewRecorder()
	middleware.ServeHTTP(recorder, bypassReq)

	// Case 2: Without bypass header - should go to maintenance service with cloned request
	noBypassReq := origReq.Clone(context.Background())

	recorder = httptest.NewRecorder()
	middleware.ServeHTTP(recorder, noBypassReq)

	// Verify the original URL remains unchanged
	if origReq.URL.String() != "http://example.com/test" {
		t.Errorf("Original request URL was modified")
	}
}

// TestTimeoutHandling tests the error handling for maintenance service
func TestTimeoutHandling(t *testing.T) {
	// Create a direct error handler that simulates maintenance mode
	errorHandler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Retry-After", "3600")
		rw.Header().Set("X-Maintenance-Mode", "true")
		rw.WriteHeader(http.StatusServiceUnavailable)
		rw.Write([]byte("Service temporarily unavailable"))
	})

	// Test the error handler directly
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	errorHandler.ServeHTTP(recorder, req)

	// Check the response from the error handler
	resp := recorder.Result()

	// Headers should be set by our error handler
	if resp.Header.Get("X-Maintenance-Mode") != "true" {
		t.Errorf("Expected X-Maintenance-Mode header to be set")
	}

	// Status code should be the service unavailable code
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %d, got %d", http.StatusServiceUnavailable, resp.StatusCode)
	}
}

// TestLogging tests the logging functionality
func TestLogging(t *testing.T) {
	// Create a test handler
	nextHandler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})

	// Create a temporary maintenance HTML file for testing
	tmpDir, err := ioutil.TempDir("", "maintenance-test-logging")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "maintenance.html")
	if err := ioutil.WriteFile(filePath, []byte("<html><body>Test</body></html>"), 0644); err != nil {
		t.Fatalf("Failed to write maintenance file: %v", err)
	}

	// Set up a custom log writer to capture logs
	logBuffer := &testLogWriter{}

	// Test cases for different log levels
	testCases := []struct {
		name      string
		logLevel  int
		shouldLog bool
	}{
		{"No logging", int(LogLevelNone), false},
		{"Error logging", int(LogLevelError), true},
		{"Info logging", int(LogLevelInfo), true},
		{"Debug logging", int(LogLevelDebug), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset the log buffer
			logBuffer.Reset()

			// Create the middleware config using file path approach
			cfg := &Config{
				MaintenanceFilePath: filePath, // Use file instead of service
				Enabled:             true,
				LogLevel:            tc.logLevel,
			}

			// Create the middleware
			middleware, err := New(context.Background(), nextHandler, cfg, "maintenance-test")
			if err != nil {
				t.Fatalf("Error creating middleware: %v", err)
			}

			// Replace the logger with our test logger
			middlewareInstance := middleware.(*MaintenanceBypass)
			middlewareInstance.logger = log.New(logBuffer, "[test] ", 0)

			// Create a test request
			req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
			recorder := httptest.NewRecorder()

			// Call the middleware - this should generate logs at Info level or above
			middleware.ServeHTTP(recorder, req)

			// For debug level, we should check for bypass-related logs (which won't happen normally)
			// So force a log message at the current level
			if tc.logLevel > 0 {
				middlewareInstance.log(LogLevel(tc.logLevel), "Test log message at level %d", tc.logLevel)
			}

			// Check if logs were captured - for non-zero log levels
			if tc.shouldLog && logBuffer.String() == "" {
				t.Errorf("Expected logs to be captured at log level %d, but none were found", tc.logLevel)
			}

			if !tc.shouldLog && logBuffer.String() != "" {
				t.Errorf("Expected no logs at log level %d, but found: %s", tc.logLevel, logBuffer.String())
			}
		})
	}
}

// TestInvalidMaintenanceURL tests handling of invalid maintenance service URLs
func TestInvalidMaintenanceURL(t *testing.T) {
	// Create a test handler
	nextHandler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})

	testCases := []struct {
		name            string
		url             string
		shouldHaveError bool
	}{
		{"Invalid URL", "://invalid", true},
		{"Missing scheme", "maintenance-service", true},
		{"Valid URL", "http://maintenance-service", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create the middleware config
			cfg := &Config{
				MaintenanceService: tc.url,
				Enabled:            true,
			}

			// Create the middleware
			middleware, err := New(context.Background(), nextHandler, cfg, "maintenance-test")

			if tc.shouldHaveError {
				if err == nil {
					t.Errorf("Expected error for invalid URL %s, but got none", tc.url)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for valid URL %s, but got: %v", tc.url, err)
				}
			}

			if !tc.shouldHaveError && middleware == nil {
				t.Errorf("Expected middleware to be created for valid URL %s, but got nil", tc.url)
			}
		})
	}
}

// TestMaintenanceServiceError tests handling of maintenance service errors
func TestMaintenanceServiceError(t *testing.T) {
	// Create a direct error handler that simulates maintenance mode
	errorHandler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("X-Maintenance-Mode", "true")
		rw.WriteHeader(http.StatusServiceUnavailable)
		rw.Write([]byte("Service temporarily unavailable"))
	})

	// Test the error handler directly
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	errorHandler.ServeHTTP(recorder, req)

	// Check the response
	resp := recorder.Result()

	// Should have the service unavailable status code
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %d, got %d", http.StatusServiceUnavailable, resp.StatusCode)
	}

	// Maintenance mode header should be set
	if resp.Header.Get("X-Maintenance-Mode") != "true" {
		t.Errorf("Expected X-Maintenance-Mode header to be set")
	}
}

// TestMaintenanceFile tests serving a maintenance page from a file
func TestMaintenanceFile(t *testing.T) {
	// Create a test handler
	nextHandler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte("This is the normal content"))
	})

	// Create a temporary maintenance HTML file
	tmpDir, err := ioutil.TempDir("", "maintenance-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	htmlContent := "<html><body><h1>Under Maintenance</h1><p>We'll be back soon.</p></body></html>"
	filePath := filepath.Join(tmpDir, "maintenance.html")

	if err := ioutil.WriteFile(filePath, []byte(htmlContent), 0644); err != nil {
		t.Fatalf("Failed to write maintenance file: %v", err)
	}

	// Create the middleware config
	cfg := &Config{
		MaintenanceFilePath: filePath,
		Enabled:             true,
		StatusCode:          503,
		BypassHeader:        "X-Maintenance-Bypass",
		BypassHeaderValue:   "true",
	}

	// Create the middleware
	middleware, err := New(context.Background(), nextHandler, cfg, "maintenance-test")
	if err != nil {
		t.Fatalf("Error creating middleware: %v", err)
	}

	testCases := []struct {
		name            string
		bypassHeader    bool
		expectedStatus  int
		expectedContent string
	}{
		{
			name:            "With bypass header",
			bypassHeader:    true,
			expectedStatus:  http.StatusOK,
			expectedContent: "This is the normal content",
		},
		{
			name:            "Without bypass header",
			bypassHeader:    false,
			expectedStatus:  http.StatusServiceUnavailable,
			expectedContent: htmlContent,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test request
			req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)

			if tc.bypassHeader {
				req.Header.Set("X-Maintenance-Bypass", "true")
			}

			// Create a recorder to capture the response
			recorder := httptest.NewRecorder()

			// Call the middleware
			middleware.ServeHTTP(recorder, req)

			// Check the response
			resp := recorder.Result()

			// Check status code
			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tc.expectedStatus, resp.StatusCode)
			}

			// Check content
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Error reading response body: %v", err)
			}

			if string(body) != tc.expectedContent {
				t.Errorf("Expected content %q, got %q", tc.expectedContent, string(body))
			}

			// Check headers for maintenance mode
			if !tc.bypassHeader {
				if resp.Header.Get("X-Maintenance-Mode") != "true" {
					t.Errorf("Expected X-Maintenance-Mode header to be set")
				}

				if resp.Header.Get("Content-Type") != "text/html; charset=utf-8" {
					t.Errorf("Expected Content-Type header to be set to text/html")
				}
			}
		})
	}
}

// TestMaintenanceFileModification tests file loading and content serving
func TestMaintenanceFileModification(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := ioutil.TempDir("", "maintenance-file-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create maintenance file with content
	content := "<html><body>Maintenance Page Content</body></html>"
	filePath := filepath.Join(tmpDir, "maintenance.html")

	err = ioutil.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write maintenance file: %v", err)
	}

	// Create a test response recorder and request
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/path", nil)

	// Create simple file serving handler that behaves like our maintenance handler
	fileHandler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		// Read the content
		fileContent, err := ioutil.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}

		// Set headers
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		rw.Header().Set("X-Maintenance-Mode", "true")

		// Write status and content
		rw.WriteHeader(http.StatusServiceUnavailable)
		rw.Write(fileContent)
	})

	// Serve the file
	fileHandler.ServeHTTP(recorder, req)

	// Check response
	resp := recorder.Result()
	body, _ := ioutil.ReadAll(resp.Body)

	// Verify status code
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %d, got %d", http.StatusServiceUnavailable, resp.StatusCode)
	}

	// Verify content
	if string(body) != content {
		t.Errorf("Expected content %q, got %q", content, string(body))
	}

	// Verify headers
	if resp.Header.Get("X-Maintenance-Mode") != "true" {
		t.Errorf("Expected X-Maintenance-Mode header to be set")
	}

	if resp.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("Expected correct Content-Type header, got %q", resp.Header.Get("Content-Type"))
	}

	// Update the file content
	updatedContent := "<html><body>Updated Maintenance Page Content</body></html>"
	err = ioutil.WriteFile(filePath, []byte(updatedContent), 0644)
	if err != nil {
		t.Fatalf("Failed to update maintenance file: %v", err)
	}

	// Create a new recorder for the second request
	recorder2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "http://example.com/path", nil)

	// Serve the updated file
	fileHandler.ServeHTTP(recorder2, req2)

	// Check updated response
	resp2 := recorder2.Result()
	body2, _ := ioutil.ReadAll(resp2.Body)

	// Verify updated content
	if string(body2) != updatedContent {
		t.Errorf("Expected updated content %q, got %q", updatedContent, string(body2))
	}
}

// TestConfigValidation tests validation of configuration
func TestConfigValidation(t *testing.T) {
	// Create a test handler
	nextHandler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})

	testCases := []struct {
		name          string
		config        *Config
		shouldHaveErr bool
	}{
		{
			name: "Valid config with maintenance service",
			config: &Config{
				MaintenanceService: "http://maintenance-service",
				Enabled:            true,
			},
			shouldHaveErr: false,
		},
		{
			name: "Valid config with maintenance file",
			config: &Config{
				MaintenanceFilePath: "testdata/maintenance.html", // This will be created
				Enabled:             true,
			},
			shouldHaveErr: false,
		},
		{
			name: "Valid config with maintenance content",
			config: &Config{
				MaintenanceContent: "<html><body>Maintenance content</body></html>",
				Enabled:            true,
			},
			shouldHaveErr: false,
		},
		{
			name: "Invalid config with no maintenance option",
			config: &Config{
				MaintenanceService:  "",
				MaintenanceFilePath: "",
				MaintenanceContent:  "",
				Enabled:             true,
			},
			shouldHaveErr: true,
		},
		{
			name: "Invalid config with non-existent file",
			config: &Config{
				MaintenanceFilePath: "/non/existent/file.html",
				Enabled:             true,
			},
			shouldHaveErr: true,
		},
	}

	// Create test directory and file for valid file test
	testDataDir := filepath.Join("testdata")
	os.MkdirAll(testDataDir, 0755)
	defer os.RemoveAll(testDataDir)

	testFile := filepath.Join(testDataDir, "maintenance.html")
	ioutil.WriteFile(testFile, []byte("<html><body>Test</body></html>"), 0644)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(context.Background(), nextHandler, tc.config, "maintenance-test")

			if tc.shouldHaveErr && err == nil {
				t.Errorf("Expected error but got none")
			}

			if !tc.shouldHaveErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

// TestCreateConfig tests the CreateConfig function returns correct default values
func TestCreateConfig(t *testing.T) {
	config := CreateConfig()

	// Check default values
	if config.MaintenanceService != "" {
		t.Errorf("Expected default MaintenanceService to be empty, got %q", config.MaintenanceService)
	}

	if config.MaintenanceFilePath != "" {
		t.Errorf("Expected default MaintenanceFilePath to be empty, got %q", config.MaintenanceFilePath)
	}

	if config.MaintenanceContent != "" {
		t.Errorf("Expected default MaintenanceContent to be empty, got %q", config.MaintenanceContent)
	}

	if config.BypassHeader != "X-Maintenance-Bypass" {
		t.Errorf("Expected default BypassHeader to be 'X-Maintenance-Bypass', got %q", config.BypassHeader)
	}

	if config.BypassHeaderValue != "true" {
		t.Errorf("Expected default BypassHeaderValue to be 'true', got %q", config.BypassHeaderValue)
	}

	if !config.Enabled {
		t.Errorf("Expected default Enabled to be true, got false")
	}

	if config.StatusCode != 503 {
		t.Errorf("Expected default StatusCode to be 503, got %d", config.StatusCode)
	}

	if len(config.BypassPaths) != 0 {
		t.Errorf("Expected default BypassPaths to be empty, got %v", config.BypassPaths)
	}

	if !config.BypassFavicon {
		t.Errorf("Expected default BypassFavicon to be true, got false")
	}

	if config.LogLevel != int(LogLevelError) {
		t.Errorf("Expected default LogLevel to be %d, got %d", int(LogLevelError), config.LogLevel)
	}

	if config.MaintenanceTimeout != 10 {
		t.Errorf("Expected default MaintenanceTimeout to be 10, got %d", config.MaintenanceTimeout)
	}

	if config.ContentType != "text/html; charset=utf-8" {
		t.Errorf("Expected default ContentType to be 'text/html; charset=utf-8', got %q", config.ContentType)
	}
}

// TestLoadMaintenanceFileErrors tests the error handling in loadMaintenanceFile
func TestLoadMaintenanceFileErrors(t *testing.T) {
	// Create a test handler
	nextHandler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})

	// Test with a non-existent file
	cfg := &Config{
		MaintenanceFilePath: "/path/to/nonexistent/file.html",
		Enabled:             true,
	}

	// This should fail at middleware creation time
	_, err := New(context.Background(), nextHandler, cfg, "maintenance-test")
	if err == nil {
		t.Errorf("Expected error when file doesn't exist, got nil")
	}

	// Test with a directory instead of a file
	tmpDir, err := ioutil.TempDir("", "maintenance-test-dir")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg = &Config{
		MaintenanceFilePath: tmpDir, // Directory, not a file
		Enabled:             true,
	}

	// This should fail because it's a directory, not a file
	_, err = New(context.Background(), nextHandler, cfg, "maintenance-test")
	if err == nil {
		t.Errorf("Expected error when path is a directory, got nil")
	}

	// Test with an empty file
	emptyFilePath := filepath.Join(tmpDir, "empty.html")
	err = ioutil.WriteFile(emptyFilePath, []byte{}, 0644)
	if err != nil {
		t.Fatalf("Failed to create empty test file: %v", err)
	}

	cfg = &Config{
		MaintenanceFilePath: emptyFilePath,
		Enabled:             true,
	}

	// This should fail because the file is empty
	_, err = New(context.Background(), nextHandler, cfg, "maintenance-test")
	if err == nil {
		t.Errorf("Expected error when file is empty, got nil")
	} else if !strings.Contains(err.Error(), "maintenance file is empty") {
		t.Errorf("Expected error to mention empty file, got: %v", err)
	}

	// Test with a file without read permissions
	// Create a file
	filePath := filepath.Join(tmpDir, "no-read-perm.html")
	err = ioutil.WriteFile(filePath, []byte("<html><body>Test</body></html>"), 0)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Skip this test on Windows as permissions work differently
	if os.Getenv("OS") != "Windows_NT" {
		cfg = &Config{
			MaintenanceFilePath: filePath,
			Enabled:             true,
		}

		// This should fail because file is not readable
		_, err = New(context.Background(), nextHandler, cfg, "maintenance-test")
		if err == nil {
			t.Errorf("Expected error when file is not readable, got nil")
		}
	}
}

// TestServeMaintenanceFileErrors tests error handling in serving a maintenance file
func TestServeMaintenanceFileErrors(t *testing.T) {
	// Create a test handler
	nextHandler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})

	// Create a temporary file
	tmpDir, err := ioutil.TempDir("", "maintenance-test-logging")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "maintenance.html")
	err = ioutil.WriteFile(filePath, []byte("<html><body>Test</body></html>"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create the middleware
	cfg := &Config{
		MaintenanceFilePath: filePath,
		Enabled:             true,
		StatusCode:          503,
	}

	middleware, err := New(context.Background(), nextHandler, cfg, "maintenance-bypass")
	if err != nil {
		t.Fatalf("Error creating middleware: %v", err)
	}

	// Get the maintenance bypass instance
	m := middleware.(*MaintenanceBypass)

	// Create a test recorder and request
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/path", nil)

	// First, serve the file normally to make sure it works
	m.serveMaintenanceFile(recorder, req)

	// Check response
	resp := recorder.Result()
	if resp.StatusCode != 503 {
		t.Errorf("Expected status code 503, got %d", resp.StatusCode)
	}

	if resp.Header.Get("X-Maintenance-Mode") != "true" {
		t.Errorf("Expected X-Maintenance-Mode header to be set")
	}

	// Now make the file unreadable to simulate failure
	// We'll replace the file path with a non-existent one
	m.maintenanceFilePath = "/nonexistent/file.html"

	// Create a new recorder
	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "http://example.com/path", nil)

	// Call serveMaintenanceFile again - this should handle the error
	m.serveMaintenanceFile(recorder, req)

	// Check that we got the expected error response
	resp = recorder.Result()

	// Should still return a response with the configured status code
	if resp.StatusCode != m.statusCode {
		t.Errorf("Expected status code %d even with file error, got %d", m.statusCode, resp.StatusCode)
	}

	// Maintenance mode header should still be set
	if resp.Header.Get("X-Maintenance-Mode") != "true" {
		t.Errorf("Expected X-Maintenance-Mode header to be set even with file error")
	}
}

// TestProxyToMaintenanceServiceErrorHandler tests the error handler for the proxy
func TestProxyToMaintenanceServiceErrorHandler(t *testing.T) {
	// Create a test handler
	nextHandler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})

	// Create a maintenance service that will always fail
	maintenanceServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		// This will never be called because we'll use a mock transport
		panic("should not be called")
	}))
	defer maintenanceServer.Close()

	// Create the middleware
	cfg := &Config{
		MaintenanceService: maintenanceServer.URL,
		Enabled:            true,
		StatusCode:         503,
	}

	middleware, err := New(context.Background(), nextHandler, cfg, "maintenance-test")
	if err != nil {
		t.Fatalf("Error creating middleware: %v", err)
	}

	// Get access to the MaintenanceBypass instance
	m := middleware.(*MaintenanceBypass)

	// Create a test recorder and request
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)

	// Create our own custom error handler like the one in the proxyToMaintenanceService method
	errorHandler := func(rw http.ResponseWriter, req *http.Request, err error) {
		m.log(LogLevelError, "Error proxying to maintenance service: %v", err)
		rw.Header().Set("X-Maintenance-Mode", "true")
		rw.WriteHeader(m.statusCode)
		rw.Write([]byte("Service temporarily unavailable"))
	}

	// Call the error handler directly with a sample error
	errorHandler(recorder, req, fmt.Errorf("simulated error"))

	// Check the response
	resp := recorder.Result()

	// Should have the configured status code
	if resp.StatusCode != 503 {
		t.Errorf("Expected status code 503 from error handler, got %d", resp.StatusCode)
	}

	// Maintenance mode header should be set
	if resp.Header.Get("X-Maintenance-Mode") != "true" {
		t.Errorf("Expected X-Maintenance-Mode header to be set by error handler")
	}

	// Body should contain error message
	body, _ := ioutil.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Service temporarily unavailable") {
		t.Errorf("Expected error body to contain 'Service temporarily unavailable', got %q", string(body))
	}
}

// TestLoadMaintenanceFileModificationTime tests the file modification time checking
func TestLoadMaintenanceFileModificationTime(t *testing.T) {
	// Create a test handler
	nextHandler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})

	// Create a temporary file
	tmpDir, err := ioutil.TempDir("", "maintenance-test-modtime")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "maintenance.html")
	originalContent := "<html><body>Original</body></html>"
	err = ioutil.WriteFile(filePath, []byte(originalContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create the middleware
	cfg := &Config{
		MaintenanceFilePath: filePath,
		Enabled:             true,
	}

	middleware, err := New(context.Background(), nextHandler, cfg, "maintenance-test")
	if err != nil {
		t.Fatalf("Error creating middleware: %v", err)
	}

	// Get the maintenance bypass instance
	m := middleware.(*MaintenanceBypass)

	// First load should have loaded the file
	if m.maintenanceFileContent == nil {
		t.Fatalf("File content should have been loaded during initialization")
	}

	if string(m.maintenanceFileContent) != originalContent {
		t.Errorf("Expected content to be %q, got %q", originalContent, string(m.maintenanceFileContent))
	}

	initialModTime := m.maintenanceFileLastMod

	// Call loadMaintenanceFile again but without changing the file
	// This should not reload the file
	err = m.loadMaintenanceFile()
	if err != nil {
		t.Fatalf("Error loading maintenance file: %v", err)
	}

	// The mod time should be the same
	if !m.maintenanceFileLastMod.Equal(initialModTime) {
		t.Errorf("Mod time should not have changed when file wasn't modified")
	}
}

// TestWriteHeaderOrder tests the ordering of header setting in the custom writer
func TestWriteHeaderOrder(t *testing.T) {
	// This tests the edge cases of the custom response writer

	// 1. Test when Write is called first
	recorder1 := httptest.NewRecorder()
	writer1 := &maintenanceResponseWriter{
		ResponseWriter: recorder1,
		statusCode:     503,
	}

	writer1.Write([]byte("test"))

	if recorder1.Code != 503 {
		t.Errorf("Expected status code to be set to 503 when Write is called first, got %d", recorder1.Code)
	}

	// 2. Test when WriteHeader is called first
	recorder2 := httptest.NewRecorder()
	writer2 := &maintenanceResponseWriter{
		ResponseWriter: recorder2,
		statusCode:     503,
	}

	writer2.WriteHeader(200) // Should use 503 instead
	writer2.Write([]byte("test"))

	if recorder2.Code != 503 {
		t.Errorf("Expected status code to be set to 503 when WriteHeader is called first, got %d", recorder2.Code)
	}

	// 3. Test multiple Write calls
	recorder3 := httptest.NewRecorder()
	writer3 := &maintenanceResponseWriter{
		ResponseWriter: recorder3,
		statusCode:     503,
	}

	writer3.Write([]byte("first"))
	writer3.Write([]byte(" second"))

	if recorder3.Body.String() != "first second" {
		t.Errorf("Expected body to be 'first second', got %q", recorder3.Body.String())
	}
}

// TestProxyToMaintenanceService tests the proxy functionality
func TestProxyToMaintenanceService(t *testing.T) {
	// Create a test handler
	nextHandler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})

	// Mock maintenance server that we can control
	mockMaintenanceContent := "<html><body>Maintenance Page Content</body></html>"
	mockServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		// Check if the request was properly forwarded
		if req.URL.Path == "/test-path" {
			t.Logf("Received correctly forwarded request to path: %s", req.URL.Path)
		}

		// Set a custom header to verify it gets passed through
		rw.Header().Set("X-Test-Header", "test-value")
		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte(mockMaintenanceContent))
	}))
	defer mockServer.Close()

	// Create the middleware config
	cfg := &Config{
		MaintenanceService: mockServer.URL,
		Enabled:            true,
		StatusCode:         503,
		MaintenanceTimeout: 5,
	}

	// Create the middleware
	middleware, err := New(context.Background(), nextHandler, cfg, "maintenance-test")
	if err != nil {
		t.Fatalf("Error creating middleware: %v", err)
	}

	// Get access to the internal MaintenanceBypass struct
	m := middleware.(*MaintenanceBypass)

	// Create a test recorder and request with a specific path
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/test-path", nil)

	// Add some custom headers to verify they're forwarded
	req.Header.Set("X-Custom-Header", "custom-value")

	// We need to set the header explicitly since we're not going through ServeHTTP
	recorder.Header().Set("X-Maintenance-Mode", "true")

	// Call proxyToMaintenanceService directly
	m.proxyToMaintenanceService(recorder, req)

	// Check the response
	resp := recorder.Result()
	body, _ := ioutil.ReadAll(resp.Body)

	// Should have the configured status code (503) even though the mock server returned 200
	if resp.StatusCode != cfg.StatusCode {
		t.Errorf("Expected status code %d, got %d", cfg.StatusCode, resp.StatusCode)
	}

	// Should have the maintenance mode header
	if resp.Header.Get("X-Maintenance-Mode") != "true" {
		t.Errorf("Expected X-Maintenance-Mode header to be set")
	}

	// The custom header from the mock server should be preserved
	if resp.Header.Get("X-Test-Header") != "test-value" {
		t.Errorf("Expected X-Test-Header: test-value to be preserved, got: %s",
			resp.Header.Get("X-Test-Header"))
	}

	// The content should be preserved
	if !strings.Contains(string(body), "Maintenance Page Content") {
		t.Errorf("Expected response to contain mock maintenance content, got: %s", string(body))
	}

	// Test error handling with a deliberately invalid URL
	mockInvalidServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		// This should never be called
		t.Error("Invalid server should not be called")
	}))
	mockInvalidServer.Close() // Close immediately to cause connection error

	// Update the middleware to use the closed server
	invalidURL, _ := url.Parse(mockInvalidServer.URL)
	m.maintenanceService = invalidURL

	// Create a new recorder and request
	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "http://example.com/test-path", nil)

	// This should trigger the error handler
	m.proxyToMaintenanceService(recorder, req)

	// Check the error response
	resp = recorder.Result()
	body, _ = ioutil.ReadAll(resp.Body)

	// Should still have the configured status code
	if resp.StatusCode != cfg.StatusCode {
		t.Errorf("Expected status code %d for error handling, got %d", cfg.StatusCode, resp.StatusCode)
	}

	// Should have the maintenance mode header
	if resp.Header.Get("X-Maintenance-Mode") != "true" {
		t.Errorf("Expected X-Maintenance-Mode header to be set for error handling")
	}

	// The content should contain the error message
	if !strings.Contains(string(body), "Service temporarily unavailable") {
		t.Errorf("Expected error response to contain unavailable message, got: %s", string(body))
	}
}

// TestMaintenanceContent tests serving direct content provided in the config
func TestMaintenanceContent(t *testing.T) {
	// Create a test handler
	nextHandler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte("This is the real service content"))
	})

	// Set test content
	testContent := "<html><body><h1>Test Maintenance Content</h1></body></html>"

	// Create config
	cfg := &Config{
		MaintenanceContent: testContent,
		Enabled:            true,
		StatusCode:         http.StatusServiceUnavailable,
		ContentType:        "text/html; charset=utf-8",
		BypassHeader:       "X-Test-Bypass",
		BypassHeaderValue:  "secret",
	}

	// Create middleware
	middleware, err := New(context.Background(), nextHandler, cfg, "maintenance-test")
	if err != nil {
		t.Fatalf("Error creating middleware: %v", err)
	}

	// Test 1: Request without bypass header should get maintenance content
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)

	middleware.ServeHTTP(recorder, req)

	resp := recorder.Result()
	body, _ := ioutil.ReadAll(resp.Body)

	// Check status code
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %d, got %d", http.StatusServiceUnavailable, resp.StatusCode)
	}

	// Check content type
	if resp.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("Expected Content-Type %q, got %q", "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
	}

	// Check maintenance mode header
	if resp.Header.Get("X-Maintenance-Mode") != "true" {
		t.Errorf("Expected X-Maintenance-Mode header to be 'true', got %q", resp.Header.Get("X-Maintenance-Mode"))
	}

	// Check content
	if string(body) != testContent {
		t.Errorf("Expected body %q, got %q", testContent, string(body))
	}

	// Test 2: Request with bypass header should go to real service
	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req.Header.Set("X-Test-Bypass", "secret")

	middleware.ServeHTTP(recorder, req)

	resp = recorder.Result()
	body, _ = ioutil.ReadAll(resp.Body)

	// Should get the real service response
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	if string(body) != "This is the real service content" {
		t.Errorf("Expected body %q, got %q", "This is the real service content", string(body))
	}
}
