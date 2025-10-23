---
id: aws-bedrock
title: Connect AWS Bedrock
sidebar_position: 3
---

# Connect AWS Bedrock

This guide will help you configure Envoy AI Gateway to work with AWS Bedrock's foundation models, including Llama, Anthropic Claude, and other models available on AWS Bedrock.

## Prerequisites

Before you begin, you'll need:

- AWS credentials with access to Bedrock
- Basic setup completed from the [Basic Usage](../basic-usage.md) guide
- Basic configuration removed as described in the [Advanced Configuration](./index.md) overview

## AWS Credentials Setup

Ensure you have:

1. An AWS account with Bedrock access enabled
2. AWS credentials with permissions to:
   - `bedrock:InvokeModel`
   - `bedrock:ListFoundationModels`
   - `aws-marketplace:ViewSubscriptions` ( for Anthropic models )
3. Your AWS access key ID and secret access key
4. Enabled model access to "Llama 3.2 1B Instruct" in the `us-east-1` region
   - If you want to use a different AWS region, you must update all instances of the string
     `us-east-1` with the desired region in `aws.yaml` downloaded below.

:::tip AWS Best Practices
Consider using AWS IAM roles and limited-scope credentials for production environments.
:::

## Configuration Steps

:::info Ready to proceed?
Ensure you have followed the steps in [Connect Providers](../connect-providers/)
:::

### 1. Download configuration template

```shell
curl -O https://raw.githubusercontent.com/envoyproxy/ai-gateway/main/examples/basic/aws.yaml
```

### 2. Configure AWS Credentials

Edit the `aws.yaml` file to replace these placeholder values:

- `AWS_ACCESS_KEY_ID`: Your AWS access key ID
- `AWS_SECRET_ACCESS_KEY`: Your AWS secret access key

:::caution Security Note
Make sure to keep your AWS credentials secure and never commit them to version control.
The credentials will be stored in Kubernetes secrets.
:::

### 3. Apply Configuration

Apply the updated configuration and wait for the Gateway pod to be ready. If you already have a Gateway running,
then the secret credential update will be picked up automatically in a few seconds.

```shell
kubectl apply -f aws.yaml

kubectl wait pods --timeout=2m \
  -l gateway.envoyproxy.io/owning-gateway-name=envoy-ai-gateway-basic \
  -n envoy-gateway-system \
  --for=condition=Ready
```

### 4. Test the Configuration

You should have set `$GATEWAY_URL` as part of the basic setup before connecting to providers.
See the [Basic Usage](../basic-usage.md) page for instructions.

To access a Llama model with chat completion endpoint:

```shell
curl -H "Content-Type: application/json" \
  -d '{
    "model": "us.meta.llama3-2-1b-instruct-v1:0",
    "messages": [
      {
        "role": "user",
        "content": "Hi."
      }
    ]
  }' \
  $GATEWAY_URL/v1/chat/completions
```

To access an Anthropic model with chat completion endpoint:

```shell
curl -H "Content-Type: application/json" \
  -d '{
    "model": "anthropic.claude-3-5-sonnet-20241022-v2:0",
    "messages": [
      {
        "role": "user",
        "content": "What is capital of France?"
      }
    ],
    "max_completion_tokens": 100
  }' \
  $GATEWAY_URL/v1/chat/completions
```

Expected output:

```json
{
  "choices": [
    {
      "finish_reason": "stop",
      "index": 0,
      "message": {
        "content": "The capital of France is Paris.",
        "role": "assistant"
      }
    }
  ],
  "object": "chat.completion",
  "usage": { "completion_tokens": 8, "prompt_tokens": 13, "total_tokens": 21 }
}
```

You can also access an Anthropic model with native Anthropic messages endpoint:

```shell
curl -H "Content-Type: application/json" \
  -d '{
    "model": "anthropic.claude-3-5-sonnet-20241022-v2:0",
    "messages": [
      {
        "role": "user",
        "content": "What is capital of France?"
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

## Troubleshooting

If you encounter issues:

1. **Verify your AWS credentials are correct and active**

   ```shell
   # Check if credentials are properly configured
   kubectl get secret -n default -o yaml
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
| 400        | Invalid request format                          | Verify request body matches the expected API format                  |
| 500        | AWS Bedrock internal error                      | Check AWS Bedrock service status and retry after a short delay       |

## Configuring More Models

To use more models, add more [AIGatewayRouteRule]s to the `aws.yaml` file with the [model ID] in the `value` field. For example, to use [Claude 3 Sonnet]

```yaml
apiVersion: aigateway.envoyproxy.io/v1alpha1
kind: AIGatewayRoute
metadata:
  name: envoy-ai-gateway-basic-aws
  namespace: default
spec:
  parentRefs:
    - name: envoy-ai-gateway-basic
      kind: Gateway
      group: gateway.networking.k8s.io
  rules:
    - matches:
        - headers:
            - type: Exact
              name: x-ai-eg-model
              value: anthropic.claude-3-sonnet-20240229-v1:0
      backendRefs:
        - name: envoy-ai-gateway-basic-aws
```

## Using Anthropic Native API

When using Anthropic models on AWS Bedrock, you have two options:

1. **OpenAI-compatible format** (`/v1/chat/completions`) - Works with most models but may not support all Anthropic-specific features
2. **Native Anthropic API** (`/anthropic/v1/messages`) - Provides full access to Anthropic-specific features (only for Anthropic models)

### Streaming with Native Anthropic API

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

## Advanced Features with Anthropic Models

Since the gateway supports the native Anthropic API, you have full access to Anthropic-specific features:

### Extended Thinking

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

### Prompt Caching

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

### Tool Use (Function Calling)

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

[AIGatewayRouteRule]: ../../api/api.mdx#aigatewayrouterule
[model ID]: https://docs.aws.amazon.com/bedrock/latest/userguide/models-supported.html
[Claude 3 Sonnet]: https://docs.anthropic.com/en/docs/about-claude/models#model-comparison-table
