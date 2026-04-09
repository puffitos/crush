# Bedrock provider serves foundation model IDs instead of inference profile IDs

**Catwalk version:** `v0.30.3`

## Problem

Catwalk's `bedrock.json` lists models using their foundation model IDs (e.g.
`anthropic.claude-sonnet-4-6`). These IDs are what `aws bedrock
list-foundation-models` returns. However, the Bedrock runtime API does **not**
accept foundation model IDs directly — it requires **inference profile IDs**
(e.g. `eu.anthropic.claude-sonnet-4-6`), which are what `aws bedrock
list-inference-profiles` returns.

Any consumer using catwalk's bedrock model IDs verbatim will get the following
error from the Bedrock API:

```
bad request: POST "https://bedrock-runtime.eu-central-1.amazonaws.com/v1/messages":
400 Bad Request {"message":"The provided model identifier is invalid."}
```

## Observed behaviour (eu-central-1)

The following is based on direct experience using crush with an AWS account
hosted in `eu-central-1`. Other regions may behave differently and have not
been verified, but the underlying cause is the same for all non-global regions.

Initially the problem was intermittent. Normal prompting appeared to work, but
secondary model calls would silently fail with the 400 error above:

- Session title generation would fail and fall back silently:
  ```
  error generating title with small model; trying big model
  ```
- Agentic fetch tool calls would fail entirely:
  ```
  agentic_fetch: error generating response —
  bad request: 400 Bad Request {"message":"The provided model identifier is invalid."}
  ```

After a recent crush update, the problem became consistent — even primary
prompts started returning 400 errors, making the tool unusable with the
bedrock provider. At that point the root cause was investigated and traced back
to the model IDs catwalk provides.

The full error chain observed in the logs:

```
{"level":"ERROR","msg":"error generating title with small model; trying big model",
 "err":"bad request: POST \"https://bedrock-runtime.eu-central-1.amazonaws.com/v1/messages\":
 400 Bad Request {\"message\":\"The provided model identifier is invalid.\"}"}

{"level":"ERROR","msg":"agentic_fetch: error generating response",
 "error":"bad request: POST \"https://bedrock-runtime.eu-central-1.amazonaws.com/v1/messages\":
 400 Bad Request {\"message\":\"The provided model identifier is invalid.\"}"}
```

And on startup after the catwalk update that removed the region prefix logic:

```
Failed to configure selected models: failed to select default models:
default large model anthropic.claude-sonnet-4-6 not found for provider bedrock
```

## Background

AWS Bedrock requires cross-region inference profile IDs for all API calls.
These IDs carry a region-group prefix that determines which AWS infrastructure
the request is routed through:

| Region | Prefix | Example |
|---|---|---|
| `us-*`, `ca-central-1` | `us.` | `us.anthropic.claude-sonnet-4-6` |
| `eu-*` | `eu.` | `eu.anthropic.claude-sonnet-4-6` |
| `ap-northeast-1` | `jp.` | `jp.anthropic.claude-sonnet-4-6` |
| `ap-southeast-2` | `au.` | `au.anthropic.claude-sonnet-4-6` |
| All regions | `global.` | `global.anthropic.claude-sonnet-4-6` |

Some regions (e.g. `ap-northeast-2`, `sa-east-1`) only have `global.` profiles
and no regional prefix.

Catwalk's model list mirrors `list-foundation-models`, which returns bare IDs
with no prefix. These IDs are not directly callable anywhere — every region
requires a prefixed inference profile ID.

## Proposed Solutions

### Option 1 — Consumer-side prefix (current workaround)

The consumer reads `AWS_REGION`, derives the prefix, and prepends it to all
model IDs and default model references. Catwalk exposes helper methods to make
this safe and atomic:

```go
// PrefixModelIDs prepends prefix to all model IDs and default model references.
func (p *Provider) PrefixModelIDs(prefix string)

// ApplyBedrockRegion derives and applies the correct prefix for a given AWS region.
func (p *Provider) ApplyBedrockRegion(region string)
```

Usage in the consumer:

```go
p.ApplyBedrockRegion(os.Getenv("AWS_REGION"))
prepared.Models = p.Models
```

**Advantages:**
- Less invasive code change.
- Simpler to maintain.

**Drawbacks:**
- Requires `AWS_REGION` to be set; does not handle regions that only have
  `global.` profiles; the prefix mapping logic must be kept in sync with AWS
  as new regions are added.
- Adds new exported functions which require consumers to update their code.

### Option 2 — Catwalk serves inference profile IDs directly (preferred)

Add a `regions` field to each model in `bedrock.json` listing the available
inference profile prefixes (e.g. `["us", "eu", "global"]`). Catwalk reads
`AWS_REGION` at startup and resolves each model's ID to the correct inference
profile, falling back to the `global` profile when no regional one is available.

```json
{
  "id": "bedrock",
  "default_large_model_id": "global.anthropic.claude-sonnet-4-6",
  "default_small_model_id": "global.anthropic.claude-haiku-4-5-20251001-v1:0",
  "models": [
    {
      "id": "anthropic.claude-sonnet-4-6",
      "regions": ["us", "eu", "global"],
      ...
    }
  ]
}
```

The consumer receives a `Provider` with model IDs already resolved to the
correct inference profile for the user's region. No transformation is needed
on the consumer side. If `AWS_REGION` is not set, `global.*` profiles are
returned, which work in all regions.

**Advantages:**
- The model list is self-contained and correct out of the box.
- Consumers do not need to know anything about inference profile prefixes.
- No consumer code changes required.

**Drawbacks:**
- The `regions` list per model requires manual maintenance as AWS adds new
  regional profiles.
