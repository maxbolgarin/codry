# Claude/Anthropic Integration Guide

This guide explains how to configure and use Anthropic's Claude models with the code review service. Claude is particularly excellent for code analysis, reasoning, and detailed technical reviews.

## 🎯 Features

- **Latest Claude Models**: Claude 3.5 Sonnet, Claude 3.5 Haiku, Claude 3 Opus
- **Superior Code Understanding**: Exceptional at analyzing complex code patterns
- **Detailed Reviews**: In-depth analysis with thoughtful suggestions
- **Long Context**: Handle large files and complex diffs effectively
- **Safety-First**: Built-in safety measures and responsible AI practices

## 🚀 Quick Start

### 1. Anthropic API Setup

1. Go to [Anthropic Console](https://console.anthropic.com/)
2. Sign up or log in to your account
3. Navigate to API Keys section
4. Create a new API key
5. Copy the key (starts with `sk-ant-...`)

### 2. Configuration

Update your `config.yaml` with Claude configuration:

#### Standard Claude Configuration
```yaml
agent:
  type: "claude"
  api_key: "${CLAUDE_API_KEY}"  # Your Anthropic API key
  model: "claude-3-5-haiku-20241022"  # Recommended for cost-effectiveness
  max_retries: 3
  retry_delay: 5s
  temperature: 0.1              # Lower = more consistent, higher = more creative
  max_tokens: 4000              # Adjust based on your needs
```

#### High-Quality Configuration
```yaml
agent:
  type: "claude"
  api_key: "${CLAUDE_API_KEY}"
  model: "claude-3-5-sonnet-20241022"  # Best quality model
  max_retries: 3
  retry_delay: 10s              # Claude can be slower
  temperature: 0.05             # Very consistent
  max_tokens: 8000              # Detailed reviews
```

### 3. Environment Variables

Set the required environment variables:

```bash
# For Claude
export CLAUDE_API_KEY="sk-ant-your-claude-api-key"

# Start the service
./codry --config config.yaml
```

## 🤖 Supported Models

### Claude 3.5 Models (Latest)

| Model | Description | Use Case | Cost | Context |
|-------|-------------|----------|------|---------|
| `claude-3-5-sonnet-20241022` | Latest Sonnet | **Best quality reviews** | $$$ | 200K tokens |
| `claude-3-5-haiku-20241022` | Latest Haiku | **Recommended** - Fast & cost-effective | $ | 200K tokens |

### Claude 3 Models

| Model | Description | Use Case | Cost | Context |
|-------|-------------|----------|------|---------|
| `claude-3-opus-20240229` | Most capable | Premium analysis | $$$$ | 200K tokens |
| `claude-3-sonnet-20240229` | Balanced | Quality reviews | $$ | 200K tokens |
| `claude-3-haiku-20240307` | Fast | Budget-friendly | $ | 200K tokens |

## 🔧 Advanced Configuration

### Model-Specific Settings

#### For Code Reviews (Recommended)
```yaml
agent:
  type: "claude"
  model: "claude-3-5-haiku-20241022"
  temperature: 0.1    # Consistent analysis
  max_tokens: 3000    # Comprehensive reviews
```

#### For Detailed Analysis
```yaml
agent:
  type: "claude"
  model: "claude-3-5-sonnet-20241022"
  temperature: 0.05   # Very consistent
  max_tokens: 8000    # Thorough analysis
```

#### For Budget-Conscious Usage
```yaml
agent:
  type: "claude"
  model: "claude-3-5-haiku-20241022"
  temperature: 0.2    # Slightly more creative
  max_tokens: 2000    # Concise reviews
```

### Rate Limiting & Performance

Configure appropriate settings for Claude's characteristics:

```yaml
agent:
  max_retries: 3      # Claude is generally reliable
  retry_delay: 10s    # Claude can be slower than other models
  max_tokens: 4000    # Balance between detail and cost

review:
  max_files_per_mr: 30        # Claude handles complex files well
  min_files_for_description: 3 # Generate descriptions for smaller changes too
  processing_delay: 8s        # Allow extra time for Claude's processing
```

## 💰 Cost Optimization

### Tips for Reducing Costs

1. **Use Claude 3.5 Haiku**: Best cost/performance ratio
2. **Optimize max_tokens**: Set appropriate limits for your use case
3. **Smart filtering**: Focus on important files
4. **Temperature tuning**: Lower temperature = more focused responses
5. **Batch processing**: Process multiple small files together

### Cost Comparison (Approximate)

| Model | Input Cost | Output Cost | Best For |
|-------|------------|-------------|----------|
| Claude 3.5 Haiku | $0.25/1M tokens | $1.25/1M tokens | **Recommended** |
| Claude 3.5 Sonnet | $3.00/1M tokens | $15.00/1M tokens | High quality |
| Claude 3 Opus | $15.00/1M tokens | $75.00/1M tokens | Premium analysis |

### Monitoring Usage

Track your usage in Anthropic Console:
- Set up usage alerts
- Monitor token consumption
- Use debug logging to track token usage per review

## 🔒 Security & Best Practices

### API Key Security
- **Never commit API keys** to version control
- Use environment variables or secure vaults
- Rotate API keys regularly
- Set up usage alerts and limits

### Claude-Specific Benefits
- **Safety-first design**: Built-in harmful content detection
- **Transparent reasoning**: Clear explanations of suggestions
- **Context awareness**: Excellent at understanding code relationships
- **Reliable output**: Consistent, well-structured responses

## 🧪 Testing Your Setup

### Test Connection
```bash
# Start the service with debug logging
LOG_LEVEL=debug ./codry --config config.yaml

# Check logs for connection test
# You should see: "Claude connection test successful"
```

### Test Review
1. Create a test pull request
2. Add the bot as reviewer
3. Check logs for API calls and token usage
4. Verify comments are created with Claude's detailed analysis

### Debug Common Issues

#### Authentication Errors
```
Error: Claude API error: Authentication failed
```
**Solution**: Check your API key format (should start with `sk-ant-`)

#### Rate Limit Errors
```
Error: Claude API error: Rate limit exceeded
```
**Solution**: Increase `retry_delay` or reduce request frequency

#### Model Not Found
```
Error: Claude API error: Invalid model specified
```
**Solution**: Use a valid model name from the supported list

#### Timeout Issues
```
Error: failed to make API request: context deadline exceeded
```
**Solution**: Claude can be slower; increase timeout in client configuration

## 🚀 Migration Guide

### From OpenAI to Claude
```yaml
# Change this:
agent:
  type: "openai"
  api_key: "${OPENAI_API_KEY}"
  model: "gpt-4o-mini"

# To this:
agent:
  type: "claude"
  api_key: "${CLAUDE_API_KEY}"
  model: "claude-3-5-haiku-20241022"
```

### From Gemini to Claude
```yaml
# Change this:
agent:
  type: "gemini"
  api_key: "${GEMINI_API_KEY}"
  model: "gemini-2.5-flash-preview-05-20"

# To this:
agent:
  type: "claude"
  api_key: "${CLAUDE_API_KEY}"
  model: "claude-3-5-haiku-20241022"
```

## 📊 Performance Comparison

| Feature | Claude 3.5 Haiku | Claude 3.5 Sonnet | GPT-4o-mini | Gemini 2.5 Flash |
|---------|-------------------|-------------------|--------------|-------------------|
| Code Quality | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| Reasoning | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ |
| Speed | ⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| Cost | ⭐⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| Context | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ |

## 🌟 Claude's Strengths for Code Review

### **1. Superior Reasoning**
- Understands complex code relationships
- Provides thoughtful, contextual suggestions
- Excellent at spotting logical issues

### **2. Detailed Analysis**
- Comprehensive security reviews
- Performance optimization suggestions
- Architecture improvement recommendations

### **3. Clear Communication**
- Well-structured feedback
- Clear explanations of issues
- Actionable improvement suggestions

### **4. Long Context Handling**
- Analyzes large files effectively
- Understands cross-file dependencies
- Maintains context across large diffs

## 🆘 Troubleshooting

### Enable Debug Logging
```yaml
# Add to your config for detailed Claude API logs
logging:
  level: debug
```

### Common Issues & Solutions

#### Slow Response Times
- **Problem**: Claude taking longer than expected
- **Solution**: Increase retry_delay, use Haiku model for speed

#### High Token Usage
- **Problem**: Costs higher than expected
- **Solution**: Reduce max_tokens, use temperature tuning

#### Rate Limits
- **Problem**: Getting rate limited frequently
- **Solution**: Increase retry_delay, implement exponential backoff

#### Context Length Errors
- **Problem**: Input too long for model
- **Solution**: Implement better file filtering, split large diffs

## 📚 Best Practices

### **1. Model Selection**
- **Claude 3.5 Haiku**: Daily code reviews, cost-sensitive projects
- **Claude 3.5 Sonnet**: Important releases, complex architecture reviews
- **Claude 3 Opus**: Critical systems, security-focused reviews

### **2. Prompt Optimization**
- Use clear, specific instructions
- Leverage Claude's reasoning capabilities
- Ask for structured output formats

### **3. Token Management**
- Monitor usage patterns
- Set appropriate max_tokens limits
- Filter files effectively

### **4. Integration Tips**
- Use Claude for detailed reviews
- Combine with faster models for different use cases
- Leverage Claude's long context for large PRs

## 🔗 Useful Resources

- [Anthropic Documentation](https://docs.anthropic.com/)
- [Claude API Reference](https://docs.anthropic.com/claude/reference/)
- [Model Comparison](https://docs.anthropic.com/claude/docs/models-overview)
- [Best Practices Guide](https://docs.anthropic.com/claude/docs/prompt-engineering)

## 🎯 Real-World Usage Examples

### **Startup Configuration**
```yaml
# Cost-optimized for regular reviews
agent:
  type: "claude"
  model: "claude-3-5-haiku-20241022"
  max_tokens: 2500
  temperature: 0.1
```

### **Enterprise Configuration**
```yaml
# Quality-focused for critical reviews
agent:
  type: "claude"
  model: "claude-3-5-sonnet-20241022"
  max_tokens: 6000
  temperature: 0.05
```

### **Security-Focused Configuration**
```yaml
# Detailed security analysis
agent:
  type: "claude"
  model: "claude-3-5-sonnet-20241022"
  max_tokens: 8000
  temperature: 0.0
review:
  enable_description_generation: true
  enable_code_review: true
  max_files_per_mr: 20  # Focus on thorough review
```

Your Claude-powered code review bot is ready to provide intelligent, thoughtful code analysis with industry-leading reasoning capabilities! 🤖✨

**Claude excels at:**
- 🔍 **Deep Code Analysis**: Understanding complex logic and relationships
- 🛡️ **Security Reviews**: Identifying potential vulnerabilities
- 🏗️ **Architecture Feedback**: Suggesting structural improvements
- 📝 **Clear Communication**: Providing actionable, well-explained feedback 