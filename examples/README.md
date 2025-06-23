# Example Workflows

This directory contains example GitHub Actions workflows that demonstrate how to use the Review Agent in different scenarios.

## Quick Start

Copy any of these workflow files to your repository's `.github/workflows/` directory and customize as needed.

## Available Examples

### 1. `basic-review.yml` - Basic Setup
The simplest possible configuration for automated code reviews on pull requests.

**Best for:** Small projects, getting started, minimal configuration

```yaml
- uses: gdsources/claude-code-review-agent@v1
  with:
    github-token: ${{ secrets.GITHUB_TOKEN }}
    claude-api-key: ${{ secrets.CLAUDE_API_KEY }}
```

### 2. `go-project-review.yml` - Go Projects
Optimized workflow for Go projects with common Go patterns and exclusions.

**Best for:** Go applications, microservices, CLI tools

**Features:**
- Focuses on `.go` files and `go.mod`
- Excludes test files and vendor directory
- Higher confidence threshold for Go-specific patterns

### 3. `javascript-review.yml` - JavaScript/TypeScript
Tailored for Node.js, React, and other JavaScript frameworks.

**Best for:** Frontend applications, Node.js services, npm packages

**Features:**
- Covers JS/TS/JSX/TSX files
- Excludes build artifacts and test files
- Reviews draft PRs (common in JS development)

### 4. `manual-trigger.yml` - Manual Reviews
Allows triggering code reviews manually with custom parameters.

**Best for:** On-demand reviews, testing different models, reviewing specific PRs

**Features:**
- Manual workflow dispatch
- Choose Claude model
- Specify custom paths
- Force review option

### 5. `comment-triggered.yml` - Comment Commands
Triggers reviews when team members post specific comments on PRs.

**Best for:** Large teams, controlled review process, on-demand reviews

**Commands:**
- `/review` - Basic review
- `/review model:claude-3-7-haiku-20250109` - Specific model
- `/review paths:src/**` - Specific paths
- `/review --force` - Force review

### 6. `advanced-configuration.yml` - Full Features
Demonstrates all available features and advanced configuration options.

**Best for:** Enterprise projects, complex requirements, multiple languages

**Features:**
- Dynamic model selection based on PR size
- Comprehensive path filtering
- Smart threshold adjustment
- Status reporting and summaries

## Configuration Options

### Required Inputs
- `github-token`: Use `${{ secrets.GITHUB_TOKEN }}` (automatically provided)
- `claude-api-key`: Add to repository secrets

### Optional Inputs
- `claude-model`: Choose AI model (`claude-sonnet-4-20250514`, `claude-3-7-sonnet-20250109`, etc.)
- `review-paths`: Comma-separated paths to review
- `exclude-paths`: Comma-separated paths to exclude
- `comment-threshold`: Confidence threshold (0-1)
- `skip-draft`: Skip draft PRs (default: true)
- `skip-labels`: Labels that skip review

### Permissions Required
```yaml
permissions:
  contents: read
  pull-requests: write
```

## Getting Started

1. **Choose an example** that matches your project type
2. **Copy the workflow file** to `.github/workflows/` in your repository
3. **Add the required secret**:
   - Go to repository Settings → Secrets and variables → Actions
   - Add `CLAUDE_API_KEY` with your Claude API key
4. **Customize the configuration** to match your project structure
5. **Test with a pull request**

## Common Customizations

### Language-Specific Paths
```yaml
# Python
review-paths: '**/*.py'
exclude-paths: '__pycache__/**,*.pyc,venv/**'

# Java
review-paths: 'src/**/*.java'
exclude-paths: 'target/**,*.class'

# C++
review-paths: 'src/**/*.{cpp,hpp,h,cc}'
exclude-paths: 'build/**,*.o,*.so'
```

### Different Triggers
```yaml
# Only on specific branches
on:
  pull_request:
    branches: [main, develop]

# Only for specific file changes
on:
  pull_request:
    paths: ['src/**', 'lib/**']

# Schedule regular reviews
on:
  schedule:
    - cron: '0 9 * * MON'  # Every Monday at 9 AM
```

### Team-Specific Labels
```yaml
skip-labels: 'skip-review,wip,dependencies,auto-generated,third-party'
```

## Troubleshooting

### Common Issues

1. **Permission denied**: Ensure the workflow has `pull-requests: write` permission
2. **API key not found**: Verify `CLAUDE_API_KEY` is added to repository secrets
3. **No files reviewed**: Check `review-paths` and `exclude-paths` patterns
4. **Too many comments**: Increase `comment-threshold` value

### Debug Mode

Add this step to enable debug logging:
```yaml
- name: Debug Review
  run: echo "Debug mode enabled"
  env:
    RUNNER_DEBUG: 1
```

## Support

For questions or issues:
1. Check the main README.md for detailed documentation
2. Review the workflow logs for error messages
3. Create an issue in the repository with your workflow configuration