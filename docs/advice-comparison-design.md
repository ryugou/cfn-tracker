# Advice Comparison Design

## Goal

Compare two advice generation modes from the same player data:

- DB-only: uses local metrics, benchmark averages, trend deltas, advice history, and feedback.
- GraphRAG: uses the same DB input plus vegapunk evidence such as SF6 knowledge, related symptoms, side-effect patterns, qualitative observation summaries, and similar past advice cases.

The comparison is meant to evaluate whether GraphRAG adds value beyond a reasonably strong DB + LLM baseline.

## Source of Truth

The local SQLite database remains the source of truth for measured facts:

- play stats snapshots
- benchmark comparison players and averages
- advice runs
- advice candidates
- user feedback

Vegapunk is used as an interpretation and evidence layer, not as a replacement for local data.
It must not be treated as the source of truth for exact metric values, deltas, win rates, benchmark averages, or other measured facts.

The intended split is:

- RDB/API: exact numbers, aggregation, trend calculation, benchmark comparison, and history windows.
- Vegapunk/PunkRecord: SF6 knowledge, advice actions, qualitative observation summaries, side-effect hypotheses, user feedback, and similar past cases.
- LLM: combines the current RDB facts with the retrieved PunkRecord context and produces advice.

GraphRAG evidence is allowed to suggest plausible explanations and analogies, but it should not claim causal proof.

## Current MVP

The first implementation intentionally avoids making GraphRAG mandatory:

- Generate one DB-only candidate and one GraphRAG candidate from the same DB input.
- Use the same LLM prompt structure for both modes when an OpenAI-compatible chat endpoint is configured.
- Fall back to deterministic DB rules when the LLM is not configured or fails.
- Persist both candidates in `advice_runs` / `advice_candidates`.
- Persist user ratings in `advice_feedback`.
- Query an OpenAI-compatible LLM endpoint when available:
  - `ADVICE_LLM_BASE_URL`, for example `http://localhost:11434/v1`
  - `ADVICE_LLM_MODEL`
  - `ADVICE_LLM_API_KEY`, optional bearer token
- Query vegapunk via `grpcurl` when available:
  - `VEGAPUNK_GRPC_TARGET`, default `vegapunk:6840`
  - `VEGAPUNK_SCHEMA`, default `sf6-advice`
  - `VEGAPUNK_TOKEN`, optional bearer token
- If vegapunk is unreachable, the GraphRAG card still renders with a connection/evidence note.

## Intended Evolution

The LLM prompt receives:

- the DB facts and trend summary
- active / past advice outcomes
- user feedback
- GraphRAG search results and traceable narrative chains

DB-only and GraphRAG should keep the same common prompt, output schema, and evaluation criteria. The only intended difference is the context block:

- DB-only: DB facts and trends.
- GraphRAG: the same DB facts and trends plus PunkRecord evidence. PunkRecord evidence may reference prior qualitative observations, advice, and side effects, but exact numeric facts must still come from the DB.

The output should remain structured as an advice candidate:

- theme
- rationale
- action
- drill
- success criteria
- watched metrics
- risks / side effects
- evidence

## Evaluation

Compare DB-only and GraphRAG with these scores:

- usefulness
- specificity
- trust
- actionability
- result after the target match window

The key hypothesis is not that GraphRAG wins on the first advice. The expected advantage is that it improves after advice history, side effects, qualitative observation summaries, and feedback accumulate.
