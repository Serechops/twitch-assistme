// Package ai provides the OpenAI-powered voice command processor for AssistMe.
// It transcribes audio via Whisper and dispatches Twitch stream management
// actions via the OpenAI Responses API with function calling and web search.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/responses"
)

// namedReader wraps bytes.Reader to provide a filename hint to the multipart encoder,
// allowing OpenAI Whisper to determine the audio format from the file extension.
type namedReader struct {
	*bytes.Reader
	filename string
}

func (n *namedReader) Filename() string { return n.filename }

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
	CreateClip          func() error
}

// CommandResult is the outcome of a voice command, returned to the frontend.
type CommandResult struct {
	Transcript string   `json:"transcript"`
	Message    string   `json:"message"`
	Actions    []string `json:"actions"`
}

// GameAssistantResult is the outcome of a game guide query.
type GameAssistantResult struct {
	Answer  string       `json:"answer"`
	Sources []GameSource `json:"sources"`
}

// GameSource is a web search citation returned with a game guide answer.
type GameSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Processor transcribes audio and executes AI-driven Twitch commands.
type Processor struct {
	client         openai.Client
	handlers       ActionHandlers
	mu             sync.Mutex
	voiceSessionID string
	gameSessionID  string
}

// NewProcessor constructs a Processor with the given OpenAI API key and action handlers.
func NewProcessor(apiKey string, handlers ActionHandlers) *Processor {
	return &Processor{
		client:   openai.NewClient(option.WithAPIKey(apiKey)),
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

// transcribe sends audio to OpenAI Whisper and returns the transcript text.
func (p *Processor) transcribe(ctx context.Context, audioData []byte) (string, error) {
	resp, err := p.client.Audio.Transcriptions.New(ctx, openai.AudioTranscriptionNewParams{
		File:  &namedReader{Reader: bytes.NewReader(audioData), filename: "audio.webm"},
		Model: openai.AudioModelWhisper1,
	})
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}

const voiceSystemPrompt = `You are AssistMe, an AI assistant integrated into a Twitch stream management dashboard.
You help the broadcaster manage their stream hands-free via voice commands.

You have access to the following Twitch actions:
- create_poll: Start a viewer poll with a question and 2–5 choices
- start_raid: Initiate a raid to another Twitch channel
- cancel_raid: Cancel the current pending raid
- update_stream_title: Update the live stream title
- update_stream_game: Change the stream category/game
- end_poll: End the currently active poll
- create_channel_point_reward: Create a new channel point reward
- pause_reward: Pause a channel point reward
- resume_reward: Resume a paused channel point reward
- create_clip: Create a clip from the current live stream

When you receive a voice command, decide which action(s) to take and call the appropriate tool(s).
Be concise and action-oriented. If no action is appropriate, explain why.
Always confirm what you did in a brief, friendly response.`

// processCommand sends the transcript to the Responses API, executes any tool calls,
// and returns a human-readable result. It uses previous_response_id for conversation state.
func (p *Processor) processCommand(ctx context.Context, transcript string) (*CommandResult, error) {
	p.mu.Lock()
	prevID := p.voiceSessionID
	p.mu.Unlock()

	reqParams := responses.ResponseNewParams{
		Model:        openai.ChatModelGPT4oMini,
		Instructions: param.NewOpt(voiceSystemPrompt),
		Input:        responses.ResponseNewParamsInputUnion{OfString: param.NewOpt(transcript)},
		Tools:        buildTools(),
		Store:        param.NewOpt(true),
	}
	if prevID != "" {
		reqParams.PreviousResponseID = param.NewOpt(prevID)
	}

	resp, err := p.client.Responses.New(ctx, reqParams)
	if err != nil {
		return nil, fmt.Errorf("responses API: %w", err)
	}

	p.mu.Lock()
	p.voiceSessionID = resp.ID
	p.mu.Unlock()

	var actionsPerformed []string
	var toolOutputs []responses.ResponseInputItemUnionParam

	for _, item := range resp.Output {
		if item.Type == "function_call" {
			result, execErr := p.executeTool(item.Name, item.Arguments)
			if execErr != nil {
				result = fmt.Sprintf("error: %s", execErr.Error())
			} else {
				actionsPerformed = append(actionsPerformed, item.Name)
			}
			toolOutputs = append(toolOutputs, responses.ResponseInputItemParamOfFunctionCallOutput(item.CallID, result))
		}
	}

	if len(toolOutputs) == 0 {
		return &CommandResult{
			Transcript: transcript,
			Message:    resp.OutputText(),
			Actions:    actionsPerformed,
		}, nil
	}

	// Submit tool results and get the final human-readable response.
	finalParams := responses.ResponseNewParams{
		Model:              openai.ChatModelGPT4oMini,
		Input:              responses.ResponseNewParamsInputUnion{OfInputItemList: responses.ResponseInputParam(toolOutputs)},
		PreviousResponseID: param.NewOpt(resp.ID),
		Store:              param.NewOpt(true),
	}
	finalResp, err := p.client.Responses.New(ctx, finalParams)
	if err != nil {
		return nil, fmt.Errorf("final response: %w", err)
	}

	p.mu.Lock()
	p.voiceSessionID = finalResp.ID
	p.mu.Unlock()

	return &CommandResult{
		Transcript: transcript,
		Message:    finalResp.OutputText(),
		Actions:    actionsPerformed,
	}, nil
}

// AskGameGuide answers a gameplay question about the streamer's current game using web search.
// It maintains a per-session conversation thread via previous_response_id.
func (p *Processor) AskGameGuide(ctx context.Context, question, gameName, streamTitle string) (*GameAssistantResult, error) {
	p.mu.Lock()
	prevID := p.gameSessionID
	p.mu.Unlock()

	game := gameName
	if game == "" {
		game = "an unknown game"
	}

	instructions := fmt.Sprintf(
		"You are a game guide assistant embedded in a Twitch streaming dashboard. "+
			"The streamer is currently playing %s (stream title: %q). "+
			"Your ONLY job is to answer gameplay questions about %s. "+
			"You MUST use the web_search tool to look up accurate information before responding. "+
			"Never refuse gameplay questions — always search and answer. "+
			"Be concise and practical. Always include sources.",
		game, streamTitle, game,
	)

	reqParams := responses.ResponseNewParams{
		Model:        openai.ChatModelGPT4o,
		Instructions: param.NewOpt(instructions),
		Input:        responses.ResponseNewParamsInputUnion{OfString: param.NewOpt(question)},
		Tools: []responses.ToolUnionParam{
			responses.ToolParamOfWebSearchPreview(responses.WebSearchToolTypeWebSearchPreview),
		},
		// Force web_search_preview specifically — prevents the model from skipping the
		// search and replying from training data (which causes refusal responses).
		ToolChoice: responses.ResponseNewParamsToolChoiceUnion{
			OfHostedTool: &responses.ToolChoiceTypesParam{
				Type: responses.ToolChoiceTypesTypeWebSearchPreview,
			},
		},
		Store: param.NewOpt(true),
	}
	if prevID != "" {
		reqParams.PreviousResponseID = param.NewOpt(prevID)
	}

	resp, err := p.client.Responses.New(ctx, reqParams)
	if err != nil {
		// On error, drop the session so a stale ID doesn't poison future requests.
		p.mu.Lock()
		p.gameSessionID = ""
		p.mu.Unlock()
		return nil, fmt.Errorf("game guide: %w", err)
	}

	p.mu.Lock()
	p.gameSessionID = resp.ID
	p.mu.Unlock()

	answer := resp.OutputText()

	// If the model produced no web_search_call items, the search was skipped.
	// Clear the session so the next question starts clean.
	hasSearch := false
	for _, item := range resp.Output {
		if item.Type == "web_search_call" {
			hasSearch = true
			break
		}
	}
	if !hasSearch {
		p.mu.Lock()
		p.gameSessionID = ""
		p.mu.Unlock()
		return nil, fmt.Errorf("web search was not performed — please try again")
	}

	// Extract URL citations from message content annotations.
	seen := map[string]bool{}
	var sources []GameSource
	for _, item := range resp.Output {
		if item.Type == "message" {
			for _, content := range item.Content {
				if content.Type == "output_text" {
					for _, ann := range content.Annotations {
						if ann.Type == "url_citation" && !seen[ann.URL] {
							seen[ann.URL] = true
							sources = append(sources, GameSource{Title: ann.Title, URL: ann.URL})
						}
					}
				}
			}
		}
	}

	return &GameAssistantResult{Answer: answer, Sources: sources}, nil
}

// ClearVoiceSession resets the voice command conversation thread so the next
// command starts a fresh context.
func (p *Processor) ClearVoiceSession() {
	p.mu.Lock()
	p.voiceSessionID = ""
	p.mu.Unlock()
}

// ClearGameSession resets the game guide conversation thread.
func (p *Processor) ClearGameSession() {
	p.mu.Lock()
	p.gameSessionID = ""
	p.mu.Unlock()
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

	case "create_clip":
		if p.handlers.CreateClip == nil {
			return "", fmt.Errorf("create_clip handler not configured")
		}
		if err := p.handlers.CreateClip(); err != nil {
			return "", err
		}
		return "Clip created from the live stream!", nil

	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// SuggestClipTitle uses GPT-4o mini to generate a catchy clip title based on
// the current stream context (title and game/category).
func (p *Processor) SuggestClipTitle(ctx context.Context, streamTitle, gameName string) (string, error) {
	prompt := fmt.Sprintf(
		"You are a Twitch clip title generator. Given the stream context below, suggest a single short, catchy, engaging clip title (max 8 words). Output ONLY the title, no quotes, no explanation.\n\nStream title: %s\nGame/Category: %s",
		streamTitle, gameName,
	)
	resp, err := p.client.Responses.New(ctx, responses.ResponseNewParams{
		Model:           openai.ChatModelGPT4oMini,
		Input:           responses.ResponseNewParamsInputUnion{OfString: param.NewOpt(prompt)},
		MaxOutputTokens: param.NewOpt[int64](30),
		Store:           param.NewOpt(false),
	})
	if err != nil {
		return "", fmt.Errorf("suggest clip title: %w", err)
	}
	return strings.TrimSpace(resp.OutputText()), nil
}

// buildTools returns the set of Responses API tool definitions for Twitch stream management.
func buildTools() []responses.ToolUnionParam {
	return []responses.ToolUnionParam{
		{OfFunction: &responses.FunctionToolParam{
			Name:        "create_poll",
			Description: param.NewOpt("Create a viewer poll on the Twitch channel. Use when the broadcaster wants to start a poll or vote."),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title": map[string]any{
						"type":        "string",
						"description": "The poll question, e.g. 'What game should we play next?'",
					},
					"choices": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"minItems":    2,
						"maxItems":    5,
						"description": "Poll answer choices (2–5 options)",
					},
					"duration_seconds": map[string]any{
						"type":        "integer",
						"description": "How long the poll runs in seconds (15–1800, default 60)",
						"minimum":     15,
						"maximum":     1800,
					},
				},
				"required": []string{"title", "choices"},
			},
			Strict: param.NewOpt(false),
		}},
		{OfFunction: &responses.FunctionToolParam{
			Name:        "start_raid",
			Description: param.NewOpt("Initiate a Twitch raid to another live channel. Use when the broadcaster says 'raid' followed by a channel name."),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"channel_login": map[string]any{
						"type":        "string",
						"description": "The Twitch login name (username) of the channel to raid, e.g. 'pokimane'",
					},
				},
				"required": []string{"channel_login"},
			},
			Strict: param.NewOpt(false),
		}},
		{OfFunction: &responses.FunctionToolParam{
			Name:        "cancel_raid",
			Description: param.NewOpt("Cancel the currently pending raid. Use when the broadcaster says to cancel or stop a raid."),
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
			Strict:      param.NewOpt(false),
		}},
		{OfFunction: &responses.FunctionToolParam{
			Name:        "update_stream_title",
			Description: param.NewOpt("Update the live stream title/name. Use when the broadcaster wants to change what the stream is called."),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title": map[string]any{
						"type":        "string",
						"description": "The new stream title, e.g. 'Playing Minecraft with viewers! !discord'",
					},
				},
				"required": []string{"title"},
			},
			Strict: param.NewOpt(false),
		}},
		{OfFunction: &responses.FunctionToolParam{
			Name:        "update_stream_game",
			Description: param.NewOpt("Change the stream category or game. Use when the broadcaster switches games or wants to update what they're playing."),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"game_name": map[string]any{
						"type":        "string",
						"description": "The name of the game or category, e.g. 'Minecraft', 'Just Chatting', 'Fortnite'",
					},
				},
				"required": []string{"game_name"},
			},
			Strict: param.NewOpt(false),
		}},
		{OfFunction: &responses.FunctionToolParam{
			Name:        "end_poll",
			Description: param.NewOpt("End the currently active poll and display results to viewers. Use when the broadcaster says to end, stop, or close the current poll."),
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
			Strict:      param.NewOpt(false),
		}},
		{OfFunction: &responses.FunctionToolParam{
			Name:        "create_channel_point_reward",
			Description: param.NewOpt("Create a new channel point custom reward that viewers can redeem."),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title": map[string]any{
						"type":        "string",
						"description": "The reward name, e.g. 'Hydration Check'",
					},
					"cost": map[string]any{
						"type":        "integer",
						"description": "Channel point cost for the reward",
						"minimum":     1,
					},
					"prompt": map[string]any{
						"type":        "string",
						"description": "Optional description shown to viewers when redeeming",
					},
				},
				"required": []string{"title", "cost"},
			},
			Strict: param.NewOpt(false),
		}},
		{OfFunction: &responses.FunctionToolParam{
			Name:        "pause_reward",
			Description: param.NewOpt("Pause a channel point reward so viewers cannot redeem it. Use when the broadcaster says to pause, disable, or turn off a reward."),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"reward_title": map[string]any{
						"type":        "string",
						"description": "The title of the reward to pause, e.g. 'Pick My Song'",
					},
				},
				"required": []string{"reward_title"},
			},
			Strict: param.NewOpt(false),
		}},
		{OfFunction: &responses.FunctionToolParam{
			Name:        "resume_reward",
			Description: param.NewOpt("Resume (unpause) a paused channel point reward so viewers can redeem it again. Use when the broadcaster says to resume, enable, or turn on a reward."),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"reward_title": map[string]any{
						"type":        "string",
						"description": "The title of the reward to resume, e.g. 'Pick My Song'",
					},
				},
				"required": []string{"reward_title"},
			},
			Strict: param.NewOpt(false),
		}},
		{OfFunction: &responses.FunctionToolParam{
			Name:        "create_clip",
			Description: param.NewOpt("Create a clip from the current live stream. Use when the broadcaster says 'clip that', 'make a clip', or 'create a clip'."),
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
			Strict:      param.NewOpt(false),
		}},
	}
}
