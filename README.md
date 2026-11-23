# Datacircle tools for the Google Gen AI Go SDK

Find the best GTM APIs. We benchmark all the GTM APIs and we put the results on our website: https://datacircle.dev

This Go module gives the [Google Gen AI Go SDK](https://github.com/googleapis/go-genai) (`google.golang.org/genai`) four function tools on Datacircle's open API, https://api.datacircle.dev/v1. You need no key and no account.

| Tool | What the tool gives | Call |
| --- | --- | --- |
| `datacircle_list_apis` | Every GTM API on Datacircle, with the API's slug, number of plans, number of providers and best plan | `GET /apis` |
| `datacircle_best_plans` | One API's plans, sorted by score out of 100, best first. Each plan comes with a rank and a price per 1,000 calls | `GET /apis/{slug}/best` |
| `datacircle_search_plans` | Plans that match a provider or plan name, on all APIs or on one API. Sort by score, provider or price | `GET /plans` |
| `datacircle_provider` | The APIs a provider sells, and the provider's rank on each API | `GET /providers/{slug}` |

## Install

```bash
go get github.com/waynehamadi/genai-go-datacircle
```

The module works with `google.golang.org/genai` v1.0.0 and later, on the Gemini API and on Vertex AI.

## Generate content

`Tool` gives the four Datacircle tools as one `genai.Tool`. Put the tool in a config's `Tools` field. `Responses` runs the function calls of a response and gives one part that answers each call.

```go
import (
	datacircle "github.com/waynehamadi/genai-go-datacircle"
	"google.golang.org/genai"
)

client, err := genai.NewClient(ctx, nil) // GEMINI_API_KEY, or GOOGLE_API_KEY before v1.9.0
if err != nil {
	log.Fatal(err)
}
tools := datacircle.New(nil)
config := &genai.GenerateContentConfig{Tools: []*genai.Tool{tools.Tool()}}
contents := []*genai.Content{genai.NewContentFromText("Who are the 3 best providers for the email-finder API?", genai.RoleUser)}
for {
	resp, err := client.Models.GenerateContent(ctx, "gemini-3.8-flash", contents, config)
	if err != nil {
		log.Fatal(err)
	}
	calls := resp.FunctionCalls()
	if len(calls) == 0 {
		fmt.Println(resp.Text())
		break
	}
	contents = append(contents, resp.Candidates[0].Content,
		genai.NewContentFromParts(tools.Responses(ctx, calls), genai.RoleUser))
}
```

Send the model's turn back unchanged, with the thought signatures. When you stream, save the parts of each chunk to build the model's turn. Save the `FunctionCalls()` of each chunk too. Give the calls to `Responses` when the stream ends.

## Chats

```go
chat, err := client.Chats.Create(ctx, "gemini-3.8-flash", config, nil)
if err != nil {
	log.Fatal(err)
}
resp, err := chat.SendMessage(ctx, genai.Part{Text: "Which provider ranks first for the SERP API?"})
for err == nil && len(resp.FunctionCalls()) > 0 {
	resp, err = chat.Send(ctx, tools.Responses(ctx, resp.FunctionCalls())...)
}
if err != nil {
	log.Fatal(err)
}
fmt.Println(resp.Text())
```

The chat stores the model's turns for you. `SendMessageStream` stores them too. Releases before v1.5.0 have no `Send`: pass each part to `SendMessage` as a value instead.

## Your own functions

You can add your own functions to the tool: `append(tool.FunctionDeclarations, yours...)`. Gemini wants an answer for each call of a turn. Loop over the calls and answer each one. For a call to one of the four Datacircle functions, `Response` gives the part that answers the call. For a call to one of your functions, `Response` gives `false`, and you write that answer yourself. `Handles(name)` tells you if a call goes to one of the four Datacircle functions. `Declarations` gives the declarations of the four Datacircle functions only.

## Call a tool yourself

```go
out, err := datacircle.New(nil).Run(ctx, datacircle.BestPlansName, map[string]any{"slug": "email-finder", "limit": 3})
```

Each tool gives back a JSON object. For a list of plans, each plan comes with a rank, provider, plan, score and price per 1,000 (`perThousand`, in dollars). Each plan also links to the plan's page on datacircle.dev, which lists every field. A tool that asks for a slug also takes an API's or a provider's name.

`Response` and `Responses` give the model `{"error": "..."}` when data is missing, like an unknown slug. The error holds the API's own words and what to send instead. The model can then call again. `Response` and `Responses` give a network failure the same way. `Run` returns a missing-data error or a network failure as an error.

`datacircle.Config` sets the `HTTPClient` the tools use (a timeout, a proxy). Pass `nil` to use `http.DefaultClient`.

## Network

Each tool call makes one HTTPS GET request to https://api.datacircle.dev/v1 and nothing else. The request holds only the tool's own arguments: no key, no config, nothing from your chats. The API takes up to 60 calls a minute from one IP address. The tool waits once, for a minute at most, when the API asks the tool to wait. The module depends only on the Gen AI Go SDK and `golang.org/x/text`. The module stores nothing.

## Links

- Datacircle: https://datacircle.dev
- The API: https://datacircle.dev/api
- The API's spec: https://api.datacircle.dev/v1/openapi.json
- What each key means: https://datacircle.dev/fields

MIT license.
