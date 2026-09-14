# Retain raw Panda feeds until processing succeeds

Upstream feeds are transient, so a parser defect must not prevent capture of data that may disappear before the defect is fixed. Persist complete responses before asynchronous processing and retain failed captures for retry. Once references and continuity results are committed, discard the raw response to bound payload storage; retain capture timestamps and feed sightings because scheduling and continuity still depend on them.
