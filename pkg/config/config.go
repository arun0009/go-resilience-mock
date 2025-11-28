package config

import (
	"encoding/json"
	"log"
	"net/http"
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

// CircuitBreakerConfig defines the configuration for the circuit breaker
type CircuitBreakerConfig struct {
	FailureThreshold int           `yaml:"failureThreshold"`
	SuccessThreshold int           `yaml:"successThreshold"`
	Timeout          time.Duration `yaml:"timeout"`
}

// CircuitBreakerState tracks the runtime state of the circuit breaker
type CircuitBreakerState struct {
	State          string    // "closed", "open", "half-open"
	Failures       int       // Consecutive failures
	Successes      int       // Consecutive successes
	LastFailure    time.Time // Time of last failure
	LastTransition time.Time // Time of last state change
	Mutex          sync.Mutex
}

// JSONBody is a helper type to handle both string and structured JSON in YAML
type JSONBody json.RawMessage

// UnmarshalYAML implements the yaml.Unmarshaler interface
func (j *JSONBody) UnmarshalYAML(value *yaml.Node) error {
	var obj interface{}
	if err := value.Decode(&obj); err != nil {
		return err
	}

	// If it's a string, it might be a raw JSON string or just a plain string
	if str, ok := obj.(string); ok {
		*j = JSONBody(str)
		return nil
	}

	// If it's structured data (map/slice), marshal it to JSON
	bytes, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	*j = JSONBody(bytes)
	return nil
}

// MarshalJSON implements the json.Marshaler interface
func (j JSONBody) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return json.RawMessage(j).MarshalJSON()
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (j *JSONBody) UnmarshalJSON(data []byte) error {
	*j = JSONBody(data)
	return nil
}

// MatchConfig defines rules for matching a request to a scenario
type MatchConfig struct {
	Headers map[string]string `yaml:"headers"`
	Query   map[string]string `yaml:"query"`
	Body    JSONBody          `yaml:"body"`
}

// Scenario defines a sequence of custom responses for a specific path
type Scenario struct {
	Path           string               `yaml:"path"`
	Method         string               `yaml:"method"`
	Matches        MatchConfig          `yaml:"matches"`
	Responses      []Response           `yaml:"responses"`
	CircuitBreaker CircuitBreakerConfig `yaml:"circuitBreaker"`
	CBState        *CircuitBreakerState `yaml:"-"` // Runtime state
	Index          int32                // Current response index (atomic operations)
}

// Response defines a custom response
type Response struct {
	Status      int               `yaml:"status"`
	Delay       time.Duration     `yaml:"delay"`
	DelayRange  string            `yaml:"delayRange"` // e.g., "100ms-500ms"
	Body        JSONBody          `yaml:"body"`
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
	StatusCode  int // Response status code
	Headers     http.Header
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
	scenarios      sync.Map // map[string][]*Scenario (key: path_method)
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

		for i := range loadedScenarios {
			// Initialize runtime state
			loadedScenarios[i].CBState = &CircuitBreakerState{
				State: "closed",
			}
			addScenarioLocked(&loadedScenarios[i])
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
func AddScenario(s *Scenario) {
	configLock.Lock()
	defer configLock.Unlock()
	addScenarioLocked(s)
}

// addScenarioLocked adds a scenario without locking (caller must hold configLock)
func addScenarioLocked(s *Scenario) {
	if s.CBState == nil {
		s.CBState = &CircuitBreakerState{State: "closed"}
	}
	key := s.Path + "_" + s.Method

	// Load existing list or create new
	if v, ok := scenarios.Load(key); ok {
		oldList := v.([]*Scenario)
		// Copy-On-Write: Create a new slice to avoid race conditions with readers
		// who might be iterating over the old slice.
		newList := make([]*Scenario, len(oldList)+1)
		copy(newList, oldList)
		newList[len(oldList)] = s
		scenarios.Store(key, newList)
	} else {
		scenarios.Store(key, []*Scenario{s})
	}
}

// GetScenarios returns a copy of the loaded scenarios map
func GetScenarios() *sync.Map {
	return &scenarios
}

// GetRateLimiter returns the global rate limiter
func GetRateLimiter() *rate.Limiter {
	return rateLimiter
}

// GetRequestHistory returns the history mutex.
// Callers must Lock the mutex before accessing RequestHistory directly.
func GetHistoryMutex() *sync.Mutex {
	return &HistoryMutex
}

// ResetDefaults resets the configuration to defaults (useful for testing)
func ResetDefaults() {
	configLock.Lock()
	currentConfig = DefaultConfig
	rateLimiter = nil
	configLock.Unlock()

	HistoryMutex.Lock()
	RequestHistory = []RequestRecord{}
	HistoryMutex.Unlock()
}

// GetRegistry returns the global Prometheus registry
func GetRegistry() *prometheus.Registry {
	return registry
}

// GetConfig returns the current server config
func GetConfig() Config {
	return currentConfig
}
