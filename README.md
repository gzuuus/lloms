# LLoms - Simple LLM Chat Client with MCP Integration

LLoms is a simple command-line chat client that interfaces with language models through Ollama while supporting the integration of external tools via MCP.

## Features

- Clean and simple command-line interface for chatting with LLMs
- Configurable settings via YAML file
- MCP integration for tool use capabilities
- Adjustable generation parameters (temperature, repeat penalty, etc.)

## Quick Start

1. Create a `config.yml` file in the project directory (see Configuration section)
2. Run the application:
   ```bash
   go run .
   ```

## Configuration

LLoms is configured via a `config.yml` file. Here's an example configuration:

```yaml
ollama_url: "http://localhost:11434"
chat_model: "llama3"
tools_model: "llama3"
system_prompt: "You are LLoms, a helpful assistant that answers briefly."
enable_mcp: true
temperature: 0.5
repeat_last_n: 3
repeat_penalty: 1.5
tools_temperature: 0.2
tools_repeat_last_n: 2
tools_repeat_penalty: 1.0
mcp:
  servers:
    - name: "weather"
      command: "python"
      args:
        - "-m"
        - "weather_tool"
```

### Configuration Options

| Option | Description |
|--------|-------------|
| `ollama_url` | URL for the Ollama API server |
| `chat_model` | Model to use for general chat |
| `tools_model` | Model to use when evaluating tool use |
| `system_prompt` | Initial instructions for the AI |
| `enable_mcp` | Whether to enable MCP tools integration |
| `temperature` | Randomness in generation (0-1) |
| `repeat_last_n` | Number of tokens to consider for repeat penalty |
| `repeat_penalty` | Penalty for repetition |
| `mcp.servers` | List of MCP servers to connect to |

## MCP Tools Integration

LLoms supports MCP for integrating external tools with LLMs. Configure your MCP tools in the `config.yml` file under the `mcp.servers` section.

Each server needs:
- `name`: A name for the server
- `command`: The executable to run
- `args`: Command line arguments for the executable

## Usage

Once running, you can:
- Type your messages and press Enter to chat
- Type 'exit' or 'quit' to end the conversation

## Requirements

- Go 1.20 or higher
- Ollama running locally or on a remote server
- For MCP functionality: Compatible MCP tool servers
