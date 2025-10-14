---
id: aws-bedrock
title: Connect AWS Bedrock
sidebar_position: 3
---

# Connect AWS Bedrock

This guide will help you configure Envoy AI Gateway to work with AWS Bedrock's foundation models.

## Prerequisites

Before you begin, you'll need:

- AWS credentials with access to Bedrock
- Basic setup completed from the [Basic Usage](../basic-usage.md) guide
- Basic configuration removed as described in the [Advanced Configuration](./index.md) overview

## Authentication Methods

Envoy AI Gateway supports three authentication methods for AWS Bedrock:

1. **EKS Pod Identity** - Recommended for production on EKS (v1.24+)
2. **IRSA (IAM Roles for Service Accounts)** - Recommended for production on EKS (all versions)
3. **Static Credentials** - For development, testing, or non-EKS environments

### Method 1: EKS Pod Identity (Recommended for EKS 1.24+)

EKS Pod Identity is the newer, simpler way to grant AWS permissions to your pods. It's easier to set up than IRSA and doesn't require OIDC provider configuration.

**Prerequisites:**

- Amazon EKS cluster version 1.24 or later
- EKS Pod Identity Agent installed (automatic in newer EKS versions)
- IAM role with Bedrock permissions
- Enabled model access to "Llama 3.2 1B Instruct" in the `us-east-1` region

**Benefits:**

- ✅ No static credentials to manage or rotate
- ✅ Automatic credential refresh by AWS
- ✅ Fine-grained IAM permissions per Gateway
- ✅ Simpler setup than IRSA (no OIDC provider needed)
- ✅ Better security audit trail

[Jump to EKS Pod Identity Configuration →](#eks-pod-identity-configuration-steps)

### Method 2: IRSA (Recommended for EKS, all versions)

IRSA allows your pods to assume IAM roles without storing static credentials. This works on all EKS versions and is the traditional method for AWS authentication.

**Prerequisites:**

- Amazon EKS cluster with OIDC provider enabled
- IAM role with Bedrock permissions configured for your cluster
- Enabled model access to "Llama 3.2 1B Instruct" in the `us-east-1` region

**Benefits:**

- ✅ No static credentials to manage or rotate
- ✅ Automatic credential refresh by AWS
- ✅ Fine-grained IAM permissions per Gateway
- ✅ Better security audit trail
- ✅ Works on all EKS versions

[Jump to IRSA Configuration →](#irsa-configuration-steps)

### Method 3: Static Credentials

Use AWS access key ID and secret for authentication. Suitable for development, testing, or when running outside of EKS.

**Prerequisites:**

- AWS access key ID and secret access key
- Credentials with permissions to:
  - `bedrock:InvokeModel`
  - `bedrock:ListFoundationModels`
- Enabled model access to "Llama 3.2 1B Instruct" in the `us-east-1` region

:::caution Production Warning
Static credentials are not recommended for production. Consider using IRSA on EKS or other credential rotation mechanisms.
:::

[Jump to Static Credentials Configuration →](#static-credentials-configuration-steps)

---

## EKS Pod Identity Configuration Steps

:::info Ready to proceed?
Ensure you have followed the steps in [Connect Providers](../connect-providers/) and have an EKS cluster version 1.24 or later.
:::

### 1. Verify EKS Pod Identity Agent

First, verify that the EKS Pod Identity Agent is installed in your cluster:

```shell
kubectl get daemonset eks-pod-identity-agent -n kube-system
```

If not installed, install it using:

```shell
aws eks create-addon --cluster-name YOUR_CLUSTER_NAME --addon-name eks-pod-identity-agent
```

### 2. Create IAM Policy for Bedrock

Create a file named `bedrock-policy.json`:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "bedrock:InvokeModel",
        "bedrock:InvokeModelWithResponseStream",
        "bedrock:ListFoundationModels"
      ],
      "Resource": "*"
    }
  ]
}
```

Create the policy:

```shell
aws iam create-policy \
  --policy-name AIGatewayBedrockAccess \
  --policy-document file://bedrock-policy.json
```

### 3. Create IAM Role for Pod Identity

Create a trust policy file named `pod-identity-trust-policy.json`:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "pods.eks.amazonaws.com"
      },
      "Action": ["sts:AssumeRole", "sts:TagSession"]
    }
  ]
}
```

Create the role and attach the policy:

```shell
# Get your AWS account ID
ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)

# Create the role
aws iam create-role \
  --role-name AIGatewayBedrockPodIdentityRole \
  --assume-role-policy-document file://pod-identity-trust-policy.json

# Attach the policy
aws iam attach-role-policy \
  --role-name AIGatewayBedrockPodIdentityRole \
  --policy-arn arn:aws:iam::$ACCOUNT_ID:policy/AIGatewayBedrockAccess

# Get the role ARN (save this for next step)
aws iam get-role --role-name AIGatewayBedrockPodIdentityRole --query Role.Arn --output text
```

### 4. Create Pod Identity Association

Create the association between your ServiceAccount and the IAM role:

```shell
aws eks create-pod-identity-association \
  --cluster-name YOUR_CLUSTER_NAME \
  --namespace envoy-gateway-system \
  --service-account ai-gateway-dataplane-aws \
  --role-arn arn:aws:iam::$ACCOUNT_ID:role/AIGatewayBedrockPodIdentityRole
```

### 5. Download and Apply Pod Identity Configuration

Download the configuration template:

```shell
curl -O https://raw.githubusercontent.com/envoyproxy/ai-gateway/main/examples/basic/aws-pod-identity.yaml
```

**No modifications needed!** The configuration automatically uses the AWS credential chain, which includes Pod Identity.

Apply the configuration:

```shell
kubectl apply -f aws-pod-identity.yaml

# Wait for the Gateway pod to be ready
kubectl wait pods --timeout=2m \
  -l gateway.envoyproxy.io/owning-gateway-name=envoy-ai-gateway-basic \
  -n envoy-gateway-system \
  --for=condition=Ready
```

### 6. Verify Pod Identity is Working

Check that the pod has the AWS environment variables:

```shell
POD_NAME=$(kubectl get pod -n envoy-gateway-system \
  -l gateway.envoyproxy.io/owning-gateway-name=envoy-ai-gateway-basic \
  -o jsonpath='{.items[0].metadata.name}')

# Check for Pod Identity environment variables
kubectl exec -n envoy-gateway-system $POD_NAME -c extproc -- env | grep AWS
```

With Pod Identity, you should see environment variables set by the EKS Pod Identity Agent.

### 7. Test the Configuration

```shell
# Set GATEWAY_URL if not already set
export GATEWAY_URL=$(kubectl get gateway envoy-ai-gateway-basic -n default -o jsonpath='{.status.addresses[0].value}')

# Test request
curl -H "Content-Type: application/json" \
  -d '{
    "model": "us.meta.llama3-2-1b-instruct-v1:0",
    "messages": [
      {
        "role": "user",
        "content": "Hello from EKS Pod Identity!"
      }
    ]
  }' \
  http://$GATEWAY_URL/v1/chat/completions
```

If successful, you should receive a response from AWS Bedrock using Pod Identity! 🎉

---

## IRSA Configuration Steps

:::info Ready to proceed?
Ensure you have followed the steps in [Connect Providers](../connect-providers/) and have an EKS cluster with OIDC provider enabled.
:::

:::tip Consider EKS Pod Identity
If you're on EKS 1.24+, consider using [EKS Pod Identity](#eks-pod-identity-configuration-steps) instead. It's simpler to set up and doesn't require OIDC provider configuration.
:::

### 1. Create IAM Role for IRSA

First, you need to create an IAM role that your pods will assume. This role needs:

- Trust policy allowing your EKS cluster's OIDC provider
- Permissions policy for Bedrock access

**Step 1a: Get your EKS cluster's OIDC provider ARN**

```shell
# Get your cluster's OIDC provider URL
OIDC_PROVIDER=$(aws eks describe-cluster --name YOUR_CLUSTER_NAME --query "cluster.identity.oidc.issuer" --output text | sed -e "s/^https:\/\///")

# Get your AWS account ID
ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)

echo "OIDC Provider: $OIDC_PROVIDER"
echo "Account ID: $ACCOUNT_ID"
```

**Step 1b: Create IAM policy for Bedrock**

Create a file named `bedrock-policy.json`:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "bedrock:InvokeModel",
        "bedrock:InvokeModelWithResponseStream",
        "bedrock:ListFoundationModels"
      ],
      "Resource": "*"
    }
  ]
}
```

Create the policy:

```shell
aws iam create-policy \
  --policy-name AIGatewayBedrockAccess \
  --policy-document file://bedrock-policy.json
```

**Step 1c: Create IAM role with trust policy**

Create a file named `trust-policy.json` (replace `YOUR_CLUSTER_OIDC_PROVIDER` with your OIDC provider):

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::YOUR_ACCOUNT_ID:oidc-provider/YOUR_CLUSTER_OIDC_PROVIDER"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "YOUR_CLUSTER_OIDC_PROVIDER:sub": "system:serviceaccount:envoy-gateway-system:ai-gateway-dataplane-irsa",
          "YOUR_CLUSTER_OIDC_PROVIDER:aud": "sts.amazonaws.com"
        }
      }
    }
  ]
}
```

Create the role and attach the policy:

```shell
# Create the role
aws iam create-role \
  --role-name AIGatewayBedrockRole \
  --assume-role-policy-document file://trust-policy.json

# Attach the policy
aws iam attach-role-policy \
  --role-name AIGatewayBedrockRole \
  --policy-arn arn:aws:iam::$ACCOUNT_ID:policy/AIGatewayBedrockAccess

# Get the role ARN (save this for next step)
aws iam get-role --role-name AIGatewayBedrockRole --query Role.Arn --output text
```

### 2. Download and Configure IRSA Template

Download the IRSA configuration template:

```shell
curl -O https://raw.githubusercontent.com/envoyproxy/ai-gateway/main/examples/basic/aws-irsa.yaml
```

Edit `aws-irsa.yaml` and replace:

- `ACCOUNT_ID`: Your AWS account ID (from step 1)
- `arn:aws:iam::ACCOUNT_ID:role/ai-gateway-bedrock-role`: Your IAM role ARN from step 1c

The key part to update:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ai-gateway-dataplane-irsa
  namespace: envoy-gateway-system
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::YOUR_ACCOUNT_ID:role/AIGatewayBedrockRole # ← Update this
```

### 3. Apply IRSA Configuration

Apply the configuration:

```shell
kubectl apply -f aws-irsa.yaml

# Wait for the Gateway pod to be ready
kubectl wait pods --timeout=2m \
  -l gateway.envoyproxy.io/owning-gateway-name=envoy-ai-gateway-basic \
  -n envoy-gateway-system \
  --for=condition=Ready
```

### 4. Verify IRSA is Working

Check that the pod has the IRSA environment variables:

```shell
POD_NAME=$(kubectl get pod -n envoy-gateway-system \
  -l gateway.envoyproxy.io/owning-gateway-name=envoy-ai-gateway-basic \
  -o jsonpath='{.items[0].metadata.name}')

# Check environment variables
kubectl exec -n envoy-gateway-system $POD_NAME -c extproc -- env | grep AWS
```

You should see:

```
AWS_ROLE_ARN=arn:aws:iam::YOUR_ACCOUNT_ID:role/AIGatewayBedrockRole
AWS_WEB_IDENTITY_TOKEN_FILE=/var/run/secrets/eks.amazonaws.com/serviceaccount/token
```

### 5. Test the Configuration

```shell
# Set GATEWAY_URL if not already set
export GATEWAY_URL=$(kubectl get gateway envoy-ai-gateway-basic -n default -o jsonpath='{.status.addresses[0].value}')

# Test request
curl -H "Content-Type: application/json" \
  -d '{
    "model": "us.meta.llama3-2-1b-instruct-v1:0",
    "messages": [
      {
        "role": "user",
        "content": "Hello from IRSA!"
      }
    ]
  }' \
  http://$GATEWAY_URL/v1/chat/completions
```

If successful, you should receive a response from AWS Bedrock without any static credentials! 🎉

---

## Static Credentials Configuration Steps

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
The credentials will be stored in Kubernetes secrets. For production, use EKS Pod Identity or IRSA instead.
:::

### 3. Apply Static Credentials Configuration

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

## Troubleshooting

If you encounter issues:

1. **Verify your authentication method is configured correctly:**
   - For **EKS Pod Identity**: Check that the Pod Identity association exists and the Pod Identity Agent is running
     ```shell
     aws eks list-pod-identity-associations --cluster-name YOUR_CLUSTER_NAME
     kubectl get daemonset eks-pod-identity-agent -n kube-system
     ```
   - For **IRSA**: Check that the ServiceAccount has the correct annotation and environment variables are set
     ```shell
     kubectl get sa ai-gateway-dataplane-aws -n envoy-gateway-system -o yaml
     kubectl exec -n envoy-gateway-system $POD_NAME -c extproc -- env | grep AWS
     ```
   - For **Static Credentials**: Verify the secret exists and contains the correct keys
     ```shell
     kubectl get secret -n default
     ```

2. Check pod status:

   ```shell
   kubectl get pods -n envoy-gateway-system
   ```

3. View data plane logs:

   ```shell
   POD_NAME=$(kubectl get pod -n envoy-gateway-system \
     -l gateway.envoyproxy.io/owning-gateway-name=envoy-ai-gateway-basic \
     -o jsonpath='{.items[0].metadata.name}')
   kubectl logs -n envoy-gateway-system $POD_NAME -c extproc
   ```

4. View controller logs:

   ```shell
   kubectl logs -n envoy-ai-gateway-system deployment/ai-gateway-controller
   ```

5. Common errors:
   - **401/403**: Invalid credentials or insufficient IAM permissions
   - **404**: Model not found or not available in the specified region
   - **429**: Rate limit exceeded
   - **AssumeRole errors**: Check IAM role trust policy and permissions

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

[AIGatewayRouteRule]: ../../api/api.mdx#aigatewayrouterule
[model ID]: https://docs.aws.amazon.com/bedrock/latest/userguide/models-supported.html
[Claude 3 Sonnet]: https://docs.anthropic.com/en/docs/about-claude/models#model-comparison-table
