#!/bin/bash
set -e

# GitHub Action entrypoint script for review-agent
# This script handles the GitHub Actions environment and calls the review-agent

echo "🤖 Starting Code Review Agent GitHub Action"

# Function to extract repository info from GitHub context
get_repository_info() {
    if [ -n "$ACTION_REPOSITORY" ]; then
        echo "$ACTION_REPOSITORY"
    elif [ -n "$GITHUB_REPOSITORY" ]; then
        echo "$GITHUB_REPOSITORY"
    else
        echo ""
    fi
}

# Function to get PR number from GitHub context
get_pr_number() {
    if [ -n "$ACTION_PR_NUMBER" ]; then
        echo "$ACTION_PR_NUMBER"
    elif [ -n "$GITHUB_EVENT_NAME" ] && [ "$GITHUB_EVENT_NAME" = "pull_request" ]; then
        # Extract from GitHub event JSON
        if [ -f "$GITHUB_EVENT_PATH" ]; then
            jq -r '.pull_request.number // empty' "$GITHUB_EVENT_PATH"
        fi
    else
        echo ""
    fi
}

# Parse repository into owner and repo
REPOSITORY=$(get_repository_info)
if [ -z "$REPOSITORY" ]; then
    echo "❌ Error: Could not determine repository. Please set 'repository' input or ensure GITHUB_REPOSITORY is set."
    exit 1
fi

OWNER=$(echo "$REPOSITORY" | cut -d'/' -f1)
REPO=$(echo "$REPOSITORY" | cut -d'/' -f2)

# Get PR number
PR_NUMBER=$(get_pr_number)
if [ -z "$PR_NUMBER" ]; then
    echo "❌ Error: Could not determine PR number. Please set 'pr-number' input or ensure this is running on a pull_request event."
    exit 1
fi

echo "📋 Repository: $OWNER/$REPO"
echo "🔢 PR Number: $PR_NUMBER"
echo "🤖 Model: ${CLAUDE_MODEL:-claude-sonnet-4-20250514}"

# Check if this is a draft PR and skip-draft is enabled
if [ "$ACTION_SKIP_DRAFT" = "true" ] && [ -f "$GITHUB_EVENT_PATH" ]; then
    IS_DRAFT=$(jq -r '.pull_request.draft // false' "$GITHUB_EVENT_PATH")
    if [ "$IS_DRAFT" = "true" ]; then
        echo "⏭️  Skipping review for draft PR (skip-draft is enabled)"
        echo "review-status=skipped" >> $GITHUB_OUTPUT
        echo "comments-posted=0" >> $GITHUB_OUTPUT
        exit 0
    fi
fi

# Check for skip labels
if [ -n "$ACTION_SKIP_LABELS" ] && [ -f "$GITHUB_EVENT_PATH" ]; then
    LABELS=$(jq -r '.pull_request.labels[].name // empty' "$GITHUB_EVENT_PATH" | tr '\n' ',')
    IFS=',' read -ra SKIP_LABELS_ARRAY <<< "$ACTION_SKIP_LABELS"
    
    for skip_label in "${SKIP_LABELS_ARRAY[@]}"; do
        if [[ ",$LABELS," == *",$skip_label,"* ]]; then
            echo "⏭️  Skipping review due to label: $skip_label"
            echo "review-status=skipped" >> $GITHUB_OUTPUT
            echo "comments-posted=0" >> $GITHUB_OUTPUT
            exit 0
        fi
    done
fi

# Prepare review command
REVIEW_CMD="/app/review-agent review --owner $OWNER --repo $REPO --pr $PR_NUMBER"

# Add model if specified
if [ -n "$CLAUDE_MODEL" ]; then
    REVIEW_CMD="$REVIEW_CMD --claude-model $CLAUDE_MODEL"
fi

# Debug: Show environment for troubleshooting
if [ "$RUNNER_DEBUG" = "1" ]; then
    echo "🔍 Debug: Environment variables"
    echo "GH_TOKEN: ${GH_TOKEN:0:10}..."
    echo "CLAUDE_API_KEY: ${CLAUDE_API_KEY:0:10}..."
    echo "Full command: $REVIEW_CMD"
fi

# Run the review and capture output
echo "🚀 Running code review..."
set +e  # Don't exit on error so we can capture the exit code
REVIEW_OUTPUT=$($REVIEW_CMD 2>&1)
REVIEW_EXIT_CODE=$?
set -e

# Parse review results from output
COMMENTS_POSTED=0
if [ $REVIEW_EXIT_CODE -eq 0 ]; then
    echo "✅ Review completed successfully"
    echo "review-status=success" >> $GITHUB_OUTPUT
    
    # Parse JSON output from the review command
    REVIEW_JSON=$(echo "$REVIEW_OUTPUT" | grep "REVIEW_RESULT_JSON:" | sed 's/REVIEW_RESULT_JSON://')
    if [ -n "$REVIEW_JSON" ]; then
        # Extract comments_posted from JSON using jq if available, otherwise use grep/sed
        if command -v jq >/dev/null 2>&1; then
            COMMENTS_POSTED=$(echo "$REVIEW_JSON" | jq -r '.comments_posted // 0')
        else
            # Fallback parsing without jq
            COMMENTS_POSTED=$(echo "$REVIEW_JSON" | sed -n 's/.*"comments_posted":\([0-9]*\).*/\1/p')
            # If no match found, default to 0
            if [ -z "$COMMENTS_POSTED" ]; then
                COMMENTS_POSTED=0
            fi
        fi
    fi
    
    # Try to get the PR URL
    if [ -n "$GITHUB_SERVER_URL" ] && [ -n "$GITHUB_REPOSITORY" ]; then
        echo "review-url=${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}/pull/${PR_NUMBER}" >> $GITHUB_OUTPUT
    fi
    
    echo "comments-posted=${COMMENTS_POSTED}" >> $GITHUB_OUTPUT
    echo "📊 Posted ${COMMENTS_POSTED} review comments"
else
    echo "❌ Review failed with exit code: $REVIEW_EXIT_CODE"
    echo "review-status=failed" >> $GITHUB_OUTPUT
    echo "comments-posted=0" >> $GITHUB_OUTPUT
    # Still output the error for debugging
    echo "$REVIEW_OUTPUT"
    exit $REVIEW_EXIT_CODE
fi