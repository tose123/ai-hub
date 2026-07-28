## OpenAI

```sql
UPDATE "public"."channels"
SET "header_override" = $header_override$
{
  "User-Agent": "Codex Desktop/0.145.0-alpha.18 (Mac OS 26.5.2; arm64) unknown (Codex Desktop; 26.715.21425)"
}
$header_override$
WHERE "group" LIKE '%OpenAI×%' OR "group" LIKE '%GPT-Image×%';
```

## Claude

```sql
UPDATE "public"."channels"
SET "header_override" = $header_override$
{
  "User-Agent": "claude-code/2.1.212 (cli)"
}
$header_override$
WHERE "group" LIKE '%Claude×%';
```
