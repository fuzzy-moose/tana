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

**Gallery**:
A titled, ordered collection of pages drawn from supported image files, independent of any single library. Its content can comprise a subset of one source or files from multiple sources across libraries.

**Gallery page**:
A supported image source file's occurrence in a gallery, numbered consecutively from 1. A file can occur more than once in a gallery and have different page numbers in different galleries.

**Reading spread**:
One or two consecutive gallery pages presented together as a reading unit.

**Reading progress**:
The proportion of a gallery through the last page of the current reading spread, including the visible pages. It describes the current position, not a history of pages read.
