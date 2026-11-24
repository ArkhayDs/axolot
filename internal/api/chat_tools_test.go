package api

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/arkhayds/axolot/internal/provider"
	"github.com/arkhayds/axolot/internal/secret"
	"github.com/arkhayds/axolot/internal/store"
)

// toolboxServer is a Server able to open sealed tokens, with the sealed token "secret".
func toolboxServer(t *testing.T) (*Server, []byte) {
	t.Helper()
	box, err := secret.NewBox("test")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := box.Seal("secret")
	if err != nil {
		t.Fatal(err)
	}
	return &Server{cfg: Config{Box: box}, oauthSources: map[int64]*oauthSource{}}, sealed
}

func TestToolboxOffersEnabledServersToolsUnderTheirName(t *testing.T) {
	ts := mcpServerWithToken(t)
	s, sealed := toolboxServer(t)

	tools := s.openToolbox(context.Background(), []store.McpServer{
		{Name: "sentinelle", URL: ts.URL, Enabled: true, TokenEnc: sealed},
		{Name: "off", URL: ts.URL, Enabled: false, TokenEnc: sealed},
	})
	defer tools.close()

	if len(tools.tools) != 1 || tools.tools[0].Name != "sentinelle__get_kpis" || len(tools.warnings) != 0 {
		t.Fatalf("tools=%+v warnings=%v", tools.tools, tools.warnings)
	}
	got, err := tools.call(context.Background(), "sentinelle__get_kpis", `{}`)
	if err != nil || got != "kpis" {
		t.Fatalf("got %q, err %v", got, err)
	}
	if _, err := tools.call(context.Background(), "get_kpis", `{}`); err == nil {
		t.Fatal("unknown tool called")
	}
}

func TestToolboxWarnsAboutServersItCannotOpen(t *testing.T) {
	ts := mcpServerWithToken(t)
	s, sealed := toolboxServer(t)

	tools := s.openToolbox(context.Background(), []store.McpServer{
		{Name: "ok", URL: ts.URL, Enabled: true, TokenEnc: sealed},
		{Name: "down", URL: ts.URL, Enabled: true},
		{Name: "lost", URL: ts.URL, Enabled: true, OAuthEnc: []byte("garbage")},
	})
	defer tools.close()

	want := []string{"down : Jeton refusé ou manquant, outils absents", "lost : connexion requise, outils absents"}
	if !slices.Equal(tools.warnings, want) || len(tools.tools) != 1 {
		t.Fatalf("tools=%+v warnings=%v", tools.tools, tools.warnings)
	}
}

// scripted answers each Chat call with the next result or error, repeating the last result once exhausted.
type scripted struct {
	results []provider.ChatResult
	errs    []error
	reqs    []provider.ChatRequest
}

func (p *scripted) Chat(_ context.Context, req provider.ChatRequest, onDelta func(string)) (provider.ChatResult, error) {
	i := len(p.reqs)
	p.reqs = append(p.reqs, req)
	if i < len(p.errs) && p.errs[i] != nil {
		return provider.ChatResult{}, p.errs[i]
	}
	result := p.results[min(i, len(p.results)-1)]
	if result.Output != "" {
		onDelta(result.Output)
	}
	return result, nil
}

func (p *scripted) Embed(context.Context, string, []string) ([][]float32, error) { return nil, nil }

func (p *scripted) Models(context.Context) ([]provider.Model, error) { return nil, nil }

// recorder collects the streamed events.
func recorder() (*[]map[string]any, func(map[string]any)) {
	events := &[]map[string]any{}
	return events, func(e map[string]any) { *events = append(*events, e) }
}

// question is a turn with a single user message.
var question = provider.ChatRequest{Model: "m", Messages: []provider.Message{{Role: "user", Content: "KPIs ?"}}}

func TestAnswerRunsToolsThenAnswers(t *testing.T) {
	ts := mcpServerWithToken(t)
	s, sealed := toolboxServer(t)
	tools := s.openToolbox(context.Background(), []store.McpServer{{Name: "sentinelle", URL: ts.URL, Enabled: true, TokenEnc: sealed}})
	defer tools.close()
	ttft := int64(5)
	model := &scripted{results: []provider.ChatResult{
		{ToolCalls: []provider.ToolCall{{ID: "c1", Name: "sentinelle__get_kpis", Arguments: `{}`}}},
		{Output: "42", TTFTMs: &ttft},
	}}
	events, send := recorder()

	result, err := answer(context.Background(), model, question, tools, send)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.reqs) != 2 || len(model.reqs[0].Tools) != 1 {
		t.Fatalf("requests=%+v", model.reqs)
	}
	sent := model.reqs[1].Messages
	call, reply := sent[len(sent)-2], sent[len(sent)-1]
	if call.Role != "assistant" || len(call.ToolCalls) != 1 || reply.Role != "tool" || reply.Content != "kpis" || reply.ToolCallID != "c1" {
		t.Fatalf("messages=%+v", sent)
	}
	var types []string
	for _, e := range *events {
		types = append(types, e["type"].(string))
	}
	if !slices.Equal(types, []string{"tool_call", "tool_result", "delta"}) || (*events)[1]["result"] != "kpis" || (*events)[1]["error"] != false {
		t.Fatalf("events=%v", *events)
	}
	if result.TTFTMs == nil || *result.TTFTMs < ttft {
		t.Fatalf("ttft=%v", result.TTFTMs)
	}
}

func TestAnswerReturnsToolFailuresToTheModel(t *testing.T) {
	tools := &toolbox{tools: []provider.Tool{{Name: "srv__get_kpis"}}, routes: map[string]toolRoute{}}
	model := &scripted{results: []provider.ChatResult{
		{ToolCalls: []provider.ToolCall{{ID: "c1", Name: "nope", Arguments: `{}`}}},
		{Output: "Désolé"},
	}}
	events, send := recorder()

	if _, err := answer(context.Background(), model, question, tools, send); err != nil {
		t.Fatal(err)
	}
	sent := model.reqs[1].Messages
	if reply := sent[len(sent)-1]; reply.Role != "tool" || !strings.HasPrefix(reply.Content, "Error: ") {
		t.Fatalf("reply=%+v", reply)
	}
	if (*events)[1]["error"] != true {
		t.Fatalf("events=%v", *events)
	}
}

func TestAnswerStopsAfterTheRoundCap(t *testing.T) {
	tools := &toolbox{tools: []provider.Tool{{Name: "srv__get_kpis"}}, routes: map[string]toolRoute{}}
	model := &scripted{results: []provider.ChatResult{{ToolCalls: []provider.ToolCall{{ID: "c", Name: "srv__get_kpis"}}}}}
	events, send := recorder()

	if _, err := answer(context.Background(), model, question, tools, send); err == nil || len(model.reqs) != chatMaxRounds {
		t.Fatalf("err=%v after %d calls", err, len(model.reqs))
	}
	if len(*events) != 2*(chatMaxRounds-1) {
		t.Fatalf("the last round's tools ran: %d events", len(*events))
	}
}

func TestRefusesTools(t *testing.T) {
	for _, msg := range []string{
		`{"error":"gemma3:4b does not support tools"}`,
		`"auto" tool choice requires --enable-auto-tool-choice and --tool-call-parser to be set`,
		`tools param requires --jinja flag`,
	} {
		if !refusesTools(errors.New(msg)) {
			t.Errorf("not seen as a refusal: %s", msg)
		}
	}
	if refusesTools(errors.New("connection refused")) {
		t.Error("network error seen as a refusal")
	}
}

func TestAnswerAsksAgainWithoutToolsWhenTheModelHasNone(t *testing.T) {
	tools := &toolbox{tools: []provider.Tool{{Name: "srv__get_kpis"}}, routes: map[string]toolRoute{}}
	model := &scripted{
		errs:    []error{errors.New(`/api/chat returned 400: {"error":"gemma3:4b does not support tools"}`)},
		results: []provider.ChatResult{{}, {Output: "Bonjour"}},
	}
	events, send := recorder()

	if _, err := answer(context.Background(), model, question, tools, send); err != nil {
		t.Fatal(err)
	}
	if len(model.reqs) != 2 || model.reqs[1].Tools != nil {
		t.Fatalf("requests=%+v", model.reqs)
	}
	if first := (*events)[0]; first["type"] != "notice" || first["text"] != "Ce modèle n'appelle pas d'outils" {
		t.Fatalf("events=%v", *events)
	}
}

func TestOfferedName(t *testing.T) {
	long := strings.Repeat("a", 32)
	for tool, want := range map[string]string{
		"get_kpis":              "sentinelle__get_kpis",
		"kpis.get":              "sentinelle__kpis_get",
		"météo":                 "sentinelle__m_t_o",
		strings.Repeat("b", 60): "sentinelle__" + strings.Repeat("b", 52),
	} {
		if got := offeredName("sentinelle", tool); got != want {
			t.Errorf("%q: got %q", tool, got)
		}
	}
	if got := offeredName(long, long); len(got) != 64 {
		t.Errorf("longest name has %d characters", len(got))
	}
}

func TestCutToolResult(t *testing.T) {
	atLimit := strings.Repeat("é", toolResultMax)
	if got := cutToolResult(atLimit); got != atLimit {
		t.Error("result at the limit changed")
	}
	got := cutToolResult(atLimit + "é")
	if !strings.HasPrefix(got, atLimit) || !strings.HasSuffix(got, toolCutNotice) || !utf8.ValidString(got) {
		t.Errorf("cut result ends with %q", got[len(got)-80:])
	}
}
