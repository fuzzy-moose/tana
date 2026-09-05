# Tana

Tana manages and reads local comics and manga, with a separate collector for upstream metadata.

## Language

**Library**:
A user-registered collection identified independently of its display name and associated with exactly one library root. A user can register multiple libraries.

**Library root**:
The directory containing a library's comic and manga files, as seen by the machine running Tana. It may reside on local storage or an operating-system-mounted NAS.

**Library availability**:
Whether a registered library's root is currently present as a directory. This does not guarantee readable contents or a connected NAS; temporary unavailability does not mean the library or its comics were removed.
