## OpenAI

- 去除 image_generation
- 默认开启 web_search

```sql
UPDATE "public"."channels"
SET "param_override" = $param_override$
{
  "operations": [
    {
      "path": "include",
      "mode": "set",
      "value": [],
      "keep_origin": true,
      "logic": "AND",
      "conditions": [
        { "path": "request_path", "mode": "prefix", "value": "/v1/responses" },
        { "path": "request_headers.user-agent", "mode": "contains", "value": "Codex", "invert": true, "pass_missing_key": true }
      ]
    },
    {
      "path": "include",
      "mode": "append",
      "value": "web_search_call.action.sources",
      "logic": "AND",
      "conditions": [
        { "path": "request_path", "mode": "prefix", "value": "/v1/responses" },
        { "path": "request_headers.user-agent", "mode": "contains", "value": "Codex", "invert": true, "pass_missing_key": true },
        { "path": "include.#(==\"web_search_call.action.sources\")", "mode": "full", "value": "web_search_call.action.sources", "invert": true, "pass_missing_key": true }
      ]
    },
    {
      "path": "tools",
      "mode": "set",
      "value": [],
      "keep_origin": true,
      "logic": "AND",
      "conditions": [
        { "path": "request_path", "mode": "prefix", "value": "/v1/responses" },
        { "path": "request_headers.user-agent", "mode": "contains", "value": "Codex", "invert": true, "pass_missing_key": true }
      ]
    },
    {
      "path": "tools",
      "mode": "prune_objects",
      "value": { "recursive": false, "where": { "type": "image_generation" } },
      "logic": "AND",
      "conditions": [
        { "path": "request_path", "mode": "prefix", "value": "/v1/responses" },
        { "path": "request_headers.user-agent", "mode": "contains", "value": "Codex", "invert": true, "pass_missing_key": true },
        { "path": "tools.#(type==\"image_generation\").type", "mode": "full", "value": "image_generation" }
      ]
    },
    {
      "path": "tools",
      "mode": "append",
      "value": { "type": "web_search", "search_context_size": "medium" },
      "logic": "AND",
      "conditions": [
        { "path": "request_path", "mode": "prefix", "value": "/v1/responses" },
        { "path": "request_headers.user-agent", "mode": "contains", "value": "Codex", "invert": true, "pass_missing_key": true },
        { "path": "tools.#(type==\"web_search\").type", "mode": "full", "value": "web_search", "invert": true, "pass_missing_key": true }
      ]
    },
    {
      "path": "tool_choice",
      "mode": "set",
      "value": "auto",
      "logic": "AND",
      "conditions": [
        { "path": "request_path", "mode": "prefix", "value": "/v1/responses" },
        { "path": "request_headers.user-agent", "mode": "contains", "value": "Codex", "invert": true, "pass_missing_key": true }
      ]
    }
  ]
}
$param_override$
WHERE "group" LIKE '%OpenAI×%';
```

## xAI

- 默认开启 web_search
- x_search 与 openai(/v1/responses) 兼容性一般

```sql
UPDATE "public"."channels"
SET "param_override" = $param_override$
{
  "operations": [
    {
      "path": "tools",
      "mode": "set",
      "value": [],
      "keep_origin": true,
      "conditions": [
        { "path": "request_path", "mode": "prefix", "value": "/v1/responses" }
      ]
    },
    {
      "path": "tools",
      "mode": "append",
      "value": { "type": "web_search" },
      "logic": "AND",
      "conditions": [
        { "path": "request_path", "mode": "prefix", "value": "/v1/responses" },
        { "path": "tools.#(type==\"web_search\").type", "mode": "full", "value": "web_search", "invert": true, "pass_missing_key": true }
      ]
    }
  ]
}
$param_override$
WHERE "group" LIKE '%Grok×%';
```

## Gemini

- 默认开启 web_search

```sql
UPDATE "public"."channels"
SET "param_override" = $param_override$
{
  "operations": [
    {
      "path": "tools",
      "mode": "set",
      "value": [],
      "keep_origin": true,
      "conditions": [
        { "path": "request_path", "mode": "prefix", "value": "/v1beta/models/gemini" }
      ]
    },
    {
      "path": "tools",
      "mode": "append",
      "value": { "googleSearch": {} },
      "logic": "AND",
      "conditions": [
        { "path": "request_path", "mode": "prefix", "value": "/v1beta/models/gemini" },
        { "path": "tools.#(googleSearch).googleSearch", "mode": "contains", "value": "{", "invert": true, "pass_missing_key": true }
      ]
    }
  ]
}
$param_override$
WHERE "group" LIKE '%Gemini×%';
```
