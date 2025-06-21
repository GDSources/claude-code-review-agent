#!/bin/bash

# Test webhook endpoint with sample GitHub PR event

set -e

# Configuration
WEBHOOK_URL="${WEBHOOK_URL:-http://localhost:8080/webhook}"
WEBHOOK_SECRET="${WEBHOOK_SECRET:-test-webhook-secret}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}🧪 Testing webhook endpoint: $WEBHOOK_URL${NC}"

# Sample GitHub PR event payload
PAYLOAD='{
  "action": "opened",
  "number": 123,
  "pull_request": {
    "id": 123456,
    "number": 123,
    "title": "Test PR for webhook",
    "body": "This is a test pull request",
    "state": "open",
    "head": {
      "ref": "feature-branch",
      "sha": "abc123def456789",
      "repo": {
        "id": 789,
        "name": "test-repo",
        "full_name": "testowner/test-repo",
        "private": false,
        "owner": {
          "id": 456,
          "login": "testowner"
        }
      }
    },
    "base": {
      "ref": "main",
      "sha": "def456ghi789012",
      "repo": {
        "id": 789,
        "name": "test-repo",
        "full_name": "testowner/test-repo",
        "private": false,
        "owner": {
          "id": 456,
          "login": "testowner"
        }
      }
    },
    "user": {
      "id": 123,
      "login": "contributor"
    }
  },
  "repository": {
    "id": 789,
    "name": "test-repo",
    "full_name": "testowner/test-repo",
    "private": false,
    "owner": {
      "id": 456,
      "login": "testowner"
    }
  }
}'

# Generate HMAC signature
if command -v openssl >/dev/null 2>&1; then
    SIGNATURE="sha256=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "$WEBHOOK_SECRET" | sed 's/^.* //')"
    echo -e "${GREEN}✓ Generated HMAC signature${NC}"
else
    echo -e "${RED}❌ OpenSSL not available for signature generation${NC}"
    echo -e "${YELLOW}⚠️  Continuing without signature (will likely fail)${NC}"
    SIGNATURE=""
fi

# Test 1: Health check
echo -e "\n${YELLOW}1. Testing health endpoint...${NC}"
if curl -s -f "$WEBHOOK_URL/../health" >/dev/null; then
    echo -e "${GREEN}✓ Health check passed${NC}"
else
    echo -e "${RED}❌ Health check failed${NC}"
    exit 1
fi

# Test 2: Webhook with proper signature
echo -e "\n${YELLOW}2. Testing webhook with valid payload...${NC}"
RESPONSE=$(curl -s -w "%{http_code}" -o /tmp/webhook_response.txt \
    -X POST \
    -H "Content-Type: application/json" \
    -H "X-GitHub-Event: pull_request" \
    -H "X-Hub-Signature-256: $SIGNATURE" \
    -d "$PAYLOAD" \
    "$WEBHOOK_URL")

if [ "$RESPONSE" = "200" ]; then
    echo -e "${GREEN}✓ Webhook accepted (HTTP 200)${NC}"
    if [ -f /tmp/webhook_response.txt ]; then
        echo "Response: $(cat /tmp/webhook_response.txt)"
    fi
else
    echo -e "${RED}❌ Webhook failed (HTTP $RESPONSE)${NC}"
    if [ -f /tmp/webhook_response.txt ]; then
        echo "Response: $(cat /tmp/webhook_response.txt)"
    fi
fi

# Test 3: Webhook without signature (should fail)
echo -e "\n${YELLOW}3. Testing webhook without signature (should fail)...${NC}"
RESPONSE=$(curl -s -w "%{http_code}" -o /tmp/webhook_response_no_sig.txt \
    -X POST \
    -H "Content-Type: application/json" \
    -H "X-GitHub-Event: pull_request" \
    -d "$PAYLOAD" \
    "$WEBHOOK_URL")

if [ "$RESPONSE" = "401" ]; then
    echo -e "${GREEN}✓ Webhook correctly rejected without signature (HTTP 401)${NC}"
else
    echo -e "${RED}❌ Webhook should have been rejected (got HTTP $RESPONSE)${NC}"
fi

# Test 4: Ping event
echo -e "\n${YELLOW}4. Testing ping event...${NC}"
PING_PAYLOAD='{"zen": "Keep it logically awesome."}'
if command -v openssl >/dev/null 2>&1; then
    PING_SIGNATURE="sha256=$(echo -n "$PING_PAYLOAD" | openssl dgst -sha256 -hmac "$WEBHOOK_SECRET" | sed 's/^.* //')"
else
    PING_SIGNATURE=""
fi

RESPONSE=$(curl -s -w "%{http_code}" -o /tmp/ping_response.txt \
    -X POST \
    -H "Content-Type: application/json" \
    -H "X-GitHub-Event: ping" \
    -H "X-Hub-Signature-256: $PING_SIGNATURE" \
    -d "$PING_PAYLOAD" \
    "$WEBHOOK_URL")

if [ "$RESPONSE" = "200" ]; then
    echo -e "${GREEN}✓ Ping event handled (HTTP 200)${NC}"
else
    echo -e "${RED}❌ Ping event failed (HTTP $RESPONSE)${NC}"
fi

# Cleanup
rm -f /tmp/webhook_response.txt /tmp/webhook_response_no_sig.txt /tmp/ping_response.txt

echo -e "\n${GREEN}🎉 Webhook testing completed!${NC}"