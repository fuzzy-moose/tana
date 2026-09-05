# Panda feed entry

All example values are invented; the thumbnail uses a reserved example domain.
The entry normally sits inside an Atom `<feed>`.
Its Atom namespace is declared here so the example stands alone.

```xml
<entry xmlns="http://www.w3.org/2005/Atom">
  <title>[Example Circle] Tea &amp; Sketches [English]</title>
  <link rel="alternate" type="text/html" href="https://example.test/g/1234567/0123456789/" />
  <id>tag:example.test,2026-09-05:1234567</id>
  <updated>2026-09-05T12:34:56Z</updated>
  <author>
    <name>example_uploader</name>
  </author>
  <summary type="text">An illustrated afternoon of tea and sketching.</summary>
  <content type="xhtml">
    <div xmlns="http://www.w3.org/1999/xhtml">
      <p style="font-weight:bold">[Example Circle] Tea &amp; Sketches [English]</p>
      <img src="https://thumbs.example.test/w/01/234/56789-example.webp" />
      <p>Tags: language:english, artist:example artist<br /><br />Description: An illustrated afternoon of tea and sketching.</p>
    </div>
  </content>
</entry>
```

The preview represented by this example is:

| Field         | Value                                                     |
|---------------|-----------------------------------------------------------|
| Gallery ID    | `1234567`                                                 |
| Gallery token | `0123456789`                                              |
| Title         | `[Example Circle] Tea & Sketches [English]`               |
| Thumbnail URL | `https://thumbs.example.test/w/01/234/56789-example.webp` |

The gallery link carries both ID and token; the Atom `<id>` is a separate identifier without a token.
The thumbnail is nested XHTML, not an Atom enclosure.
Tags and description appear as text rather than structured metadata fields.
The feed supports discovery and quick previews; full gallery metadata comes from later retrieval.
