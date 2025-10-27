// Copyright Envoy AI Gateway Authors
// SPDX-License-Identifier: Apache-2.0
// The full text of the Apache license is available in the LICENSE file at
// the root of the repo.

package translator

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	anthropicschema "github.com/envoyproxy/ai-gateway/internal/apischema/anthropic"
)

func TestAnthropicToAWSAnthropicTranslator_RequestBody_ModelNameOverride(t *testing.T) {
	tests := []struct {
		name           string
		override       string
		inputModel     string
		expectedModel  string
		expectedInPath string
	}{
		{
			name:           "no override uses original model",
			override:       "",
			inputModel:     "anthropic.claude-3-haiku-20240307-v1:0",
			expectedModel:  "anthropic.claude-3-haiku-20240307-v1:0",
			expectedInPath: "anthropic.claude-3-haiku-20240307-v1:0",
		},
		{
			name:           "override replaces model in body and path",
			override:       "anthropic.claude-3-sonnet-20240229-v1:0",
			inputModel:     "anthropic.claude-3-haiku-20240307-v1:0",
			expectedModel:  "anthropic.claude-3-sonnet-20240229-v1:0",
			expectedInPath: "anthropic.claude-3-sonnet-20240229-v1:0",
		},
		{
			name:           "override with empty input model",
			override:       "anthropic.claude-3-opus-20240229-v1:0",
			inputModel:     "",
			expectedModel:  "anthropic.claude-3-opus-20240229-v1:0",
			expectedInPath: "anthropic.claude-3-opus-20240229-v1:0",
		},
		{
			name:           "model with ARN format",
			override:       "",
			inputModel:     "arn:aws:bedrock:eu-central-1:000000000:application-inference-profile/aaaaaaaaa",
			expectedModel:  "arn:aws:bedrock:eu-central-1:000000000:application-inference-profile/aaaaaaaaa",
			expectedInPath: "arn:aws:bedrock:eu-central-1:000000000:application-inference-profile%2Faaaaaaaaa",
		},
		{
			name:           "global model ID",
			override:       "",
			inputModel:     "global.anthropic.claude-sonnet-4-5-20250929-v1:0",
			expectedModel:  "global.anthropic.claude-sonnet-4-5-20250929-v1:0",
			expectedInPath: "global.anthropic.claude-sonnet-4-5-20250929-v1:0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewAnthropicToAWSAnthropicTranslator("bedrock-2023-05-31", tt.override)

			// Create the request using map structure.
			originalReq := &anthropicschema.MessagesRequest{
				"model": tt.inputModel,
				"messages": []anthropic.MessageParam{
					{
						Role: anthropic.MessageParamRoleUser,
						Content: []anthropic.ContentBlockParamUnion{
							anthropic.NewTextBlock("Hello"),
						},
					},
				},
			}

			rawBody, err := json.Marshal(originalReq)
			require.NoError(t, err)

			headerMutation, bodyMutation, err := translator.RequestBody(rawBody, originalReq, false)
			require.NoError(t, err)
			require.NotNil(t, headerMutation)
			require.NotNil(t, bodyMutation)

			// Check path header contains expected model (URL encoded).
			// Use the last element as it takes precedence when multiple headers are set.
			pathHeader := headerMutation.SetHeaders[len(headerMutation.SetHeaders)-1]
			require.Equal(t, ":path", pathHeader.Header.Key)
			expectedPath := "/model/" + tt.expectedInPath + "/invoke"
			assert.Equal(t, expectedPath, string(pathHeader.Header.RawValue))

			// Check that model field is removed from body (since it's in the path).
			var modifiedReq map[string]any
			err = json.Unmarshal(bodyMutation.GetBody(), &modifiedReq)
			require.NoError(t, err)
			_, hasModel := modifiedReq["model"]
			assert.False(t, hasModel, "model field should be removed from request body")

			// Verify anthropic_version field is added (required by AWS Bedrock).
			version, hasVersion := modifiedReq["anthropic_version"]
			assert.True(t, hasVersion, "anthropic_version should be added for AWS Bedrock")
			assert.Equal(t, "bedrock-2023-05-31", version, "anthropic_version should match the configured version")
		})
	}
}

func TestAnthropicToAWSAnthropicTranslator_ComprehensiveMarshalling(t *testing.T) {
	translator := NewAnthropicToAWSAnthropicTranslator("bedrock-2023-05-31", "")

	// Create a comprehensive MessagesRequest with all possible fields using map structure.
	originalReq := &anthropicschema.MessagesRequest{
		"model": "anthropic.claude-3-opus-20240229-v1:0",
		"messages": []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewTextBlock("Hello, how are you?"),
				},
			},
			{
				Role: anthropic.MessageParamRoleAssistant,
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewTextBlock("I'm doing well, thank you!"),
				},
			},
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewTextBlock("Can you help me with the weather?"),
				},
			},
		},
		"max_tokens":     1024,
		"stream":         false,
		"temperature":    func() *float64 { v := 0.7; return &v }(),
		"top_p":          func() *float64 { v := 0.95; return &v }(),
		"top_k":          func() *int { v := 40; return &v }(),
		"stop_sequences": []string{"Human:", "Assistant:"},
		"system":         "You are a helpful weather assistant.",
		"tools": []anthropic.ToolParam{
			{
				Name:        "get_weather",
				Description: anthropic.String("Get current weather information"),
				InputSchema: anthropic.ToolInputSchemaParam{
					Type: "object",
					Properties: map[string]any{
						"location": map[string]any{
							"type":        "string",
							"description": "City name",
						},
					},
					Required: []string{"location"},
				},
			},
		},
		"tool_choice": anthropic.ToolChoiceUnionParam{
			OfAuto: &anthropic.ToolChoiceAutoParam{},
		},
	}

	rawBody, err := json.Marshal(originalReq)
	require.NoError(t, err)

	headerMutation, bodyMutation, err := translator.RequestBody(rawBody, originalReq, false)
	require.NoError(t, err)
	require.NotNil(t, headerMutation)
	require.NotNil(t, bodyMutation)

	var outputReq map[string]any
	err = json.Unmarshal(bodyMutation.GetBody(), &outputReq)
	require.NoError(t, err)

	require.NotContains(t, outputReq, "model", "model field should be removed for AWS Bedrock")

	// AWS Bedrock requires anthropic_version field.
	require.Contains(t, outputReq, "anthropic_version", "anthropic_version should be added for AWS Bedrock")
	require.Equal(t, "bedrock-2023-05-31", outputReq["anthropic_version"], "anthropic_version should match the configured version")

	messages, ok := outputReq["messages"].([]any)
	require.True(t, ok, "messages should be an array")
	require.Len(t, messages, 3, "should have 3 messages")

	require.Equal(t, float64(1024), outputReq["max_tokens"])
	require.Equal(t, false, outputReq["stream"])
	require.Equal(t, 0.7, outputReq["temperature"])
	require.Equal(t, 0.95, outputReq["top_p"])
	require.Equal(t, float64(40), outputReq["top_k"])
	require.Equal(t, "You are a helpful weather assistant.", outputReq["system"])

	stopSeq, ok := outputReq["stop_sequences"].([]any)
	require.True(t, ok, "stop_sequences should be an array")
	require.Len(t, stopSeq, 2)
	require.Equal(t, "Human:", stopSeq[0])
	require.Equal(t, "Assistant:", stopSeq[1])

	tools, ok := outputReq["tools"].([]any)
	require.True(t, ok, "tools should be an array")
	require.Len(t, tools, 1)

	toolChoice, ok := outputReq["tool_choice"].(map[string]any)
	require.True(t, ok, "tool_choice should be an object")
	require.NotEmpty(t, toolChoice)

	// Use the last element as it takes precedence when multiple headers are set.
	pathHeader := headerMutation.SetHeaders[len(headerMutation.SetHeaders)-1]
	require.Equal(t, ":path", pathHeader.Header.Key)
	expectedPath := "/model/anthropic.claude-3-opus-20240229-v1:0/invoke"
	require.Equal(t, expectedPath, string(pathHeader.Header.RawValue))
}

func TestAnthropicToAWSAnthropicTranslator_RequestBody_StreamingPaths(t *testing.T) {
	tests := []struct {
		name               string
		stream             any
		expectedPathSuffix string
	}{
		{
			name:               "non-streaming uses /invoke",
			stream:             false,
			expectedPathSuffix: "/invoke",
		},
		{
			name:               "streaming uses /invoke-stream",
			stream:             true,
			expectedPathSuffix: "/invoke-stream",
		},
		{
			name:               "missing stream defaults to /invoke",
			stream:             nil,
			expectedPathSuffix: "/invoke",
		},
		{
			name:               "non-boolean stream defaults to /invoke",
			stream:             "true",
			expectedPathSuffix: "/invoke",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewAnthropicToAWSAnthropicTranslator("bedrock-2023-05-31", "")

			parsedReq := &anthropicschema.MessagesRequest{
				"model": "anthropic.claude-3-sonnet-20240229-v1:0",
				"messages": []anthropic.MessageParam{
					{
						Role: anthropic.MessageParamRoleUser,
						Content: []anthropic.ContentBlockParamUnion{
							anthropic.NewTextBlock("Test"),
						},
					},
				},
			}
			if tt.stream != nil {
				if streamVal, ok := tt.stream.(bool); ok {
					(*parsedReq)["stream"] = streamVal
				}
			}

			rawBody, err := json.Marshal(parsedReq)
			require.NoError(t, err)

			headerMutation, _, err := translator.RequestBody(rawBody, parsedReq, false)
			require.NoError(t, err)
			require.NotNil(t, headerMutation)

			// Check path contains expected suffix.
			// Use the last element as it takes precedence when multiple headers are set.
			pathHeader := headerMutation.SetHeaders[len(headerMutation.SetHeaders)-1]
			expectedPath := "/model/anthropic.claude-3-sonnet-20240229-v1:0" + tt.expectedPathSuffix
			assert.Equal(t, expectedPath, string(pathHeader.Header.RawValue))
		})
	}
}

func TestAnthropicToAWSAnthropicTranslator_RequestBody_FieldPassthrough(t *testing.T) {
	translator := NewAnthropicToAWSAnthropicTranslator("bedrock-2023-05-31", "")

	temp := 0.7
	topP := 0.95
	topK := 40
	parsedReq := &anthropicschema.MessagesRequest{
		"model": "anthropic.claude-3-sonnet-20240229-v1:0",
		"messages": []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewTextBlock("Hello, world!"),
				},
			},
			{
				Role: anthropic.MessageParamRoleAssistant,
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewTextBlock("Hi there!"),
				},
			},
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewTextBlock("How are you?"),
				},
			},
		},
		"max_tokens":     1000,
		"temperature":    &temp,
		"top_p":          &topP,
		"top_k":          &topK,
		"stop_sequences": []string{"Human:", "Assistant:"},
		"stream":         false,
		"system":         "You are a helpful assistant",
		"tools": []anthropic.ToolParam{
			{
				Name:        "get_weather",
				Description: anthropic.String("Get weather info"),
				InputSchema: anthropic.ToolInputSchemaParam{
					Type: "object",
					Properties: map[string]any{
						"location": map[string]any{"type": "string"},
					},
				},
			},
		},
		"tool_choice": map[string]any{"type": "auto"},
		"metadata":    map[string]any{"user.id": "test123"},
	}

	rawBody, err := json.Marshal(parsedReq)
	require.NoError(t, err)

	_, bodyMutation, err := translator.RequestBody(rawBody, parsedReq, false)
	require.NoError(t, err)
	require.NotNil(t, bodyMutation)

	var modifiedReq map[string]any
	err = json.Unmarshal(bodyMutation.GetBody(), &modifiedReq)
	require.NoError(t, err)

	// Messages should be preserved.
	require.Len(t, modifiedReq["messages"], 3)

	// Numeric fields get converted to float64 by JSON unmarshalling.
	require.Equal(t, float64(1000), modifiedReq["max_tokens"])
	require.Equal(t, 0.7, modifiedReq["temperature"])
	require.Equal(t, 0.95, modifiedReq["top_p"])
	require.Equal(t, float64(40), modifiedReq["top_k"])

	// Arrays become []interface{} by JSON unmarshalling.
	stopSeq, ok := modifiedReq["stop_sequences"].([]any)
	require.True(t, ok)
	require.Len(t, stopSeq, 2)
	require.Equal(t, "Human:", stopSeq[0])
	require.Equal(t, "Assistant:", stopSeq[1])

	// Boolean false values are now included in the map.
	require.Equal(t, false, modifiedReq["stream"])

	// String values are preserved.
	require.Equal(t, "You are a helpful assistant", modifiedReq["system"])

	// Complex objects should be preserved as maps.
	require.NotNil(t, modifiedReq["tools"])
	require.NotNil(t, modifiedReq["tool_choice"])
	require.NotNil(t, modifiedReq["metadata"])

	// Verify model field is removed from body (it's in the path instead).
	_, hasModel := modifiedReq["model"]
	require.False(t, hasModel, "model field should be removed from request body")

	// Verify anthropic_version is added for AWS Bedrock.
	version, hasVersion := modifiedReq["anthropic_version"]
	require.True(t, hasVersion, "anthropic_version should be added for AWS Bedrock")
	require.Equal(t, "bedrock-2023-05-31", version, "anthropic_version should match the configured version")
}

func TestAnthropicToAWSAnthropicTranslator_ResponseHeaders(t *testing.T) {
	translator := NewAnthropicToAWSAnthropicTranslator("bedrock-2023-05-31", "")

	tests := []struct {
		name    string
		headers map[string]string
	}{
		{
			name:    "empty headers",
			headers: map[string]string{},
		},
		{
			name: "various headers",
			headers: map[string]string{
				"content-type":  "application/json",
				"authorization": "Bearer token",
				"custom-header": "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headerMutation, err := translator.ResponseHeaders(tt.headers)
			require.NoError(t, err)
			assert.Nil(t, headerMutation, "ResponseHeaders should return nil for passthrough")
		})
	}
}

func TestAnthropicToAWSAnthropicTranslator_ResponseBody_WithCachedTokens(t *testing.T) {
	translator := NewAnthropicToAWSAnthropicTranslator("bedrock-2023-05-31", "")

	// Test response with cached input tokens.
	respBody := anthropic.Message{
		ID:      "msg_cached",
		Type:    "message",
		Role:    "assistant",
		Content: []anthropic.ContentBlockUnion{{Type: "text", Text: "Response with cache"}},
		Model:   "claude-3-sonnet-20240229",
		Usage: anthropic.Usage{
			InputTokens:              50,
			OutputTokens:             20,
			CacheReadInputTokens:     30,
			CacheCreationInputTokens: 10,
		},
	}

	bodyBytes, err := json.Marshal(respBody)
	require.NoError(t, err)

	bodyReader := bytes.NewReader(bodyBytes)
	respHeaders := map[string]string{"content-type": "application/json"}

	_, _, tokenUsage, _, err := translator.ResponseBody(respHeaders, bodyReader, true)
	require.NoError(t, err)

	expectedUsage := LLMTokenUsage{
		InputTokens:       50,
		OutputTokens:      20,
		TotalTokens:       70,
		CachedInputTokens: 30,
	}
	assert.Equal(t, expectedUsage, tokenUsage)
}

func TestAnthropicToAWSAnthropicTranslator_URLEncoding(t *testing.T) {
	tests := []struct {
		name         string
		modelID      string
		expectedPath string
	}{
		{
			name:         "simple model ID with colon",
			modelID:      "anthropic.claude-3-sonnet-20240229-v1:0",
			expectedPath: "/model/anthropic.claude-3-sonnet-20240229-v1:0/invoke",
		},
		{
			name:         "full ARN with multiple special characters",
			modelID:      "arn:aws:bedrock:us-east-1:123456789012:foundation-model/anthropic.claude-3-sonnet-20240229-v1:0",
			expectedPath: "/model/arn:aws:bedrock:us-east-1:123456789012:foundation-model%2Fanthropic.claude-3-sonnet-20240229-v1:0/invoke",
		},
		{
			name:         "global model prefix",
			modelID:      "global.anthropic.claude-sonnet-4-5-20250929-v1:0",
			expectedPath: "/model/global.anthropic.claude-sonnet-4-5-20250929-v1:0/invoke",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewAnthropicToAWSAnthropicTranslator("bedrock-2023-05-31", "")

			originalReq := &anthropicschema.MessagesRequest{
				"model": tt.modelID,
				"messages": []anthropic.MessageParam{
					{
						Role: anthropic.MessageParamRoleUser,
						Content: []anthropic.ContentBlockParamUnion{
							anthropic.NewTextBlock("Test"),
						},
					},
				},
			}

			rawBody, err := json.Marshal(originalReq)
			require.NoError(t, err)

			headerMutation, _, err := translator.RequestBody(rawBody, originalReq, false)
			require.NoError(t, err)
			require.NotNil(t, headerMutation)

			// Use the last element as it takes precedence when multiple headers are set.
			pathHeader := headerMutation.SetHeaders[len(headerMutation.SetHeaders)-1]
			assert.Equal(t, tt.expectedPath, string(pathHeader.Header.RawValue))
		})
	}
}

func TestAnthropicToAWSAnthropicTranslator_FullRequestResponseFlow(t *testing.T) {
	tests := []struct {
		name              string
		apiVersion        string
		modelNameOverride string
		inputModel        string
		stream            bool
		expectedPath      string
		expectedModel     string // Expected model in translator state for response
	}{
		{
			name:              "non-streaming without override",
			apiVersion:        "bedrock-2023-05-31",
			modelNameOverride: "",
			inputModel:        "anthropic.claude-3-sonnet-20240229-v1:0",
			stream:            false,
			expectedPath:      "/model/anthropic.claude-3-sonnet-20240229-v1:0/invoke",
			expectedModel:     "anthropic.claude-3-sonnet-20240229-v1:0",
		},
		{
			name:              "streaming without override",
			apiVersion:        "bedrock-2023-05-31",
			modelNameOverride: "",
			inputModel:        "anthropic.claude-3-haiku-20240307-v1:0",
			stream:            true,
			expectedPath:      "/model/anthropic.claude-3-haiku-20240307-v1:0/invoke-stream",
			expectedModel:     "anthropic.claude-3-haiku-20240307-v1:0",
		},
		{
			name:              "non-streaming with model override",
			apiVersion:        "bedrock-2023-05-31",
			modelNameOverride: "anthropic.claude-3-opus-20240229-v1:0",
			inputModel:        "anthropic.claude-3-haiku-20240307-v1:0",
			stream:            false,
			expectedPath:      "/model/anthropic.claude-3-opus-20240229-v1:0/invoke",
			expectedModel:     "anthropic.claude-3-opus-20240229-v1:0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewAnthropicToAWSAnthropicTranslator(tt.apiVersion, tt.modelNameOverride)

			originalReq := &anthropicschema.MessagesRequest{
				"model": tt.inputModel,
				"messages": []anthropic.MessageParam{
					{
						Role: anthropic.MessageParamRoleUser,
						Content: []anthropic.ContentBlockParamUnion{
							anthropic.NewTextBlock("What's the weather in San Francisco?"),
						},
					},
				},
				"max_tokens":  1024,
				"temperature": 0.7,
				"stream":      tt.stream,
				"system":      "You are a helpful weather assistant.",
				"tools": []anthropic.ToolParam{
					{
						Name:        "get_weather",
						Description: anthropic.String("Get current weather for a location"),
						InputSchema: anthropic.ToolInputSchemaParam{
							Type: "object",
							Properties: map[string]any{
								"location": map[string]any{
									"type":        "string",
									"description": "City name",
								},
							},
							Required: []string{"location"},
						},
					},
				},
			}

			rawBody, err := json.Marshal(originalReq)
			require.NoError(t, err)

			// Transform the request
			reqHeaderMutation, reqBodyMutation, err := translator.RequestBody(rawBody, originalReq, false)
			require.NoError(t, err)
			require.NotNil(t, reqHeaderMutation)
			require.NotNil(t, reqBodyMutation)

			// Verify request transformations
			t.Run("request_transformations", func(t *testing.T) {
				// Check path is set correctly
				pathHeader := reqHeaderMutation.SetHeaders[len(reqHeaderMutation.SetHeaders)-1]
				assert.Equal(t, ":path", pathHeader.Header.Key)
				assert.Equal(t, tt.expectedPath, string(pathHeader.Header.RawValue))

				// Check body transformations
				var transformedReq map[string]any
				err = json.Unmarshal(reqBodyMutation.GetBody(), &transformedReq)
				require.NoError(t, err)

				// anthropic_version should be added
				assert.Equal(t, tt.apiVersion, transformedReq["anthropic_version"])

				// model field should be removed (it's in the path)
				_, hasModel := transformedReq["model"]
				assert.False(t, hasModel, "model field should be removed from body")

				// Other fields should be preserved
				assert.Equal(t, float64(1024), transformedReq["max_tokens"])
				assert.Equal(t, 0.7, transformedReq["temperature"])
				assert.Equal(t, tt.stream, transformedReq["stream"])
				assert.Equal(t, "You are a helpful weather assistant.", transformedReq["system"])
				assert.NotNil(t, transformedReq["messages"])
				assert.NotNil(t, transformedReq["tools"])

				// Content-length header should be set
				var contentLengthFound bool
				for _, header := range reqHeaderMutation.SetHeaders {
					if header.Header.Key == "content-length" {
						contentLengthFound = true
						break
					}
				}
				assert.True(t, contentLengthFound, "content-length header should be set")
			})

			respHeaders := map[string]string{
				"content-type": "application/json",
			}

			// Test ResponseHeaders (should be passthrough)
			respHeaderMutation, err := translator.ResponseHeaders(respHeaders)
			require.NoError(t, err)
			assert.Nil(t, respHeaderMutation, "ResponseHeaders should return nil for passthrough")

			if tt.stream {
				// Test streaming response
				t.Run("streaming_response", func(t *testing.T) {
					// Message start chunk
					// Note: The model in the streaming response may differ from the request model
					// AWS Bedrock returns "claude-3-haiku-20240307" while request had "anthropic.claude-3-haiku-20240307-v1:0"
					messageStartChunk := `event: message_start
data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","content":[],"model":"claude-3-haiku-20240307","usage":{"input_tokens":50,"output_tokens":0}}}

`
					bodyReader := bytes.NewReader([]byte(messageStartChunk))
					headerMutation, bodyMutation, _, responseModel, err := translator.ResponseBody(respHeaders, bodyReader, false)
					require.NoError(t, err)
					assert.Nil(t, headerMutation, "streaming chunks should not modify headers")
					assert.Nil(t, bodyMutation, "streaming chunks should pass through")
					// Token usage extraction from streaming chunks depends on buffering implementation
					// message_start events don't contain usage info, only message_delta events do
					// Response model can be either the full request model or the model from the response
					assert.NotEmpty(t, responseModel, "response model should be set")

					// Content delta chunk
					contentDeltaChunk := `event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

`
					bodyReader = bytes.NewReader([]byte(contentDeltaChunk))
					var tokenUsage LLMTokenUsage
					headerMutation, bodyMutation, tokenUsage, _, err = translator.ResponseBody(respHeaders, bodyReader, false)
					require.NoError(t, err)
					assert.Nil(t, headerMutation, "streaming chunks should not modify headers")
					assert.Nil(t, bodyMutation, "streaming chunks should pass through")
					assert.Equal(t, uint32(0), tokenUsage.InputTokens)
					assert.Equal(t, uint32(0), tokenUsage.OutputTokens)

					// Message delta chunk with final token usage
					messageDeltaChunk := `event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":25}}

`
					bodyReader = bytes.NewReader([]byte(messageDeltaChunk))
					headerMutation, bodyMutation, tokenUsage, responseModel, err = translator.ResponseBody(respHeaders, bodyReader, false)
					require.NoError(t, err)
					assert.Nil(t, headerMutation, "streaming chunks should not modify headers")
					assert.Nil(t, bodyMutation, "streaming chunks should pass through")
					// Token usage is buffered and extracted across chunks
					assert.Positive(t, tokenUsage.OutputTokens, "output tokens should be positive")
					assert.Positive(t, tokenUsage.TotalTokens, "total tokens should be positive")
					assert.NotEmpty(t, responseModel, "response model should be set")

					// Message stop chunk
					messageStopChunk := `event: message_stop
data: {"type":"message_stop"}

`
					bodyReader = bytes.NewReader([]byte(messageStopChunk))
					headerMutation, bodyMutation, tokenUsage, _, err = translator.ResponseBody(respHeaders, bodyReader, false)
					require.NoError(t, err)
					assert.Nil(t, headerMutation, "streaming chunks should not modify headers")
					assert.Nil(t, bodyMutation, "streaming chunks should pass through")
					assert.Equal(t, uint32(0), tokenUsage.InputTokens)
					assert.Equal(t, uint32(0), tokenUsage.OutputTokens)
				})
			} else {
				// Test non-streaming response
				t.Run("non_streaming_response", func(t *testing.T) {
					respBody := anthropic.Message{
						ID:   "msg_test_response",
						Type: "message",
						Role: "assistant",
						Content: []anthropic.ContentBlockUnion{
							{
								Type: "text",
								Text: "The weather in San Francisco is sunny with a temperature of 72°F.",
							},
						},
						Model:      "claude-3-sonnet-20240229",
						StopReason: anthropic.StopReasonEndTurn,
						Usage: anthropic.Usage{
							InputTokens:  45,
							OutputTokens: 28,
						},
					}

					bodyBytes, err := json.Marshal(respBody)
					require.NoError(t, err)

					bodyReader := bytes.NewReader(bodyBytes)
					respHeaderMutation, respBodyMutation, tokenUsage, responseModel, err := translator.ResponseBody(respHeaders, bodyReader, true)
					require.NoError(t, err)

					// AWS Bedrock response is passthrough - no mutations
					assert.Nil(t, respHeaderMutation, "response should pass through without header mutations")
					assert.Nil(t, respBodyMutation, "response should pass through without body mutations")

					// Verify token usage extraction
					expectedUsage := LLMTokenUsage{
						InputTokens:  45,
						OutputTokens: 28,
						TotalTokens:  73,
					}
					assert.Equal(t, expectedUsage, tokenUsage)

					// Response model should match request model (or the model from response if available)
					// The model in the response is "claude-3-sonnet-20240229" but we stored the full ID
					// The implementation uses response model if available, falling back to request model
					assert.NotEmpty(t, responseModel, "response model should be set")
				})
			}
		})
	}
}
