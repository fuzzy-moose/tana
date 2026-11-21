# 05: The score page shows the favorites behind the model

What to build: the score page describes the model as learned from the Panda favorites. Its evidence section shows the favorites in scope, the last favorite sync and the new favorites since the active model. It warns when fewer than 50 favorites are in scope, and when none is, it says that no model is possible and what to do. The weights table shows for each value how many favorites and how many catalog galleries carry it. For the candidate at the threshold in the field, the page shows the share of the held-out favorites that the threshold hides, and the share of the recent catalog that it hides beside the share that the listing hides now. It names the suggested threshold as the one that hides at most 5% of the held-out favorites. No text of the web app mentions verdicts any more.

This ticket changes the web only, and it ends the window in which the score page throws on the collector's new answer (spec **Fit** 3). Two numbers come later from the collector and need no further page change: the held-out counts and the suggested threshold are zero until ticket 06, and `new_favorites` is `null` until ticket 07.

This is spec behaviours 20, 22, 23, 24, 25 and 29 (web side), the web side of 18, 27 and 28. Fixed in spec **Design → Web** (Score types, Score page texts):

- `web/src/panda/score.ts`: the types of the spec, and the two shares: the held-out share is `(always_hidden + buckets below the threshold) / total`, the recent share is `(recent.always_hidden + buckets below the threshold) / recent.total`, and the share that the listing hides now keeps its rule on top of the recent share.
- `web/src/panda/PandaScorePage.tsx`: the intro text, the evidence list, the warning, the "no model" text, the table headers, and the candidate's threshold block with the two lines and the suggested text, all with the exact texts of the spec. The "Error rates" block goes. The line under the active threshold uses the same recent share.

The test file's preview fixture gets the new shapes. Tests of the removed counts, of the three old "no model" reasons and of the error rates go, and no new test pins that a text is gone (spec **Design → Test seams**). Tests of behaviour that stays (50 rows and "Show more", the weights filter, overrides, pins, favorite authors, thresholds, the unavailable collector, the replaced candidate) stay with the new fixture.

Test seams: `web/src/panda/score.test.ts` for the two shares, `web/src/panda/PandaScorePage.test.tsx` with a mocked `fetch`.

Blocked by: 03, 04

Status: resolved

- [x] The evidence section shows "Favorites in scope" with `evidence.favorites`, and "Last Panda favorite sync" with the time of `favorites_synced_at` or "never" for null.
- [x] With an active model and a number in `evidence.new_favorites`, it also shows "New favorites since the active model was calculated" with that number. Without an active model, and for `new_favorites: null`, that row is absent.
- [x] With a candidate and `few_favorites` true, the page shows "Fewer than 50 favorites are in scope. Weights and the measurement from so little evidence are unreliable." and "Apply model and threshold" stays enabled. The warning shows only then.
- [x] For `no_model: 'no_favorites'` the page shows "No model is possible: no Panda favorite is in scope. Sync your Panda favorites, or widen the default filter." in place of the weights table, the threshold field for a new model and the apply button. The active model section, the overrides, the pinned weights and the favorite authors still render and work.
- [x] A candidate with `weight_count` 0 shows the threshold field and the apply button, and a weights table without rows.
- [x] The weights table has the column headers "Tag, uploader or category", "Weight", "Favorites", "Catalog", "Active weight", "Override" and "Pin", in this order, and each row shows its `favorites` and `catalog` numbers.
- [x] The intro reads "Calculate a model that scores catalog galleries by their tags, uploader and category, learned from your Panda favorites inside the default filter."
- [x] With a candidate and a threshold in the field, the page shows "Hides <x> % of the held-out favorites. A favorite is held out when its weights come from the other favorites only.", where x is `always_hidden` plus the counts of the buckets below the threshold, over `total`, in whole percent. The line follows the field without a request, and x is 0 for a `total` of 0.
- [x] The page shows "Hides <y> % of the newest <n> catalog galleries (now: <z> %)": y is `recent.always_hidden` plus the candidate's buckets below the threshold, over `recent.total`; z is the same share with the active model's histogram and threshold, the share of `always_hidden` alone when only overrides exist, and 0 while the score filter is not in force.
- [x] The line under the active threshold shows the recent share for the active model's histogram at the threshold in that field.
- [x] The page shows "Suggested: <t> (hides at most 5 % of the held-out favorites)" with `suggested_threshold_tenths` in one decimal, and the candidate field starts at the active threshold, or at the suggested one without an active model, as before.
- [x] The held-out share and the recent share are 0 for a total of 0.
- [x] An unavailable collector still shows its message with Retry, and a replaced candidate still calculates again.
- [x] Removal: a search for "verdict", "not_for_me", "Not for me", "Interesting", "neutral" and "Error rates" in any letter case finds nothing in the non-test files under `web/src`, except the one place in `web/src/navigation.ts` that reads the `verdict` parameter (ticket 01). `HeldOutClass`, the three old values of `NoScoreModel`, and the fields `interesting`, `not_for_me`, `neutral`, `counted_favorites`, `new_verdicts` and `few_verdicts` no longer exist in `score.ts`.
- [x] The `frontend` checks of the green check pass.

## Resolution

`web/src/panda/score.ts` mirrors the favorites-only preview (`ScoreEvidence`, `ScoreWeight` counts, one `HeldOut`, `NoScoreModel = 'no_favorites'`, `RecentCatalog` without verdict counts), and the recent share no longer adds verdicts. `PandaScorePage.tsx` shows the new intro, evidence rows, warning, "no model" text, `Favorites`/`Catalog` columns and the held-out and suggested threshold lines; the error rates block is gone. Tests in `score.test.ts` and `PandaScorePage.test.tsx` use the new fixture.
