# Bitbucket Integration Guide

This guide explains how to configure and use Bitbucket with the code review service. Bitbucket provides robust Git hosting with integrated CI/CD and comprehensive API support.

## 🎯 Features

- **Bitbucket Cloud & Server**: Support for both cloud and self-hosted Bitbucket
- **Pull Request Reviews**: Automatic AI-powered code reviews
- **Webhook Integration**: Real-time pull request event processing
- **Reviewer Triggers**: Start reviews when bot is added as reviewer
- **Enterprise Ready**: Bitbucket Server and Data Center support

## 🚀 Quick Start

### 1. Bitbucket Authentication Setup

#### Option A: App Password (Recommended)
1. Go to your Bitbucket account settings
2. Navigate to **App passwords** under **Access management**
3. Click **Create app password**
4. Set permissions:
   - **Repositories**: Read, Write
   - **Pull requests**: Read, Write
   - **Webhooks**: Read, Write
5. Copy the generated app password

#### Option B: OAuth Consumer
1. Go to your workspace settings
2. Navigate to **OAuth consumers**
3. Click **Add consumer**
4. Set permissions: Repositories (Read, Write), Pull requests (Read, Write)
5. Note the Key and Secret

### 2. Configuration

Update your `config.yaml` with Bitbucket configuration:

#### Standard Bitbucket Cloud Configuration
```yaml
provider:
  type: "bitbucket"
  base_url: "https://api.bitbucket.org"  # Default for Bitbucket Cloud
  token: "${BITBUCKET_TOKEN}"            # Your app password
  webhook_secret: "${BITBUCKET_WEBHOOK_SECRET}"
  bot_username: "codry-bot"              # Your bot username
  rate_limit_wait: 1m

agent:
  type: "claude"  # or "openai", "gemini"
  api_key: "${CLAUDE_API_KEY}"
  model: "claude-3-5-haiku-20241022"
```

#### Bitbucket Server (Self-hosted) Configuration
```yaml
provider:
  type: "bitbucket"
  base_url: "https://bitbucket.yourcompany.com/rest/api/1.0"
  token: "${BITBUCKET_TOKEN}"
  webhook_secret: "${BITBUCKET_WEBHOOK_SECRET}"
  bot_username: "codry-bot"
  rate_limit_wait: 2m  # Server might need higher rate limits

agent:
  type: "openai"  # For enterprise environments
  api_key: "${OPENAI_API_KEY}"
  model: "gpt-4o-mini"
```

### 3. Environment Variables

Set the required environment variables:

```bash
# For Bitbucket
export BITBUCKET_TOKEN="your-app-password-here"
export BITBUCKET_WEBHOOK_SECRET="your-webhook-secret"

# For AI provider (choose one)
export CLAUDE_API_KEY="sk-ant-your-claude-api-key"
export OPENAI_API_KEY="sk-your-openai-api-key"
export GEMINI_API_KEY="your-gemini-api-key"

# Start the service
./codry --config config.yaml
```

## 🔧 Webhook Setup

### 1. Create Webhook

For each repository you want to monitor:

1. Go to **Repository settings** → **Webhooks**
2. Click **Add webhook**
3. Configure:
   - **Title**: "Codry AI Code Review"
   - **URL**: `https://your-server.com/webhook`
   - **Triggers**: Select "Pull request events"
   - **Active**: ✅ Checked

### 2. Webhook Events

The service responds to these Bitbucket webhook events:

| Event | Action | Description |
|-------|--------|-------------|
| `pullrequest:created` | `created` | New pull request opened |
| `pullrequest:updated` | `updated` | Pull request updated |
| `pullrequest:fulfilled` | `merged` | Pull request merged |
| `pullrequest:rejected` | `declined` | Pull request declined |
| `pullrequest:reviewer_added` | `reviewer_added` | Reviewer added to PR |

### 3. Test Webhook

```bash
# Check webhook delivery in repository settings
# Look for successful 200 responses

# Check service logs
docker logs codry-service

# Test with a sample PR
curl -X POST https://your-server.com/webhook \
  -H "Content-Type: application/json" \
  -H "X-Event-Key: pullrequest:created" \
  -d '{"pullrequest": {...}}'
```

## 🎛️ How to Trigger Reviews

### Method 1: Add Bot as Reviewer (Recommended)
1. Open your pull request
2. Add your bot username to **Reviewers**
3. Review triggers automatically when bot is added
4. Bot will analyze changes and post comments

### Method 2: Standard PR Events
- **Creating new PR**: Automatic review on creation
- **Updating existing PR**: Review when new commits are pushed
- **Force review**: Remove and re-add bot as reviewer

### Method 3: Manual Comments
- Comment mentioning the bot: `@codry-bot please review`
- Bot responds to direct mentions in PR comments

## 🔐 Authentication & Permissions

### Required Permissions

For the bot account, ensure these permissions:

#### Repository Level
- **Repository access**: Read, Write
- **Pull requests**: Read, Write, Create comments
- **Source code**: Read (to analyze diffs)

#### Workspace Level (if applicable)
- **Webhooks**: Read, Write (for webhook management)
- **Projects**: Read (for project-level configurations)

### Security Best Practices

```yaml
# Use app passwords instead of OAuth for simpler setup
provider:
  token: "${BITBUCKET_APP_PASSWORD}"  # More secure than password

# Set webhook secrets for validation
provider:
  webhook_secret: "${BITBUCKET_WEBHOOK_SECRET}"

# Use environment variables, never hardcode
# ❌ Don't do this:
# token: "ATBB-xxxx-yyyy"

# ✅ Do this:
# token: "${BITBUCKET_TOKEN}"
```

## 🏢 Enterprise Features

### Bitbucket Server Support

```yaml
# For Bitbucket Server (self-hosted)
provider:
  type: "bitbucket"
  base_url: "https://bitbucket.company.com/rest/api/1.0"
  token: "${BITBUCKET_SERVER_TOKEN}"
  bot_username: "ai-reviewer"
  rate_limit_wait: 2m  # Higher for server instances

# Advanced server configuration
review:
  max_files_per_mr: 30       # Lower for server instances
  processing_delay: 10s      # Allow more time
  file_filter:
    max_file_size: 8000      # Smaller files for server
```

### Bitbucket Data Center

```yaml
# For large enterprise environments
provider:
  type: "bitbucket"
  base_url: "https://bitbucket-dc.company.com/rest/api/1.0"
  token: "${BITBUCKET_DC_TOKEN}"
  rate_limit_wait: 5m        # Conservative rate limiting

# Load balancer considerations
server:
  timeout: 60s               # Higher timeout
  address: ":8080"
```

### Integration with Bitbucket Pipelines

```yaml
# bitbucket-pipelines.yml
pipelines:
  pull-requests:
    '**':
      - step:
          name: Trigger AI Review
          script:
            - curl -X POST "${CODRY_WEBHOOK_URL}" \
              -H "Authorization: Bearer ${CODRY_API_KEY}" \
              -d '{"pr_id": "${BITBUCKET_PR_ID}"}'
```

## 📊 Repository Formats

### Project Identification

Bitbucket uses workspace/repository format:

```bash
# Repository URL: https://bitbucket.org/myworkspace/myrepo
# Project ID: myworkspace/myrepo

# API calls use this format:
# GET /2.0/repositories/myworkspace/myrepo/pullrequests/123
```

### Workspace vs User Repositories

```yaml
# User repositories: username/repo-name
# Example: john-doe/my-project

# Workspace repositories: workspace-name/repo-name  
# Example: acme-corp/payment-service

# Both work the same way with the API
```

## 🧪 Testing Your Setup

### Test Connection
```bash
# Start with debug logging
LOG_LEVEL=debug ./codry --config config.yaml

# Should see in logs:
# "Bitbucket provider initialized successfully"
```

### Test API Access
```bash
# Test repository access
curl -u "x-token-auth:${BITBUCKET_TOKEN}" \
  "https://api.bitbucket.org/2.0/repositories/workspace/repo"

# Test webhook delivery
# Create a test PR and check logs for webhook processing
```

### Debug Common Issues

#### Authentication Errors
```
Error: Bitbucket API error: 401 - Unauthorized
```
**Solution**: Check app password permissions and expiry

#### Repository Access Errors
```
Error: Bitbucket API error: 403 - Forbidden
```
**Solution**: Ensure bot has repository read/write access

#### Webhook Delivery Issues
```
Error: failed to parse Bitbucket webhook payload
```
**Solution**: Check webhook URL and event configuration

#### Rate Limiting
```
Error: Bitbucket API error: 429 - Too Many Requests
```
**Solution**: Increase `rate_limit_wait` in configuration

## 🔄 Migration Guide

### From GitHub to Bitbucket
```yaml
# Change this:
provider:
  type: "github"
  token: "${GITHUB_TOKEN}"
  base_url: "https://github.com"

# To this:
provider:
  type: "bitbucket"
  token: "${BITBUCKET_TOKEN}"
  base_url: "https://api.bitbucket.org"
```

### From GitLab to Bitbucket
```yaml
# Change this:
provider:
  type: "gitlab"
  token: "${GITLAB_TOKEN}"
  base_url: "https://gitlab.com"

# To this:
provider:
  type: "bitbucket"
  token: "${BITBUCKET_TOKEN}"
  base_url: "https://api.bitbucket.org"
```

## 📋 Feature Comparison

| Feature | Bitbucket Cloud | Bitbucket Server | GitHub | GitLab |
|---------|----------------|------------------|--------|---------|
| Webhook Events | ✅ Full | ✅ Full | ✅ Full | ✅ Full |
| PR Comments | ✅ | ✅ | ✅ | ✅ |
| Inline Comments | ✅ | ✅ | ✅ | ✅ |
| Reviewer Triggers | ✅ | ✅ | ✅ | ✅ |
| Description Updates | ✅ | ✅ | ✅ | ✅ |
| Enterprise Support | ✅ | ✅ | ✅ | ✅ |

## 🎯 Real-World Usage Examples

### **Startup Configuration**
```yaml
# Simple cloud setup
provider:
  type: "bitbucket"
  token: "${BITBUCKET_TOKEN}"
  bot_username: "ai-reviewer"

agent:
  type: "claude"
  model: "claude-3-5-haiku-20241022"
  max_tokens: 2500
```

### **Enterprise Configuration**
```yaml
# Enterprise server setup
provider:
  type: "bitbucket"
  base_url: "https://bitbucket.company.com/rest/api/1.0"
  token: "${BITBUCKET_SERVER_TOKEN}"
  bot_username: "code-review-bot"
  rate_limit_wait: 2m

agent:
  type: "openai"
  model: "gpt-4o"
  max_tokens: 4000
```

### **High-Volume Configuration**
```yaml
# For teams with many repositories
provider:
  type: "bitbucket"
  rate_limit_wait: 30s  # Faster for high volume

review:
  max_files_per_mr: 20  # Focus on important files
  file_filter:
    max_file_size: 5000
  processing_delay: 3s
```

## 🔗 Useful Resources

- [Bitbucket API Documentation](https://developer.atlassian.com/bitbucket/api/2/reference/)
- [Webhook Documentation](https://support.atlassian.com/bitbucket-cloud/docs/manage-webhooks/)
- [App Passwords Guide](https://support.atlassian.com/bitbucket-cloud/docs/app-passwords/)
- [Bitbucket Server API](https://docs.atlassian.com/bitbucket-server/rest/7.21.0/bitbucket-rest.html)

## 🆘 Troubleshooting

### Enable Debug Logging
```yaml
# Add to your config for detailed logs
logging:
  level: debug
```

### Common Issues & Solutions

#### App Password Issues
- **Problem**: Authentication failures
- **Solution**: Recreate app password with correct permissions

#### Webhook Not Triggering
- **Problem**: No events received
- **Solution**: Check webhook URL, ensure service is accessible

#### Server Connection Issues
- **Problem**: Cannot connect to Bitbucket Server
- **Solution**: Verify base_url, check network connectivity

#### Rate Limiting
- **Problem**: Too many API calls
- **Solution**: Increase rate_limit_wait, optimize file filtering

### Support Channels
- Check service logs: `docker logs codry-service`
- Webhook logs in Bitbucket repository settings
- Network connectivity: `curl -I https://api.bitbucket.org`

Your Bitbucket-powered code review bot is ready to provide intelligent feedback on every pull request! 🚀

**Bitbucket Integration Benefits:**
- 🔧 **Easy Setup**: Simple app password authentication
- 🏢 **Enterprise Ready**: Server and Data Center support
- 🔄 **Flexible Triggering**: Multiple ways to start reviews
- 📊 **Rich Integration**: Deep Bitbucket feature support 