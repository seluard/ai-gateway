// Copyright Envoy AI Gateway Authors
// SPDX-License-Identifier: Apache-2.0
// The full text of the Apache license is available in the LICENSE file at
// the root of the repo.

package translator

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"

	anthropicschema "github.com/envoyproxy/ai-gateway/internal/apischema/anthropic"
	"github.com/envoyproxy/ai-gateway/internal/internalapi"
)

// NewAnthropicToGCPAnthropicTranslator creates a translator for Anthropic to GCP Anthropic format.
// This is essentially a passthrough translator with GCP-specific modifications.
func NewAnthropicToGCPAnthropicTranslator(apiVersion string, modelNameOverride internalapi.ModelNameOverride) AnthropicMessagesTranslator {
	return &anthropicToGCPAnthropicTranslator{
		apiVersion:        apiVersion,
		modelNameOverride: modelNameOverride,
		responseHandler:   newAnthropicResponseHandler(),
	}
}

type anthropicToGCPAnthropicTranslator struct {
	apiVersion        string
	modelNameOverride internalapi.ModelNameOverride
	requestModel      internalapi.RequestModel
	responseHandler   *anthropicResponseHandler
}

// RequestBody implements [AnthropicMessagesTranslator.RequestBody] for Anthropic to GCP Anthropic translation.
// This handles the transformation from native Anthropic format to GCP Anthropic format.
func (a *anthropicToGCPAnthropicTranslator) RequestBody(_ []byte, body *anthropicschema.MessagesRequest, _ bool) (
	headerMutation *extprocv3.HeaderMutation, bodyMutation *extprocv3.BodyMutation, err error,
) {
	// Extract model name for GCP endpoint from the parsed request.
	modelName := body.GetModel()

	// Work directly with the map since MessagesRequest is already map[string]interface{}.
	anthropicReq := make(map[string]any)
	maps.Copy(anthropicReq, *body)

	// Apply model name override if configured.
	a.requestModel = applyModelNameOverride(modelName, a.modelNameOverride)

	// Remove the model field since GCP doesn't want it in the body.
	delete(anthropicReq, "model")

	// Add GCP-specific anthropic_version field (required by GCP Vertex AI).
	// Uses backend config version (e.g., "vertex-2023-10-16" for GCP Vertex AI).
	if a.apiVersion == "" {
		return nil, nil, fmt.Errorf("anthropic_version is required for GCP Vertex AI but not provided in backend configuration")
	}
	anthropicReq[anthropicVersionKey] = a.apiVersion

	// Marshal the modified request.
	mutatedBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal modified request: %w", err)
	}

	// Determine the GCP path based on whether streaming is requested.
	specifier := "rawPredict"
	if stream, ok := anthropicReq["stream"].(bool); ok && stream {
		specifier = "streamRawPredict"
	}

	pathSuffix := buildGCPModelPathSuffix(gcpModelPublisherAnthropic, a.requestModel, specifier)

	headerMutation, bodyMutation = buildRequestMutations(pathSuffix, mutatedBody)
	return
}

// ResponseHeaders implements [AnthropicMessagesTranslator.ResponseHeaders] for Anthropic to GCP Anthropic.
func (a *anthropicToGCPAnthropicTranslator) ResponseHeaders(_ map[string]string) (
	headerMutation *extprocv3.HeaderMutation, err error,
) {
	// For Anthropic to GCP Anthropic, no header transformation is needed.
	return nil, nil
}

// ResponseBody implements [AnthropicMessagesTranslator.ResponseBody] for Anthropic to GCP Anthropic.
// This delegates to the shared anthropicResponseHandler since GCP Vertex AI returns the native Anthropic response format.
func (a *anthropicToGCPAnthropicTranslator) ResponseBody(headers map[string]string, body io.Reader, endOfStream bool) (
	headerMutation *extprocv3.HeaderMutation, bodyMutation *extprocv3.BodyMutation, tokenUsage LLMTokenUsage, responseModel string, err error,
) {
	return a.responseHandler.ResponseBody(headers, body, endOfStream, a.requestModel)
}
