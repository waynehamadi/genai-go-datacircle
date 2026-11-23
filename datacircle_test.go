package datacircle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/genai"
)

func TestDeclarations(t *testing.T) {
	tools := New(nil)
	decls := tools.Declarations()
	if len(decls) != 4 || len(tools.Tool().FunctionDeclarations) != 4 {
		t.Fatalf("%d declarations, want 4", len(decls))
	}
	decls[0].Parameters.Properties["limit"].Description = "changed"
	if tools.Declarations()[0].Parameters.Properties["limit"].Description == "changed" {
		t.Error("a declaration changed by its caller changed the next ones")
	}
	// As a config holds them, beside the app's own and a built-in tool.
	config := genai.GenerateContentConfig{Tools: []*genai.Tool{tools.Tool(), {GoogleSearch: &genai.GoogleSearch{}}}}
	sent, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Tools []struct {
			FunctionDeclarations []map[string]any `json:"functionDeclarations"`
		} `json:"tools"`
	}
	if json.Unmarshal(sent, &got) != nil || len(got.Tools) != 2 || len(got.Tools[0].FunctionDeclarations) != 4 {
		t.Fatalf("sent %s", sent)
	}
	for i, name := range Names {
		f := got.Tools[0].FunctionDeclarations[i]
		if f["name"] != name || f["description"] != abouts[name] {
			t.Errorf("declaration %d: %+v", i, f)
		}
		p, _ := json.Marshal(f["parameters"])
		switch name {
		case BestPlansName, ProviderName:
			if !strings.Contains(string(p), `"required":["slug"]`) {
				t.Errorf("%s requires no slug: %s", name, p)
			}
		default:
			if strings.Contains(string(p), `"required"`) {
				t.Errorf("%s requires an input: %s", name, p)
			}
		}
		if name == SearchPlansName && !strings.Contains(string(p), `"enum":["score","provider","price"]`) {
			t.Errorf("%s's sort has no enum: %s", name, p)
		}
		if !strings.Contains(string(p), `"limit":{"description":"How many`) || !strings.Contains(string(p), `"type":"OBJECT"`) ||
			!strings.Contains(string(p), `"minimum":1`) || !strings.Contains(string(p), `"type":"INTEGER"`) {
			t.Errorf("%s's parameters: %s", name, p)
		}
	}
}

// run calls a tool on arguments given as JSON, decoded as the SDK decodes a FunctionCall's Args.
func run(tools *Toolset, ctx context.Context, name, arguments string) (string, error) {
	var args map[string]any
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", err
		}
	}
	v, err := tools.Run(ctx, name, args)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(v)
	return string(out), nil
}

func TestCalls(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		switch {
		case r.URL.Path == "/apis":
			w.Write([]byte(`[{"api":"Employee profile","slug":"linkedin-profile","sheetPage":"s"},{"api":"Employee email","slug":"email-finder","plans":3}]`))
		case r.URL.Path == "/apis/email-finder/best":
			w.Write([]byte(`{"api":"Employee email","plans":[{"provider":"A","rank":1,"terms":"long"}]}`))
		case r.URL.Path == "/plans" && (r.URL.Query().Get("api") == "Bright Data" || r.URL.Query().Get("api") == "bright-data"):
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"api must be one of: Employee email"}`))
		case r.URL.Path == "/plans" && strings.Contains(r.URL.Query().Get("query"), "-"):
			w.Write([]byte(`{"total":0,"plans":[]}`))
		case r.URL.Path == "/plans":
			w.Write([]byte(`{"total":1,"plans":[{"provider":"Bright Data","rank":2}]}`))
		case r.URL.Path == "/providers/bright-data":
			w.Write([]byte(`{"provider":"Bright Data","apis":[{"api":"Employee profile","plans":[{"plan":"C"}]},{"api":"Employee email","rank":4,"plans":[{"plan":"B","rank":9},{"plan":"A","rank":4}]}]}`))
		case r.URL.Path == "/providers/down":
			w.WriteHeader(502)
		default:
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"no API has the slug nope"}`))
		}
	}))
	defer server.Close()
	tools := New(&Config{BaseURL: server.URL})
	ctx := context.Background()
	for _, call := range []struct {
		name, arguments string
		fails           bool
		has             string
	}{
		{ListAPIsName, `{"query":"email"}`, false, `"total":1`},
		{ListAPIsName, `{}`, false, `"total":2`},
		{BestPlansName, `{"slug":"Email Finder","limit":"2"}`, false, `"provider":"A"`},
		{SearchPlansName, `{"api":"Bright Data"}`, false, `No API is named`},
		{SearchPlansName, `{"api":"Bright Data","query":"growth"}`, true, `No API is named "Bright Data". datacircle_list_apis gives every API's name and slug.`},
		{SearchPlansName, `{"api":"bright-data","query":null}`, false, `these are the plans whose name holds \"bright data\".","plans":[{"provider":"Bright Data"`},
		{SearchPlansName, `{"query":"bright-data","sort":"price","limit":null}`, false, `"plans":[{"provider":"Bright Data"`},
		{ProviderName, `{"slug":"Bright Data"}`, false, `"apis":[{"api":"Employee email","best":{"plan":"A"`},
		{ProviderName, `{"slug":"bright-data","limit":1}`, false, `"ranked":1,"total":2`},
		{ProviderName, `{"slug":"bright-data","limit":1}`, false, `{"about":"Bright Data has plans on 2 APIs on Datacircle and a rank on 1 of them (1 is the best). Its best rank: 4, on Employee email.","apis":[`},
		{BestPlansName, `{"slug":"nope"}`, true, `no API has the slug nope. datacircle_list_apis gives every API's slug. A provider's plans: datacircle_search_plans`},
		{BestPlansName, `{"provider":"Bright Data","limit":2}`, true, `Give an API's slug, like email-finder: datacircle_list_apis gives every slug. A provider's plans:`},
		{ListAPIsName, `{"query":"Bright Data"}`, false, `"note":"No API's name holds every word of \"Bright Data\". A provider's plans: datacircle_search_plans`},
		{ProviderName, `{"slug":"down"}`, true, `datacircle: 502 Bad Gateway`},
		{BestPlansName, `{"slug":"email-finder","limit":[2]}`, true, `The arguments are no JSON object of this tool's parameters`},
		{ProviderName, `{"slug":{"name":"x"}}`, true, `The arguments are no JSON object of this tool's parameters`},
		{BestPlansName, `{"slug":"email-finder","limit":2.0,"extra":true}`, false, `"provider":"A"`},
		{ProviderName, `{"slug":"nope"}`, true, `no API has the slug nope. datacircle_search_plans with words of its name as query gives each plan's provider.`},
		{ListAPIsName, ``, false, `"total":2`},
		{"get_weather", `{}`, true, `datacircle: no tool is named "get_weather"`},
	} {
		got, err := run(tools, ctx, call.name, call.arguments)
		if err != nil {
			got = err.Error()
		}
		if (err != nil) != call.fails || !strings.Contains(got, call.has) {
			t.Errorf("%s(%s) = %s, %v; want it to hold %s", call.name, call.arguments, got, err, call.has)
		}
		if strings.Contains(got, "terms") || strings.Contains(got, "sheetPage") {
			t.Errorf("%s gave a key it drops: %s", call.name, got)
		}
	}
	got, _ := run(tools, ctx, ListAPIsName, `{"query":"employee email"}`)
	if strings.Contains(got, "note") {
		t.Errorf("datacircle_list_apis notes a query an API's name holds every word of: %s", got)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := tools.Run(canceled, ListAPIsName, nil); err != context.Canceled {
		t.Errorf("a canceled call gave %v", err)
	}
	if want := "/apis/email-finder/best?limit=2"; paths[2] != want {
		t.Errorf("called %s, want %s", paths[2], want)
	}
	if want := "/plans?query=bright+data&sort=price"; paths[10] != want {
		t.Errorf("called %s, want %s", paths[10], want)
	}

	// A response's function calls, decoded as the SDK decodes them, into the parts that answer them.
	var response genai.GenerateContentResponse
	if err := json.Unmarshal([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"Let me look."},
		{"functionCall":{"id":"c1","name":"datacircle_best_plans","args":{"slug":"email-finder"}}},
		{"functionCall":{"id":"c2","name":"get_weather","args":{}}},
		{"functionCall":{"name":"datacircle_best_plans","args":{"slug":"nope"}}},
		{"functionCall":{"id":"c4","name":"datacircle_provider","args":{"slug":"down"}}}]}}]}`), &response); err != nil {
		t.Fatal(err)
	}
	parts := tools.Responses(ctx, response.FunctionCalls())
	turn, err := json.Marshal(genai.NewContentFromParts(parts, genai.RoleUser))
	if want := `{"parts":[{"functionResponse":{"id":"c1","name":"datacircle_best_plans","response":{"api":"Employee email","plans":[{"provider":"A","rank":1}]}}},` +
		`{"functionResponse":{"name":"datacircle_best_plans","response":{"error":"no API has the slug nope. datacircle_list_apis gives every API's slug. A provider's plans: datacircle_search_plans with its name as query, like bright data, or datacircle_provider."}}},` +
		`{"functionResponse":{"id":"c4","name":"datacircle_provider","response":{"error":"datacircle: 502 Bad Gateway"}}}],"role":"user"}`; err != nil || string(turn) != want {
		t.Errorf("Responses gave %s, %v; want %s", turn, err, want)
	}
	if part, ok := tools.Response(ctx, nil); ok || part != nil {
		t.Errorf("Response(nil) = %v, %v", part, ok)
	}
	if part, ok := tools.Response(ctx, &genai.FunctionCall{Name: ListAPIsName}); !ok || part.FunctionResponse.Response["total"] != 2 {
		t.Errorf("a call with no Args gave %+v", part.FunctionResponse)
	}
}

// TestLive calls each tool once on the API itself: DATACIRCLE_LIVE=1 go test -run Live.
func TestLive(t *testing.T) {
	if os.Getenv("DATACIRCLE_LIVE") == "" {
		t.Skip("DATACIRCLE_LIVE is unset")
	}
	tools := New(nil)
	ctx := context.Background()
	for _, call := range []struct{ name, arguments, has string }{
		{ListAPIsName, `{"query":"email finder","limit":3}`, `"slug":"email-finder"`},
		{BestPlansName, `{"slug":"email-finder","limit":3}`, `"rank":1`},
		{SearchPlansName, `{"query":"bright data","sort":"price","limit":3}`, `"provider":"Bright Data"`},
		{ProviderName, `{"slug":"Bright Data","limit":3}`, `"about":"Bright Data has plans on`},
		{BestPlansName, `{"slug":"bright-data"}`, `"error":`},
	} {
		var args map[string]any
		json.Unmarshal([]byte(call.arguments), &args)
		part, ok := tools.Response(ctx, &genai.FunctionCall{ID: "live", Name: call.name, Args: args})
		sent, _ := json.Marshal(part)
		if !ok || !strings.Contains(string(sent), call.has) || !strings.Contains(string(sent), `"id":"live"`) {
			t.Errorf("%s(%s) = %s; want it to hold %s", call.name, call.arguments, sent, call.has)
		}
		t.Logf("%s(%s) = %.300s", call.name, call.arguments, sent)
	}
}
