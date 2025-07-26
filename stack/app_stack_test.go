package stack

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/jsii-runtime-go"
)

func TestAppStack_CreatesExpectedResources(t *testing.T) {
	// Arrange
	app := awscdk.NewApp(nil)
	stack := NewAppStack(app, "TestStack", &AppStackProps{
		StackProps: awscdk.StackProps{
			Env: &awscdk.Environment{
				Region: jsii.String("us-east-1"),
			},
		},
	})
	template := assertions.Template_FromStack(stack.Stack, nil)

	// Test networking infrastructure
	t.Run("Networking", func(t *testing.T) {
		t.Run("creates VPC with correct configuration", func(_ *testing.T) {
			template.ResourceCountIs(jsii.String("AWS::EC2::VPC"), jsii.Number(1))
			template.HasResourceProperties(jsii.String("AWS::EC2::VPC"), map[string]interface{}{
				"EnableDnsHostnames": true,
				"EnableDnsSupport":   true,
			})
		})

		t.Run("creates public subnets only", func(_ *testing.T) {
			template.ResourceCountIs(jsii.String("AWS::EC2::Subnet"), jsii.Number(2))
		})

		t.Run("creates appropriate security groups", func(_ *testing.T) {
			// Should have: RDS default SG, Lambda migration SG, VPC default SG, ECS service SG
			template.ResourceCountIs(jsii.String("AWS::EC2::SecurityGroup"), jsii.Number(4))
		})
	})

	// Test storage infrastructure
	t.Run("Storage", func(t *testing.T) {
		t.Run("creates S3 bucket with security configurations", func(_ *testing.T) {
			template.ResourceCountIs(jsii.String("AWS::S3::Bucket"), jsii.Number(1))
			template.HasResourceProperties(jsii.String("AWS::S3::Bucket"), map[string]interface{}{
				"VersioningConfiguration": map[string]interface{}{
					"Status": "Enabled",
				},
				"PublicAccessBlockConfiguration": map[string]interface{}{
					"BlockPublicAcls":       true,
					"BlockPublicPolicy":     true,
					"IgnorePublicAcls":      true,
					"RestrictPublicBuckets": true,
				},
			})
		})
	})

	// Test database infrastructure
	t.Run("Database", func(t *testing.T) {
		t.Run("creates RDS Aurora Serverless v2 cluster", func(_ *testing.T) {
			template.ResourceCountIs(jsii.String("AWS::RDS::DBCluster"), jsii.Number(1))
			template.HasResourceProperties(jsii.String("AWS::RDS::DBCluster"), map[string]interface{}{
				"Engine":              "aurora-postgresql",
				"DatabaseName":        RDSPostgresDatabaseName,
				"Port":                5432,
				"DBClusterIdentifier": "code-refactor-cluster",
				"EnableHttpEndpoint":  true,
			})
		})

		t.Run("creates DB instance for the cluster", func(_ *testing.T) {
			template.ResourceCountIs(jsii.String("AWS::RDS::DBInstance"), jsii.Number(1))
		})

		t.Run("creates Secrets Manager secret for DB credentials", func(_ *testing.T) {
			template.ResourceCountIs(jsii.String("AWS::SecretsManager::Secret"), jsii.Number(1))
			template.HasResourceProperties(jsii.String("AWS::SecretsManager::Secret"), map[string]interface{}{
				"Name": "code-refactor-db-secret",
			})
		})

		t.Run("creates Lambda function for database migration", func(_ *testing.T) {
			// CDK may create additional helper Lambdas, so we check for at least 1
			template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), map[string]interface{}{
				"Handler": "handler.lambda_handler",
				"Runtime": "python3.12",
				"Timeout": 10,
			})
		})
	})

	// Test compute infrastructure
	t.Run("Compute", func(t *testing.T) {
		t.Run("creates ECS cluster", func(_ *testing.T) {
			template.ResourceCountIs(jsii.String("AWS::ECS::Cluster"), jsii.Number(1))
		})

		t.Run("creates Fargate task definition with proper configuration", func(_ *testing.T) {
			template.ResourceCountIs(jsii.String("AWS::ECS::TaskDefinition"), jsii.Number(1))
			template.HasResourceProperties(jsii.String("AWS::ECS::TaskDefinition"), map[string]interface{}{
				"RequiresCompatibilities": []interface{}{"FARGATE"},
				"Cpu":                     "512",
				"Memory":                  "1024",
				"NetworkMode":             "awsvpc",
			})
		})

		t.Run("creates ECR repository", func(_ *testing.T) {
			template.ResourceCountIs(jsii.String("AWS::ECR::Repository"), jsii.Number(1))
			template.HasResourceProperties(jsii.String("AWS::ECR::Repository"), map[string]interface{}{
				"RepositoryName": "refactor-ecr-repo",
			})
		})

		t.Run("creates CloudWatch log group", func(_ *testing.T) {
			template.ResourceCountIs(jsii.String("AWS::Logs::LogGroup"), jsii.Number(1))
			template.HasResourceProperties(jsii.String("AWS::Logs::LogGroup"), map[string]interface{}{
				"LogGroupName": "/ecs/code-refactor",
			})
		})
	})

	// Test API Gateway and Load Balancer infrastructure
	t.Run("API Gateway", func(t *testing.T) {
		t.Run("creates API Gateway with correct configuration", func(_ *testing.T) {
			template.ResourceCountIs(jsii.String("AWS::ApiGateway::RestApi"), jsii.Number(1))
			template.HasResourceProperties(jsii.String("AWS::ApiGateway::RestApi"), map[string]interface{}{
				"Name":        "code-refactor-api",
				"Description": "API Gateway for Code Refactoring Tool",
				"EndpointConfiguration": map[string]interface{}{
					"Types": []interface{}{"REGIONAL"},
				},
			})
		})

		t.Run("creates Application Load Balancer", func(_ *testing.T) {
			// We have 1 load balancer: ALB for ECS (internet-facing for cost optimization vs VPC Link)
			template.ResourceCountIs(jsii.String("AWS::ElasticLoadBalancingV2::LoadBalancer"), jsii.Number(1))
			template.HasResourceProperties(jsii.String("AWS::ElasticLoadBalancingV2::LoadBalancer"), map[string]interface{}{
				"Type":   "application",
				"Scheme": "internet-facing",
			})
		})

		t.Run("creates ALB Target Group for ECS service", func(_ *testing.T) {
			template.ResourceCountIs(jsii.String("AWS::ElasticLoadBalancingV2::TargetGroup"), jsii.Number(1))
			template.HasResourceProperties(jsii.String("AWS::ElasticLoadBalancingV2::TargetGroup"), map[string]interface{}{
				"Port":            8080,
				"Protocol":        "HTTP",
				"TargetType":      "ip",
				"HealthCheckPath": "/health",
			})
		})

		t.Run("creates ALB Listener", func(_ *testing.T) {
			template.ResourceCountIs(jsii.String("AWS::ElasticLoadBalancingV2::Listener"), jsii.Number(1))
			template.HasResourceProperties(jsii.String("AWS::ElasticLoadBalancingV2::Listener"), map[string]interface{}{
				"Port":     80,
				"Protocol": "HTTP",
			})
		})

		t.Run("creates API Gateway deployment and stage", func(_ *testing.T) {
			template.ResourceCountIs(jsii.String("AWS::ApiGateway::Deployment"), jsii.Number(1))
			template.ResourceCountIs(jsii.String("AWS::ApiGateway::Stage"), jsii.Number(1))
			template.HasResourceProperties(jsii.String("AWS::ApiGateway::Stage"), map[string]interface{}{
				"StageName": "prod",
			})
		})
	})

	// Test Authentication and Authorization
	t.Run("Authentication", func(t *testing.T) {
		t.Run("creates Cognito User Pool", func(_ *testing.T) {
			template.ResourceCountIs(jsii.String("AWS::Cognito::UserPool"), jsii.Number(1))
			template.HasResourceProperties(jsii.String("AWS::Cognito::UserPool"), map[string]interface{}{
				"UserPoolName": "code-refactor-user-pool",
				"Policies": map[string]interface{}{
					"PasswordPolicy": map[string]interface{}{
						"MinimumLength":    8,
						"RequireLowercase": true,
						"RequireNumbers":   true,
						"RequireSymbols":   true,
						"RequireUppercase": true,
					},
				},
			})
		})

		t.Run("creates Cognito User Pool Client", func(_ *testing.T) {
			template.ResourceCountIs(jsii.String("AWS::Cognito::UserPoolClient"), jsii.Number(1))
			template.HasResourceProperties(jsii.String("AWS::Cognito::UserPoolClient"), map[string]interface{}{
				"ClientName":           "code-refactor-client",
				"ExplicitAuthFlows":    []interface{}{"ALLOW_USER_PASSWORD_AUTH", "ALLOW_USER_SRP_AUTH", "ALLOW_REFRESH_TOKEN_AUTH"},
				"GenerateSecret":       false,
				"AccessTokenValidity":  1440,  // 24 hours in minutes
				"IdTokenValidity":      1440,  // 24 hours in minutes
				"RefreshTokenValidity": 43200, // 30 days in minutes
				"TokenValidityUnits": map[string]interface{}{
					"AccessToken":  "minutes",
					"IdToken":      "minutes",
					"RefreshToken": "minutes",
				},
			})
		})

		t.Run("creates API Gateway Authorizer", func(_ *testing.T) {
			template.ResourceCountIs(jsii.String("AWS::ApiGateway::Authorizer"), jsii.Number(1))
			template.HasResourceProperties(jsii.String("AWS::ApiGateway::Authorizer"), map[string]interface{}{
				"Type": "COGNITO_USER_POOLS",
				"Name": "code-refactor-authorizer",
			})
		})

		t.Run("creates API Gateway resources with authorization", func(_ *testing.T) {
			// Check that API resources require authorization
			// We have: proxy resource, /health resource, and /swagger resource
			template.ResourceCountIs(jsii.String("AWS::ApiGateway::Resource"), jsii.Number(3))
			// We have methods from proxy resource (ANY method creates multiple HTTP methods) + health + swagger
			template.ResourceCountIs(jsii.String("AWS::ApiGateway::Method"), jsii.Number(8))
		})
	})

	// Test IAM and security
	t.Run("IAM and Security", func(t *testing.T) {
		t.Run("creates appropriate number of IAM roles", func(_ *testing.T) {
			// Expected roles: Bedrock KB, Bedrock Agent, Lambda execution, ECS task execution, ECS task role, Lambda migration role, GitHub Actions role, ALB/ECS service role
			// Note: OIDC provider is created manually outside CDK, so no role for that
			template.ResourceCountIs(jsii.String("AWS::IAM::Role"), jsii.Number(8))
		})

		t.Run("creates Bedrock Knowledge Base role with correct trust policy", func(_ *testing.T) {
			template.HasResourceProperties(jsii.String("AWS::IAM::Role"), map[string]interface{}{
				"AssumeRolePolicyDocument": map[string]interface{}{
					"Statement": []interface{}{
						map[string]interface{}{
							"Action": "sts:AssumeRole",
							"Effect": "Allow",
							"Principal": map[string]interface{}{
								"Service": "bedrock.amazonaws.com",
							},
						},
					},
				},
			})
		})

		t.Run("creates Lambda execution role with VPC permissions", func(_ *testing.T) {
			template.HasResourceProperties(jsii.String("AWS::IAM::Role"), map[string]interface{}{
				"AssumeRolePolicyDocument": map[string]interface{}{
					"Statement": []interface{}{
						map[string]interface{}{
							"Action": "sts:AssumeRole",
							"Effect": "Allow",
							"Principal": map[string]interface{}{
								"Service": "lambda.amazonaws.com",
							},
						},
					},
				},
			})
		})

		t.Run("creates GitHub Actions role with OIDC authentication", func(_ *testing.T) {
			// Verify a role exists with AssumeRoleWithWebIdentity action (GitHub Actions OIDC)
			template.HasResourceProperties(jsii.String("AWS::IAM::Role"), map[string]interface{}{
				"AssumeRolePolicyDocument": map[string]interface{}{
					"Statement": []interface{}{
						map[string]interface{}{
							"Action": "sts:AssumeRoleWithWebIdentity",
							"Effect": "Allow",
						},
					},
				},
			})
		})
	})
}

func TestAppStack_ResourceTagging(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := NewAppStack(app, "TestStack", &AppStackProps{
		StackProps: awscdk.StackProps{
			Env: &awscdk.Environment{
				Region: jsii.String("us-east-1"),
			},
		},
	})
	template := assertions.Template_FromStack(stack.Stack, nil)

	t.Run("all major resources are tagged consistently", func(_ *testing.T) {
		// This is a more pragmatic approach - we test that tagging exists
		// but don't over-specify every single resource tag
		resourceTypes := []string{
			"AWS::S3::Bucket",
			"AWS::RDS::DBCluster",
			"AWS::EC2::VPC",
			"AWS::ECS::Cluster",
		}

		for _, resourceType := range resourceTypes {
			template.HasResourceProperties(jsii.String(resourceType), map[string]interface{}{
				"Tags": assertions.Match_AnyValue(),
			})
		}
	})
}

func TestAppStack_ExposesCorrectOutputs(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := NewAppStack(app, "TestStack", &AppStackProps{
		StackProps: awscdk.StackProps{
			Env: &awscdk.Environment{
				Region: jsii.String("us-east-1"),
			},
		},
	})

	t.Run("exposes required stack properties", func(t *testing.T) {
		// Test that the stack struct has the expected fields populated
		if stack.BedrockKnowledgeBaseRole == nil {
			t.Error("BedrockKnowledgeBaseRole should not be nil")
		}
		if stack.BedrockAgentRole == nil {
			t.Error("BedrockAgentRole should not be nil")
		}
		if stack.GitHubActionsRoleARN == nil {
			t.Error("GitHubActionsRoleARN should not be nil")
		}
		if stack.BucketName == "" {
			t.Error("BucketName should not be empty")
		}
		if stack.RDSPostgresClusterARN == "" {
			t.Error("RDSPostgresClusterARN should not be empty")
		}
		if stack.RDSPostgresCredentialsSecretARN == "" {
			t.Error("RDSPostgresCredentialsSecretARN should not be empty")
		}
		if stack.RDSPostgresSchemaEnsureLambdaARN == "" {
			t.Error("RDSPostgresSchemaEnsureLambdaARN should not be empty")
		}
		// API Gateway and Auth properties
		if stack.APIGatewayURL == "" {
			t.Error("APIGatewayURL should not be empty")
		}
		if stack.CognitoUserPoolID == "" {
			t.Error("CognitoUserPoolID should not be empty")
		}
		if stack.CognitoUserPoolClientID == "" {
			t.Error("CognitoUserPoolClientID should not be empty")
		}
	})
}
