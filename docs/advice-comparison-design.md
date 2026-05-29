# Advice Comparison Design

## Goal

Compare two advice generation modes from the same player data:

- DB-only: uses local metrics, benchmark averages, trend deltas, advice history, and feedback.
- GraphRAG: uses the same DB input plus vegapunk evidence such as SF6 knowledge, related symptoms, side-effect patterns, and similar past advice cases.

The comparison is meant to evaluate whether GraphRAG adds value beyond a reasonably strong DB + LLM baseline.

## Source of Truth

The local SQLite database remains the source of truth for measured facts:

- play stats snapshots
- benchmark comparison players and averages
- advice runs
- advice candidates
- user feedback

Vegapunk is used as an interpretation and evidence layer, not as a replacement for local data.

## Current MVP

The first implementation intentionally avoids making GraphRAG mandatory:

- Generate one DB-only candidate and one GraphRAG candidate from the same DB input.
- Persist both candidates in `advice_runs` / `advice_candidates`.
- Persist user ratings in `advice_feedback`.
- Query vegapunk via `grpcurl` when available:
  - `VEGAPUNK_GRPC_TARGET`, default `vegapunk:6840`
  - `VEGAPUNK_SCHEMA`, default `sf6-advice`
  - `VEGAPUNK_TOKEN`, optional bearer token
- If vegapunk is unreachable, the GraphRAG card still renders with a connection/evidence note.

## Intended Evolution

The next stages should replace deterministic advice text with an LLM prompt that receives:

- the DB facts and trend summary
- active / past advice outcomes
- user feedback
- GraphRAG search results and traceable chains

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

The key hypothesis is not that GraphRAG wins on the first advice. The expected advantage is that it improves after advice history, side effects, and feedback accumulate.
