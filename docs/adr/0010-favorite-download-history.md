---
status: accepted
---

# Preserve favorite download history independently of downloads

The first manually started favorites sync establishes a baseline across all categories, including on an existing installation, to avoid downloading an existing backlog. All categories must finish before automatic downloads begin; discoveries during initialization and its retries belong to the baseline. The collector supports one fixed Panda account, with no account switching or separate account baselines.

Favorite observation history survives category moves, re-favoriting, and removal of membership or download jobs. Cancelling or deleting a download therefore does not restore automatic eligibility, keeping those user actions effective across later collection runs. Eligibility follows first observation rather than upstream favorite timestamps, so previously missed entries discovered by full re-sync can qualify after initialization. Local library presence does not affect eligibility, keeping collection independent of library availability.

Download categories start with none selected so users explicitly choose the scope; enabling a category does not backfill observed favorites. Accepted download requests survive configuration changes and collection failures, avoiding lost downloads when their favorites are subsequently recognized as known.
