package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/arkhayds/axolot/internal/provider"
)

// toolRoundCLI is a CLI script asking for the weather tool, then answering with what the bridge's MCP server returns for it.
// @param wait - shell run before the tool call, to let the results come first
func toolRoundCLI(wait string) string {
	return `config=$(sed -n '/^--mcp-config$/{n;p;}' "$(dirname "$0")/args")
url=$(printf '%s' "$config" | sed 's/.*"url":"\([^"]*\)".*/\1/')
token=$(printf '%s' "$config" | sed 's/.*"Bearer \([^"]*\)".*/\1/')
cat <<'EOF'
{"type":"system","subtype":"init","tools":["mcp__axolot__weather"]}
{"type":"assistant","message":{"content":[{"type":"thinking","thinking":""},{"type":"tool_use","id":"toolu_1","name":"mcp__axolot__weather","input":{"city":"Paris"}}]}}
{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":100,"cache_read_input_tokens":20,"output_tokens":7}}}
EOF
` + wait + `
reply=$(wget -q -O - --header 'Content-Type: application/json' --header 'Accept: application/json, text/event-stream' \
	--header "Authorization: Bearer $token" \
	--post-data '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"weather","arguments":{"city":"Paris"},"_meta":{"claudecode/toolUseId":"toolu_1"}}}' "$url")
text=$(printf '%s' "$reply" | sed 's/.*"text":"\([^"]*\)".*/\1/')
printf '{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"Il fait %s"}}}\n' "$text"
sleep 0.05
cat <<'EOF'
{"type":"result","is_error":false,"result":"Il fait 21 C","usage":{"input_tokens":230,"cache_read_input_tokens":40,"output_tokens":12}}
EOF`
}

func TestChatHandsToolRoundsToAxolot(t *testing.T) {
	for name, wait := range map[string]string{"call first": "", "results first": "sleep 0.5"} {
		t.Run(name, func(t *testing.T) {
			server, dir := startBridge(t, toolRoundCLI(wait))
			question := user("Quel temps à Paris ?")
			tools := []provider.Tool{{Name: "weather", Description: "Météo", Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)}}

			round, err := client(server).Chat(context.Background(), provider.ChatRequest{Model: testModel, Messages: []provider.Message{question}, Tools: tools}, func(string) {})
			if err != nil {
				t.Fatal(err)
			}
			want := provider.ToolCall{ID: "toolu_1", Name: "weather", Arguments: `{"city":"Paris"}`}
			if len(round.ToolCalls) != 1 || round.ToolCalls[0] != want {
				t.Fatalf("tool calls = %+v", round.ToolCalls)
			}
			if *round.PromptTokens != 120 || *round.OutputTokens != 7 || round.CLIStartupMs == nil {
				t.Errorf("round usage = %d/%d, startup = %v", *round.PromptTokens, *round.OutputTokens, round.CLIStartupMs)
			}
			if args := readRecorded(t, dir, "args"); !strings.Contains(args, "--allowedTools\nmcp__axolot\n") {
				t.Errorf("args = %s", args)
			}

			messages := []provider.Message{question, {Role: "assistant", ToolCalls: round.ToolCalls}, {Role: "tool", Content: "21 C", ToolCallID: "toolu_1"}}
			deltas, final, err := ask(t, server, messages...)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(deltas, "|") != "Il fait 21 C" {
				t.Errorf("deltas = %v", deltas)
			}
			if *final.PromptTokens != 150 || *final.OutputTokens != 5 || final.CLIStartupMs != nil {
				t.Errorf("final usage = %d/%d, startup = %v", *final.PromptTokens, *final.OutputTokens, final.CLIStartupMs)
			}
			if final.TokPerSec != nil {
				t.Errorf("rate = %v, want none: the CLI sends the text after a tool result in one block", *final.TokPerSec)
			}
		})
	}
}

// twoRoundsCLI is a CLI script calling the weather tool for Paris, then for Lyon, then answering with both results.
const twoRoundsCLI = `config=$(sed -n '/^--mcp-config$/{n;p;}' "$(dirname "$0")/args")
url=$(printf '%s' "$config" | sed 's/.*"url":"\([^"]*\)".*/\1/')
token=$(printf '%s' "$config" | sed 's/.*"Bearer \([^"]*\)".*/\1/')
call() {
	wget -q -O - --header 'Content-Type: application/json' --header 'Accept: application/json, text/event-stream' \
		--header "Authorization: Bearer $token" \
		--post-data "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"weather\",\"arguments\":{},\"_meta\":{\"claudecode/toolUseId\":\"$1\"}}}" "$url" |
		sed 's/.*"text":"\([^"]*\)".*/\1/'
}
cat <<'EOF'
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_1","name":"mcp__axolot__weather","input":{"city":"Paris"}}]}}
{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":100,"cache_read_input_tokens":20,"output_tokens":7}}}
EOF
paris=$(call toolu_1)
cat <<'EOF'
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_2","name":"mcp__axolot__weather","input":{"city":"Lyon"}}]}}
{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":50,"cache_read_input_tokens":10,"output_tokens":4}}}
EOF
lyon=$(call toolu_2)
printf '{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"%s, %s"}}}\n' "$paris" "$lyon"
cat <<'EOF'
{"type":"result","is_error":false,"result":"21 C, 18 C","usage":{"input_tokens":230,"cache_read_input_tokens":40,"output_tokens":12}}
EOF`

func TestSecondToolRoundCarriesOnlyItsCalls(t *testing.T) {
	server, _ := startBridge(t, twoRoundsCLI)
	question := user("Quel temps à Paris et à Lyon ?")
	first, err := client(server).Chat(context.Background(), provider.ChatRequest{Model: testModel, Messages: []provider.Message{question}, Tools: []provider.Tool{{Name: "weather"}}}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	messages := []provider.Message{question, {Role: "assistant", ToolCalls: first.ToolCalls}, {Role: "tool", Content: "21 C", ToolCallID: "toolu_1"}}
	_, second, err := ask(t, server, messages...)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.ToolCalls) != 1 || second.ToolCalls[0].ID != "toolu_2" {
		t.Fatalf("second round calls = %+v", second.ToolCalls)
	}
	if *second.PromptTokens != 60 || *second.OutputTokens != 4 || second.CLIStartupMs != nil {
		t.Errorf("second round usage = %d/%d, startup = %v", *second.PromptTokens, *second.OutputTokens, second.CLIStartupMs)
	}

	messages = append(messages, provider.Message{Role: "assistant", ToolCalls: second.ToolCalls}, provider.Message{Role: "tool", Content: "18 C", ToolCallID: "toolu_2"})
	deltas, final, err := ask(t, server, messages...)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(deltas, "|") != "21 C, 18 C" {
		t.Errorf("deltas = %v", deltas)
	}
	if *final.PromptTokens != 90 || *final.OutputTokens != 1 {
		t.Errorf("final usage = %d/%d, want the result minus both rounds", *final.PromptTokens, *final.OutputTokens)
	}
}

func TestToolResultsResumeOnlyOnce(t *testing.T) {
	server, _ := startBridge(t, toolRoundCLI(""))
	question := user("Quel temps à Paris ?")
	round, err := client(server).Chat(context.Background(), provider.ChatRequest{Model: testModel, Messages: []provider.Message{question}, Tools: []provider.Tool{{Name: "weather"}}}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	messages := []provider.Message{question, {Role: "assistant", ToolCalls: round.ToolCalls}, {Role: "tool", Content: "21 C", ToolCallID: "toolu_1"}}
	if _, _, err := ask(t, server, messages...); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ask(t, server, messages...); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("second resume err = %v", err)
	}
}
