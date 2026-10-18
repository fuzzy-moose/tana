# Gallery Search Specification

## Query syntax

Search terms are separated by spaces or unquoted commas. Commas inside quotes are literal text. Terms without a
namespace, qualifier, or exact-tag marker search both titles and tags.

| Syntax           | Meaning                                                                                        |
|------------------|------------------------------------------------------------------------------------------------|
| `"text here"`    | Search the quoted text as one exact phrase. Quoting also permits spaces in a single term.      |
| `term_one`       | Replace underscores with spaces and treat the result as one term.                              |
| `-term`          | Exclude galleries matching the term.                                                           |
| `~term`          | Add the term to an OR group. A gallery may contain one or more terms in the group.             |
| `namespace:term` | Match a tag in the named namespace.                                                            |
| `term$`          | Match the exact tag value. The marker applies to tags only and may also be used on exclusions. |

Terms joined by spaces are ANDed unless they are prefixed with `~`. All `~` terms form one OR group; when present, at
least one MUST match. Ordinary required terms and exclusions always apply. For example, `red ~blue ~green -yellow`
requires red and either blue or green, and excludes yellow. An empty query matches every gallery. An exclusion-only
query matches every gallery not excluded.

Operator characters inside quotes are literal except for a trailing `$`, which marks an exact tag value;
`artist:"artist y$"` matches that complete tag value. The marker MUST be inside the quotes when the value is quoted.
`title:"~hello"` searches for the text `~hello`. Within quotes, `\"`
represents a literal quote and `\\` represents a literal backslash. Combined prefixes such as `-~blue` are invalid.

Examples:

```text
a:artist-y c:character-x
```

Requires both the `artist-y` artist tag and the `character-x` character tag.

```text
series:show-z -tag:blocked$
```

Requires the `show-z` series tag and excludes the exact `blocked` tag.

```text
~theme-a ~theme-b
```

Matches galleries containing either theme, including galleries containing both.

## Namespaces

The system MUST support these namespaces and the listed short forms.

| Namespace   | Short form |
|-------------|------------|
| `artist`    | `a`        |
| `character` | `c`        |
| `cosplayer` | `cos`      |
| `female`    | `f`        |
| `group`     | `g`        |
| `language`  | `l`        |
| `location`  | `loc`      |
| `male`      | `m`        |
| `mixed`     | `x`        |
| `other`     | `o`        |
| `parody`    | `p`        |
| `reclass`   | `r`        |

Custom namespaces MUST also be supported. Prefix resolution MUST prefer qualifiers, then existing full namespace names,
then the listed short forms. When resolved as a short form, it MUST have the same matching behavior as its full namespace.
Namespace names and short forms are case-insensitive.

## Qualifiers

Qualifiers select a metadata field or alter tag matching.

| Qualifier | Behavior                                                                    |
|-----------|-----------------------------------------------------------------------------|
| `tag:`    | Search tags in every namespace. This prevents a term from matching a title. |
| `title:`  | Search the title.                                                           |

Examples:

```text
a:"artist y" c:"character x" series:show-z
```

Requires three tags, including two values containing spaces.

```text
title:"story arc" -title:2010 -title:2011
```

Searches titles for the exact phrase and excludes either year from the title.

```text
title:"story arc" -2010 -2011
```

Searches the phrase in the title and excludes galleries containing either year in the title or tags.

## Matching rules

Unqualified terms without `$` MUST search title and tag fields. A namespace, `tag:` qualifier, or `$` marker MUST
restrict matching to tags. `title:` MUST restrict matching to titles; combining it with `$` is invalid.

Matching MUST be case-insensitive. Underscores in search values MUST be converted to spaces.

Title matching MUST find a term in the middle of a word. For example, `berry` matches `Berry Blue` and `Strawberry`.

Tag matching MUST begin at the start of the complete tag or at the beginning of a word within the tag. For example,
`berry` can match `berry blue` but not `strawberry`. Spaces, hyphens, and dots delimit tag words.

Exact tag matching exists because prefix matching can return unintended longer tags. For example, `topic-a` may match
both `topic-a` and `topic-a extended`; `topic-a$` MUST match only the exact value.
Without a namespace, an exact tag term matches that value across all namespaces.

## Tag auto-completion

The UI MUST suggest tags from the full tag catalog, including tags no longer assigned to any gallery, independently of
the galleries matching the rest of the query.
Suggestions MUST respect the active namespace and show both namespace and value. No tag suggestions are shown inside
a `title:` term.
Suggestions MUST appear only after at least one character of the tag value has been typed; an empty input or
`artist:` alone shows no suggestions.

Selecting a suggestion MUST replace the term at the cursor with a namespace-qualified exact tag term, quoting values
containing spaces and preserving any `-` or `~` prefix. For example, selecting the tag `artist:artist y` inserts
`artist:"artist y$"`.

Suggestions MUST rank exact values first, followed by prefixes at word boundaries, with alphabetical ordering for ties.
The UI MUST show at most 10 suggestions and support arrow-key navigation, Enter to select, and Escape to dismiss.
Selecting a suggestion MUST only insert the term; a separate submission runs the search.

## Invalid queries

The UI MUST allow incomplete syntax while typing. On submission, invalid queries, including unknown namespaces and
unclosed quotes, MUST show one generic invalid-query message. Detailed diagnostics are not required.
