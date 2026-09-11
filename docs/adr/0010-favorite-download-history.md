---
status: accepted
---

# Preserve favorite download history independently of downloads

Automatic favorite downloads use lasting knowledge of previously observed favorite galleries, including an automatically initialized baseline covering all categories, to avoid downloading an existing backlog. Category moves and re-favoriting preserve that knowledge; cancelling or deleting a download does not restore automatic eligibility, because those user actions must remain effective across later collection runs. Favorite observation history therefore survives removal of favorite membership and download jobs; enabling a category does not backfill previously observed favorites.

Eligibility follows first observation rather than upstream favorite timestamps, so previously missed entries discovered by full re-sync can qualify. After bootstrap, discoveries retain their download requests even if the surrounding collection run fails, avoiding lost downloads when those favorites are subsequently recognized as known. Local library presence does not affect eligibility, keeping collection independent of library availability.

The collector supports one fixed Panda account; account switching and separate account baselines are outside this design.

The first manually started favorites sync establishes the baseline across all categories, including on an existing installation, so activation never treats the stored backlog as new downloads. All categories must finish before automatic downloads can begin; discoveries during initialization and its retries belong to the baseline.

Download categories start with none selected so users explicitly choose the scope. Configuration changes affect future discoveries, preserving already accepted download requests.
