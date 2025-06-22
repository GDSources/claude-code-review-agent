#!/bin/bash

# Final Verification Script for Review Agent
# This script MUST be run before any commit to ensure code quality

set -e  # Exit on any error

echo "🔍 Final Verification Checks for Review Agent:"
echo "=============================================="
echo ""

# Check 1: Go Code Formatting
echo "1. 📝 Formatting Check:"
if [ "$(gofmt -s -l . | wc -l)" -gt 0 ]; then
  echo "❌ Code formatting issues found:"
  gofmt -s -l .
  echo ""
  echo "🔧 Fix with: gofmt -s -w ."
  exit 1
else
  echo "✅ All Go code is properly formatted"
fi

echo ""

# Check 2: Test Suite
echo "2. 🧪 Test Suite:"
if go test ./... >/dev/null 2>&1; then
  echo "✅ All tests pass"
else
  echo "❌ Some tests are failing:"
  echo "🔧 Running tests with output to see failures..."
  go test ./...
  exit 1
fi

echo ""

# Check 3: Static Analysis
echo "3. 🔍 Static Analysis (go vet):"
if go vet ./... >/dev/null 2>&1; then
  echo "✅ Go vet passes - no issues found"
else
  echo "❌ Go vet found issues:"
  go vet ./...
  exit 1
fi

echo ""

# Check 4: golangci-lint
echo "4. 🔧 Comprehensive Linting (golangci-lint):"
if $(go env GOPATH)/bin/golangci-lint run >/dev/null 2>&1; then
  echo "✅ golangci-lint passes - no issues found"
else
  echo "❌ golangci-lint found issues:"
  $(go env GOPATH)/bin/golangci-lint run
  exit 1
fi

echo ""

# Check 5: Build Verification
echo "5. 🏗️  Build Verification:"
if go build -o /tmp/review-agent-test cmd/agent/main.go >/dev/null 2>&1; then
  echo "✅ Project builds successfully"
  rm -f /tmp/review-agent-test
else
  echo "❌ Build failed:"
  go build -o /tmp/review-agent-test cmd/agent/main.go
  exit 1
fi

echo ""

# Check 6: Git Status
echo "6. 📊 Git Status:"
if git status --porcelain | grep -q .; then
  echo "ℹ️  Uncommitted changes present:"
  git status --short
  echo ""
  echo "🔧 Stage changes with: git add ."
else
  echo "✅ Working directory clean"
fi

echo ""

# Check 7: Dependency Check
echo "7. 📦 Dependency Check:"
go mod tidy >/dev/null 2>&1
if git diff --quiet go.mod 2>/dev/null && (! git ls-files --error-unmatch go.sum >/dev/null 2>&1 || git diff --quiet go.sum 2>/dev/null); then
  echo "✅ Dependencies are up to date"
else
  echo "ℹ️  Dependencies updated - go.mod or go.sum changed"
  echo "🔧 Review changes and commit if needed"
fi

echo ""
echo "🎯 All critical verification checks passed!"
echo ""
echo "✅ Code is ready for commit"
echo "✅ Formatting is correct"  
echo "✅ All tests pass"
echo "✅ Static analysis clean"
echo "✅ Comprehensive linting clean"
echo "✅ Build successful"
echo ""
echo "🚀 You can now safely commit your changes!"