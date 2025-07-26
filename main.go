// Package main provides the entry point for the AWS CDK application.
package main

import (
	"code-refactoring-infra/stack"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/jsii-runtime-go"
)

func main() {
	app := awscdk.NewApp(nil)

	infrastructureStack := stack.NewAppStack(app, "CodeRefactorInfra", &stack.AppStackProps{
		StackProps: awscdk.StackProps{
			Env: &awscdk.Environment{
				Region: jsii.String("us-east-1"),
			},
		},
	})

	// Output BedrockKnowledgeBaseRoleArn
	awscdk.NewCfnOutput(infrastructureStack.Stack, jsii.String("BedrockKnowledgeBaseRoleArn"), &awscdk.CfnOutputProps{
		Value: infrastructureStack.BedrockKnowledgeBaseRole,
	})

	// Output BedrockAgentRoleArn
	awscdk.NewCfnOutput(infrastructureStack.Stack, jsii.String("BedrockAgentRoleArn"), &awscdk.CfnOutputProps{
		Value: infrastructureStack.BedrockAgentRole,
	})

	// Output GitHubActionsRoleARN
	awscdk.NewCfnOutput(infrastructureStack.Stack, jsii.String("GitHubActionsRoleARN"), &awscdk.CfnOutputProps{
		Value: infrastructureStack.GitHubActionsRoleARN,
	})

	// Output BucketName
	awscdk.NewCfnOutput(infrastructureStack.Stack, jsii.String("BucketName"), &awscdk.CfnOutputProps{
		Value: &infrastructureStack.BucketName,
	})

	// Output Account
	awscdk.NewCfnOutput(infrastructureStack.Stack, jsii.String("Account"), &awscdk.CfnOutputProps{
		Value: &infrastructureStack.Account,
	})

	// Output Region
	awscdk.NewCfnOutput(infrastructureStack.Stack, jsii.String("Region"), &awscdk.CfnOutputProps{
		Value: &infrastructureStack.Region,
	})

	// Output RDSPostgresClusterARN
	awscdk.NewCfnOutput(infrastructureStack.Stack, jsii.String("RDSPostgresInstanceARN"), &awscdk.CfnOutputProps{
		Value: &infrastructureStack.RDSPostgresClusterARN,
	})

	// Output RDSPostgresCredentialsSecretARN
	awscdk.NewCfnOutput(infrastructureStack.Stack, jsii.String("RDSPostgresCredentialsSecretARN"), &awscdk.CfnOutputProps{
		Value: &infrastructureStack.RDSPostgresCredentialsSecretARN,
	})

	// Output RDS Postgres Schema Ensure Lambda ARN
	awscdk.NewCfnOutput(infrastructureStack.Stack, jsii.String("RDSPostgresSchemaEnsureLambdaARN"), &awscdk.CfnOutputProps{
		Value: &infrastructureStack.RDSPostgresSchemaEnsureLambdaARN,
	})

	app.Synth(nil)
}
