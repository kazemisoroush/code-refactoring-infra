# API Authentication Guide

This document describes how to authenticate with the Code Refactoring API.

## Authentication Overview

The API uses **Amazon Cognito User Pools** for authentication. All API endpoints (except `/health`, `/swagger`, and `/auth`) require a valid JWT token in the Authorization header.

## Quick Start - Create Your Default User

### Step 1: Get Your Configuration Values
```bash
# Get stack outputs
STACK_OUTPUTS=$(aws cloudformation describe-stacks --stack-name CodeRefactorInfra --query 'Stacks[0].Outputs')

# Extract values
USER_POOL_ID=$(echo $STACK_OUTPUTS | jq -r '.[] | select(.OutputKey=="CognitoUserPoolID") | .OutputValue')
CLIENT_ID=$(echo $STACK_OUTPUTS | jq -r '.[] | select(.OutputKey=="CognitoUserPoolClientID") | .OutputValue')
API_URL=$(echo $STACK_OUTPUTS | jq -r '.[] | select(.OutputKey=="APIGatewayURL") | .OutputValue')
HOSTED_UI_URL=$(echo $STACK_OUTPUTS | jq -r '.[] | select(.OutputKey=="CognitoHostedUIURL") | .OutputValue')

echo "User Pool ID: $USER_POOL_ID"
echo "Client ID: $CLIENT_ID"
echo "API URL: $API_URL"
echo "Hosted UI URL: $HOSTED_UI_URL"
```

### Step 2: Create Your Default User
```bash
# Set your preferred credentials
YOUR_EMAIL="your-email@example.com"  # Change this to your email
YOUR_PASSWORD="YourSecurePassword123!"  # Change this to your preferred password

# Create user
aws cognito-idp admin-create-user \
  --user-pool-id $USER_POOL_ID \
  --username $YOUR_EMAIL \
  --temporary-password $YOUR_PASSWORD \
  --message-action SUPPRESS \
  --user-attributes Name=email,Value=$YOUR_EMAIL Name=email_verified,Value=true

# Set permanent password
aws cognito-idp admin-set-user-password \
  --user-pool-id $USER_POOL_ID \
  --username $YOUR_EMAIL \
  --password $YOUR_PASSWORD \
  --permanent

echo "✅ Default user created successfully!"
echo "📧 Email: $YOUR_EMAIL"
echo "🔑 Password: $YOUR_PASSWORD"
```

### Step 3: Get Authentication Token
```bash
# Authenticate and get tokens
TOKEN_RESPONSE=$(aws cognito-idp admin-initiate-auth \
  --user-pool-id $USER_POOL_ID \
  --client-id $CLIENT_ID \
  --auth-flow ADMIN_NO_SRP_AUTH \
  --auth-parameters USERNAME=$YOUR_EMAIL,PASSWORD=$YOUR_PASSWORD)

# Extract ID token (this is what you need for API calls)
ID_TOKEN=$(echo $TOKEN_RESPONSE | jq -r '.AuthenticationResult.IdToken')

echo "🎯 Your ID Token: $ID_TOKEN"
```

### Step 4: Test API Authentication
```bash
# Test health endpoint (no auth required)
echo "Testing health endpoint (no auth):"
curl -s $API_URL/health | jq .

# Test protected endpoint (auth required)
echo "Testing protected endpoint (with auth):"
curl -s -H "Authorization: Bearer $ID_TOKEN" $API_URL/some-protected-endpoint

# Test without auth (should get 401)
echo "Testing protected endpoint (without auth - should fail):"
curl -s $API_URL/some-protected-endpoint
```

## Complete Setup Script

Here's a complete script you can run to set everything up:

```bash
#!/bin/bash
# setup-auth.sh - Complete authentication setup script

set -e

echo "🚀 Setting up Code Refactoring API Authentication"

# Get stack outputs
echo "📋 Getting CloudFormation stack outputs..."
STACK_OUTPUTS=$(aws cloudformation describe-stacks --stack-name CodeRefactorInfra --query 'Stacks[0].Outputs')

USER_POOL_ID=$(echo $STACK_OUTPUTS | jq -r '.[] | select(.OutputKey=="CognitoUserPoolID") | .OutputValue')
CLIENT_ID=$(echo $STACK_OUTPUTS | jq -r '.[] | select(.OutputKey=="CognitoUserPoolClientID") | .OutputValue')
API_URL=$(echo $STACK_OUTPUTS | jq -r '.[] | select(.OutputKey=="APIGatewayURL") | .OutputValue')
HOSTED_UI_URL=$(echo $STACK_OUTPUTS | jq -r '.[] | select(.OutputKey=="CognitoHostedUIURL") | .OutputValue')

echo "✅ Configuration loaded:"
echo "   User Pool ID: $USER_POOL_ID"
echo "   Client ID: $CLIENT_ID"
echo "   API URL: $API_URL"
echo "   Hosted UI: $HOSTED_UI_URL"

# Prompt for user credentials
read -p "📧 Enter your email: " YOUR_EMAIL
read -s -p "🔑 Enter your password (min 8 chars, must include uppercase, lowercase, number, symbol): " YOUR_PASSWORD
echo

# Create user
echo "👤 Creating user..."
if aws cognito-idp admin-create-user \
  --user-pool-id $USER_POOL_ID \
  --username $YOUR_EMAIL \
  --temporary-password $YOUR_PASSWORD \
  --message-action SUPPRESS \
  --user-attributes Name=email,Value=$YOUR_EMAIL Name=email_verified,Value=true >/dev/null 2>&1; then
  echo "✅ User created successfully"
else
  echo "⚠️  User might already exist, continuing..."
fi

# Set permanent password
echo "🔒 Setting permanent password..."
aws cognito-idp admin-set-user-password \
  --user-pool-id $USER_POOL_ID \
  --username $YOUR_EMAIL \
  --password $YOUR_PASSWORD \
  --permanent

# Get authentication token
echo "🎫 Getting authentication token..."
TOKEN_RESPONSE=$(aws cognito-idp admin-initiate-auth \
  --user-pool-id $USER_POOL_ID \
  --client-id $CLIENT_ID \
  --auth-flow ADMIN_NO_SRP_AUTH \
  --auth-parameters USERNAME=$YOUR_EMAIL,PASSWORD=$YOUR_PASSWORD)

ID_TOKEN=$(echo $TOKEN_RESPONSE | jq -r '.AuthenticationResult.IdToken')

# Test API
echo "🧪 Testing API endpoints..."

echo "   Health check (public):"
HEALTH_RESPONSE=$(curl -s $API_URL/health)
echo "   Response: $HEALTH_RESPONSE"

echo "   Protected endpoint test:"
AUTH_TEST=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $ID_TOKEN" $API_URL/protected-test)
echo "   HTTP Status: $AUTH_TEST"

echo ""
echo "🎉 Setup complete!"
echo ""
echo "📝 Save these credentials:"
echo "   Email: $YOUR_EMAIL"
echo "   Password: $YOUR_PASSWORD"
echo "   ID Token: $ID_TOKEN"
echo ""
echo "🔗 Quick links:"
echo "   API Base URL: $API_URL"
echo "   Hosted UI Login: $HOSTED_UI_URL/login?client_id=$CLIENT_ID&response_type=token&scope=email+openid+profile&redirect_uri=https://localhost:3000/callback"
echo ""
echo "💡 To use in API calls:"
echo "   curl -H \"Authorization: Bearer $ID_TOKEN\" $API_URL/your-endpoint"
```

Save this as `setup-auth.sh`, make it executable with `chmod +x setup-auth.sh`, and run it with `./setup-auth.sh`.

### Option 1: Using Cognito Hosted UI (Easiest)

1. **Get your authentication URLs** from the CloudFormation stack outputs:
   ```bash
   # Get the hosted UI URL
   aws cloudformation describe-stacks --stack-name CodeRefactorInfra \
     --query 'Stacks[0].Outputs[?OutputKey==`CognitoHostedUIURL`].OutputValue' --output text
   
   # Get the User Pool Client ID  
   aws cloudformation describe-stacks --stack-name CodeRefactorInfra \
     --query 'Stacks[0].Outputs[?OutputKey==`CognitoUserPoolClientID`].OutputValue' --output text
   ```

2. **Sign up/Sign in via Hosted UI**:
   - Visit: `https://code-refactor-{ACCOUNT_ID}.auth.{REGION}.amazoncognito.com/login?client_id={CLIENT_ID}&response_type=token&scope=email+openid+profile&redirect_uri=https://localhost:3000/callback`
   - Replace `{ACCOUNT_ID}`, `{REGION}`, and `{CLIENT_ID}` with your actual values
   - Create an account or sign in
   - After successful authentication, extract the `id_token` from the redirect URL

3. **Use the token in API calls**:
   ```bash
   curl -H "Authorization: Bearer YOUR_ID_TOKEN_HERE" \
        https://YOUR_API_GATEWAY_URL/your-endpoint
   ```

### Option 2: Programmatic Authentication

#### Using AWS CLI/SDK (For testing/scripts)

```bash
# Sign up a new user
aws cognito-idp admin-create-user \
  --user-pool-id YOUR_USER_POOL_ID \
  --username your-email@example.com \
  --temporary-password TempPassword123! \
  --message-action SUPPRESS

# Set permanent password  
aws cognito-idp admin-set-user-password \
  --user-pool-id YOUR_USER_POOL_ID \
  --username your-email@example.com \
  --password YourNewPassword123! \
  --permanent

# Authenticate and get tokens
aws cognito-idp admin-initiate-auth \
  --user-pool-id YOUR_USER_POOL_ID \
  --client-id YOUR_CLIENT_ID \
  --auth-flow ADMIN_NO_SRP_AUTH \
  --auth-parameters USERNAME=your-email@example.com,PASSWORD=YourNewPassword123!
```

#### Using cURL (Direct API calls)

```bash
# 1. Sign up a new user
curl -X POST https://cognito-idp.{REGION}.amazonaws.com/ \
  -H "Content-Type: application/x-amz-json-1.1" \
  -H "X-Amz-Target: AWSCognitoIdentityProviderService.SignUp" \
  -d '{
    "ClientId": "YOUR_CLIENT_ID",
    "Username": "your-email@example.com", 
    "Password": "YourPassword123!",
    "UserAttributes": [
      {"Name": "email", "Value": "your-email@example.com"}
    ]
  }'

# 2. Confirm sign up (check your email for confirmation code)
curl -X POST https://cognito-idp.{REGION}.amazonaws.com/ \
  -H "Content-Type: application/x-amz-json-1.1" \
  -H "X-Amz-Target: AWSCognitoIdentityProviderService.ConfirmSignUp" \
  -d '{
    "ClientId": "YOUR_CLIENT_ID",
    "Username": "your-email@example.com",
    "ConfirmationCode": "123456"
  }'

# 3. Sign in and get tokens
curl -X POST https://cognito-idp.{REGION}.amazonaws.com/ \
  -H "Content-Type: application/x-amz-json-1.1" \
  -H "X-Amz-Target: AWSCognitoIdentityProviderService.InitiateAuth" \
  -d '{
    "ClientId": "YOUR_CLIENT_ID",
    "AuthFlow": "USER_PASSWORD_AUTH",
    "AuthParameters": {
      "USERNAME": "your-email@example.com",
      "PASSWORD": "YourPassword123!"
    }
  }'
```

## Token Usage

### Authorization Header Format
```
Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...
```

### Token Types
- **ID Token**: Use this for API authorization (what you need)
- **Access Token**: For accessing other AWS resources  
- **Refresh Token**: To get new tokens when they expire

### Token Expiration
- **ID Token**: 24 hours
- **Access Token**: 24 hours  
- **Refresh Token**: 30 days

## Example API Calls

### Test Authentication
```bash
# Health check (no auth required)
curl https://YOUR_API_GATEWAY_URL/health

# Protected endpoint (auth required)
curl -H "Authorization: Bearer YOUR_ID_TOKEN" \
     https://YOUR_API_GATEWAY_URL/your-protected-endpoint
```

### Common HTTP Status Codes
- **200**: Success
- **401**: Unauthorized (missing or invalid token)
- **403**: Forbidden (valid token but insufficient permissions)

## Environment Variables for Your Application

When integrating authentication into your application, you'll need these values:

```bash
# From CloudFormation outputs
COGNITO_USER_POOL_ID=us-east-1_xxxxxxxxx
COGNITO_USER_POOL_CLIENT_ID=xxxxxxxxxxxxxxxxxxxxxxxxxx  
COGNITO_REGION=us-east-1
COGNITO_HOSTED_UI_URL=https://code-refactor-123456789012.auth.us-east-1.amazoncognito.com
API_GATEWAY_URL=https://xxxxxxxxxx.execute-api.us-east-1.amazonaws.com/prod
```

## Troubleshooting

### Common Issues

1. **401 Unauthorized**
   - Check if token is included in Authorization header
   - Verify token hasn't expired
   - Ensure you're using the ID token, not access token

2. **Token Expired**
   - Use refresh token to get new tokens
   - Re-authenticate if refresh token is also expired

3. **Invalid Token Format**
   - Ensure Bearer prefix: `Authorization: Bearer TOKEN`
   - Check for extra spaces or newlines in token

### Getting Help
- Check CloudWatch logs for detailed error messages
- Use AWS CLI `decode-authorization-message` for detailed error info
- Verify Cognito User Pool configuration in AWS Console

## Security Best Practices

1. **Never expose tokens in logs or URLs**
2. **Store tokens securely** (use secure storage mechanisms)
3. **Implement token refresh logic** in your applications
4. **Use HTTPS only** for all API calls
5. **Implement proper session management** in web applications

## Integration with OpenAPI/Swagger

Add this security definition to your OpenAPI spec:

```yaml
components:
  securitySchemes:
    CognitoAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT
      description: JWT token from Amazon Cognito User Pool

security:
  - CognitoAuth: []
```

For individual endpoints that don't require auth:
```yaml
paths:
  /health:
    get:
      security: []  # Override global security
```

---

**Note**: This authentication system is designed for development and testing. For production use, consider implementing additional security measures like WAF, rate limiting, and monitoring.
