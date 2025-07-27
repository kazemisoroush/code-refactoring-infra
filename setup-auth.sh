#!/bin/bash
# setup-auth.sh - Complete authentication setup script for Code Refactoring API

set -e

echo "🚀 Setting up Code Refactoring API Authentication"

# Check prerequisites
if ! command -v aws &> /dev/null; then
    echo "❌ AWS CLI not found. Please install AWS CLI first."
    exit 1
fi

if ! command -v jq &> /dev/null; then
    echo "❌ jq not found. Please install jq first."
    exit 1
fi

# Get stack outputs
echo "📋 Getting CloudFormation stack outputs..."
STACK_OUTPUTS=$(aws cloudformation describe-stacks --stack-name CodeRefactorInfra --query 'Stacks[0].Outputs')

if [ $? -ne 0 ]; then
    echo "❌ Failed to get stack outputs. Make sure the CodeRefactorInfra stack is deployed."
    exit 1
fi

USER_POOL_ID=$(echo $STACK_OUTPUTS | jq -r '.[] | select(.OutputKey=="CognitoUserPoolID") | .OutputValue')
CLIENT_ID=$(echo $STACK_OUTPUTS | jq -r '.[] | select(.OutputKey=="CognitoUserPoolClientID") | .OutputValue')
API_URL=$(echo $STACK_OUTPUTS | jq -r '.[] | select(.OutputKey=="APIGatewayURL") | .OutputValue')

echo "✅ Configuration loaded:"
echo "   User Pool ID: $USER_POOL_ID"
echo "   Client ID: $CLIENT_ID"
echo "   API URL: $API_URL"

# Set default credentials (you can change these)
DEFAULT_EMAIL="admin@code-refactor.dev"
DEFAULT_PASSWORD="CodeRefactor123!"

echo ""
echo "👤 Creating default user with credentials:"
echo "   Email: $DEFAULT_EMAIL"
echo "   Password: $DEFAULT_PASSWORD"
echo ""

# Create user
echo "🔨 Creating user..."
if aws cognito-idp admin-create-user \
  --user-pool-id $USER_POOL_ID \
  --username $DEFAULT_EMAIL \
  --temporary-password $DEFAULT_PASSWORD \
  --message-action SUPPRESS \
  --user-attributes Name=email,Value=$DEFAULT_EMAIL Name=email_verified,Value=true >/dev/null 2>&1; then
  echo "✅ User created successfully"
else
  echo "⚠️  User might already exist, continuing..."
fi

# Set permanent password
echo "🔒 Setting permanent password..."
aws cognito-idp admin-set-user-password \
  --user-pool-id $USER_POOL_ID \
  --username $DEFAULT_EMAIL \
  --password $DEFAULT_PASSWORD \
  --permanent >/dev/null 2>&1

echo "✅ Password set to permanent"

# Get authentication token
echo "🎫 Getting authentication token..."
TOKEN_RESPONSE=$(aws cognito-idp admin-initiate-auth \
  --user-pool-id $USER_POOL_ID \
  --client-id $CLIENT_ID \
  --auth-flow ADMIN_NO_SRP_AUTH \
  --auth-parameters USERNAME=$DEFAULT_EMAIL,PASSWORD=$DEFAULT_PASSWORD)

ID_TOKEN=$(echo $TOKEN_RESPONSE | jq -r '.AuthenticationResult.IdToken')

# Test API
echo "🧪 Testing API endpoints..."

echo "   Health check (public):"
HEALTH_RESPONSE=$(curl -s $API_URL/health 2>/dev/null || echo "Endpoint not available")
echo "   Response: $HEALTH_RESPONSE"

echo "   Authentication test:"
AUTH_TEST=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $ID_TOKEN" $API_URL/health 2>/dev/null || echo "000")
echo "   HTTP Status with auth: $AUTH_TEST"

echo ""
echo "🎉 Setup complete!"
echo ""
echo "📝 Your default user credentials:"
echo "   Email: $DEFAULT_EMAIL"
echo "   Password: $DEFAULT_PASSWORD"
echo ""
echo "🔑 Your current ID Token:"
echo "$ID_TOKEN"
echo ""
echo "💡 To use in API calls:"
echo "   curl -H \"Authorization: Bearer $ID_TOKEN\" $API_URL/your-endpoint"
echo ""
echo "🔄 To get a new token later:"
echo "   aws cognito-idp admin-initiate-auth \\"
echo "     --user-pool-id $USER_POOL_ID \\"
echo "     --client-id $CLIENT_ID \\"
echo "     --auth-flow ADMIN_NO_SRP_AUTH \\"
echo "     --auth-parameters USERNAME=$DEFAULT_EMAIL,PASSWORD=$DEFAULT_PASSWORD"
echo ""
echo "📄 For more details, see AUTHENTICATION.md"
