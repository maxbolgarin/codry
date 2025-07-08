# 🤖 Codry - AI-Powered Code Review Service

[![Go Report Card](https://goreportcard.com/badge/github.com/maxbolgarin/codry)](https://goreportcard.com/report/github.com/maxbolgarin/codry)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

An intelligent code review service that leverages multiple AI providers to automatically review pull requests and merge requests across different VCS platforms.

## 🌟 Features

### **Multi-Platform Support**
- ✅ **GitLab** - Complete integration with GitLab CE/EE
- ✅ **GitHub** - Full GitHub.com and GitHub Enterprise support
- ✅ **Bitbucket** - Complete Bitbucket Cloud and Server support

### **Multiple AI Models**
- ✅ **Google Gemini** - Gemini 2.5 Flash/Pro, cost-effective and fast
- ✅ **OpenAI ChatGPT** - GPT-4o, GPT-4o-mini, GPT-3.5-turbo
- ✅ **Claude/Anthropic** - Claude 3.5 Sonnet/Haiku, excellent reasoning
- ✅ **Azure OpenAI** - Enterprise-grade OpenAI integration
- ✅ **Local Models** - Ollama, LocalAI (OpenAI-compatible APIs)
- 🔄 **More coming** - Claude, Cohere, and other providers

### **Smart Features**
- 🤖 **Automatic Reviews** - AI-powered code analysis and suggestions
- 📝 **MR/PR Descriptions** - Auto-generate comprehensive descriptions
- 🎯 **Reviewer-based Triggers** - Start reviews when bot is added as reviewer
- 🔍 **File Filtering** - Smart filtering by extension, size, and paths
- 🛡️ **Security-First** - Webhook validation and secure token handling
- 📊 **Enterprise Ready** - Rate limiting, monitoring, and compliance features

## 🚀 Quick Start

### 1. Download & Configure

```bash
# Download the latest release
wget https://github.com/maxbolgarin/codry/releases/latest/download/codry

# Make it executable
chmod +x codry

# Copy example configuration
cp config.example.yaml config.yaml

# Edit configuration
nano config.yaml
```

### 2. Choose Your AI Provider

#### Option A: Claude (Recommended for Code Reviews)
```yaml
agent:
  type: "claude"
  api_key: "${CLAUDE_API_KEY}"
  model: "claude-3-5-haiku-20241022"  # Cost-effective
  # model: "claude-3-5-sonnet-20241022"  # Best quality
```

#### Option B: OpenAI ChatGPT
```yaml
agent:
  type: "openai"
  api_key: "${OPENAI_API_KEY}"
  model: "gpt-4o-mini"  # Fast and affordable
  # model: "gpt-4o"  # Best quality
```

#### Option C: Google Gemini
```yaml
agent:
  type: "gemini"
  api_key: "${GEMINI_API_KEY}"
  model: "gemini-2.5-flash-preview-05-20"  # Fast and free tier
```

### 3. Set Environment Variables

```bash
# For Claude
export CLAUDE_API_KEY="sk-ant-your-claude-api-key"

# For OpenAI
export OPENAI_API_KEY="sk-your-openai-api-key"

# For Gemini
export GEMINI_API_KEY="your-gemini-api-key"

# For your VCS platform
export GITLAB_TOKEN="glpat-your-gitlab-token"
export GITHUB_TOKEN="ghp_your-github-token"

# Webhook secrets
export GITLAB_WEBHOOK_SECRET="your-webhook-secret"
export GITHUB_WEBHOOK_SECRET="your-webhook-secret"
```

### 4. Start the Service

```bash
./codry --config config.yaml
```

## 🔧 Platform Setup Guides

### **GitLab Setup**
Basic GitLab configuration is included in the main config. For advanced features, see the documentation.

### **GitHub Setup**
For detailed GitHub integration including webhook setup and reviewer triggers:
📖 **[GitHub Setup Guide](GITHUB_SETUP.md)**

### **Bitbucket Setup**
For complete Bitbucket Cloud and Server integration with webhook configuration:
📖 **[Bitbucket Setup Guide](BITBUCKET_SETUP.md)**

## 🤖 AI Provider Guides

### **Claude/Anthropic Setup**
For Claude 3.5 models with superior reasoning capabilities:
📖 **[Claude Setup Guide](CLAUDE_SETUP.md)**

### **OpenAI ChatGPT Setup**
For GPT-4o and other OpenAI models including Azure OpenAI:
📖 **[OpenAI Setup Guide](OPENAI_SETUP.md)**

### **Google Gemini Setup**
Gemini configuration is straightforward - just get an API key from Google AI Studio and configure as shown above.

## 📋 Configuration Options

### **Minimal Configuration**
```yaml
server:
  address: ":8080"

provider:
  type: "github"  # or "gitlab", "bitbucket"
  token: "${GITHUB_TOKEN}"
  webhook_secret: "${GITHUB_WEBHOOK_SECRET}"
  bot_username: "codry-bot"

agent:
  type: "claude"  # or "openai", "gemini"
  api_key: "${CLAUDE_API_KEY}"
  model: "claude-3-5-haiku-20241022"
```

### **Advanced Configuration**
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
  bot_username: "codry-bot"
  rate_limit_wait: 1m

agent:
  type: "claude"
  api_key: "${CLAUDE_API_KEY}"
  model: "claude-3-5-sonnet-20241022"
  max_retries: 3
  retry_delay: 10s
  temperature: 0.05
  max_tokens: 6000

review:
  file_filter:
    max_file_size: 10000
    allowed_extensions: [".go", ".js", ".ts", ".py", ".java"]
    excluded_paths: ["vendor/", "node_modules/", "*.min.js"]
  max_files_per_mr: 50
  enable_description_generation: true
  enable_code_review: true
  min_files_for_description: 3
  processing_delay: 5s
```

## 🛠️ Development

### Building from Source

```bash
# Clone repository
git clone https://github.com/maxbolgarin/codry.git
cd codry

# Build
make build

# Run tests
make test

# Run with development config
make dev
```

### Project Structure

```
codry/
├── cmd/main/           # Application entry point
├── internal/
│   ├── agents/         # AI provider implementations
│   │   ├── claude/     # Claude/Anthropic integration
│   │   ├── openai/     # OpenAI ChatGPT integration
│   │   └── gemini/     # Google Gemini integration
│   ├── providers/      # VCS platform implementations
│   │   ├── github/     # GitHub integration
│   │   └── gitlab/     # GitLab integration
│   ├── config/         # Configuration management
│   ├── webhook/        # Webhook handling
│   ├── review/         # Review orchestration
│   └── service/        # Core business logic
├── config.example.yaml # Example configuration
└── docs/              # Setup guides and documentation
```

## 🌟 Why Multiple AI Providers?

Different AI models excel at different tasks:

| Model | Best For | Speed | Cost | Code Quality |
|-------|----------|-------|------|--------------|
| **Claude 3.5 Haiku** | Daily reviews, reasoning | ⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| **Claude 3.5 Sonnet** | Complex analysis | ⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐⭐ |
| **GPT-4o-mini** | Fast reviews | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| **GPT-4o** | Comprehensive reviews | ⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐ |
| **Gemini 2.5 Flash** | Budget-conscious | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ |

## 🔒 Security Features

- **Webhook Signature Validation** - Cryptographic verification of incoming webhooks
- **Rate Limiting** - Built-in protection against abuse
- **Token Security** - Secure handling of API keys and access tokens
- **Enterprise Support** - GitHub Enterprise, GitLab Enterprise compatibility
- **Local Model Support** - Complete privacy with self-hosted models

## 📊 Enterprise Features

- **Multi-tenant Support** - Handle multiple organizations
- **Audit Logging** - Comprehensive activity tracking
- **Cost Monitoring** - Track AI API usage and costs
- **Custom Filtering** - Advanced file and change filtering
- **Compliance Ready** - SOC2, GDPR-compatible deployment options

## 🎯 Use Cases

### **Startup Teams**
- Cost-effective daily code reviews
- Automated PR descriptions
- Consistent code quality enforcement

### **Enterprise Organizations**
- Scalable code review automation
- Multi-platform repository support
- Compliance and security focused reviews

### **Open Source Projects**
- Community contribution reviews
- Automated feedback for contributors
- Consistent review standards

## 🤝 Contributing

We welcome contributions! Please see our [contributing guidelines](CONTRIBUTING.md) for details.

### Current Priorities
- Bitbucket provider implementation
- Additional AI providers (Cohere, Mistral)
- Advanced filtering and routing
- Performance optimizations
- Enhanced enterprise features

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🆘 Support

- 📖 **Documentation**: Comprehensive setup guides for each platform
- 🐛 **Issues**: [GitHub Issues](https://github.com/maxbolgarin/codry/issues)
- 💬 **Discussions**: [GitHub Discussions](https://github.com/maxbolgarin/codry/discussions)

## ⭐ Star History

If you find this project useful, please consider giving it a star! ⭐

---

**Built with ❤️ for developers who want intelligent, automated code reviews across any platform with any AI model.**
