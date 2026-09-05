# Retain raw Panda feeds independently of processing

Upstream feeds are transient, so a parser defect must not prevent capture of data that may disappear before the defect is fixed. Persist complete responses before asynchronous processing and retain them indefinitely, including successfully processed feeds, to preserve the source material for recovering missed references.
