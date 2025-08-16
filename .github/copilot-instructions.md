# Copilot Instructions

## Coding Standards

- This file should always updated manually by human.
- Always take TDD (Test-Driven Development) approach when writing code.
- Always code in a way that is dependency injection friendly. This means that you should design your code in such a way that it can be easily tested with mock dependencies.
- All unit tests should live in the same directory as the code they are testing with a `*.test.*` suffix.
- All tests should be broken down into 3 parts: Arrange, Act, Assert. This helps in maintaining clarity and structure in tests.
- Always follow SOLID principles and design patterns.
- Never create a test case that is not adding any value.
- Never write Markdown documentation unless you are specifically asked to do so. This is not about code comments, but rather about documentation files.
- Never finish your code with unused code no matter how small it is.
- Never add redundant fields to structs or classes. Only add fields that you are asked to add.
- Always expose least amount of information to another high-level module or component. Only expose what is necessary for the other module or component to function correctly.
- When building the project to verify that the build is successful always add built files to the `.gitignore` file to avoid committing them.
- Always log errors and exceptions in a way that they can be easily traced and debugged. But at the same time, avoid over-logging. Ideally you should log at the highest level of abstraction that makes sense for the application.
- Never let a command run for more than 60 seconds without checking if it is still running. If it is not running, stop it and investigate the issue.
- Try avoid changing the code that is not related to the task at hand.
- Always follow "The Rule of Silence" in Unix that states that when a program has nothing surprising, interesting or useful to say, it should say nothing.
- Never determine the application flow with exceptions.

# Project Specific Guidelines

- Avoid writing random bash script files to solve problems. Issues should solved in more holistic ways, such as modifying existing code or adding new features.
- This repo is controlplane focused only avoid adding dataplane specific code.

# Pre-commit Guidelines

- Always make sure `make ci` passes before committing code.
- When you run `make ci`, it contains all commands needed to verify the code quality. Don't run the subcommands manually.
