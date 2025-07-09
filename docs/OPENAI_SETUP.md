# OpenAI/ChatGPT Integration Guide

This guide explains how to configure and use OpenAI's ChatGPT and other OpenAI API-compatible models with the code review service.

## 🎯 Features

- **Multiple Models**: GPT-4o, GPT-4o-mini, GPT-3.5-turbo, GPT-4-turbo
- **Azure OpenAI**: Full support for Azure OpenAI Service
- **Local Models**: Compatible with Ollama, LocalAI, and other OpenAI-compatible APIs
- **Enterprise Ready**: Custom endpoints and authentication methods
- **Cost Control**: Configurable token limits and temperature settings


## 🚀 Quick Start

### 1. OpenAI API Setup

#### Option A: Standard OpenAI API
1. Go to [OpenAI Platform](https://platform.openai.com/)
2. Sign up or log in to your account
3. Navigate to API Keys section
4. Create a new API key
5. Copy the key (starts with `sk-...`)

#### Option B: Azure OpenAI
1. Set up Azure OpenAI resource in Azure Portal
2. Deploy your chosen model (e.g., GPT-4o)
3. Get your resource endpoint and API key
4. Note your deployment name

#### Option C: Local Models (Ollama/LocalAI)
1. Install [Ollama](https://ollama.ai/) or LocalAI
2. Pull your desired model: `ollama pull llama3`
3. Start the service: `ollama serve`
4. Use the local endpoint

### 2. Configuration

Update your `config.yaml` with one of these configurations:

#### Standard OpenAI Configuration
```yaml
agent:
  type: "openai"
  api_key: "${OPENAI_API_KEY}"  # Your OpenAI API key
  model: "gpt-4o-mini"          # Recommended for cost-effectiveness
  max_retries: 3
  retry_delay: 5s
  temperature: 0.1              # Lower = more consistent, higher = more creative
  max_tokens: 4000              # Adjust based on your needs
```

#### Azure OpenAI Configuration
```yaml
agent:
  type: "openai"
  api_key: "${AZURE_OPENAI_API_KEY}"
  model: "gpt-4o"               # Your deployment name
  base_url: "https://your-resource.openai.azure.com"
  max_retries: 3
  retry_delay: 5s
  temperature: 0.1
  max_tokens: 4000
```

#### Local Model Configuration
```yaml
agent:
  type: "openai"
  api_key: "dummy"              # Not used for local models
  model: "llama3"               # Your model name
  base_url: "http://localhost:11434"  # Ollama default endpoint
  max_retries: 3
  retry_delay: 5s
  temperature: 0.1
  max_tokens: 4000
```

### 3. Environment Variables

Set the appropriate environment variables:

```bash
# For Standard OpenAI
export OPENAI_API_KEY="sk-your-openai-api-key"

# For Azure OpenAI
export AZURE_OPENAI_API_KEY="your-azure-api-key"

# Start the service
./codry --config config.yaml
```

## 🤖 Supported Models

### OpenAI Models

| Model | Description | Use Case | Cost |
|-------|-------------|----------|------|
| `gpt-4o` | Latest GPT-4 Omni | Best quality reviews | $$$ |
| `gpt-4o-mini` | Mini version of GPT-4o | **Recommended** - Great balance | $ |
| `gpt-3.5-turbo` | Fast and efficient | Budget-friendly option | $ |
| `gpt-4-turbo` | Previous generation GPT-4 | High-quality reviews | $$$ |

### Azure OpenAI Models
- Same models as standard OpenAI
- Use your deployment name as the model
- Better for enterprise compliance

### Local/Open Source Models
- **Llama 3**: Excellent code understanding
- **CodeLlama**: Specialized for code
- **Mistral**: Good general purpose
- **Qwen**: Strong multilingual support

## 🔧 Advanced Configuration

### Model-Specific Settings

#### For Code Reviews (Recommended)
```yaml
agent:
  type: "openai"
  model: "gpt-4o-mini"
  temperature: 0.1    # More consistent reviews
  max_tokens: 2000    # Sufficient for most reviews
```

#### For Creative Descriptions
```yaml
agent:
  type: "openai"
  model: "gpt-4o"
  temperature: 0.3    # More creative descriptions
  max_tokens: 1000    # Concise descriptions
```

#### For Detailed Analysis
```yaml
agent:
  type: "openai"
  model: "gpt-4o"
  temperature: 0.05   # Very consistent
  max_tokens: 8000    # Detailed analysis
```

### Custom Endpoints

#### Ollama Configuration
```yaml
agent:
  type: "openai"
  api_key: "dummy"
  model: "codellama:7b"
  base_url: "http://localhost:11434"
```

#### LocalAI Configuration
```yaml
agent:
  type: "openai"
  api_key: "dummy"
  model: "gpt-3.5-turbo"
  base_url: "http://localhost:8080"
```

#### OpenRouter Configuration
```yaml
agent:
  type: "openai"
  api_key: "${OPENROUTER_API_KEY}"
  model: "openai/gpt-4o-mini"
  base_url: "https://openrouter.ai/api"
```

### Rate Limiting & Costs

Configure appropriate limits to control costs:

```yaml
agent:
  max_retries: 2      # Reduce retries to save costs
  retry_delay: 10s    # Longer delays for rate limits
  max_tokens: 2000    # Limit response length

review:
  max_files_per_mr: 20        # Limit files reviewed
  min_files_for_description: 5 # Only generate descriptions for larger changes
```

## 💰 Cost Optimization

### Tips for Reducing Costs

1. **Use GPT-4o-mini**: Similar quality to GPT-3.5-turbo but newer
2. **Limit max_tokens**: Set appropriate limits for your use case
3. **Filter files**: Review only important file types
4. **Batch processing**: Process multiple small files together
5. **Smart triggers**: Only review when bot is added as reviewer

### Cost Comparison (Approximate)

| Model | Input Cost | Output Cost | Best For |
|-------|------------|-------------|----------|
| GPT-4o-mini | $0.15/1M tokens | $0.60/1M tokens | **Recommended** |
| GPT-3.5-turbo | $0.50/1M tokens | $1.50/1M tokens | Budget option |
| GPT-4o | $5.00/1M tokens | $15.00/1M tokens | Premium quality |

### Monitoring Usage

Track your usage with OpenAI's dashboard:
- Set up billing alerts
- Monitor token usage in logs
- Use the service's debug logging for token counts

## 🔒 Security & Best Practices

### API Key Security
- **Never commit API keys** to version control
- Use environment variables or secure vaults
- Rotate API keys regularly
- Set up usage alerts

### Azure OpenAI Benefits
- **Enterprise compliance**: SOC 2, GDPR compliant
- **Private endpoints**: No data sent to OpenAI
- **Better SLAs**: Enterprise-grade availability
- **Cost control**: Built-in Azure billing controls

### Local Model Benefits
- **Complete privacy**: No external API calls
- **No usage costs**: Only infrastructure costs
- **Custom models**: Fine-tune for your codebase
- **Offline operation**: Works without internet

## 🧪 Testing Your Setup

### Test Connection
```bash
# Start the service with debug logging
LOG_LEVEL=debug ./codry --config config.yaml

# Check logs for connection test
# You should see: "OpenAI connection test successful"
```

### Test Review
1. Create a test pull request
2. Add the bot as reviewer
3. Check logs for API calls and token usage
4. Verify comments are created

### Debug Common Issues

#### Authentication Errors
```
Error: OpenAI API error: Incorrect API key provided
```
**Solution**: Check your API key and environment variables

#### Rate Limit Errors
```
Error: OpenAI API error: Rate limit exceeded
```
**Solution**: Increase `retry_delay` or reduce request frequency

#### Model Not Found
```
Error: OpenAI API error: The model 'gpt-5' does not exist
```
**Solution**: Use a valid model name from the supported list

#### Azure Specific Issues
```
Error: API request failed with status 404
```
**Solution**: Check your base_url and deployment name

## 🚀 Migration Guide

### From Gemini to OpenAI
```yaml
# Change this:
agent:
  type: "gemini"
  api_key: "${GEMINI_API_KEY}"
  model: "gemini-2.5-flash-preview-05-20"

# To this:
agent:
  type: "openai"
  api_key: "${OPENAI_API_KEY}"
  model: "gpt-4o-mini"
```

### Multi-Agent Setup
You can run multiple instances with different agents:

```bash
# Instance 1: OpenAI for GitHub
OPENAI_API_KEY=sk-... ./codry --config config-github-openai.yaml

# Instance 2: Gemini for GitLab  
GEMINI_API_KEY=... ./codry --config config-gitlab-gemini.yaml
```

## 📊 Performance Comparison

| Feature | GPT-4o-mini | GPT-4o | Gemini 2.5 Flash | Local Llama3 |
|---------|--------------|---------|-------------------|---------------|
| Code Quality | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ |
| Speed | ⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐ |
| Cost | ⭐⭐⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| Privacy | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ |

## 🆘 Troubleshooting

### Enable Debug Logging
```yaml
# Add to your config for detailed OpenAI API logs
logging:
  level: debug
```

### Common Issues & Solutions

#### High Token Usage
- **Problem**: Costs higher than expected
- **Solution**: Reduce `max_tokens`, filter more files, use smaller model

#### Slow Responses
- **Problem**: Reviews taking too long
- **Solution**: Use `gpt-4o-mini` or increase timeout settings

#### Rate Limits
- **Problem**: Getting rate limited frequently
- **Solution**: Increase `retry_delay`, reduce concurrent requests

#### Azure Authentication
- **Problem**: 401 errors with Azure OpenAI
- **Solution**: Use `api-key` header format, check endpoint URL

## 📚 Next Steps

1. **Monitor Usage**: Set up billing alerts and usage tracking
2. **Optimize Prompts**: Customize prompts for your coding standards
3. **A/B Testing**: Compare different models for your use case
4. **Custom Endpoints**: Set up local models for sensitive code

Your OpenAI-powered code review bot is ready to provide intelligent, cost-effective code analysis! 🤖✨

## 🔗 Useful Links

- [OpenAI API Documentation](https://platform.openai.com/docs)
- [Azure OpenAI Service](https://azure.microsoft.com/en-us/products/ai-services/openai-service)
- [Ollama Documentation](https://ollama.ai/docs)
- [OpenRouter](https://openrouter.ai/) - Access to multiple models 