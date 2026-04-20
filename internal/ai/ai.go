// Package ai provides the OpenAI-powered voice command processor for AssistMe.
// It transcribes audio via Whisper and dispatches Twitch stream management
// actions via GPT-4o function calling.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// ActionHandlers are the Twitch action callbacks the AI can invoke.
// Each handler must be safe to call concurrently (the caller provides these).
type ActionHandlers struct {
	CreatePoll          func(title string, choices []string, durationSecs int) error
	EndActivePoll       func() error
	StartRaidByLogin    func(channelLogin string) error
	CancelRaid          func() error
	UpdateStreamTitle   func(title string) error
	UpdateStreamGame    func(gameName string) error
	CreateChannelReward func(title string, cost int, prompt string) error
	PauseReward         func(title string) error
	ResumeReward        func(title string) error
}

// CommandResult is the outcome of a voice command, returned to the frontend.
type CommandResult struct {
	Transcript string   `json:"transcript"`
	Message    string   `json:"message"`
	Actions    []string `json:"actions"`
}

// Processor transcribes audio and executes AI-driven Twitch commands.
type Processor struct {
	client   *openai.Client
	handlers ActionHandlers
}

// NewProcessor constructs a Processor with the given OpenAI API key and action handlers.
func NewProcessor(apiKey string, handlers ActionHandlers) *Processor {
	return &Processor{
		client:   openai.NewClient(apiKey),
		handlers: handlers,
	}
}

// ProcessAudio transcribes the given audio bytes (WebM/Opus from the browser) and
// executes any Twitch management commands the AI decides to run.
func (p *Processor) ProcessAudio(ctx context.Context, audioData []byte) (*CommandResult, error) {
	transcript, err := p.transcribe(ctx, audioData)
	if err != nil {
		return nil, fmt.Errorf("transcription: %w", err)
	}
	if strings.TrimSpace(transcript) == "" {
		return &CommandResult{Transcript: "", Message: "No speech detected."}, nil
	}
	return p.processCommand(ctx, transcript)
}

// transcribe sends audio to OpenAI Whisper and returns the text.
func (p *Processor) transcribe(ctx context.Context, audioData []byte) (string, error) {
	req := openai.AudioRequest{
		Model:    openai.Whisper1,
		Reader:   bytes.NewReader(audioData),
		FilePath: "audio.webm", // extension tells Whisper the format
	}
	resp, err := p.client.CreateTranscription(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}

// processCommand sends the transcript to GPT-4o with Twitch tool definitions,
// executes any requested tool calls, and returns a human-readable result.
func (p *Processor) processCommand(ctx context.Context, transcript string) (*CommandResult, error) {
	tools := buildTools()

	messages := []openai.ChatCompletionMessage{
		{
			Role: openai.ChatMessageRoleSystem,
			Content: `You are AssistMe, an AI assistant integrated into a Twitch stream management dashboard.
You help the broadcaster manage their stream hands-free via voice commands.

You have access to the following Twitch actions:
- create_poll: Start a viewer poll with a question and 2–5 choices
- start_raid: Initiate a raid to another Twitch channel
- cancel_raid: Cancel the current pending raid
- update_stream_title: Update the live stream title
- update_stream_game: Change the stream category/game
- create_channel_point_reward: Create a new channel point reward

When you receive a voice command, decide which action(s) to take and call the appropriate tool(s).
Be concise and action-oriented. If no action is appropriate, explain why.
Always confirm what you did in a brief, friendly response.`,
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: transcript,
		},
	}

	resp, err := p.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    openai.GPT4oMini,
		Messages: messages,
		Tools:    tools,
	})
	if err != nil {
		return nil, fmt.Errorf("chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from AI")
	}

	choice := resp.Choices[0]
	var actionsPerformed []string

	// Execute each tool call the model requested.
	if len(choice.Message.ToolCalls) > 0 {
		messages = append(messages, choice.Message)

		for _, tc := range choice.Message.ToolCalls {
			result, execErr := p.executeTool(tc.Function.Name, tc.Function.Arguments)
			if execErr != nil {
				result = fmt.Sprintf("error: %s", execErr.Error())
			} else {
				actionsPerformed = append(actionsPerformed, tc.Function.Name)
			}
			messages = append(messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				ToolCallID: tc.ID,
				Content:    result,
			})
		}

		// Second call: get the final human-readable response.
		finalResp, err := p.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model:    openai.GPT4oMini,
			Messages: messages,
		})
		if err != nil {
			return nil, fmt.Errorf("final response: %w", err)
		}
		if len(finalResp.Choices) == 0 {
			return nil, fmt.Errorf("no final response from AI")
		}
		return &CommandResult{
			Transcript: transcript,
			Message:    finalResp.Choices[0].Message.Content,
			Actions:    actionsPerformed,
		}, nil
	}

	// No tool calls — the model just replied with text.
	return &CommandResult{
		Transcript: transcript,
		Message:    choice.Message.Content,
		Actions:    actionsPerformed,
	}, nil
}

// executeTool routes a tool call name to the appropriate handler.
func (p *Processor) executeTool(name, argsJSON string) (string, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid tool arguments: %w", err)
	}

	getString := func(key string) string {
		if v, ok := args[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	getInt := func(key string, def int) int {
		if v, ok := args[key]; ok {
			switch n := v.(type) {
			case float64:
				return int(n)
			case int:
				return n
			}
		}
		return def
	}
	getStrings := func(key string) []string {
		if v, ok := args[key]; ok {
			if arr, ok := v.([]any); ok {
				out := make([]string, 0, len(arr))
				for _, item := range arr {
					if s, ok := item.(string); ok {
						out = append(out, s)
					}
				}
				return out
			}
		}
		return nil
	}

	switch name {
	case "create_poll":
		title := getString("title")
		choices := getStrings("choices")
		duration := getInt("duration_seconds", 60)
		if title == "" || len(choices) < 2 {
			return "", fmt.Errorf("create_poll requires title and at least 2 choices")
		}
		if err := p.handlers.CreatePoll(title, choices, duration); err != nil {
			return "", err
		}
		return fmt.Sprintf("Poll '%s' created with %d choices for %d seconds.", title, len(choices), duration), nil

	case "start_raid":
		login := getString("channel_login")
		if login == "" {
			return "", fmt.Errorf("start_raid requires channel_login")
		}
		if err := p.handlers.StartRaidByLogin(login); err != nil {
			return "", err
		}
		return fmt.Sprintf("Raid to '%s' started.", login), nil

	case "cancel_raid":
		if err := p.handlers.CancelRaid(); err != nil {
			return "", err
		}
		return "Raid cancelled.", nil

	case "update_stream_title":
		title := getString("title")
		if title == "" {
			return "", fmt.Errorf("update_stream_title requires title")
		}
		if err := p.handlers.UpdateStreamTitle(title); err != nil {
			return "", err
		}
		return fmt.Sprintf("Stream title updated to '%s'.", title), nil

	case "update_stream_game":
		game := getString("game_name")
		if game == "" {
			return "", fmt.Errorf("update_stream_game requires game_name")
		}
		if err := p.handlers.UpdateStreamGame(game); err != nil {
			return "", err
		}
		return fmt.Sprintf("Stream game/category updated to '%s'.", game), nil

	case "end_poll":
		if err := p.handlers.EndActivePoll(); err != nil {
			return "", err
		}
		return "Active poll ended and results shown.", nil

	case "create_channel_point_reward":
		title := getString("title")
		cost := getInt("cost", 100)
		prompt := getString("prompt")
		if title == "" {
			return "", fmt.Errorf("create_channel_point_reward requires title")
		}
		if err := p.handlers.CreateChannelReward(title, cost, prompt); err != nil {
			return "", err
		}
		return fmt.Sprintf("Channel point reward '%s' (%d points) created.", title, cost), nil

	case "pause_reward":
		title := getString("reward_title")
		if title == "" {
			return "", fmt.Errorf("pause_reward requires reward_title")
		}
		if err := p.handlers.PauseReward(title); err != nil {
			return "", err
		}
		return fmt.Sprintf("Reward '%s' paused.", title), nil

	case "resume_reward":
		title := getString("reward_title")
		if title == "" {
			return "", fmt.Errorf("resume_reward requires reward_title")
		}
		if err := p.handlers.ResumeReward(title); err != nil {
			return "", err
		}
		return fmt.Sprintf("Reward '%s' resumed.", title), nil

	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// buildTools returns the set of OpenAI tool definitions for Twitch stream management.
func buildTools() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "create_poll",
				Description: "Create a viewer poll on the Twitch channel. Use when the broadcaster wants to start a poll or vote.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"title": {
							"type": "string",
							"description": "The poll question, e.g. 'What game should we play next?'"
						},
						"choices": {
							"type": "array",
							"items": {"type": "string"},
							"minItems": 2,
							"maxItems": 5,
							"description": "Poll answer choices (2–5 options)"
						},
						"duration_seconds": {
							"type": "integer",
							"description": "How long the poll runs in seconds (15–1800, default 60)",
							"minimum": 15,
							"maximum": 1800
						}
					},
					"required": ["title", "choices"]
				}`),
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "start_raid",
				Description: "Initiate a Twitch raid to another live channel. Use when the broadcaster says 'raid' followed by a channel name.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"channel_login": {
							"type": "string",
							"description": "The Twitch login name (username) of the channel to raid, e.g. 'pokimane'"
						}
					},
					"required": ["channel_login"]
				}`),
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "cancel_raid",
				Description: "Cancel the currently pending raid. Use when the broadcaster says to cancel or stop a raid.",
				Parameters:  json.RawMessage(`{"type": "object", "properties": {}}`),
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "update_stream_title",
				Description: "Update the live stream title/name. Use when the broadcaster wants to change what the stream is called.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"title": {
							"type": "string",
							"description": "The new stream title, e.g. 'Playing Minecraft with viewers! !discord'"
						}
					},
					"required": ["title"]
				}`),
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "update_stream_game",
				Description: "Change the stream category or game. Use when the broadcaster switches games or wants to update what they're playing.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"game_name": {
							"type": "string",
							"description": "The name of the game or category, e.g. 'Minecraft', 'Just Chatting', 'Fortnite'"
						}
					},
					"required": ["game_name"]
				}`),
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "end_poll",
				Description: "End the currently active poll and display results to viewers. Use when the broadcaster says to end, stop, or close the current poll.",
				Parameters:  json.RawMessage(`{"type": "object", "properties": {}}`),
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "create_channel_point_reward",
				Description: "Create a new channel point custom reward that viewers can redeem.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"title": {
							"type": "string",
							"description": "The reward name, e.g. 'Hydration Check'"
						},
						"cost": {
							"type": "integer",
							"description": "Channel point cost for the reward",
							"minimum": 1
						},
						"prompt": {
							"type": "string",
							"description": "Optional description shown to viewers when redeeming"
						}
					},
					"required": ["title", "cost"]
				}`),
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "pause_reward",
				Description: "Pause a channel point reward so viewers cannot redeem it. Use when the broadcaster says to pause, disable, or turn off a reward.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"reward_title": {
							"type": "string",
							"description": "The title of the reward to pause, e.g. 'Pick My Song'"
						}
					},
					"required": ["reward_title"]
				}`),
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "resume_reward",
				Description: "Resume (unpause) a paused channel point reward so viewers can redeem it again. Use when the broadcaster says to resume, enable, or turn on a reward.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"reward_title": {
							"type": "string",
							"description": "The title of the reward to resume, e.g. 'Pick My Song'"
						}
					},
					"required": ["reward_title"]
				}`),
			},
		},
	}
}
