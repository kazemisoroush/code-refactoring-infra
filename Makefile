# Infrastructure-specific Makefile
test:
	@echo "Running infrastructure tests..."
	@cd stack && go test -v ./...
	@cd rds_schema_lambda && pytest .
	@echo "Infrastructure tests passed."

lint:
	@echo "Running infrastructure linters..."
	@cd stack && golangci-lint -v run
	@echo "Running Python linter..."
	@cd rds_schema_lambda && pylint --rcfile=../.pylintrc *.py
	@echo "Infrastructure linting passed."

deploy:
	@echo "Deploying infrastructure..."
	@cdk bootstrap
	@cdk deploy --require-approval never
	@echo "Infrastructure deployment done."

destroy:
	@echo "Destroying infrastructure..."
	@cdk destroy --all --force
	@aws secretsmanager delete-secret \
		--secret-id code-refactor-db-secret \
		--force-delete-without-recovery || true
	@echo "Cleaning up any remaining ENIs..."
	@aws ec2 describe-network-interfaces \
		--filters "Name=tag:project,Values=CodeRefactoring" \
		--query "NetworkInterfaces[?Status=='available'].NetworkInterfaceId" \
		--output text | xargs -r -n1 aws ec2 delete-network-interface --network-interface-id || true
	@echo "Infrastructure destruction done."

clean:
	@echo "Cleaning CDK artifacts..."
	@rm -rf cdk.out/
	@echo "Clean completed."

.PHONY: test lint deploy destroy clean

ci: test lint