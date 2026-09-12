# Tana

Tana manages and reads local comics and manga, with a separate collector for upstream metadata.

## Language

**Library**:
A user-registered collection identified independently of its display name and associated with exactly one library root. A user can register multiple libraries.

**Library root**:
The directory containing a library's comic and manga files, as seen by the machine running Tana. It may reside on local storage or an operating-system-mounted NAS.

**Library availability**:
Whether a registered library's root is currently present as a directory. This does not guarantee readable contents or a connected NAS; temporary unavailability does not mean the library or its comics were removed.

**Source**:
A `.cbz` or `.zip` archive, or a directory containing files and no subdirectories or qualifying archives. Its inventory includes all contained files, including those unsupported for reading; qualifying archives take precedence over their containing directory.

**Scan**:
Discovery of new sources in one or all registered libraries, followed by their import and automatic gallery creation where supported images exist.

**Import**:
Registration of a source and its complete file inventory in the catalog. File contents remain at their existing locations.

**Source file**:
A file cataloged as part of a source, whether or not it is a supported image, reusable across galleries. Files at every path inside an archive belong to its inventory; a nested archive is an ordinary source file.

**Gallery metadata**:
Descriptive information belonging to a gallery, including its title and tags. It may be supplied by metadata providers or edited by the user.

**Metadata provider**:
A supplier of gallery metadata. Different providers may supply different information from different origins, including files within a source.

**Panda**:
A metadata provider supplying gallery metadata. Its upstream galleries are uniquely identified by gallery ID, independently of galleries in Tana's catalog.

**Collector**:
The separate service that collects Panda favorite collections, gallery references, and metadata for Tana, and owns Panda downloads and their retained files.
_Avoid_: Connector

**Panda gallery reference**:
An upstream Panda gallery's ID together with the token needed to access it. The gallery ID alone determines identity; a gallery's token is assumed immutable.

**Panda gallery candidate**:
A possible upstream Panda gallery ID inferred from a source's name. It may be unrelated to that source and does not supply an access token.

**Panda favorite collection**:
One favorites category belonging to a Panda account on a particular Panda host, whose entries identify upstream galleries through Panda gallery references. Its identity is the host, Panda account key, and category ID; its name may change.

**Panda account key**:
A stable identifier for a Panda account's collected favorite collections, independent of changes to its authentication and settings cookies.

**Panda favorite**:
A Panda gallery reference belonging to a favorite collection, together with the time it was favorited. The same upstream gallery may be discovered independently through other collection paths.

**Panda favorite sync**:
Collection of a favorite collection's newest entries since its previously collected state, treating a changed favorite timestamp as a new occurrence. An initial sync collects the entire collection.

**Panda favorite full re-sync**:
Collection of an entire favorite collection to replace its stored membership, including removal of favorites no longer present upstream. Removal from a favorite collection does not remove a gallery from the gallery reference inventory.

**Panda favorite baseline**:
The favorites across all categories observed during automatic initialization and excluded from automatic downloading. The baseline is established only when every category has been successfully collected, including any retries.

**New Panda favorite**:
A gallery first observed as a favorite after the baseline is established, regardless of its upstream favorite timestamp. Re-favoriting or moving a previously observed gallery between categories does not make it new.

**Panda automatic download category**:
A favorite category configured to trigger collector downloads for new Panda favorites discovered during a manually started collection run. Enabling a category does not make previously observed favorites eligible.

**Missing Panda favorite**:
A Panda favorite with no corresponding source in the local Tana library. A download retained by the collector does not make the favorite locally present.

**Panda download job**:
A durable request for the collector to obtain an upstream Panda gallery's original archive, identified by gallery ID. Repeated requests reuse the existing job or retained archive; failed jobs require explicit retry after bounded automatic retries are exhausted.

**Retained Panda archive**:
A completed original archive owned and stored by the collector across restarts until explicitly deleted; retrieval does not remove it. Its presence does not establish a source in a Tana library.

**Cancelled Panda download job**:
A Panda download job stopped by explicit request, with its partial archive removed. The job remains available for explicit retry.

**Panda enrichment**:
Automatic application of collected Panda metadata to a source-linked gallery using a Panda gallery candidate. Pending enrichment is an outstanding local intent to obtain and apply that metadata, distinct from a collector metadata fetch job.

**Gallery reference inventory**:
The distinct Panda gallery references discovered through feeds, sitemaps, Panda favorite collections, or Panda metadata, or supplied by Tana and confirmed by Panda. Tana-supplied references enter the inventory only after successful metadata retrieval confirms their tokens; references discovered through feeds, sitemaps, Panda favorite collections, or Panda metadata may await retrieval.

**Panda reference import**:
Submission of Panda gallery references through Tana for admission to the gallery reference inventory, complete when every submitted reference has a final outcome. Matching inventory references count as already known; an import admits a new reference only after successful metadata retrieval confirms its token.

**Accepted Panda reference import**:
A Panda reference import whose complete input file has been received and whose processing is durably owned by the collector. Acceptance survives the submitting session and collector restarts, independently of later validation outcomes.

**Panda sitemap collection**:
Discovery of Panda gallery references through the child sitemaps listed in Panda's sitemap index. Discovered references join the gallery reference inventory for metadata collection; sitemap collection does not request gallery archives.

**Collected Panda metadata**:
The latest successfully retrieved metadata for an upstream Panda gallery, retained indefinitely even if the gallery becomes unavailable upstream.

**Panda catalog**:
The browsable collection of collected Panda metadata across all discovery paths, with one entry per upstream gallery. Its coverage is limited to metadata already collected.

**Metadata refresh time**:
The time of the last successful retrieval of collected Panda metadata. Failed retrieval attempts do not advance it.

**Metadata fetch job**:
A durable request from Tana to retrieve fresh Panda metadata for supplied gallery references, with a pending, successful, or failed outcome for each reference. Queued references are not evidence of valid tokens or membership in the gallery reference inventory.

**Raw feed**:
A captured upstream feed in its original form, retained independently of its parsed entries. Panda feed entries identify upstream galleries for later metadata retrieval.

**Feed gallery reference**:
A Panda gallery reference observed in a particular raw feed. The same gallery can appear in multiple raw feeds, with one sighting per gallery per raw feed.

**Feed continuity check**:
A check for gallery IDs already seen in other processed feeds when a raw feed is processed. A nonempty feed containing only previously unseen gallery IDs indicates a possible collection gap; empty or unparseable feeds leave continuity unknown, and the first capture has no baseline. Discovery through Panda metadata alone does not establish feed continuity.

**Tag**:
A descriptive label shared across galleries and libraries, uniquely identified by its namespace and value. Values use lowercase ASCII letters, digits, spaces, hyphens, and dots.

**Namespace**:
A service-wide named category of tags, uniquely identified by a name using only lowercase ASCII letters (`a-z`). Tags supplied without a namespace belong to `other`.

**Tag catalog**:
The shared vocabulary of tags across all libraries, including tags no longer assigned to any gallery.

**Gallery**:
A titled, ordered collection of pages drawn from supported image files, independent of any single library. Its content can comprise a subset of one source or files from multiple sources across libraries.

**Source-linked gallery**:
A gallery created from and explicitly linked to a single source, containing that source's supported images. Its pages may be reordered but not added or removed by users; deleting its source also deletes the gallery.

**Gallery page**:
A supported image source file's occurrence in a gallery, numbered consecutively from 1. A file can occur more than once in a gallery and have different page numbers in different galleries.

**Reading spread**:
One or two consecutive gallery pages presented together as a reading unit.

**Reading progress**:
The proportion of a gallery through the last page of the current reading spread, including the visible pages. It describes the current position, not a history of pages read.
