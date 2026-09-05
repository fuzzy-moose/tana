# galleryinfo.txt format

## Structure

Each sample begins with five single-line fields in this order:

| Label          | Observed value                                                     |
|----------------|--------------------------------------------------------------------|
| `Title:`       | Full gallery title, including punctuation and bracketed qualifiers |
| `Upload Time:` | UTC date and time in `YYYY-MM-DD HH:mm` form                       |
| `Uploaded By:` | Uploader name                                                      |
| `Downloaded:`  | UTC date and time in `YYYY-MM-DD HH:mm` form                       |
| `Tags:`        | Comma-separated tags                                               |

Spaces align values after the labels; their count varies between samples. Colons also occur within values, so a field's
label separator must be distinguished from value content. Both timestamps represent UTC despite omitting a timezone
suffix or offset; this meaning was supplied by the project owner and cannot be inferred from the sample text alone.

Fields may be missing or reordered. Ignore unknown headers before the comments section. Trim surrounding whitespace from
single-line field values; for duplicate fields, use the first valid occurrence. Malformed recognized fields produce
diagnostics without discarding other valid fields.

## Tags

Tags appear on one line, separated by a comma followed by a space. A tag may have a namespace separated from its value
by a colon, such as `language:english` or `artist:haruto aoki`. Tags also occur without namespaces, such as `group`,
`multi-work series`, and `all ages`. Spaces and hyphens occur within tag values.

Observed namespaces are `language`, `parody`, `character`, `group`, `artist`, `male`, and `female`. This is an observed
set, not evidence of a closed vocabulary. The samples contain no escaping or quoting examples for commas within tags.

Tana's parser assigns the namespace `other` to unnamespaced tags: `all ages` becomes `other:all ages`.

Split tags at commas, then split each namespaced tag at its first colon. Trim whitespace around namespace and value,
lowercase both, and deduplicate identical namespace/value pairs. Reject tags with an explicit empty namespace or an
empty value, retaining other valid tags. Unknown namespaces are allowed. Namespace names must contain only lowercase
ASCII letters (`[a-z]`). Tag values may contain lowercase ASCII letters, digits, spaces, hyphens, and dots
(`[a-z0-9 .-]`). These are service-wide vocabulary constraints, including metadata from other origins.

## Uploader comments

Samples 1 and 3 have a blank line after tags, a standalone `Uploader's Comments:` heading, then a blank line and
multiline comment content. Sample 2 omits the entire section.

Comments contain paragraph breaks and URLs. Sample 3 includes a literal HTML anchor. Parse comments as raw text,
preserving their content without interpreting HTML.

## Attribution footer

The last nonblank line is a required attribution footer; ignore trailing blank lines when locating it. Its position
defines its role; its wording is not a fixed marker and must not be matched against the samples' literal text. Exclude
it from uploader comments. With no wording-based marker, an omitted footer cannot reliably be distinguished from a final
data line; the positional rule will consume that line as the footer.

With comments, a blank line separates the footer from the comment text. Without comments, it immediately follows the
tags line. It is downloader attribution, distinct from the `Downloaded:` timestamp field.

## Limits of the samples

The samples do not establish multiline titles or tags, tag escaping, or alternate encodings. Tana's tolerance for
missing, reordered, duplicate, and unknown fields is an explicit interpretation documented above, rather than a property
demonstrated by the samples.
