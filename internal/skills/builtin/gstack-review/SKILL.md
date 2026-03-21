---
name: gstack-review
description: A normalized ai-workflow adaptation of gstack's pre-landing code review workflow. Use when code changes need a structured review for correctness, regression risk, missing tests, and release readiness.
---

# Gstack Review

Use this skill when code changes already exist and need a serious pre-landing review.

This is a normalized adaptation of `garrytan/gstack`'s `review` workflow.

## Primary Goal

Find issues that are likely to survive implementation but fail in production, integration, or maintenance.

## Review Priorities

Review in this order:

1. Correctness bugs
2. Behavioral regressions
3. Missing failure handling
4. Missing or weak tests
5. Contract drift
6. Completeness gaps

## Required Output Style

Present findings first, ordered by severity.
Each finding should include:

1. Severity
2. Affected file or subsystem
3. The concrete problem
4. Why it matters
5. What kind of fix is needed

If no findings exist, say that explicitly and note remaining risks or testing gaps.

## Output Contract

Write a review report to:

```text
.ai-workflow/artifacts/gstack/review/<date>-<topic-slug>.md
```

Include:

1. Findings
2. Open questions
3. Residual risks
4. Suggested next action

## Artifact Metadata Contract

Default placement: `Run.ResultMetadata`.
This is the one first-batch `gstack-*` skill that should default to the `WorkItem` / gate side.
When stored on a run result, set:

1. `artifact_namespace = gstack`
2. `artifact_type = review_report`
3. `artifact_format = markdown`
4. `artifact_relpath = .ai-workflow/artifacts/gstack/review/<date>-<topic-slug>.md`
5. `artifact_title =` a short human-readable title
6. `producer_skill = gstack-review`
7. `producer_kind = skill`
8. `summary =` a 1 to 2 sentence review summary

When finishing via `action-signal`, pass these fields in the decision payload so the artifact is indexed in `Run.ResultMetadata`.

## Gate Hint

This skill is a good candidate for future gate integration.
When a work item or thread task reaches review stage, its result should eventually be representable as a gate verdict.

## Quality Bar

Avoid vague language like "looks good overall".
A useful review names concrete risks and gives the user enough information to act.
