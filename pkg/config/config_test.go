package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestLoadConfig_MissingFile(t *testing.T) {
	// Test loading a non-existent file should return default config without error
	cfg, err := LoadConfig("non_existent_file.yaml")
	require.NoError(t, err, "Expected no error for missing file")

	assert.Equal(t, DefaultConfig.Port, cfg.Port, "Expected default port")
}

func TestGetConfig_Singleton(t *testing.T) {
	cfg1 := GetConfig()
	cfg2 := GetConfig()

	assert.Equal(t, cfg1.Port, cfg2.Port, "GetConfig should return consistent config")
}

func TestJSONBody_UnmarshalYAML(t *testing.T) {
	// Test case 1: Body as a string (legacy/mixed mode)
	yamlDataString := `
path: /test-string
method: POST
responses:
  - status: 200
    body: '{"message": "success"}'
`
	var s1 Scenario
	err := yaml.Unmarshal([]byte(yamlDataString), &s1)
	require.NoError(t, err, "Failed to unmarshal scenario with string body")
	assert.JSONEq(t, `{"message": "success"}`, string(s1.Responses[0].Body), "Body mismatch for string input")

	// Test case 2: Body as structured YAML (new mode)
	yamlDataStruct := `
path: /test-struct
method: POST
responses:
  - status: 200
    body:
      message: success
      nested:
        key: value
`
	var s2 Scenario
	err = yaml.Unmarshal([]byte(yamlDataStruct), &s2)
	require.NoError(t, err, "Failed to unmarshal scenario with structured body")
	assert.JSONEq(t, `{"message": "success", "nested": {"key": "value"}}`, string(s2.Responses[0].Body), "Body mismatch for structured input")
}
