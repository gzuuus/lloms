package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/joho/godotenv"
	"github.com/parakeet-nest/parakeet/completion"
	"github.com/parakeet-nest/parakeet/enums/option"
	"github.com/parakeet-nest/parakeet/history"
	"github.com/parakeet-nest/parakeet/llm"
	mcpstdio "github.com/parakeet-nest/parakeet/mcp-stdio"
	"gopkg.in/yaml.v2"
)

const (
	RoleSystem              = "system"
	RoleUser                = "user"
	RoleAssistant           = "assistant"
	MaxConversationMessages = 4
	defaultSystemPrompt     = "You are LLoms, a helpful assistant that answers briefly"
)

type MCPServer struct {
	Name    string   `yaml:"name"`
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}

type MCPConfig struct {
	Servers []MCPServer `yaml:"servers"`
}

type Config struct {
	OllamaURL          string    `yaml:"ollama_url"`
	ChatModel          string    `yaml:"chat_model"`
	ToolsModel         string    `yaml:"tools_model"`
	SystemPrompt       string    `yaml:"system_prompt"`
	EnableMCP          bool      `yaml:"enable_mcp"`
	Temperature        float64   `yaml:"temperature"`
	RepeatLastN        int       `yaml:"repeat_last_n"`
	RepeatPenalty      float64   `yaml:"repeat_penalty"`
	ToolsTemperature   float64   `yaml:"tools_temperature"`
	ToolsRepeatLastN   int       `yaml:"tools_repeat_last_n"`
	ToolsRepeatPenalty float64   `yaml:"tools_repeat_penalty"`
	MCP                MCPConfig `yaml:"mcp"`
}

func loadConfig() Config {
	var config Config

	_ = godotenv.Load()

	yamlFile, err := os.ReadFile("config.yml")
	if err != nil {
		log.Fatalf("Failed to read config.yaml: %v", err)
	}

	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		log.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	config.OllamaURL = getEnv("OLLAMA_HOST", config.OllamaURL)
	config.ChatModel = getEnv("LLM_CHAT", config.ChatModel)
	config.ToolsModel = getEnv("LLM_WITH_TOOLS_SUPPORT", config.ToolsModel)
	config.SystemPrompt = getEnv("SYSTEM_PROMPT", config.SystemPrompt)
	config.EnableMCP = getEnvBool("ENABLE_MCP", config.EnableMCP)
	config.Temperature = getEnvFloat("TEMPERATURE", config.Temperature)
	config.RepeatLastN = getEnvInt("REPEAT_LAST_N", config.RepeatLastN)
	config.RepeatPenalty = getEnvFloat("REPEAT_PENALTY", config.RepeatPenalty)
	config.ToolsTemperature = getEnvFloat("TOOLS_TEMPERATURE", config.ToolsTemperature)
	config.ToolsRepeatLastN = getEnvInt("TOOLS_REPEAT_LAST_N", config.ToolsRepeatLastN)
	config.ToolsRepeatPenalty = getEnvFloat("TOOLS_REPEAT_PENALTY", config.ToolsRepeatPenalty)

	return config
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return strings.ToLower(value) == "true"
}

func getEnvFloat(key string, defaultValue float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	var result float64
	_, err := fmt.Sscanf(value, "%f", &result)
	if err != nil {
		return defaultValue
	}
	return result
}

func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	var result int
	_, err := fmt.Sscanf(value, "%d", &result)
	if err != nil {
		return defaultValue
	}
	return result
}

func generateMsgID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func getLastMessages(messages []llm.Message) []llm.Message {
	if MaxConversationMessages < 0 {
		return messages
	}
	if len(messages) <= MaxConversationMessages {
		return messages
	}
	return messages[len(messages)-MaxConversationMessages:]
}

func toolExists(toolName string, tools []llm.Tool) bool {
	for _, tool := range tools {
		if tool.Function.Name == toolName {
			return true
		}
	}
	return false
}

func main() {
	config := loadConfig()
	userColor := color.New(color.FgCyan, color.Bold)
	assistantColor := color.New(color.FgGreen, color.Bold)
	systemColor := color.New(color.FgYellow)
	toolColor := color.New(color.FgMagenta)

	conversation := history.MemoryMessages{
		Messages: make(map[string]llm.MessageRecord),
	}

	_, err := conversation.SaveMessage(generateMsgID(), llm.Message{
		Role:    RoleSystem,
		Content: config.SystemPrompt,
	})
	if err != nil {
		log.Fatalf("Failed to save system message: %v", err)
	}

	var ollamaTools []llm.Tool
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mcpClient mcpstdio.Client

	if config.EnableMCP && len(config.MCP.Servers) > 0 {
		systemColor.Println("Initializing MCP client...")

		server := config.MCP.Servers[0]
		systemColor.Printf("Using MCP server: %s\n", server.Name)

		mcpClient, err = mcpstdio.NewClient(ctx, server.Command, []string{}, server.Args...)

		if err != nil {
			systemColor.Printf("Warning: Failed to initialize MCP client: %v\n", err)
			systemColor.Println("Continuing without MCP tools support.")
		} else {
			_, err = mcpClient.Initialize()
			if err != nil {
				log.Fatalln("Failed to initialize MCP client", err)
			}

			tools, err := mcpClient.ListTools()
			if err != nil {
				systemColor.Printf("Warning: Failed to get MCP tools: %v\n", err)
			} else {
				ollamaTools = tools
				toolColor.Printf("[%s] tools loaded successfully:\n", server.Name)
				for i, tool := range ollamaTools {
					toolColor.Printf("  %d. %s\n", i+1, tool.Function.Name)
				}
			}
		}
	} else if config.EnableMCP {
		systemColor.Println("MCP enabled but no servers specified in config. Continuing without MCP tools support.")
	}

	systemColor.Printf("Using model: %s\n", config.ChatModel)
	systemColor.Println("Type your message and press Enter to chat.")
	systemColor.Println("Type 'exit' or 'quit' to end the conversation.")
	systemColor.Println("-----------------------------------------------")
	systemColor.Println("🤖 LLoms chat")
	systemColor.Println("-----------------------------------------------")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		userColor.Print("You: ")
		if !scanner.Scan() {
			break
		}
		userInput := scanner.Text()
		if userInput == "exit" || userInput == "quit" {
			break
		}

		if strings.TrimSpace(userInput) == "" {
			continue
		}

		_, err := conversation.SaveMessage(generateMsgID(), llm.Message{
			Role:    RoleUser,
			Content: userInput,
		})
		if err != nil {
			log.Fatalf("Failed to save user message: %v", err)
		}

		allMessages, err := conversation.GetAllMessages()
		if err != nil {
			log.Fatalf("Failed to get conversation history: %v", err)
		}

		messages := []llm.Message{
			{Role: RoleSystem, Content: config.SystemPrompt},
		}

		messages = append(messages, getLastMessages(allMessages)...)

		chatOptions := llm.SetOptions(map[string]any{
			option.Temperature:   config.Temperature,
			option.RepeatLastN:   config.RepeatLastN,
			option.RepeatPenalty: config.RepeatPenalty,
			option.NumCtx:        25920,
			option.Mirostat:      1,
			option.MirostatTau:   5.0,
			option.MirostatEta:   0.1,
		})

		if len(ollamaTools) > 0 {
			toolsOptions := llm.SetOptions(map[string]any{
				option.Temperature:   config.ToolsTemperature,
				option.RepeatLastN:   config.ToolsRepeatLastN,
				option.RepeatPenalty: config.ToolsRepeatPenalty,
				option.NumCtx:        25920,
				option.Mirostat:      1,
				option.MirostatTau:   1.0,
				option.MirostatEta:   0.1,
				option.TopK:          40,
				option.TopP:          0.9,
			})

			toolsQuery := llm.Query{
				Model:    config.ToolsModel,
				Messages: messages,
				Tools:    ollamaTools,
				Options:  toolsOptions,
				Format:   "json",
			}

			answer, err := completion.Chat(config.OllamaURL, toolsQuery)
			if err != nil {
				systemColor.Printf("Tools check failed: %v\n", err)
				systemColor.Println("Continuing with standard chat...")
			} else if len(answer.Message.ToolCalls) > 0 {
				toolCall := answer.Message.ToolCalls[0]

				if !toolExists(toolCall.Function.Name, ollamaTools) {
					systemColor.Printf("Warning: Tool '%s' does not exist. Continuing with standard chat...\n",
						toolCall.Function.Name)
				} else {
					toolColor.Printf("🛠️ Calling tool: %s with args: %s\n",
						toolCall.Function.Name, toolCall.Function.Arguments)

					mcpResult, err := mcpClient.CallTool(toolCall.Function.Name, toolCall.Function.Arguments)

					if err != nil {
						systemColor.Printf("Tool call failed: %v\n", err)
					} else {
						contentFromTool := mcpResult.Text
						toolColor.Printf("🛠️ Tool result: %v\n",
							mcpResult)
						messages = append(messages,
							llm.Message{Role: RoleAssistant, Content: fmt.Sprintf("I used %s and got this result:", toolCall.Function.Name)},
							llm.Message{Role: RoleUser, Content: contentFromTool},
						)

						_, err = conversation.SaveMessage(generateMsgID(), llm.Message{
							Role:    RoleAssistant,
							Content: fmt.Sprintf("I used %s and got this result:", toolCall.Function.Name),
						})
						if err != nil {
							systemColor.Printf("Tool call failed: %v\n", err)
						}

						_, err = conversation.SaveMessage(generateMsgID(), llm.Message{
							Role:    RoleUser,
							Content: contentFromTool,
						})
						if err != nil {
							systemColor.Printf("Tool result failed: %v\n", err)
						}
					}
				}
			}
		}

		query := llm.Query{
			Model:    config.ChatModel,
			Messages: messages,
			Options:  chatOptions,
		}

		assistantColor.Print("LLoms: ")
		var assistantResponse strings.Builder
		_, err = completion.ChatStream(config.OllamaURL, query,
			func(answer llm.Answer) error {
				fmt.Print(answer.Message.Content)
				assistantResponse.WriteString(answer.Message.Content)
				return nil
			},
		)
		if err != nil {
			log.Fatalf("Failed to get response from LLM: %v", err)
		}
		fmt.Println()

		_, err = conversation.SaveMessage(generateMsgID(), llm.Message{
			Role:    RoleAssistant,
			Content: assistantResponse.String(),
		})

		if err != nil {
			log.Fatalf("Failed to save assistant response: %v", err)
		}
	}

	systemColor.Println("Goodbye!")
	os.Exit(0)
}
