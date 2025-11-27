package config

import (
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/time/rate"
	"gopkg.in/yaml.v3"
)

// Config holds server configuration
type Config struct {
	Port                   string        `yaml:"port"`
	EnableTLS              bool          `yaml:"enableTLS"`
	CertFile               string        `yaml:"certFile"`
	KeyFile                string        `yaml:"keyFile"`
	EnableCORS             bool          `yaml:"enableCORS"`
	LogRequests            bool          `yaml:"logRequests"`
	LogHeaders             bool          `yaml:"logHeaders"`
	LogBody                bool          `yaml:"logBody"`
	MaxBodySize            int64         `yaml:"maxBodySize"`
	Hostname               string        `yaml:"hostname"`
	RateLimitPerS          float64       `yaml:"rateLimitPerS"`
	HistorySize            int           `yaml:"historySize"`
	GlobalDelay            time.Duration `yaml:"globalDelay"`
	GlobalChaosProbability float64       `yaml:"globalChaosProbability"`
	Scenarios              []Scenario    `yaml:"-"` // Handled separately
}

// Scenario defines a sequence of custom responses for a specific path
type Scenario struct {
	Path      string     `yaml:"path"`
	Method    string     `yaml:"method"`
	Responses []Response `yaml:"responses"`
	Index     int32      // Current response index (atomic operations)
}

// Response defines a custom response
type Response struct {
	Status      int               `yaml:"status"`
	Delay       time.Duration     `yaml:"delay"`
	Body        string            `yaml:"body"`
	Headers     map[string]string `yaml:"headers"`
	Gzip        bool              `yaml:"gzip"`
	Probability float64           `yaml:"probability"`
}

// RequestRecord stores details of a recorded request
type RequestRecord struct {
	ID          string // Unique ID for replay
	Timestamp   time.Time
	Method      string
	Path        string
	Query       string
	Headers     map[string][]string
	BodySnippet string
	RemoteAddr  string
}

var (
	DefaultConfig = Config{
		Port:                   "8080",
		EnableTLS:              false,
		CertFile:               "cert.pem",
		KeyFile:                "key.pem",
		EnableCORS:             true,
		LogRequests:            true,
		LogHeaders:             false,
		LogBody:                true,
		MaxBodySize:            1024 * 1024, // 1MB
		Hostname:               "localhost",
		RateLimitPerS:          0.0,
		HistorySize:            100,
		GlobalDelay:            0,
		GlobalChaosProbability: 0.0,
	}

	configLock     sync.Mutex
	currentConfig  Config
	scenarios      sync.Map // map[string]*Scenario (key: path_method)
	RequestHistory []RequestRecord
	HistoryMutex   sync.Mutex
	RequestCounter uint64
	rateLimiter    *rate.Limiter
	registry       *prometheus.Registry
)

func init() {
	// Initialize with defaults
	currentConfig = DefaultConfig
	registry = prometheus.NewRegistry()
	// mrand.Seed is deprecated in Go 1.20+ and no longer needed for global rand
}

// LoadConfig loads server configuration from the main config file and scenario file
func LoadConfig(scenarioFile string) (Config, error) {
	configLock.Lock()
	defer configLock.Unlock()

	// 1. Load Scenarios
	data, err := os.ReadFile(scenarioFile)
	if err != nil {
		log.Printf("Warning: Failed to read %s, running without custom scenarios: %v", scenarioFile, err)
	} else {
		var loadedScenarios []Scenario
		if err := yaml.Unmarshal(data, &loadedScenarios); err != nil {
			return Config{}, err
		}

		for _, s := range loadedScenarios {
			AddScenario(s)
		}
		currentConfig.Scenarios = loadedScenarios
	}

	// 2. Load Base Config (from env vars)

	// 2. Load Base Config (from env vars)
	// Apply overrides from environment variables
	if port := os.Getenv("PORT"); port != "" {
		currentConfig.Port = port
	}
	if tls := os.Getenv("ENABLE_TLS"); tls == "true" {
		currentConfig.EnableTLS = true
	}
	if cert := os.Getenv("CERT_FILE"); cert != "" {
		currentConfig.CertFile = cert
	}
	if key := os.Getenv("KEY_FILE"); key != "" {
		currentConfig.KeyFile = key
	}
	if cors := os.Getenv("ENABLE_CORS"); cors != "" {
		currentConfig.EnableCORS = cors == "true"
	}
	if logReq := os.Getenv("LOG_REQUESTS"); logReq != "" {
		currentConfig.LogRequests = logReq == "true"
	}
	if logHead := os.Getenv("LOG_HEADERS"); logHead != "" {
		currentConfig.LogHeaders = logHead == "true"
	}
	if logBody := os.Getenv("LOG_BODY"); logBody != "" {
		currentConfig.LogBody = logBody == "true"
	}

	// Numeric / Duration Configs
	if rps := os.Getenv("RATE_LIMIT_RPS"); rps != "" {
		if val, err := strconv.ParseFloat(rps, 64); err == nil {
			currentConfig.RateLimitPerS = val
		}
	}
	if hist := os.Getenv("HISTORY_SIZE"); hist != "" {
		if val, err := strconv.Atoi(hist); err == nil {
			currentConfig.HistorySize = val
		}
	}
	if maxBody := os.Getenv("MAX_BODY_SIZE"); maxBody != "" {
		if val, err := strconv.ParseInt(maxBody, 10, 64); err == nil {
			currentConfig.MaxBodySize = val
		}
	}
	if delay := os.Getenv("ECHO_DELAY"); delay != "" {
		if val, err := time.ParseDuration(delay); err == nil {
			currentConfig.GlobalDelay = val
		}
	}
	if chaos := os.Getenv("ECHO_CHAOS_PROBABILITY"); chaos != "" {
		if val, err := strconv.ParseFloat(chaos, 64); err == nil {
			currentConfig.GlobalChaosProbability = val
		}
	}

	if currentConfig.RateLimitPerS > 0 {
		rateLimiter = rate.NewLimiter(rate.Limit(currentConfig.RateLimitPerS), int(currentConfig.RateLimitPerS))
	} else {
		rateLimiter = nil
	}

	// Update Hostname if available
	if h, err := os.Hostname(); err == nil {
		currentConfig.Hostname = h
	}

	return currentConfig, nil
}

// AddScenario adds or updates a scenario in the thread-safe map
func AddScenario(s Scenario) {
	scenarios.Store(s.Path+"_"+s.Method, s)
}

// GetScenarios returns a copy of the loaded scenarios map
func GetScenarios() *sync.Map {
	return &scenarios
}

// GetRateLimiter returns the global rate limiter
func GetRateLimiter() *rate.Limiter {
	return rateLimiter
}

// GetRequestHistory returns the recorded request history
func GetRequestHistory() ([]RequestRecord, *sync.Mutex) {
	return RequestHistory, &HistoryMutex
}

// GetRegistry returns the global Prometheus registry
func GetRegistry() *prometheus.Registry {
	return registry
}

// GetConfig returns the current server config
func GetConfig() Config {
	return currentConfig
}
