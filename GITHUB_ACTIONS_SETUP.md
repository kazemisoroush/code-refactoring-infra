# GitHub Actions Pipeline Setup

This document explains how to configure GitHub Secrets and set up the CI/CD pipeline for the Code Refactoring Infrastructure.

## Required GitHub Secrets

### 1. AWS Configuration Secrets

Set these secrets in your GitHub repository settings (Settings → Secrets and variables → Actions):

#### **AWS Infrastructure Role**
```
AWS_INFRA_ROLE_ARN
```
**Value**: The ARN of your GitHub Actions IAM role for infrastructure deployment
**Example**: `arn:aws:iam::698315877107:role/CodeRefactor-GitHubActions-ECR-Role`

#### **AWS Region**
```
AWS_REGION
```
**Value**: Your AWS region
**Example**: `us-east-1`

### 2. Authentication Secrets

#### **Default User Email** (Optional)
```
DEFAULT_USER_EMAIL
```
**Value**: Email for the default test user
**Default**: `admin@code-refactor.dev`
**Example**: `your-email@example.com`

#### **Default User Password** (Optional)
```
DEFAULT_USER_PASSWORD
```
**Value**: Password for the default test user (must meet Cognito requirements)
**Default**: `CodeRefactor123!`
**Requirements**: 
- Minimum 8 characters
- Contains uppercase letter
- Contains lowercase letter  
- Contains number
- Contains special character

#### **Application Repository Name** (Optional)
```
APP_REPO_NAME
```
**Value**: Name of your main application repository
**Default**: `code-refactoring-tool`
**Example**: `my-app-repo`

## Quick Setup Commands

### 1. Set AWS Secrets
```bash
# Get your GitHub Actions role ARN from the infrastructure stack
AWS_ROLE_ARN=$(aws cloudformation describe-stacks --stack-name CodeRefactorInfra --query 'Stacks[0].Outputs[?OutputKey==`GitHubActionsRoleARN`].OutputValue' --output text)

# Set GitHub secrets using GitHub CLI
gh secret set AWS_INFRA_ROLE_ARN --body "$AWS_ROLE_ARN"
gh secret set AWS_REGION --body "us-east-1"
```

### 2. Set Authentication Secrets
```bash
# Set your preferred default user credentials
gh secret set DEFAULT_USER_EMAIL --body "your-email@example.com"
gh secret set DEFAULT_USER_PASSWORD --body "YourSecurePassword123!"

# Set your application repository name
gh secret set APP_REPO_NAME --body "your-app-repo-name"
```

### 3. Verify Secrets
```bash
# List all secrets to verify they're set
gh secret list
```

## Pipeline Workflow

The GitHub Actions pipeline consists of 3 main jobs:

### 1. **lint-and-test**
- Runs on all branches and PRs
- Lints Go and Python code
- Runs unit tests for infrastructure and Lambda functions
- Must pass before deployment

### 2. **deploy-infra** 
- Runs only on `main` branch pushes
- Deploys AWS infrastructure using CDK
- Creates/updates default authentication user
- Stores infrastructure outputs as artifacts

### 3. **update-env-vars**
- Runs after successful infrastructure deployment
- Extracts environment variables from infrastructure
- Creates environment file for application use
- Stores as downloadable artifacts

## Pipeline Outputs

### Infrastructure Outputs Artifact
Contains:
- `stack-outputs.json` - Complete CloudFormation outputs
- `env-vars.txt` - Key environment variables

### Application Environment Variables Artifact
Contains:
- `app-env-vars.txt` - Environment variables for your application

**Example environment variables:**
```bash
# Frontend variables (for Vite/React apps)
VITE_API_BASE_URL=https://api-id.execute-api.us-east-1.amazonaws.com/prod/
VITE_COGNITO_USER_POOL_ID=us-east-1_ABC123DEF
VITE_COGNITO_CLIENT_ID=1234567890abcdef
VITE_COGNITO_HOSTED_UI_URL=https://domain.auth.us-east-1.amazoncognito.com
VITE_AWS_REGION=us-east-1

# Backend variables
API_GATEWAY_URL=https://api-id.execute-api.us-east-1.amazonaws.com/prod/
COGNITO_USER_POOL_ID=us-east-1_ABC123DEF
COGNITO_CLIENT_ID=1234567890abcdef
COGNITO_REGION=us-east-1
ECR_REPOSITORY_URI=123456789012.dkr.ecr.us-east-1.amazonaws.com/refactor-ecr-repo

# AWS Resources
RDS_POSTGRES_CREDENTIALS_SECRET_ARN=arn:aws:secretsmanager:...
RDS_POSTGRES_INSTANCE_ARN=arn:aws:rds:...
S3_BUCKET_NAME=code-refactor-bucket-...
BEDROCK_KNOWLEDGE_BASE_ROLE_ARN=arn:aws:iam::...
BEDROCK_AGENT_ROLE_ARN=arn:aws:iam::...
```

## Using Environment Variables in Your Application

### Option 1: Download from GitHub Actions Artifacts
1. Go to your GitHub Actions run
2. Download the `app-environment-variables` artifact
3. Copy `app-env-vars.txt` to your application repository
4. Source it in your application: `source app-env-vars.txt`

### Option 2: Automate with GitHub API
```bash
# Get the latest artifact download URL
ARTIFACT_URL=$(gh api repos/:owner/:repo/actions/artifacts --jq '.artifacts[] | select(.name=="app-environment-variables") | .archive_download_url' | head -1)

# Download and extract
curl -L -H "Authorization: token $GITHUB_TOKEN" $ARTIFACT_URL -o env-vars.zip
unzip env-vars.zip
source app-env-vars.txt
```

### Option 3: Integration with Application Pipeline
Add this to your application's GitHub Actions:

```yaml
jobs:
  get-infrastructure-config:
    runs-on: ubuntu-latest
    steps:
      - name: Download Infrastructure Config
        uses: dawidd6/action-download-artifact@v2
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
          workflow: ci.yaml
          repo: your-username/code-refactoring-infra
          name: app-environment-variables
          path: .
      
      - name: Load Environment Variables
        run: |
          source app-env-vars.txt
          echo "API_GATEWAY_URL=$API_GATEWAY_URL" >> $GITHUB_ENV
          echo "COGNITO_USER_POOL_ID=$COGNITO_USER_POOL_ID" >> $GITHUB_ENV
          # ... add other variables as needed
```

## Manual Authentication Testing

After the pipeline runs, test authentication manually:

```bash
# Get credentials from the pipeline output or use defaults
USER_POOL_ID="us-east-1_7uUMitR0a"  # From pipeline output
CLIENT_ID="6b3g0h2rql57dhlihr5eidak0h"  # From pipeline output
API_URL="https://rhq7ues45b.execute-api.us-east-1.amazonaws.com/prod"  # From pipeline output
EMAIL="admin@code-refactor.dev"  # From GitHub Secret DEFAULT_USER_EMAIL
PASSWORD="CodeRefactor123!"  # From GitHub Secret DEFAULT_USER_PASSWORD

# Get authentication token
TOKEN_RESPONSE=$(aws cognito-idp admin-initiate-auth \
  --user-pool-id $USER_POOL_ID \
  --client-id $CLIENT_ID \
  --auth-flow ADMIN_NO_SRP_AUTH \
  --auth-parameters USERNAME=$EMAIL,PASSWORD=$PASSWORD)

ID_TOKEN=$(echo $TOKEN_RESPONSE | jq -r '.AuthenticationResult.IdToken')

# Test API
curl -H "Authorization: Bearer $ID_TOKEN" $API_URL/your-endpoint
```

## Security Best Practices

1. **Never commit secrets to code**
2. **Use GitHub Secrets for all sensitive data**
3. **Regularly rotate authentication passwords**
4. **Use least-privilege IAM roles**
5. **Monitor GitHub Actions logs for sensitive data leaks**
6. **Set appropriate artifact retention periods**

## Troubleshooting

### Common Issues

1. **Authentication Failures**
   - Check AWS credentials are correctly configured
   - Verify IAM role has necessary permissions
   - Ensure GitHub OIDC provider is set up

2. **Missing Environment Variables**
   - Check GitHub Secrets are set correctly
   - Verify secret names match exactly
   - Check for typos in secret values

3. **Pipeline Failures**
   - Check GitHub Actions logs for specific errors
   - Verify AWS services are available in your region
   - Check CloudFormation stack exists and is healthy

### Debugging Commands

```bash
# Check GitHub secrets
gh secret list

# Test AWS authentication locally
aws sts get-caller-identity

# Check CloudFormation stack status
aws cloudformation describe-stacks --stack-name CodeRefactorInfra

# Validate CDK template
cdk synth
```

## Next Steps

1. **Set up GitHub Secrets** using the commands above
2. **Push to main branch** to trigger the pipeline
3. **Download environment variables** from the artifacts
4. **Integrate with your application** using the provided methods
5. **Test authentication** using the generated credentials

For more details on authentication, see `AUTHENTICATION.md`.
