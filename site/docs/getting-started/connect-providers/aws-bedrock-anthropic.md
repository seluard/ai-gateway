---
id: aws-bedrock-anthropic
title: Connect AWS Bedrock (Anthropic Native API)
sidebar_position: 4
---

# Connect AWS Bedrock with Anthropic Native API

This guide shows you how to configure Envoy AI Gateway to use Anthropic models on AWS Bedrock with the **native Anthropic Messages API format**. This allows you to use the `/anthropic/v1/messages` endpoint to call Claude models hosted on AWS Bedrock.

> [!NOTE]
> If you want to use AWS Bedrock models with the OpenAI-compatible format (`/v1/chat/completions`), see the [AWS Bedrock guide](./aws-bedrock.md) instead.

## Prerequisites

Before you begin, you'll need:

- AWS credentials with access to Bedrock
- Basic setup completed from the [Basic Usage](../basic-usage.md) guide
- Basic configuration removed as described in the [Advanced Configuration](./index.md) overview
- Model access enabled for Anthropic Claude models in your AWS region

## AWS Credentials Setup

Ensure you have:

1. An AWS account with Bedrock access enabled
2. AWS credentials with permissions to:
   - `bedrock:InvokeModel`
   - `bedrock:ListFoundationModels`
   - `aws-marketplace:ViewSubscriptions`
3. Your AWS access key ID and secret access key
4. Enabled model access to Anthropic Claude models in your desired AWS region (e.g., `us-east-1`)
   - Go to the AWS Bedrock console and request access to Anthropic models
   - If you want to use a different AWS region, you must update all instances of `us-east-1` with the desired region in the configuration file

> [!TIP]
> Consider using AWS IAM roles and limited-scope credentials for production environments. For EKS clusters, AWS IAM Roles for Service Accounts (IRSA) is recommended.

## Why Use the Native Anthropic API?

The native Anthropic API provides several advantages when working with Claude models:

- **Full feature support**: Access all Anthropic-specific features like extended thinking, prompt caching, and tool use
- **Consistent API**: Use the same API format you would with Anthropic's direct API
- **Better compatibility**: Avoid potential translation issues between OpenAI and Anthropic formats
- **Feature parity**: Get immediate access to new Anthropic features as they're released

## Configuration Steps

> [!IMPORTANT]
> Ensure you have followed the prerequisite steps in [Connect Providers](../connect-providers/) before proceeding.

### 1. Download Configuration Template

```shell
curl -O https://raw.githubusercontent.com/envoyproxy/ai-gateway/main/examples/basic/aws-bedrock-anthropic.yaml
```

### 2. Configure AWS Credentials

Edit the `aws-bedrock-anthropic.yaml` file to replace these placeholder values:

- `AWS_ACCESS_KEY_ID`: Your AWS access key ID
- `AWS_SECRET_ACCESS_KEY`: Your AWS secret access key
- Update the `region` field if you're using a region other than `us-east-1`
- Update the model ID in the `value` field if you want to use a different Claude model

> [!CAUTION]
> Make sure to keep your AWS credentials secure and never commit them to version control. The credentials will be stored in Kubernetes secrets.

### 3. Apply Configuration

Apply the updated configuration and wait for the Gateway pod to be ready. If you already have a Gateway running, the secret credential update will be picked up automatically in a few seconds.

```shell
kubectl apply -f aws-bedrock-anthropic.yaml

kubectl wait pods --timeout=2m \
  -l gateway.envoyproxy.io/owning-gateway-name=envoy-ai-gateway-basic \
  -n envoy-gateway-system \
  --for=condition=Ready
```

### 4. Test the Configuration

You should have set `$GATEWAY_URL` as part of the basic setup before connecting to providers. See the [Basic Usage](../basic-usage.md) page for instructions.

Test your configuration using the native Anthropic Messages API format:

```shell
curl -H "Content-Type: application/json" \
  -d '{
    "model": "anthropic.claude-3-5-sonnet-20241022-v2:0",
    "messages": [
      {
        "role": "user",
        "content": "What is the capital of France?"
      }
    ],
    "max_tokens": 100
  }' \
  $GATEWAY_URL/anthropic/v1/messages
```

Expected output:

```json
{
  "id": "msg_01XFDUDYJgAACzvnptvVoYEL",
  "type": "message",
  "role": "assistant",
  "content": [
    {
      "type": "text",
      "text": "The capital of France is Paris."
    }
  ],
  "model": "claude-3-5-sonnet-20241022",
  "stop_reason": "end_turn",
  "usage": {
    "input_tokens": 13,
    "output_tokens": 8
  }
}
```

### 5. Test Streaming

The native Anthropic API also supports streaming responses:

```shell
curl -H "Content-Type: application/json" \
  -d '{
    "model": "anthropic.claude-3-5-sonnet-20241022-v2:0",
    "messages": [
      {
        "role": "user",
        "content": "Count from 1 to 5."
      }
    ],
    "max_tokens": 100,
    "stream": true
  }' \
  $GATEWAY_URL/anthropic/v1/messages
```


## Configuring More Models

To use additional models, add more `AIGatewayRoute` rules to the configuration file. Each rule should specify a different model ID:

```yaml
apiVersion: aigateway.envoyproxy.io/v1alpha1
kind: AIGatewayRoute
metadata:
  name: envoy-ai-gateway-basic-aws-bedrock-anthropic
  namespace: default
spec:
  parentRefs:
    - name: envoy-ai-gateway-basic
      kind: Gateway
      group: gateway.networking.k8s.io
  rules:
    # Claude 3.5 Sonnet (Oct 2024)
    - matches:
        - headers:
            - type: Exact
              name: x-ai-eg-model
              value: anthropic.claude-3-5-sonnet-20241022-v2:0
      backendRefs:
        - name: envoy-ai-gateway-basic-aws-bedrock-anthropic
    # Claude 3 Opus
    - matches:
        - headers:
            - type: Exact
              name: x-ai-eg-model
              value: anthropic.claude-3-opus-20240229-v1:0
      backendRefs:
        - name: envoy-ai-gateway-basic-aws-bedrock-anthropic
```

## Advanced Features

### Using Anthropic-Specific Features

Since this configuration uses the native Anthropic API, you have full access to Anthropic-specific features:

#### Extended Thinking

```shell
curl -H "Content-Type: application/json" \
  -d '{
    "model": "anthropic.claude-3-5-sonnet-20241022-v2:0",
    "messages": [
      {
        "role": "user",
        "content": "Solve this puzzle: A farmer needs to cross a river with a fox, chicken, and bag of grain. The boat can only hold the farmer and one item. How does the farmer get everything across safely?"
      }
    ],
    "max_tokens": 1000,
    "thinking": {
      "type": "enabled",
      "budget_tokens": 5000
    }
  }' \
  $GATEWAY_URL/anthropic/v1/messages
```

#### Prompt Caching

```shell
curl -H "Content-Type: application/json" \
  -d '{
    "model": "anthropic.claude-3-5-sonnet-20241022-v2:0",
    "system": [
      {
        "type": "text",
        "text": "You are an AI assistant specialized in Python programming. You help users write clean, efficient Python code.",
        "cache_control": {"type": "ephemeral"}
      }
    ],
    "messages": [
      {
        "role": "user",
        "content": "Write a function to calculate fibonacci numbers."
      }
    ],
    "max_tokens": 500
  }' \
  $GATEWAY_URL/anthropic/v1/messages
```

#### Tool Use (Function Calling)

```shell
curl -H "Content-Type: application/json" \
  -d '{
    "model": "anthropic.claude-3-5-sonnet-20241022-v2:0",
    "messages": [
      {
        "role": "user",
        "content": "What is the weather in San Francisco?"
      }
    ],
    "max_tokens": 500,
    "tools": [
      {
        "name": "get_weather",
        "description": "Get the current weather in a given location",
        "input_schema": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "The city and state, e.g. San Francisco, CA"
            }
          },
          "required": ["location"]
        }
      }
    ]
  }' \
  $GATEWAY_URL/anthropic/v1/messages
```

## Troubleshooting

If you encounter issues:

1. **Verify your AWS credentials are correct and active**

   ```shell
   # Check if credentials are properly configured
   kubectl get secret envoy-ai-gateway-basic-aws-bedrock-anthropic-credentials -n default -o yaml
   ```

2. **Check pod status**

   ```shell
   kubectl get pods -n envoy-gateway-system
   ```

3. **View controller logs**

   ```shell
   kubectl logs -n envoy-ai-gateway-system deployment/ai-gateway-controller
   ```

4. **View gateway pod logs**

   ```shell
   kubectl logs -n envoy-gateway-system -l gateway.envoyproxy.io/owning-gateway-name=envoy-ai-gateway-basic
   ```

### Common Errors

| Error Code | Issue                                           | Solution                                                             |
| ---------- | ----------------------------------------------- | -------------------------------------------------------------------- |
| 401/403    | Invalid credentials or insufficient permissions | Verify AWS credentials and ensure Bedrock permissions are granted    |
| 404        | Model not found or not available in region      | Check model ID and ensure model access is enabled in your AWS region |
| 429        | Rate limit exceeded                             | Implement rate limiting or request quota increase from AWS           |
| 400        | Invalid request format                          | Verify request body matches Anthropic API format                     |
| 500        | AWS Bedrock internal error                      | Check AWS Bedrock service status and retry after a short delay       |

