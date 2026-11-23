package datacircle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/genai"
)

const (
	ListAPIsName    = "datacircle_list_apis"
	BestPlansName   = "datacircle_best_plans"
	SearchPlansName = "datacircle_search_plans"
	ProviderName    = "datacircle_provider"

	listAPIsAbout    = "Lists the GTM APIs on Datacircle's sheet (https://datacircle.dev), with each API's slug, how many plans and providers it has and its best plan. Call it for an API's slug you don't know: datacircle_best_plans takes a slug, like email-finder. A provider's plans are in datacircle_search_plans and datacircle_provider."
	bestPlansAbout   = "Gives an API's plans best first by their score out of 100 on Datacircle's benchmark (https://datacircle.dev), each with its rank, provider, plan, $ per 1,000 calls and its page. perThousand is a plan's price of 1,000 items in dollars, pay the plan's whole price in its currency."
	searchPlansAbout = "Searches every plan on Datacircle's sheet (https://datacircle.dev), or one API's, by provider or plan name, sorted by score, provider or price. Gives how many were found and up to limit of them. perThousand is a plan's price of 1,000 items in dollars, pay the plan's whole price in its currency."
	providerAbout    = "Gives the APIs one provider sells and its rank on each on Datacircle's benchmark (https://datacircle.dev), with its best plan there, its best rank first. total counts the APIs it sells, ranked those where it has a rank (its place among the API's providers, 1 the best)."
)

// ListAPIsInput is datacircle_list_apis' input.
type ListAPIsInput struct {
	Query Text  `json:"query,omitempty"`
	Limit Count `json:"limit,omitempty"`
}

// BestPlansInput is datacircle_best_plans' input.
type BestPlansInput struct {
	Slug  Text  `json:"slug"`
	Limit Count `json:"limit,omitempty"`
}

// SearchPlansInput is datacircle_search_plans' input.
type SearchPlansInput struct {
	API    Text  `json:"api,omitempty"`
	Query  Text  `json:"query,omitempty"`
	Sort   Text  `json:"sort,omitempty"`
	Limit  Count `json:"limit,omitempty"`
	Offset Count `json:"offset,omitempty"`
}

// ProviderInput is datacircle_provider's input.
type ProviderInput struct {
	Slug  Text  `json:"slug"`
	Limit Count `json:"limit,omitempty"`
}

// Count is a whole number a model may send as a number, as text ("3") or as null, which is 0: left out.
type Count int

func (n *Count) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch v := v.(type) {
	case float64:
		*n = Count(v)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(v))
		*n = Count(i)
	case nil:
		*n = 0
	default:
		return errors.New("a whole number")
	}
	return nil
}

// Text is a string a model may send as text, as a number (a slug like 6sense sent as 6) or as null, which is "": left out.
type Text string

func (s *Text) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch v := v.(type) {
	case string:
		*s = Text(v)
	case float64:
		*s = Text(strconv.FormatFloat(v, 'f', -1, 64))
	case nil:
		*s = ""
	default:
		return errors.New("text")
	}
	return nil
}

func text(about string) *genai.Schema {
	return &genai.Schema{Type: genai.TypeString, Description: about}
}

func count(low, high float64, about string) *genai.Schema {
	field := &genai.Schema{Type: genai.TypeInteger, Description: about, Minimum: &low}
	if high > 0 {
		field.Maximum = &high
	}
	return field
}

// parameters is a tool's input as a Gemini Schema, what the model is sent, the same on every release of the SDK and
// both its backends (the Gemini API and Vertex AI). Each call makes it anew, for the caller to change.
func parameters(name string) *genai.Schema {
	var p *genai.Schema
	switch name {
	case ListAPIsName:
		p = &genai.Schema{Properties: map[string]*genai.Schema{
			"query": text("Words of the API's name or slug, like email or linkedin profile: the APIs that hold the most of them come first. The sheet's order when left out"),
			"limit": count(1, 200, "How many APIs to give, 1 to 200. 10 when left out: the sheet has about 150"),
		}}
	case BestPlansName:
		p = &genai.Schema{Properties: map[string]*genai.Schema{
			"slug":  text("The API's slug, like email-finder or linkedin-profile, as datacircle_list_apis gives it"),
			"limit": count(1, 50, "How many plans to give, 1 to 50. 10 when left out"),
		}, Required: []string{"slug"}}
	case SearchPlansName:
		p = &genai.Schema{Properties: map[string]*genai.Schema{
			"api":   text("Only this API's plans: its name or slug, like email-finder. A provider's name goes in query, never here. Every API when left out"),
			"query": text("Words that must all appear in the provider's or plan's name, like bright data"),
			"sort": {Type: genai.TypeString, Enum: []string{"score", "provider", "price"},
				Description: "score (best first, the default), provider (A to Z) or price ($ per 1,000, lowest first)"},
			"limit":  count(1, 50, "How many plans to give, 1 to 50. 10 when left out"),
			"offset": count(0, 0, "How many found plans to skip first. 0 when left out"),
		}}
	case ProviderName:
		p = &genai.Schema{Properties: map[string]*genai.Schema{
			"slug":  text("The provider's slug, like bright-data, or its name, like Bright Data: the name is made a slug"),
			"limit": count(1, 200, "How many of its APIs to give, its best rank first, 1 to 200. 10 when left out"),
		}, Required: []string{"slug"}}
	}
	p.Type = genai.TypeObject
	return p
}

var abouts = map[string]string{ListAPIsName: listAPIsAbout, BestPlansName: bestPlansAbout, SearchPlansName: searchPlansAbout,
	ProviderName: providerAbout}

// Names are the four tools' names, in the order Declarations gives them.
var Names = []string{ListAPIsName, BestPlansName, SearchPlansName, ProviderName}

// Toolset runs the four tools: datacircle_list_apis, datacircle_best_plans, datacircle_search_plans and
// datacircle_provider. Its methods are safe to call from many goroutines.
type Toolset struct {
	c *client
}

// New gives the four tools, calling where cfg says: nil calls BaseURL with http.DefaultClient.
func New(cfg *Config) *Toolset { return &Toolset{c: newClient(cfg)} }

// Declarations gives the four tools' function declarations: each one's name, description and parameters (a Gemini
// Schema), new on each call. Put them in a genai.Tool's FunctionDeclarations, beside your own.
func (t *Toolset) Declarations() []*genai.FunctionDeclaration {
	out := make([]*genai.FunctionDeclaration, 0, len(Names))
	for _, name := range Names {
		out = append(out, &genai.FunctionDeclaration{Name: name, Description: abouts[name], Parameters: parameters(name)})
	}
	return out
}

// Tool gives a genai.Tool of the four declarations, for a GenerateContentConfig's Tools.
func (t *Toolset) Tool() *genai.Tool { return &genai.Tool{FunctionDeclarations: t.Declarations()} }

// Handles says whether name is one of the four tools'.
func (t *Toolset) Handles(name string) bool { return abouts[name] != "" }

// Run calls one tool by its name on the model's arguments (a FunctionCall's Args) and gives its result, a JSON
// object. A call the API answers with no data, like for a slug it doesn't know, or one the tool can't make, gives an
// error whose words tell the model what to send instead.
func (t *Toolset) Run(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	if !t.Handles(name) {
		return nil, fmt.Errorf("datacircle: no tool is named %q", name)
	}
	if args == nil {
		args = map[string]any{}
	}
	arguments, err := json.Marshal(args)
	if err != nil {
		return nil, advice("The arguments are no JSON object of this tool's parameters: " + err.Error())
	}
	var v map[string]any
	switch name {
	case ListAPIsName:
		var in ListAPIsInput
		if err = decode(arguments, &in); err == nil {
			v, err = listAPIs(ctx, t.c, in)
		}
	case BestPlansName:
		var in BestPlansInput
		if err = decode(arguments, &in); err == nil {
			v, err = bestPlans(ctx, t.c, in)
		}
	case SearchPlansName:
		var in SearchPlansInput
		if err = decode(arguments, &in); err == nil {
			v, err = searchPlans(ctx, t.c, in)
		}
	case ProviderName:
		var in ProviderInput
		if err = decode(arguments, &in); err == nil {
			v, err = provider(ctx, t.c, in)
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return v, nil
}

// Response runs a function call of the model's into the part that answers it, a FunctionResponse of the call's ID
// and name: its result, or {"error": ...} with the API's words or the failure's, which the model reads and can call
// again on. Its false is a call of none of the four tools, for the app to run.
func (t *Toolset) Response(ctx context.Context, call *genai.FunctionCall) (*genai.Part, bool) {
	if call == nil || !t.Handles(call.Name) {
		return nil, false
	}
	out, err := t.Run(ctx, call.Name, call.Args)
	if err != nil {
		out = map[string]any{"error": err.Error()}
	}
	return &genai.Part{FunctionResponse: &genai.FunctionResponse{ID: call.ID, Name: call.Name, Response: out}}, true
}

// Responses runs the function calls of the four tools among calls (a response's FunctionCalls()), in their order,
// and gives the part that answers each, for the next user turn. A call of another function gives none: the app runs
// it and adds its own part.
func (t *Toolset) Responses(ctx context.Context, calls []*genai.FunctionCall) []*genai.Part {
	out := []*genai.Part{}
	for _, call := range calls {
		if part, ok := t.Response(ctx, call); ok {
			out = append(out, part)
		}
	}
	return out
}

// decode reads a model's arguments; arguments it can't read are an error for the model.
func decode(arguments []byte, in any) error {
	if err := json.Unmarshal(arguments, in); err != nil {
		return advice("The arguments are no JSON object of this tool's parameters: " + err.Error())
	}
	return nil
}

// advice is a message for the model, like what to send instead: an error it reads.
type advice string

func (a advice) Error() string { return string(a) }

func listAPIs(ctx context.Context, c *client, in ListAPIsInput) (map[string]any, error) {
	var apis []map[string]any
	if err := c.get(ctx, "/apis", nil, &apis); err != nil {
		return nil, err
	}
	query := strings.TrimSpace(string(in.Query))
	words := strings.Fields(strings.ReplaceAll(strings.ToLower(query), "-", " "))
	type held struct {
		matched int
		api     map[string]any
	}
	var found []held
	for _, api := range apis {
		name := strings.ToLower(str(api["api"]) + " " + strings.ReplaceAll(str(api["slug"]), "-", " "))
		matched := 0
		for _, w := range words {
			if strings.Contains(name, w) {
				matched++
			}
		}
		if matched > 0 || len(words) == 0 {
			found = append(found, held{matched, brief(api, []string{"api", "slug", "plans", "providers", "best", "bestPage"})})
		}
	}
	// The most words matched first, the sheet's order within each.
	sort.SliceStable(found, func(i, j int) bool { return found[i].matched > found[j].matched })
	limit := 10
	if in.Limit > 0 {
		limit = clamp(int(in.Limit), 1, 200)
	}
	out := []map[string]any{}
	for i := 0; i < len(found) && i < limit; i++ {
		out = append(out, found[i].api)
	}
	v := map[string]any{"total": len(found), "apis": out}
	if len(words) > 0 && (len(found) == 0 || found[0].matched < len(words)) {
		// A provider's name sent as an API's, like Bright Data, whose data matches many APIs: where its plans are.
		v["note"] = "No API's name holds every word of " + strconv.Quote(query) + ". A provider's plans: datacircle_search_plans with its name as query, or datacircle_provider."
	}
	return v, nil
}

// forProvider tells a model that sent a provider's name to an API's tool where a provider's plans are.
const forProvider = "A provider's plans: datacircle_search_plans with its name as query, like bright data, or datacircle_provider."

func bestPlans(ctx context.Context, c *client, in BestPlansInput) (map[string]any, error) {
	slug := slugify(string(in.Slug))
	if slug == "" {
		return nil, advice("Give an API's slug, like email-finder: datacircle_list_apis gives every slug. " + forProvider)
	}
	q := url.Values{}
	if in.Limit > 0 {
		q.Set("limit", strconv.Itoa(clamp(int(in.Limit), 1, 50)))
	}
	var v map[string]any
	if err := c.get(ctx, "/apis/"+slug+"/best", q, &v); err != nil {
		if e, ok := err.(*apiError); ok && e.status == 404 {
			// A provider's name sent as the API's slug: the API's words, and where a provider's plans are.
			return nil, advice(e.message + ". datacircle_list_apis gives every API's slug. " + forProvider)
		}
		return nil, err
	}
	v["plans"] = briefPlans(plans(v["plans"]))
	return v, nil
}

func searchPlans(ctx context.Context, c *client, in SearchPlansInput) (map[string]any, error) {
	api, query, note := strings.TrimSpace(string(in.API)), strings.TrimSpace(string(in.Query)), ""
	sortBy, limit, offset := strings.ToLower(strings.TrimSpace(string(in.Sort))), int(in.Limit), int(in.Offset)
	v, err := search(ctx, c, api, query, sortBy, limit, offset)
	if e, ok := err.(*apiError); ok && e.status == 400 && strings.Contains(e.message, "api must be") {
		if query != "" {
			// The API's answer names all 150 APIs: where they are instead.
			return nil, advice("No API is named " + strconv.Quote(api) + ". datacircle_list_apis gives every API's name and slug.")
		}
		// A provider's name sent as the API: the plans whose provider or plan name holds it.
		api, query = "", api
		v, err = search(ctx, c, api, query, sortBy, limit, offset)
		note = "No API is named " + strconv.Quote(query) + ": these are the plans whose provider or plan name holds it."
	}
	if total, _ := v["total"].(float64); err == nil && total == 0 && strings.Contains(query, "-") {
		// A slug sent as a name, like bright-data, which no name holds: its words, like bright data. A name with a hyphen,
		// like A-Leads, is found as it is first.
		words := strings.Join(strings.Fields(strings.ReplaceAll(query, "-", " ")), " ")
		if again, err := search(ctx, c, api, words, sortBy, limit, offset); err == nil {
			v = again
			note = strings.TrimSpace(note + " No provider's or plan's name holds " + strconv.Quote(query) + ": these are the plans whose name holds " + strconv.Quote(words) + ".")
		}
	}
	if err != nil {
		return nil, err
	}
	if note != "" {
		v["note"] = note
	}
	v["plans"] = briefPlans(plans(v["plans"]))
	return v, nil
}

func search(ctx context.Context, c *client, api, query, sort string, limit, offset int) (map[string]any, error) {
	q := url.Values{}
	if api != "" {
		q.Set("api", api)
	}
	if query != "" {
		q.Set("query", query)
	}
	if sort == "score" || sort == "provider" || sort == "price" {
		q.Set("sort", sort)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(clamp(limit, 1, 50)))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	var v map[string]any
	err := c.get(ctx, "/plans", q, &v)
	return v, err
}

func provider(ctx context.Context, c *client, in ProviderInput) (map[string]any, error) {
	slug := slugify(string(in.Slug))
	if slug == "" {
		return nil, advice("Give a provider's slug or name, like bright-data or Bright Data")
	}
	var v map[string]any
	if err := c.get(ctx, "/providers/"+slug, nil, &v); err != nil {
		if e, ok := err.(*apiError); ok && e.status == 404 {
			// The API's words name its own address: the tool that finds a provider's name instead.
			return nil, advice(e.message + ". datacircle_search_plans with words of its name as query gives each plan's provider.")
		}
		return nil, err
	}
	apis := []map[string]any{}
	for _, entry := range plans(v["apis"]) {
		ps := plans(entry["plans"])
		kept := brief(entry, []string{"api", "slug", "rank", "bestPage"})
		kept["plans"] = len(ps)
		if best := bestPlan(ps); best != nil {
			kept["best"] = brief(best, []string{"plan", "score", "perThousand", "page"})
		}
		apis = append(apis, kept)
	}
	// Its best rank first, the APIs where it has none last, in the API's order.
	sort.SliceStable(apis, func(i, j int) bool {
		ri, iok := apis[i]["rank"].(float64)
		rj, jok := apis[j]["rank"].(float64)
		return iok && (!jok || ri < rj)
	})
	ranked := 0
	var bestAt []string
	for _, api := range apis {
		if r, ok := api["rank"].(float64); ok {
			ranked++
			if r == apis[0]["rank"] {
				bestAt = append(bestAt, str(api["api"]))
			}
		}
	}
	limit := 10
	if in.Limit > 0 {
		limit = clamp(int(in.Limit), 1, 200)
	}
	v["total"] = len(apis)
	v["ranked"] = ranked
	// The counts in words, first of the answer's keys: gpt-oss:120b read total, after 63 APIs, as where it is ranked.
	name := str(v["provider"])
	if name == "" {
		name = slug
	}
	about := fmt.Sprintf("%s has plans on %d APIs on Datacircle and a rank on %d of them (1 is the best).", name, len(apis), ranked)
	if ranked > 0 {
		about += fmt.Sprintf(" Its best rank: %v, on %s.", apis[0]["rank"], strings.Join(bestAt, ", "))
	}
	v["about"] = about
	if len(apis) > limit {
		apis = apis[:limit]
	}
	v["apis"] = apis
	return v, nil
}

// bestPlan is a provider's best plan on an API: its lowest rank, a plan with no rank having none.
func bestPlan(ps []map[string]any) map[string]any {
	var best map[string]any
	low := 0.0
	for _, p := range ps {
		if r, ok := p["rank"].(float64); ok && (best == nil || r < low) {
			best, low = p, r
		}
	}
	if best == nil && len(ps) > 0 {
		best = ps[0]
	}
	return best
}

func plans(v any) []map[string]any {
	list, _ := v.([]any)
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
