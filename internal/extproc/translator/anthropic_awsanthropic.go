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
	"net/url"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"

	anthropicschema "github.com/envoyproxy/ai-gateway/internal/apischema/anthropic"
	"github.com/envoyproxy/ai-gateway/internal/internalapi"
)

// NewAnthropicToAWSAnthropicTranslator creates a translator for Anthropic to AWS Bedrock Anthropic format.
// AWS Bedrock supports the native Anthropic Messages API, so this is essentially a passthrough
// translator with AWS-specific path modifications.
func NewAnthropicToAWSAnthropicTranslator(apiVersion string, modelNameOverride internalapi.ModelNameOverride) AnthropicMessagesTranslator {
	return &anthropicToAWSAnthropicTranslator{
		apiVersion:        apiVersion,
		modelNameOverride: modelNameOverride,
		responseHandler:   newAnthropicResponseHandler(),
	}
}

type anthropicToAWSAnthropicTranslator struct {
	apiVersion        string
	modelNameOverride internalapi.ModelNameOverride
	requestModel      internalapi.RequestModel
	responseHandler   *anthropicResponseHandler
}

// RequestBody implements [AnthropicMessagesTranslator.RequestBody] for Anthropic to AWS Bedrock Anthropic translation.
// This handles the transformation from native Anthropic format to AWS Bedrock format.
func (a *anthropicToAWSAnthropicTranslator) RequestBody(_ []byte, body *anthropicschema.MessagesRequest, _ bool) (
	headerMutation *extprocv3.HeaderMutation, bodyMutation *extprocv3.BodyMutation, err error,
) {
	// Extract model name for AWS Bedrock endpoint from the parsed request.
	modelName := body.GetModel()

	// Work directly with the map since MessagesRequest is already map[string]interface{}.
	anthropicReq := make(map[string]any)
	maps.Copy(anthropicReq, *body)

	// Apply model name override if configured.
	a.requestModel = applyModelNameOverride(modelName, a.modelNameOverride)

	// Remove the model field since AWS Bedrock doesn't want it in the body (it's in the path).
	delete(anthropicReq, "model")

	// Add AWS-Bedrock-specific anthropic_version field (required by AWS Bedrock).
	// Uses backend config version (e.g., "bedrock-2023-05-31" for AWS Bedrock).
	if a.apiVersion == "" {
		return nil, nil, fmt.Errorf("anthropic_version is required for AWS Bedrock but not provided in backend configuration")
	}
	anthropicReq[anthropicVersionKey] = a.apiVersion

	// Marshal the modified request.
	mutatedBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal modified request: %w", err)
	}

	// Determine the AWS Bedrock path based on whether streaming is requested.
	var pathTemplate string
	if stream, ok := anthropicReq["stream"].(bool); ok && stream {
		pathTemplate = "/model/%s/invoke-stream"
	} else {
		pathTemplate = "/model/%s/invoke"
	}

	// URL encode the model ID for the path to handle ARNs with special characters.
	// AWS Bedrock model IDs can be simple names (e.g., "anthropic.claude-3-5-sonnet-20241022-v2:0")
	// or full ARNs which may contain special characters.
	encodedModelID := url.PathEscape(a.requestModel)
	pathSuffix := fmt.Sprintf(pathTemplate, encodedModelID)

	headerMutation, bodyMutation = buildRequestMutations(pathSuffix, mutatedBody)
	return
}

// ResponseHeaders implements [AnthropicMessagesTranslator.ResponseHeaders] for Anthropic to AWS Bedrock Anthropic.
func (a *anthropicToAWSAnthropicTranslator) ResponseHeaders(_ map[string]string) (
	headerMutation *extprocv3.HeaderMutation, err error,
) {
	// For Anthropic to AWS Bedrock Anthropic, no header transformation is needed.
	return nil, nil
}

// ResponseBody implements [AnthropicMessagesTranslator.ResponseBody] for Anthropic to AWS Bedrock Anthropic.
// This delegates to the shared anthropicResponseHandler since AWS Bedrock returns the native Anthropic response format.
func (a *anthropicToAWSAnthropicTranslator) ResponseBody(headers map[string]string, body io.Reader, endOfStream bool) (
	headerMutation *extprocv3.HeaderMutation, bodyMutation *extprocv3.BodyMutation, tokenUsage LLMTokenUsage, responseModel string, err error,
) {
	return a.responseHandler.ResponseBody(headers, body, endOfStream, a.requestModel)
}
