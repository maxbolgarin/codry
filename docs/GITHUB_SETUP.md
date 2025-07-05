# GitHub Provider Setup Guide

This guide explains how to configure and use the GitHub provider for the code review service.

## 🚀 Quick Start

### 1. GitHub Token Setup

Create a GitHub Personal Access Token or GitHub App:

#### Option A: Personal Access Token (Recommended for testing)
1. Go to GitHub Settings → Developer settings → Personal access tokens → Tokens (classic)
2. Click "Generate new token (classic)"
3. Set expiration and select scopes:
   - `repo` (full repository access)
   - `read:user` (to get user info)
   - `write:discussion` (for comments)

4. Copy the generated token

#### Option B: GitHub App (Recommended for production)
1. Go to GitHub Settings → Developer settings → GitHub Apps
2. Click "New GitHub App"
3. Configure the app with required permissions:
   - **Repository permissions**:
     - Contents: Read
     - Pull requests: Write
     - Issues: Write
     - Metadata: Read
   - **Subscribe to events**:
     - Pull request
     - Pull request review
4. Install the app on your repository
5. Generate and download a private key

### 2. Webhook Configuration

#### For Repository Webhooks:
1. Go to your repository → Settings → Webhooks
2. Click "Add webhook"
3. Configure:
   - **Payload URL**: `https://your-domain.com/webhook`
   - **Content type**: `application/json`
   - **Secret**: Generate a random secret string
   - **Events**: Select "Pull requests"
4. Click "Add webhook"

#### For Organization Webhooks:
1. Go to Organization Settings → Webhooks
2. Follow the same steps as above

### 3. Service Configuration

Update your `config.yaml`:

```yaml
server:
  address: ":8080"
  endpoint: "/webhook"
  timeout: 30s

provider:
  type: "github"
  base_url: "https://github.com"  # or your GitHub Enterprise URL
  token: "${GITHUB_TOKEN}"
  webhook_secret: "${GITHUB_WEBHOOK_SECRET}"
  bot_username: "your-bot-username"
  rate_limit_wait: 1m

agent:
  type: "gemini"
  api_key: "${GEMINI_API_KEY}"
  # ... other agent settings

review:
  enable_description_generation: true
  enable_code_review: true
  # ... other review settings
```

### 4. Environment Variables

Set the following environment variables:

```bash
export GITHUB_TOKEN="ghp_xxxxxxxxxxxxxxxxxxxx"
export GITHUB_WEBHOOK_SECRET="your-webhook-secret"
export GEMINI_API_KEY="your-gemini-api-key"
```

## 🎛️ Triggering Reviews

The GitHub provider supports multiple trigger methods:

### Method 1: Add Bot as Reviewer (Recommended)
1. Create or update a pull request
2. Add your bot user to the reviewers list
3. The service will automatically trigger a review

**GitHub webhook event**: `pull_request` with action `review_requested`

### Method 2: Standard PR Events
Reviews are also triggered on standard PR events:
- `opened` - When a PR is opened
- `reopened` - When a PR is reopened  
- `synchronize` - When new commits are pushed
- `ready_for_review` - When a draft PR is marked ready

## 🔧 Advanced Configuration

### GitHub Enterprise Server

For GitHub Enterprise, update the base URL:

```yaml
provider:
  type: "github"
  base_url: "https://github.your-company.com"
  # ... other settings
```

### Custom Webhook Events

You can customize which events trigger reviews by modifying the webhook handler:

```go
// In internal/webhook/handler.go
relevantActions := []string{
    "opened", "reopened", "synchronize",
    "review_requested", "ready_for_review",
    // Add custom actions here
}
```

### Rate Limiting

GitHub has rate limits. Configure appropriate delays:

```yaml
provider:
  rate_limit_wait: 1m  # Wait time when rate limited

agent:
  max_retries: 3
  retry_delay: 5s
```

## 🔒 Security Considerations

### Webhook Security
- Always use webhook secrets to verify payload authenticity
- Use HTTPS endpoints for webhooks
- Rotate webhook secrets regularly

### Token Security
- Use GitHub Apps instead of personal tokens for production
- Limit token scopes to minimum required permissions
- Store tokens securely (environment variables, secrets managers)
- Rotate tokens regularly

### Network Security
- Whitelist GitHub webhook IPs if possible
- Use VPN or private networks for internal deployments
- Enable webhook payload logging for debugging (but avoid logging secrets)

## 📊 Monitoring and Debugging

### Health Checks
The service provides a health endpoint:
```bash
curl http://localhost:8080/health
```

### Webhook Testing
Test your webhook configuration:
1. Use GitHub's webhook testing feature
2. Check service logs for webhook events
3. Verify signature validation

### Common Issues

#### Issue: Webhook not triggering
- **Check**: Webhook URL is accessible
- **Check**: Webhook secret matches configuration
- **Check**: Bot user has repository access

#### Issue: API rate limits
- **Solution**: Increase `rate_limit_wait` in configuration
- **Solution**: Use GitHub App instead of personal token

#### Issue: Permission errors
- **Check**: Token has required permissions
- **Check**: Bot user is collaborator on repository

## 🏃‍♂️ Usage Examples

### Example 1: Review on Bot Assignment
```bash
# Via GitHub CLI
gh pr create --title "Add feature" --body "New feature implementation"
gh pr edit 123 --add-reviewer your-bot-username

# Via GitHub Web UI
1. Open pull request
2. Click "Reviewers" in right sidebar
3. Add your bot username
4. Review will trigger automatically
```

### Example 2: Review on PR Creation
```bash
# Create PR with bot already as reviewer
gh pr create --title "Fix bug" --body "Bug fix" --reviewer your-bot-username
```

### Example 3: Bulk Processing
The service can handle multiple PRs simultaneously:
- Each PR is processed asynchronously
- File-level reviews are created as comments
- PR descriptions are updated with AI summaries

## 🔄 Migration from GitLab

If migrating from GitLab provider:

1. **Update configuration**: Change `type: "gitlab"` to `type: "github"`
2. **Update tokens**: Replace GitLab token with GitHub token
3. **Update webhooks**: Configure GitHub webhooks instead of GitLab
4. **Update bot user**: Ensure bot user exists on GitHub
5. **Test thoroughly**: GitHub API behavior differs from GitLab

## 🆘 Troubleshooting

### Enable Debug Logging
```yaml
# Add to your configuration for verbose logging
logging:
  level: debug
```

### Common Webhook Payloads

#### review_requested event:
```json
{
  "action": "review_requested",
  "pull_request": {
    "id": 123,
    "number": 456,
    "requested_reviewers": [
      {"login": "your-bot-username"}
    ]
  }
}
```

#### opened event:
```json
{
  "action": "opened",
  "pull_request": {
    "id": 123,
    "number": 456,
    "state": "open"
  }
}
```

For more detailed troubleshooting, check the service logs and GitHub webhook delivery logs.

## 📚 Next Steps

1. **Set up monitoring**: Configure alerts for webhook failures
2. **Add metrics**: Track review success rates and performance
3. **Customize prompts**: Adjust AI prompts for your coding standards
4. **Scale up**: Deploy service with load balancing for high traffic

Your GitHub code review bot is now ready to provide AI-powered reviews! 🚀 