# Follow-ups

- Consider running PreviewScoreModel in a REPEATABLE READ transaction so the favorites, N and n counts come from one snapshot (finding 8).
- Optionally add a sentence to spec behaviour 27 saying the suggested threshold stops at the highest held-out score (findings 1/11).
