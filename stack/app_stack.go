// Package stack provides the CDK stack for the application infrastructure.
package stack

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigateway"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfront"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfrontorigins"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscognito"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecr"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awselasticloadbalancingv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsrds"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3assets"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsssm"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	// NEW IMPORT for Custom Resources
)

// AppStackProps defines the properties for the application stack.
type AppStackProps struct {
	awscdk.StackProps
}

// AppStack is the main CDK stack for the application, containing all resources.
type AppStack struct {
	awscdk.Stack
	BedrockKnowledgeBaseRole         *string
	BedrockAgentRole                 *string
	GitHubActionsRoleARN             *string
	BucketName                       string
	Region                           string
	Account                          string
	RDSPostgresClusterARN            string
	RDSPostgresCredentialsSecretARN  string
	RDSPostgresSchemaEnsureLambdaARN string
	APIGatewayURL                    string
	CognitoUserPoolID                string
	CognitoUserPoolClientID          string
	CognitoHostedUIURL               string
	// Frontend resources
	FrontendBucketName               string
	CloudFrontDistributionID         string
	CloudFrontDistributionDomainName string
}

// Resources holds the common resources that are shared across different components
type Resources struct {
	Stack   awscdk.Stack
	Vpc     awsec2.IVpc
	Account string
	Region  string
}

// NetworkingResources holds VPC and related networking components
type NetworkingResources struct {
	Vpc                    awsec2.IVpc
	SecretsManagerEndpoint awsec2.IInterfaceVpcEndpoint
}

// DatabaseResources holds RDS and related database components
type DatabaseResources struct {
	Cluster             awsrds.IDatabaseCluster
	CredentialsSecret   awssecretsmanager.ISecret
	MigrationLambda     awslambda.IFunction
	MigrationLambdaRole awsiam.Role
	MigrationLambdaSG   awsec2.ISecurityGroup
}

// BedrockResources holds Bedrock-related IAM roles and configurations
type BedrockResources struct {
	KnowledgeBaseRole awsiam.IRole
	AgentRole         awsiam.IRole
}

// ComputeResources holds ECS and Fargate resources
type ComputeResources struct {
	Cluster  awsecs.ICluster
	TaskDef  awsecs.IFargateTaskDefinition
	Service  awsecs.IFargateService
	EcrRepo  awsecr.IRepository
	LogGroup awslogs.ILogGroup
}

// StorageResources holds S3 and other storage resources
type StorageResources struct {
	Bucket awss3.IBucket
	Name   string
}

// APIGatewayResources holds API Gateway and related resources
type APIGatewayResources struct {
	RestAPI      awsapigateway.IRestApi
	LoadBalancer awselasticloadbalancingv2.IApplicationLoadBalancer
	URL          string
}

// CognitoResources holds Cognito User Pool and related authentication resources
type CognitoResources struct {
	UserPool       awscognito.IUserPool
	UserPoolClient awscognito.IUserPoolClient
	UserPoolDomain awscognito.IUserPoolDomain
	UserPoolID     string
	ClientID       string
	DomainURL      string
}

// FrontendResources holds S3 bucket and CloudFront distribution for frontend hosting
type FrontendResources struct {
	Bucket                 awss3.IBucket
	BucketName             string
	CloudFrontDistribution awscloudfront.IDistribution
	DistributionID         string
	DistributionDomainName string
}

// NewAppStack creates a new CDK stack for the application.
func NewAppStack(scope constructs.Construct, id string, props *AppStackProps) *AppStack {
	stack := awscdk.NewStack(scope, &id, &props.StackProps)

	resources := &Resources{
		Stack:   stack,
		Account: *stack.Account(),
		Region:  *stack.Region(),
	}

	// Create resources in logical order
	networking := createNetworkingResources(resources)
	resources.Vpc = networking.Vpc

	storage := createStorageResources(resources)
	database := createDatabaseResources(resources, networking)

	// Create authentication resources first
	cognito := createCognitoResources(resources)

	// Create Bedrock resources before compute resources so they're available for environment variables
	bedrock := createBedrockResources(resources, storage, database)

	// Create compute resources (ECS, Fargate, ECR) - now has access to all required resources
	compute := createComputeResources(resources, networking, database, storage, cognito, bedrock)

	// Create API Gateway resources
	apigateway := createAPIGatewayResources(resources, networking, compute, cognito, database)

	// Create frontend resources (S3 + CloudFront)
	frontend := createFrontendResources(resources)

	// Create GitHub Actions IAM role for ECR and S3 access
	// Note: OIDC provider is created manually and exists in the account
	githubRole := createGitHubActionsRole(resources, frontend)

	// Store configuration in Parameter Store and Secrets Manager
	createConfigurationStores(resources, storage, database, bedrock, cognito, apigateway, frontend, compute)

	// Create CloudFormation outputs
	awscdk.NewCfnOutput(resources.Stack, jsii.String("ECRRepositoryURI"), &awscdk.CfnOutputProps{
		Value:       compute.EcrRepo.RepositoryUri(),
		Description: jsii.String("ECR Repository URI for the application container image"),
		ExportName:  jsii.String("CodeRefactor-ECR-Repository-URI"),
	})

	awscdk.NewCfnOutput(resources.Stack, jsii.String("CognitoUserPoolID"), &awscdk.CfnOutputProps{
		Value:       jsii.String(cognito.UserPoolID),
		Description: jsii.String("Cognito User Pool ID"),
		ExportName:  jsii.String("CodeRefactor-Cognito-UserPool-ID"),
	})

	awscdk.NewCfnOutput(resources.Stack, jsii.String("CognitoUserPoolClientID"), &awscdk.CfnOutputProps{
		Value:       jsii.String(cognito.ClientID),
		Description: jsii.String("Cognito User Pool Client ID"),
		ExportName:  jsii.String("CodeRefactor-Cognito-Client-ID"),
	})

	// Add missing outputs expected by CI workflow
	awscdk.NewCfnOutput(resources.Stack, jsii.String("APIGatewayURL"), &awscdk.CfnOutputProps{
		Value:       jsii.String(apigateway.URL),
		Description: jsii.String("API Gateway URL"),
		ExportName:  jsii.String("CodeRefactor-API-Gateway-URL"),
	})

	awscdk.NewCfnOutput(resources.Stack, jsii.String("CognitoHostedUIURL"), &awscdk.CfnOutputProps{
		Value:       jsii.String(cognito.DomainURL),
		Description: jsii.String("Cognito Hosted UI URL"),
		ExportName:  jsii.String("CodeRefactor-Cognito-HostedUI-URL"),
	})

	awscdk.NewCfnOutput(resources.Stack, jsii.String("RDSPostgresCredentialsSecretARN"), &awscdk.CfnOutputProps{
		Value:       database.CredentialsSecret.SecretArn(),
		Description: jsii.String("RDS Postgres Credentials Secret ARN"),
		ExportName:  jsii.String("CodeRefactor-RDS-Credentials-Secret-ARN"),
	})

	awscdk.NewCfnOutput(resources.Stack, jsii.String("RDSPostgresInstanceARN"), &awscdk.CfnOutputProps{
		Value:       database.Cluster.ClusterArn(),
		Description: jsii.String("RDS Postgres Cluster ARN"),
		ExportName:  jsii.String("CodeRefactor-RDS-Cluster-ARN"),
	})

	awscdk.NewCfnOutput(resources.Stack, jsii.String("BucketName"), &awscdk.CfnOutputProps{
		Value:       jsii.String(storage.Name),
		Description: jsii.String("S3 Bucket Name for Bedrock Knowledge Base"),
		ExportName:  jsii.String("CodeRefactor-S3-Bucket-Name"),
	})

	awscdk.NewCfnOutput(resources.Stack, jsii.String("BedrockKnowledgeBaseRoleArn"), &awscdk.CfnOutputProps{
		Value:       bedrock.KnowledgeBaseRole.RoleArn(),
		Description: jsii.String("Bedrock Knowledge Base Service Role ARN"),
		ExportName:  jsii.String("CodeRefactor-Bedrock-KnowledgeBase-Role-ARN"),
	})

	awscdk.NewCfnOutput(resources.Stack, jsii.String("BedrockAgentRoleArn"), &awscdk.CfnOutputProps{
		Value:       bedrock.AgentRole.RoleArn(),
		Description: jsii.String("Bedrock Agent Service Role ARN"),
		ExportName:  jsii.String("CodeRefactor-Bedrock-Agent-Role-ARN"),
	})

	// Frontend outputs
	awscdk.NewCfnOutput(resources.Stack, jsii.String("FrontendBucketName"), &awscdk.CfnOutputProps{
		Value:       jsii.String(frontend.BucketName),
		Description: jsii.String("S3 Bucket Name for Frontend Hosting"),
		ExportName:  jsii.String("CodeRefactor-Frontend-Bucket-Name"),
	})

	awscdk.NewCfnOutput(resources.Stack, jsii.String("CloudFrontDistributionID"), &awscdk.CfnOutputProps{
		Value:       jsii.String(frontend.DistributionID),
		Description: jsii.String("CloudFront Distribution ID for Frontend"),
		ExportName:  jsii.String("CodeRefactor-CloudFront-Distribution-ID"),
	})

	awscdk.NewCfnOutput(resources.Stack, jsii.String("CloudFrontDistributionDomainName"), &awscdk.CfnOutputProps{
		Value:       jsii.String(frontend.DistributionDomainName),
		Description: jsii.String("CloudFront Distribution Domain Name for Frontend"),
		ExportName:  jsii.String("CodeRefactor-CloudFront-Domain-Name"),
	})

	return &AppStack{
		Stack:                            stack,
		BedrockKnowledgeBaseRole:         bedrock.KnowledgeBaseRole.RoleArn(),
		BedrockAgentRole:                 bedrock.AgentRole.RoleArn(),
		GitHubActionsRoleARN:             githubRole.RoleArn(),
		BucketName:                       storage.Name,
		Account:                          resources.Account,
		Region:                           resources.Region,
		RDSPostgresClusterARN:            *database.Cluster.ClusterArn(),
		RDSPostgresCredentialsSecretARN:  *database.CredentialsSecret.SecretArn(),
		RDSPostgresSchemaEnsureLambdaARN: *database.MigrationLambda.FunctionArn(),
		APIGatewayURL:                    apigateway.URL,
		CognitoUserPoolID:                cognito.UserPoolID,
		CognitoUserPoolClientID:          cognito.ClientID,
		CognitoHostedUIURL:               fmt.Sprintf("https://%s.auth.%s.amazoncognito.com", cognito.DomainURL, resources.Region),
		// Frontend resources
		FrontendBucketName:               frontend.BucketName,
		CloudFrontDistributionID:         frontend.DistributionID,
		CloudFrontDistributionDomainName: frontend.DistributionDomainName,
	}
}

// createNetworkingResources creates VPC and related networking components
func createNetworkingResources(resources *Resources) *NetworkingResources {
	// VPC for RDS and Fargate
	vpc := awsec2.NewVpc(resources.Stack, jsii.String("RefactorVpc"), &awsec2.VpcProps{
		MaxAzs:      jsii.Number(2),
		NatGateways: jsii.Number(0),
		SubnetConfiguration: &[]*awsec2.SubnetConfiguration{
			{
				CidrMask:   jsii.Number(24),
				Name:       jsii.String("Public"),
				SubnetType: awsec2.SubnetType_PUBLIC,
			},
		},
	})
	awscdk.Tags_Of(vpc).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Apply removal policy to VPC for clean deletion
	vpc.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)

	return &NetworkingResources{
		Vpc:                    vpc,
		SecretsManagerEndpoint: nil, // Removed VPC endpoint to avoid deletion issues
	}
}

// createStorageResources creates S3 bucket and related storage components
func createStorageResources(resources *Resources) *StorageResources {
	bucketName := fmt.Sprintf("code-refactor-bucket-%s-%s", resources.Account, resources.Region)
	bucket := awss3.NewBucket(resources.Stack, jsii.String("CodeRefactorBucket"), &awss3.BucketProps{
		BucketName:        jsii.String(bucketName),
		RemovalPolicy:     awscdk.RemovalPolicy_DESTROY,
		AutoDeleteObjects: jsii.Bool(true),
		Versioned:         jsii.Bool(true),
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
	})
	awscdk.Tags_Of(bucket).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	return &StorageResources{
		Bucket: bucket,
		Name:   bucketName,
	}
}

// createDatabaseResources creates RDS cluster, secrets, and migration lambda
func createDatabaseResources(resources *Resources, networking *NetworkingResources) *DatabaseResources {
	// Secrets Manager Secret
	credentialsSecret := awssecretsmanager.NewSecret(resources.Stack, jsii.String("CodeRefactorDbSecret"), &awssecretsmanager.SecretProps{
		SecretName: jsii.String("code-refactor-db-secret"),
		GenerateSecretString: &awssecretsmanager.SecretStringGenerator{
			SecretStringTemplate: jsii.String("{\"username\": \"postgres\"}"),
			GenerateStringKey:    jsii.String("password"),
			ExcludeCharacters:    jsii.String("\"@/\\"),
		},
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
	})
	awscdk.Tags_Of(credentialsSecret).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// RDS Postgres Serverless v2
	cluster := awsrds.NewDatabaseCluster(resources.Stack, jsii.String(RDSPostgresDatabaseName), &awsrds.DatabaseClusterProps{
		Engine: awsrds.DatabaseClusterEngine_AuroraPostgres(&awsrds.AuroraPostgresClusterEngineProps{
			Version: awsrds.AuroraPostgresEngineVersion_VER_15_12(), // Updated to latest available version to exceed AWS recommendation
		}),
		Writer: awsrds.ClusterInstance_ServerlessV2(jsii.String("writer"), &awsrds.ServerlessV2ClusterInstanceProps{
			AutoMinorVersionUpgrade: jsii.Bool(true),
		}),
		Vpc: networking.Vpc,
		VpcSubnets: &awsec2.SubnetSelection{
			SubnetType: awsec2.SubnetType_PUBLIC,
		},
		DefaultDatabaseName: jsii.String(RDSPostgresDatabaseName),
		Port:                jsii.Number(5432),
		Credentials:         awsrds.Credentials_FromSecret(credentialsSecret, jsii.String("postgres")),
		RemovalPolicy:       awscdk.RemovalPolicy_DESTROY,
		ClusterIdentifier:   jsii.String("code-refactor-cluster"),
		// Enable Data API v2 for Bedrock Knowledge Base integration
		EnableDataApi: jsii.Bool(true),
		// Configure Serverless v2 scaling
		ServerlessV2MinCapacity: jsii.Number(0.5),
		ServerlessV2MaxCapacity: jsii.Number(4.0),
	})
	awscdk.Tags_Of(cluster).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Create migration lambda and related resources
	migrationResources := createMigrationLambda(resources, networking, cluster, credentialsSecret)

	// print host and port
	fmt.Printf("RDS Postgres Cluster Endpoint: %s:%.0f\n", *cluster.ClusterEndpoint().Hostname(), *cluster.ClusterEndpoint().Port())
	fmt.Printf("RDS Postgres Credentials Secret ARN: %s\n", *credentialsSecret.SecretArn())
	fmt.Printf("RDS Postgres Migration Lambda ARN: %s\n", *migrationResources.MigrationLambda.FunctionArn())

	return &DatabaseResources{
		Cluster:             cluster,
		CredentialsSecret:   credentialsSecret,
		MigrationLambda:     migrationResources.MigrationLambda,
		MigrationLambdaRole: migrationResources.MigrationLambdaRole,
		MigrationLambdaSG:   migrationResources.MigrationLambdaSG,
	}
}

// MigrationLambdaResources holds resources specific to database migration
type MigrationLambdaResources struct {
	MigrationLambda     awslambda.IFunction
	MigrationLambdaRole awsiam.Role
	MigrationLambdaSG   awsec2.ISecurityGroup
}

// createMigrationLambda creates the database migration lambda and related resources
func createMigrationLambda(resources *Resources, networking *NetworkingResources, cluster awsrds.IDatabaseCluster, credentialsSecret awssecretsmanager.ISecret) *MigrationLambdaResources {
	// Security Group for the Migration Lambda
	migrationLambdaSG := awsec2.NewSecurityGroup(resources.Stack, jsii.String("DbMigrationLambdaSG"), &awsec2.SecurityGroupProps{
		Vpc:              networking.Vpc,
		Description:      jsii.String("Allow outbound connection to RDS Postgres for DB migrations"),
		AllowAllOutbound: jsii.Bool(true),
	})
	awscdk.Tags_Of(migrationLambdaSG).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Add inbound rule to RDS Security Group to allow connections from the Lambda SG
	cluster.Connections().AllowFrom(migrationLambdaSG, awsec2.Port_Tcp(jsii.Number(5432)), jsii.String("Allow DB migration lambda"))

	// IAM Role for the Migration Lambda
	migrationLambdaRole := awsiam.NewRole(resources.Stack, jsii.String("DbMigrationLambdaRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
	})
	awscdk.Tags_Of(migrationLambdaRole).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Apply removal policy to IAM role for clean deletion
	migrationLambdaRole.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)

	// Grant permissions
	setupMigrationLambdaPermissions(migrationLambdaRole, credentialsSecret, cluster)

	lambdaPath := filepath.Join(getThisFileDir(), "../rds_schema_lambda")

	// Lambda Function for Schema Migration
	migrationLambda := awslambda.NewFunction(resources.Stack, jsii.String("DbMigrationLambda"), &awslambda.FunctionProps{
		Handler: jsii.String("handler.lambda_handler"),
		Runtime: awslambda.Runtime_PYTHON_3_12(),
		Code: awslambda.AssetCode_FromAsset(jsii.String(lambdaPath), &awss3assets.AssetOptions{
			Bundling: &awscdk.BundlingOptions{
				Image: awslambda.Runtime_PYTHON_3_12().BundlingImage(),
				Command: jsii.Strings(
					"bash", "-c",
					"pip install -r requirements.txt -t /asset-output && cp -au . /asset-output",
				),
				User: jsii.String("root"),
			},
		}),
		Vpc: networking.Vpc,
		VpcSubnets: &awsec2.SubnetSelection{
			SubnetType: awsec2.SubnetType_PUBLIC,
		},
		SecurityGroups: &[]awsec2.ISecurityGroup{
			migrationLambdaSG,
		},
		Environment: &map[string]*string{
			"DB_SECRET_ARN":        credentialsSecret.SecretArn(),
			"DB_NAME":              jsii.String(RDSPostgresDatabaseName),
			"DB_HOST":              cluster.ClusterEndpoint().Hostname(),
			"DB_PORT":              jsii.String("5432"),
			"EMBEDDING_DIMENSIONS": jsii.String("1536"), // Default for amazon.titan-embed-text-v1
			"AUTO_MIGRATE_SCHEMA":  jsii.String("true"), // Enable automatic schema migration
		},
		Timeout:           awscdk.Duration_Seconds(jsii.Number(10)),
		Role:              migrationLambdaRole,
		AllowPublicSubnet: jsii.Bool(true),
		// Reserved concurrency to limit ENI creation
		ReservedConcurrentExecutions: jsii.Number(1),
	})
	awscdk.Tags_Of(migrationLambda).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Apply removal policies to ensure clean deletion
	migrationLambda.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)
	migrationLambdaSG.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)

	return &MigrationLambdaResources{
		MigrationLambda:     migrationLambda,
		MigrationLambdaRole: migrationLambdaRole,
		MigrationLambdaSG:   migrationLambdaSG,
	}
}

// setupMigrationLambdaPermissions configures IAM permissions for the migration lambda
func setupMigrationLambdaPermissions(role awsiam.Role, credentialsSecret awssecretsmanager.ISecret, cluster awsrds.IDatabaseCluster) {
	// Grant the Lambda role permissions to write logs to CloudWatch
	role.AddManagedPolicy(awsiam.ManagedPolicy_FromAwsManagedPolicyName(jsii.String("service-role/AWSLambdaBasicExecutionRole")))

	// For VPC access
	role.AddManagedPolicy(awsiam.ManagedPolicy_FromAwsManagedPolicyName(jsii.String("service-role/AWSLambdaVPCAccessExecutionRole")))

	// Grant the Lambda role permissions to read the database secret
	credentialsSecret.GrantRead(role, nil)

	// Grant RDS Data API permissions
	role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions: &[]*string{
			jsii.String("rds-data:ExecuteStatement"),
			jsii.String("rds-data:BatchExecuteStatement"),
			jsii.String("rds-data:BeginTransaction"),
			jsii.String("rds-data:CommitTransaction"),
			jsii.String("rds-data:RollbackTransaction"),
			jsii.String("rds-data:ExecuteSql"),
			jsii.String("rds-data:DescribeTable"),
		},
		Resources: &[]*string{
			cluster.ClusterArn(),
		},
	}))
}

// createBedrockResources creates Bedrock-related IAM roles
func createBedrockResources(resources *Resources, storage *StorageResources, database *DatabaseResources) *BedrockResources {
	knowledgeBaseRole := createBedrockKnowledgeBaseRole(resources, storage, database)
	agentRole := createBedrockAgentRole(resources)

	return &BedrockResources{
		KnowledgeBaseRole: knowledgeBaseRole,
		AgentRole:         agentRole,
	}
}

// createBedrockKnowledgeBaseRole creates the IAM role for Bedrock Knowledge Base
func createBedrockKnowledgeBaseRole(resources *Resources, storage *StorageResources, database *DatabaseResources) awsiam.IRole {
	role := awsiam.NewRole(resources.Stack, jsii.String("BedrockKnowledgeBaseRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("bedrock.amazonaws.com"), nil),
		InlinePolicies: &map[string]awsiam.PolicyDocument{
			"BedrockKbPolicy": awsiam.NewPolicyDocument(&awsiam.PolicyDocumentProps{
				Statements: &[]awsiam.PolicyStatement{
					awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
						Actions: &[]*string{
							jsii.String("s3:GetObject"),
							jsii.String("s3:ListBucket"),
						},
						Resources: &[]*string{
							storage.Bucket.BucketArn(),
							jsii.String(fmt.Sprintf("%s/*", *storage.Bucket.BucketArn())),
						},
					}),
					awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
						Actions: &[]*string{
							jsii.String("secretsmanager:GetSecretValue"),
						},
						Resources: &[]*string{
							database.CredentialsSecret.SecretArn(),
						},
					}),
					awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
						Actions: &[]*string{
							jsii.String("rds-data:ExecuteStatement"),
							jsii.String("rds-data:BatchExecuteStatement"),
							jsii.String("rds-data:BeginTransaction"),
							jsii.String("rds-data:CommitTransaction"),
							jsii.String("rds-data:RollbackTransaction"),
							jsii.String("rds-data:ExecuteSql"),
							jsii.String("rds-data:DescribeTable"),
						},
						Resources: &[]*string{
							database.Cluster.ClusterArn(),
						},
					}),
					awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
						Actions: &[]*string{
							jsii.String("rds:DescribeDBClusters"),
							jsii.String("rds:DescribeDBInstances"),
						},
						Resources: &[]*string{
							jsii.String("*"), // RDS describe operations typically require * for resource
						},
					}),
				},
			}),
		},
	})
	awscdk.Tags_Of(role).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Apply removal policy to Bedrock Knowledge Base role for clean deletion
	role.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)

	return role
}

// createBedrockAgentRole creates the IAM role for Bedrock Agent
func createBedrockAgentRole(resources *Resources) awsiam.IRole {
	foundationModelResources := make([]*string, len(FoundationModels))
	for i, model := range FoundationModels {
		foundationModelResources[i] = jsii.String(fmt.Sprintf("arn:aws:bedrock:%s::foundation-model/%s", resources.Region, model))
	}

	role := awsiam.NewRole(resources.Stack, jsii.String("BedrockAgentRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("bedrock.amazonaws.com"), nil),
		InlinePolicies: &map[string]awsiam.PolicyDocument{
			"BedrockAgentPolicy": awsiam.NewPolicyDocument(&awsiam.PolicyDocumentProps{
				Statements: &[]awsiam.PolicyStatement{
					// Model invocation permissions
					awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
						Sid:    jsii.String("AgentModelInvocationPermissions"),
						Effect: awsiam.Effect_ALLOW,
						Actions: &[]*string{
							jsii.String("bedrock:InvokeModel"),
						},
						Resources: &foundationModelResources,
					}),
					// Knowledge base query permissions
					awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
						Sid:    jsii.String("AgentKnowledgeBaseQuery"),
						Effect: awsiam.Effect_ALLOW,
						Actions: &[]*string{
							jsii.String("bedrock:Retrieve"),
							jsii.String("bedrock:RetrieveAndGenerate"),
						},
						Resources: &[]*string{
							jsii.String(fmt.Sprintf("arn:aws:bedrock:%s:%s:knowledge-base/*", resources.Region, resources.Account)),
						},
					}),
					// Prompt management console access
					awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
						Sid:    jsii.String("AgentPromptManagementConsole"),
						Effect: awsiam.Effect_ALLOW,
						Actions: &[]*string{
							jsii.String("bedrock:GetPrompt"),
						},
						Resources: &[]*string{
							jsii.String(fmt.Sprintf("arn:aws:bedrock:%s:%s:prompt/*", resources.Region, resources.Account)),
						},
					}),
				},
			}),
		},
	})
	awscdk.Tags_Of(role).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Apply removal policy to Bedrock Agent role for clean deletion
	role.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)

	return role
}

// createGitHubActionsRole creates IAM role for GitHub Actions to push to ECR and deploy frontend
func createGitHubActionsRole(resources *Resources, frontend *FrontendResources) awsiam.IRole {
	role := awsiam.NewRole(resources.Stack, jsii.String("GitHubActionsRole"), &awsiam.RoleProps{
		RoleName: jsii.String("CodeRefactor-GitHubActions-Role"), // Updated role name
		AssumedBy: awsiam.NewWebIdentityPrincipal(
			jsii.String(fmt.Sprintf("arn:aws:iam::%s:oidc-provider/token.actions.githubusercontent.com", resources.Account)),
			&map[string]interface{}{
				"StringEquals": map[string]interface{}{
					"token.actions.githubusercontent.com:aud": "sts.amazonaws.com",
				},
				"StringLike": map[string]interface{}{
					"token.actions.githubusercontent.com:sub": []interface{}{
						"repo:kazemisoroush/code-refactoring-tool:*",
						"repo:kazemisoroush/code-refactoring-ui:*",
					},
				},
			},
		),
		InlinePolicies: &map[string]awsiam.PolicyDocument{
			"ECRAccessPolicy": awsiam.NewPolicyDocument(&awsiam.PolicyDocumentProps{
				Statements: &[]awsiam.PolicyStatement{
					awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
						Actions: &[]*string{
							jsii.String("ecr:GetAuthorizationToken"),
							jsii.String("ecr:BatchCheckLayerAvailability"),
							jsii.String("ecr:GetDownloadUrlForLayer"),
							jsii.String("ecr:BatchGetImage"),
							jsii.String("ecr:PutImage"),
							jsii.String("ecr:InitiateLayerUpload"),
							jsii.String("ecr:UploadLayerPart"),
							jsii.String("ecr:CompleteLayerUpload"),
						},
						Resources: &[]*string{
							jsii.String("*"),
						},
					}),
				},
			}),
			"S3FrontendDeployPolicy": awsiam.NewPolicyDocument(&awsiam.PolicyDocumentProps{
				Statements: &[]awsiam.PolicyStatement{
					awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
						Actions: &[]*string{
							jsii.String("s3:GetObject"),
							jsii.String("s3:PutObject"),
							jsii.String("s3:DeleteObject"),
							jsii.String("s3:ListBucket"),
							jsii.String("s3:GetBucketLocation"),
						},
						Resources: &[]*string{
							frontend.Bucket.BucketArn(),
							jsii.String(fmt.Sprintf("%s/*", *frontend.Bucket.BucketArn())),
						},
					}),
				},
			}),
			"CloudFrontInvalidationPolicy": awsiam.NewPolicyDocument(&awsiam.PolicyDocumentProps{
				Statements: &[]awsiam.PolicyStatement{
					awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
						Actions: &[]*string{
							jsii.String("cloudfront:CreateInvalidation"),
							jsii.String("cloudfront:GetInvalidation"),
							jsii.String("cloudfront:ListInvalidations"),
						},
						Resources: &[]*string{
							jsii.String(fmt.Sprintf("arn:aws:cloudfront::%s:distribution/%s", resources.Account, frontend.DistributionID)),
						},
					}),
				},
			}),
			"ParameterStoreAccessPolicy": awsiam.NewPolicyDocument(&awsiam.PolicyDocumentProps{
				Statements: &[]awsiam.PolicyStatement{
					awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
						Actions: &[]*string{
							jsii.String("ssm:GetParameter"),
							jsii.String("ssm:GetParameters"),
							jsii.String("ssm:GetParametersByPath"),
						},
						Resources: &[]*string{
							jsii.String(fmt.Sprintf("arn:aws:ssm:%s:%s:parameter/code-refactor/*", resources.Region, resources.Account)),
						},
					}),
				},
			}),
			"SecretsManagerAccessPolicy": awsiam.NewPolicyDocument(&awsiam.PolicyDocumentProps{
				Statements: &[]awsiam.PolicyStatement{
					awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
						Actions: &[]*string{
							jsii.String("secretsmanager:GetSecretValue"),
							jsii.String("secretsmanager:DescribeSecret"),
						},
						Resources: &[]*string{
							jsii.String(fmt.Sprintf("arn:aws:secretsmanager:%s:%s:secret:/code-refactor/*", resources.Region, resources.Account)),
						},
					}),
				},
			}),
		},
	})
	awscdk.Tags_Of(role).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Apply removal policy for clean deletion
	role.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)

	return role
}

// createComputeResources creates ECS, Fargate, and ECR resources
func createComputeResources(resources *Resources, networking *NetworkingResources, database *DatabaseResources, storage *StorageResources, cognito *CognitoResources, bedrock *BedrockResources) *ComputeResources {
	// ECS Cluster
	cluster := awsecs.NewCluster(resources.Stack, jsii.String("RefactorCluster"), &awsecs.ClusterProps{
		Vpc: networking.Vpc,
	})
	awscdk.Tags_Of(cluster).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Apply removal policy to ECS cluster for clean deletion
	cluster.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)

	// CloudWatch Log Group
	logGroup := awslogs.NewLogGroup(resources.Stack, jsii.String("FargateLogGroup"), &awslogs.LogGroupProps{
		LogGroupName:  jsii.String("/ecs/code-refactor"),
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
	})
	awscdk.Tags_Of(logGroup).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Task Role and Definition
	taskRole := awsiam.NewRole(resources.Stack, jsii.String("RefactorTaskRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("ecs-tasks.amazonaws.com"), nil),
	})
	awscdk.Tags_Of(taskRole).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Grant the ECS task role permissions to read the database secret
	database.CredentialsSecret.GrantRead(taskRole, nil)

	// Grant the ECS task role permissions to read CloudFormation stack outputs
	taskRole.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect: awsiam.Effect_ALLOW,
		Actions: jsii.Strings(
			"cloudformation:DescribeStacks",
			"cloudformation:DescribeStackResources",
			"cloudformation:DescribeStackEvents",
		),
		Resources: jsii.Strings(
			fmt.Sprintf("arn:aws:cloudformation:%s:%s:stack/CodeRefactorInfra/*", resources.Region, resources.Account),
		),
	}))

	// Grant permissions to access Secrets Manager for database credentials
	taskRole.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect: awsiam.Effect_ALLOW,
		Actions: jsii.Strings(
			"secretsmanager:GetSecretValue",
			"secretsmanager:DescribeSecret",
		),
		Resources: jsii.Strings("*"), // Will be scoped to specific secrets in production
	}))

	// Grant permissions to access Parameter Store for configuration
	taskRole.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect: awsiam.Effect_ALLOW,
		Actions: jsii.Strings(
			"ssm:GetParameter",
			"ssm:GetParameters",
			"ssm:GetParametersByPath",
		),
		Resources: jsii.Strings(
			fmt.Sprintf("arn:aws:ssm:%s:%s:parameter/code-refactor/*", resources.Region, resources.Account),
		),
	}))

	// Apply removal policy to ECS task role for clean deletion
	taskRole.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)

	taskDef := awsecs.NewFargateTaskDefinition(resources.Stack, jsii.String("RefactorTaskDef"), &awsecs.FargateTaskDefinitionProps{
		Cpu:            jsii.Number(512),
		MemoryLimitMiB: jsii.Number(1024),
		TaskRole:       taskRole,
	})
	awscdk.Tags_Of(taskDef).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Apply removal policy to Fargate task definition for clean deletion
	taskDef.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)

	// ECR Repository
	ecrRepo := awsecr.NewRepository(resources.Stack, jsii.String("RefactorEcrRepo"), &awsecr.RepositoryProps{
		RepositoryName: jsii.String("refactor-ecr-repo"),
		RemovalPolicy:  awscdk.RemovalPolicy_DESTROY,
		EmptyOnDelete:  jsii.Bool(true), // Automatically delete images when destroying the stack
	})
	awscdk.Tags_Of(ecrRepo).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Container Definition
	container := taskDef.AddContainer(jsii.String("RefactorContainer"), &awsecs.ContainerDefinitionOptions{
		Image: awsecs.ContainerImage_FromEcrRepository(ecrRepo, jsii.String("latest")),
		Logging: awsecs.LogDrivers_AwsLogs(&awsecs.AwsLogDriverProps{
			StreamPrefix: jsii.String("refactor"),
			LogGroup:     logGroup,
		}),
		Environment: &map[string]*string{
			// Git configuration
			"GIT_TOKEN":  jsii.String("placeholder-token"), // Should be overridden in production with actual GitHub token
			"GIT_AUTHOR": jsii.String("CodeRefactorBot"),
			"GIT_EMAIL":  jsii.String("bot@code-refactor.example.com"),

			// AI Configuration - Add these new variables
			"AI_DEFAULT_PROVIDER": jsii.String("bedrock"),
			"AI_LOCAL_ENABLED":    jsii.String("false"),

			// Bedrock RDS Configuration - Fix the naming to match your Go app's envconfig tags
			"AI_BEDROCK_RDS_POSTGRES_CREDENTIALS_SECRET_ARN":   database.CredentialsSecret.SecretArn(),
			"AI_BEDROCK_RDS_POSTGRES_INSTANCE_ARN":             database.Cluster.ClusterArn(),
			"AI_BEDROCK_RDS_POSTGRES_DATABASE_NAME":            jsii.String(RDSPostgresDatabaseName),
			"AI_BEDROCK_RDS_POSTGRES_SCHEMA_ENSURE_LAMBDA_ARN": database.MigrationLambda.FunctionArn(),
			"AI_BEDROCK_REGION":                                jsii.String(resources.Region),

			// Bedrock AI Configuration - Populate with actual values from created resources
			"AI_BEDROCK_KNOWLEDGE_BASE_SERVICE_ROLE_ARN": bedrock.KnowledgeBaseRole.RoleArn(),
			"AI_BEDROCK_AGENT_SERVICE_ROLE_ARN":          bedrock.AgentRole.RoleArn(),
			"AI_BEDROCK_S3_BUCKET_NAME":                  jsii.String(storage.Name),

			// Cognito configuration - Populate with actual values from created resources
			"COGNITO_USER_POOL_ID": jsii.String(cognito.UserPoolID),
			"COGNITO_CLIENT_ID":    jsii.String(cognito.ClientID),
			"COGNITO_REGION":       jsii.String(resources.Region),

			// Metrics configuration
			"METRICS_NAMESPACE":    jsii.String("CodeRefactorTool/API"),
			"METRICS_REGION":       jsii.String(resources.Region),
			"METRICS_SERVICE_NAME": jsii.String("code-refactor-api"),
			"METRICS_ENABLED":      jsii.String("true"),

			// Application configuration
			"TIMEOUT_SECONDS": jsii.String("180"),
			"LOG_LEVEL":       jsii.String("info"),
		},
	})

	container.AddPortMappings(&awsecs.PortMapping{
		ContainerPort: jsii.Number(8080),
	})

	// Note: ECS Service will be created in createAPIGatewayResources
	// to properly configure with load balancer target group
	return &ComputeResources{
		Cluster:  cluster,
		TaskDef:  taskDef,
		Service:  nil, // Will be set later in createAPIGatewayResources
		EcrRepo:  ecrRepo,
		LogGroup: logGroup,
	}
}

// createCognitoResources creates Cognito User Pool and authentication resources
func createCognitoResources(resources *Resources) *CognitoResources {
	// Create Cognito User Pool
	userPool := awscognito.NewUserPool(resources.Stack, jsii.String("CodeRefactorUserPool"), &awscognito.UserPoolProps{
		UserPoolName:      jsii.String("code-refactor-user-pool"),
		SelfSignUpEnabled: jsii.Bool(true),
		SignInAliases: &awscognito.SignInAliases{
			Email:    jsii.Bool(true),
			Username: jsii.Bool(true),
		},
		AutoVerify: &awscognito.AutoVerifiedAttrs{
			Email: jsii.Bool(true),
		},
		PasswordPolicy: &awscognito.PasswordPolicy{
			MinLength:        jsii.Number(8),
			RequireLowercase: jsii.Bool(true),
			RequireUppercase: jsii.Bool(true),
			RequireDigits:    jsii.Bool(true),
			RequireSymbols:   jsii.Bool(true),
		},
		AccountRecovery: awscognito.AccountRecovery_EMAIL_ONLY,
	})
	awscdk.Tags_Of(userPool).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Apply removal policy to User Pool for clean deletion
	userPool.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)

	// Create User Pool Client
	userPoolClient := awscognito.NewUserPoolClient(resources.Stack, jsii.String("CodeRefactorUserPoolClient"), &awscognito.UserPoolClientProps{
		UserPool:           userPool,
		UserPoolClientName: jsii.String("code-refactor-client"),
		GenerateSecret:     jsii.Bool(false), // For public clients (web/mobile apps)
		AuthFlows: &awscognito.AuthFlow{
			UserPassword: jsii.Bool(true),
			UserSrp:      jsii.Bool(true),
		},
		OAuth: &awscognito.OAuthSettings{
			Flows: &awscognito.OAuthFlows{
				AuthorizationCodeGrant: jsii.Bool(true),
				ImplicitCodeGrant:      jsii.Bool(true),
			},
			Scopes: &[]awscognito.OAuthScope{
				awscognito.OAuthScope_EMAIL(),
				awscognito.OAuthScope_OPENID(),
				awscognito.OAuthScope_PROFILE(),
			},
			CallbackUrls: &[]*string{
				jsii.String("https://localhost:3000/callback"),
				jsii.String("https://example.com/callback"), // Replace with your actual callback URL
			},
			LogoutUrls: &[]*string{
				jsii.String("https://localhost:3000/logout"),
				jsii.String("https://example.com/logout"), // Replace with your actual logout URL
			},
		},
		IdTokenValidity:      awscdk.Duration_Hours(jsii.Number(24)),
		AccessTokenValidity:  awscdk.Duration_Hours(jsii.Number(24)),
		RefreshTokenValidity: awscdk.Duration_Days(jsii.Number(30)),
	})
	awscdk.Tags_Of(userPoolClient).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Apply removal policy to User Pool Client for clean deletion
	userPoolClient.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)

	// Create Cognito User Pool Domain for Hosted UI
	userPoolDomain := awscognito.NewUserPoolDomain(resources.Stack, jsii.String("CodeRefactorUserPoolDomain"), &awscognito.UserPoolDomainProps{
		UserPool: userPool,
		CognitoDomain: &awscognito.CognitoDomainOptions{
			DomainPrefix: jsii.String(fmt.Sprintf("code-refactor-%s", resources.Account)), // Must be globally unique
		},
	})
	awscdk.Tags_Of(userPoolDomain).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Apply removal policy to User Pool Domain for clean deletion
	userPoolDomain.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)

	return &CognitoResources{
		UserPool:       userPool,
		UserPoolClient: userPoolClient,
		UserPoolDomain: userPoolDomain,
		UserPoolID:     *userPool.UserPoolId(),
		ClientID:       *userPoolClient.UserPoolClientId(),
		DomainURL:      *userPoolDomain.DomainName(),
	}
}

// createAPIGatewayResources creates API Gateway, Load Balancer, and VPC Link resources
func createAPIGatewayResources(resources *Resources, networking *NetworkingResources, compute *ComputeResources, cognito *CognitoResources, database *DatabaseResources) *APIGatewayResources {
	// Create Application Load Balancer
	loadBalancer := awselasticloadbalancingv2.NewApplicationLoadBalancer(resources.Stack, jsii.String("CodeRefactorALB"), &awselasticloadbalancingv2.ApplicationLoadBalancerProps{
		Vpc:            networking.Vpc,
		InternetFacing: jsii.Bool(true), // Internet-facing ALB so API Gateway can reach it
		VpcSubnets: &awsec2.SubnetSelection{
			SubnetType: awsec2.SubnetType_PUBLIC, // Use public subnets for ALB
		},
	})
	awscdk.Tags_Of(loadBalancer).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Apply removal policy for clean deletion
	loadBalancer.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)

	// Create Target Group for ECS Service
	targetGroup := awselasticloadbalancingv2.NewApplicationTargetGroup(resources.Stack, jsii.String("CodeRefactorTargetGroup"), &awselasticloadbalancingv2.ApplicationTargetGroupProps{
		Port:       jsii.Number(8080),
		Protocol:   awselasticloadbalancingv2.ApplicationProtocol_HTTP,
		Vpc:        networking.Vpc,
		TargetType: awselasticloadbalancingv2.TargetType_IP,
		HealthCheck: &awselasticloadbalancingv2.HealthCheck{
			Path:                    jsii.String("/health"),
			HealthyHttpCodes:        jsii.String("200"),
			HealthyThresholdCount:   jsii.Number(2),
			UnhealthyThresholdCount: jsii.Number(3),
			Timeout:                 awscdk.Duration_Seconds(jsii.Number(5)),
			Interval:                awscdk.Duration_Seconds(jsii.Number(30)),
		},
	})
	awscdk.Tags_Of(targetGroup).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Create Security Group for ECS Service
	ecsServiceSG := awsec2.NewSecurityGroup(resources.Stack, jsii.String("EcsServiceSG"), &awsec2.SecurityGroupProps{
		Vpc:              networking.Vpc,
		Description:      jsii.String("Allow outbound connections from ECS service to RDS and other AWS services"),
		AllowAllOutbound: jsii.Bool(true),
	})
	awscdk.Tags_Of(ecsServiceSG).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Apply removal policy for clean deletion
	ecsServiceSG.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)

	// Create ECS Service
	// Start with 0 desired count to avoid chicken-and-egg problem with ECR image
	// This will be scaled up after the first image is pushed via GitHub Actions
	service := awsecs.NewFargateService(resources.Stack, jsii.String("CodeRefactorService"), &awsecs.FargateServiceProps{
		Cluster:        compute.Cluster,
		TaskDefinition: compute.TaskDef.(awsecs.TaskDefinition),
		DesiredCount:   jsii.Number(1), // Start with 0 to avoid image pull errors
		VpcSubnets: &awsec2.SubnetSelection{
			SubnetType: awsec2.SubnetType_PUBLIC,
		},
		AssignPublicIp: jsii.Bool(true), // Required for tasks in public subnets without NAT Gateway
		SecurityGroups: &[]awsec2.ISecurityGroup{ecsServiceSG},
	})
	awscdk.Tags_Of(service).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Apply removal policy for clean deletion
	service.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)

	// Attach the ECS service to the target group
	service.AttachToApplicationTargetGroup(targetGroup)

	// Allow ECS service to connect to the RDS database
	database.Cluster.Connections().AllowFrom(ecsServiceSG, awsec2.Port_Tcp(jsii.Number(5432)), jsii.String("Allow ECS service to connect to RDS"))

	// Update compute resources with the service
	compute.Service = service

	// Add Listener to Load Balancer
	loadBalancer.AddListener(jsii.String("CodeRefactorListener"), &awselasticloadbalancingv2.BaseApplicationListenerProps{
		Port:     jsii.Number(80),
		Protocol: awselasticloadbalancingv2.ApplicationProtocol_HTTP,
		DefaultTargetGroups: &[]awselasticloadbalancingv2.IApplicationTargetGroup{
			targetGroup,
		},
	})

	// Create API Gateway REST API
	api := awsapigateway.NewRestApi(resources.Stack, jsii.String("CodeRefactorAPI"), &awsapigateway.RestApiProps{
		RestApiName: jsii.String("code-refactor-api"),
		Description: jsii.String("API Gateway for Code Refactoring Tool"),
		EndpointTypes: &[]awsapigateway.EndpointType{
			awsapigateway.EndpointType_REGIONAL,
		},
		DefaultCorsPreflightOptions: &awsapigateway.CorsOptions{
			AllowOrigins: &[]*string{jsii.String("*")},
			AllowMethods: &[]*string{jsii.String("GET"), jsii.String("POST"), jsii.String("PUT"), jsii.String("DELETE"), jsii.String("OPTIONS")},
			AllowHeaders: &[]*string{jsii.String("Content-Type"), jsii.String("Authorization")},
		},
	})
	awscdk.Tags_Of(api).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Apply removal policy to API Gateway for clean deletion
	api.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)

	// Create Cognito Authorizer
	cognitoAuthorizer := awsapigateway.NewCognitoUserPoolsAuthorizer(resources.Stack, jsii.String("CodeRefactorAuthorizer"), &awsapigateway.CognitoUserPoolsAuthorizerProps{
		CognitoUserPools: &[]awscognito.IUserPool{cognito.UserPool},
		AuthorizerName:   jsii.String("code-refactor-authorizer"),
	})

	// Simple fix: use direct HTTP integration to internet-facing ALB
	albURL := fmt.Sprintf("http://%s", *loadBalancer.LoadBalancerDnsName())
	integration := awsapigateway.NewHttpIntegration(jsii.String(albURL), &awsapigateway.HttpIntegrationProps{
		Proxy: jsii.Bool(true),
	})

	// Add proxy resource to handle all paths
	api.Root().AddProxy(&awsapigateway.ProxyResourceOptions{
		DefaultIntegration: integration,
		DefaultMethodOptions: &awsapigateway.MethodOptions{
			Authorizer:        cognitoAuthorizer,
			AuthorizationType: awsapigateway.AuthorizationType_COGNITO,
		},
		AnyMethod: jsii.Bool(true),
	})

	// Add public endpoints without authorization (health check, swagger docs, auth endpoints)
	healthResource := api.Root().AddResource(jsii.String("health"), nil)
	healthResource.AddMethod(jsii.String("GET"), integration, &awsapigateway.MethodOptions{
		AuthorizationType: awsapigateway.AuthorizationType_NONE,
	})

	docsResource := api.Root().AddResource(jsii.String("swagger"), nil)
	docsResource.AddMethod(jsii.String("GET"), integration, &awsapigateway.MethodOptions{
		AuthorizationType: awsapigateway.AuthorizationType_NONE,
	})

	// Add auth resource for authentication endpoints and documentation
	authResource := api.Root().AddResource(jsii.String("auth"), nil)
	authResource.AddMethod(jsii.String("GET"), integration, &awsapigateway.MethodOptions{
		AuthorizationType: awsapigateway.AuthorizationType_NONE,
	})

	return &APIGatewayResources{
		RestAPI:      api,
		LoadBalancer: loadBalancer,
		URL:          *api.Url(),
	}
}

// createFrontendResources creates S3 bucket and CloudFront distribution for React app hosting
func createFrontendResources(resources *Resources) *FrontendResources {
	// Create S3 bucket for frontend hosting
	frontendBucketName := fmt.Sprintf("code-refactor-frontend-%s-%s", resources.Account, resources.Region)
	frontendBucket := awss3.NewBucket(resources.Stack, jsii.String("FrontendBucket"), &awss3.BucketProps{
		BucketName:        jsii.String(frontendBucketName),
		RemovalPolicy:     awscdk.RemovalPolicy_DESTROY,
		AutoDeleteObjects: jsii.Bool(true),
		// Note: Not enabling website hosting since we use CloudFront with OAI
		// Block public access at bucket level - CloudFront will access via OAI
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
	})
	awscdk.Tags_Of(frontendBucket).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Create Origin Access Identity for CloudFront to access S3
	originAccessIdentity := awscloudfront.NewOriginAccessIdentity(resources.Stack, jsii.String("FrontendOAI"), &awscloudfront.OriginAccessIdentityProps{
		Comment: jsii.String("OAI for Code Refactor Frontend"),
	})

	// Grant CloudFront OAI read access to the bucket
	frontendBucket.GrantRead(originAccessIdentity.GrantPrincipal(), jsii.String("*"))

	// Create CloudFront distribution
	distribution := awscloudfront.NewDistribution(resources.Stack, jsii.String("FrontendDistribution"), &awscloudfront.DistributionProps{
		DefaultBehavior: &awscloudfront.BehaviorOptions{
			// TODO: Replace with S3BucketOrigin when available in CDK version
			//nolint:staticcheck // S3Origin is deprecated but S3BucketOrigin not available in this CDK version
			Origin: awscloudfrontorigins.NewS3Origin(frontendBucket, &awscloudfrontorigins.S3OriginProps{
				OriginAccessIdentity: originAccessIdentity,
			}),
			ViewerProtocolPolicy: awscloudfront.ViewerProtocolPolicy_REDIRECT_TO_HTTPS,
			AllowedMethods:       awscloudfront.AllowedMethods_ALLOW_GET_HEAD(),
			CachedMethods:        awscloudfront.CachedMethods_CACHE_GET_HEAD(),
			Compress:             jsii.Bool(true),
		},
		// Configure for SPA (Single Page Application)
		DefaultRootObject: jsii.String("index.html"),
		ErrorResponses: &[]*awscloudfront.ErrorResponse{
			{
				HttpStatus:         jsii.Number(404),
				ResponseHttpStatus: jsii.Number(200),
				ResponsePagePath:   jsii.String("/index.html"),
				Ttl:                awscdk.Duration_Minutes(jsii.Number(5)),
			},
			{
				HttpStatus:         jsii.Number(403),
				ResponseHttpStatus: jsii.Number(200),
				ResponsePagePath:   jsii.String("/index.html"),
				Ttl:                awscdk.Duration_Minutes(jsii.Number(5)),
			},
		},
		Comment: jsii.String("Code Refactor Frontend Distribution"),
		// Enable for better performance
		EnableIpv6: jsii.Bool(true),
		// Price class for cost optimization (use all edge locations for production)
		PriceClass: awscloudfront.PriceClass_PRICE_CLASS_100,
	})
	awscdk.Tags_Of(distribution).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Apply removal policies for clean deletion
	frontendBucket.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)
	distribution.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)
	originAccessIdentity.ApplyRemovalPolicy(awscdk.RemovalPolicy_DESTROY)

	return &FrontendResources{
		Bucket:                 frontendBucket,
		BucketName:             frontendBucketName,
		CloudFrontDistribution: distribution,
		DistributionID:         *distribution.DistributionId(),
		DistributionDomainName: *distribution.DistributionDomainName(),
	}
}

// createConfigurationStores creates Parameter Store parameters and Secrets Manager secrets
// for both backend and frontend applications
func createConfigurationStores(resources *Resources, storage *StorageResources, database *DatabaseResources, bedrock *BedrockResources, cognito *CognitoResources, apigateway *APIGatewayResources, frontend *FrontendResources, compute *ComputeResources) {
	// Create non-secret parameters in Parameter Store
	createNonSecretParameters(resources, storage, database, cognito, apigateway, frontend, compute)

	// Create secret parameters in Secrets Manager
	createSecretParameters(resources, database, bedrock, cognito)
}

// createNonSecretParameters creates non-sensitive configuration parameters in Parameter Store
func createNonSecretParameters(resources *Resources, storage *StorageResources, database *DatabaseResources, cognito *CognitoResources, apigateway *APIGatewayResources, frontend *FrontendResources, compute *ComputeResources) {
	// Backend non-secret parameters
	backendParams := map[string]string{
		"/code-refactor/backend/api-gateway-url":                       apigateway.URL,
		"/code-refactor/backend/cognito-user-pool-id":                  cognito.UserPoolID,
		"/code-refactor/backend/cognito-region":                        resources.Region,
		"/code-refactor/backend/s3-bucket-name":                        storage.Name,
		"/code-refactor/backend/rds-cluster-arn":                       *database.Cluster.ClusterArn(),
		"/code-refactor/backend/aws-region":                            resources.Region,
		"/code-refactor/backend/aws-account-id":                        resources.Account,
		"/code-refactor/backend/ecr-repository-uri":                    *compute.EcrRepo.RepositoryUri(),
		"/code-refactor/backend/ecs-cluster-name":                      *compute.Cluster.ClusterName(),
		"/code-refactor/backend/rds-postgres-schema-ensure-lambda-arn": *database.MigrationLambda.FunctionArn(),
	}

	// Frontend non-secret parameters
	frontendParams := map[string]string{
		"/code-refactor/frontend/api-base-url":          apigateway.URL,
		"/code-refactor/frontend/cognito-user-pool-id":  cognito.UserPoolID,
		"/code-refactor/frontend/cognito-hosted-ui-url": cognito.DomainURL,
		"/code-refactor/frontend/aws-region":            resources.Region,
		"/code-refactor/frontend/cloudfront-domain":     fmt.Sprintf("https://%s", frontend.DistributionDomainName),
	}

	// Deployment parameters
	deploymentParams := map[string]string{
		"/code-refactor/deployment/frontend-bucket":            frontend.BucketName,
		"/code-refactor/deployment/cloudfront-distribution-id": frontend.DistributionID,
		"/code-refactor/deployment/ecr-repository-uri":         *compute.EcrRepo.RepositoryUri(),
		"/code-refactor/deployment/aws-region":                 resources.Region,
	}

	// Create all non-secret parameters
	allParams := make(map[string]string)
	for k, v := range backendParams {
		allParams[k] = v
	}
	for k, v := range frontendParams {
		allParams[k] = v
	}
	for k, v := range deploymentParams {
		allParams[k] = v
	}

	for paramName, paramValue := range allParams {
		// Create a clean construct ID from the parameter name
		constructID := strings.ReplaceAll(strings.ReplaceAll(strings.TrimPrefix(paramName, "/code-refactor/"), "/", ""), "-", "")
		param := awsssm.NewStringParameter(resources.Stack, jsii.String(fmt.Sprintf("Param%s", constructID)), &awsssm.StringParameterProps{
			ParameterName: jsii.String(paramName),
			StringValue:   jsii.String(paramValue),
			Description:   jsii.String(fmt.Sprintf("Configuration parameter for %s", paramName)),
			Tier:          awsssm.ParameterTier_STANDARD,
		})
		awscdk.Tags_Of(param).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)
	}
}

// createSecretParameters creates sensitive configuration parameters in Secrets Manager
func createSecretParameters(resources *Resources, database *DatabaseResources, bedrock *BedrockResources, cognito *CognitoResources) {
	// Backend secrets
	backendSecrets := map[string]interface{}{
		"rds_credentials_secret_arn":      *database.CredentialsSecret.SecretArn(),
		"bedrock_knowledge_base_role_arn": *bedrock.KnowledgeBaseRole.RoleArn(),
		"bedrock_agent_role_arn":          *bedrock.AgentRole.RoleArn(),
		"cognito_client_id":               cognito.ClientID,
	}

	// Create backend secrets in Secrets Manager
	backendSecret := awssecretsmanager.NewSecret(resources.Stack, jsii.String("BackendSecrets"), &awssecretsmanager.SecretProps{
		SecretName:  jsii.String("/code-refactor/backend/secrets"),
		Description: jsii.String("Backend application secrets"),
		SecretObjectValue: &map[string]awscdk.SecretValue{
			"rds_credentials_secret_arn":      awscdk.SecretValue_UnsafePlainText(jsii.String(fmt.Sprintf("%v", backendSecrets["rds_credentials_secret_arn"]))),
			"bedrock_knowledge_base_role_arn": awscdk.SecretValue_UnsafePlainText(jsii.String(fmt.Sprintf("%v", backendSecrets["bedrock_knowledge_base_role_arn"]))),
			"bedrock_agent_role_arn":          awscdk.SecretValue_UnsafePlainText(jsii.String(fmt.Sprintf("%v", backendSecrets["bedrock_agent_role_arn"]))),
			"cognito_client_id":               awscdk.SecretValue_UnsafePlainText(jsii.String(fmt.Sprintf("%v", backendSecrets["cognito_client_id"]))),
		},
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
	})
	awscdk.Tags_Of(backendSecret).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)

	// Frontend secrets (if any - typically frontend apps have fewer secrets)
	frontendSecrets := map[string]interface{}{
		"cognito_client_id": cognito.ClientID,
	}

	// Create frontend secrets in Secrets Manager
	frontendSecret := awssecretsmanager.NewSecret(resources.Stack, jsii.String("FrontendSecrets"), &awssecretsmanager.SecretProps{
		SecretName:  jsii.String("/code-refactor/frontend/secrets"),
		Description: jsii.String("Frontend application secrets"),
		SecretObjectValue: &map[string]awscdk.SecretValue{
			"cognito_client_id": awscdk.SecretValue_UnsafePlainText(jsii.String(fmt.Sprintf("%v", frontendSecrets["cognito_client_id"]))),
		},
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
	})
	awscdk.Tags_Of(frontendSecret).Add(jsii.String(DefaultResourceTagKey), jsii.String(DefaultResourceTagValue), nil)
}

func getThisFileDir() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("unable to get current file path")
	}
	return filepath.Dir(filename)
}
