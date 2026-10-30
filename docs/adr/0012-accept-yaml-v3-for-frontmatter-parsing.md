# Use yaml.v3 for book-note frontmatter parsing

**Status**: accepted

The books shelf (spec 010) parses YAML frontmatter from Obsidian book notes, so the
`internal/braind/books` package decodes the metadata block with
`gopkg.in/yaml.v3` rather than a hand-rolled line parser. The corpus is genuine YAML,
not a flat `key: value` subset: block sequences (`tags:`), keys with parentheses and
spaces (`status (Media):`), emoji values (`interest: 🔥🔥`), and values whose YAML
type is surprising (`finished: 2025-05-31` decodes to `time.Time`, while `rating:
4.5/5` stays a string only because of the slash). A hand-rolled parser would own
exactly these coercion and quoting edge cases forever, and each mis-parse silently
corrupts a shelf entry. `yaml.v3` is the standard Go choice, pure Go, and adds no
transitive dependencies. Following ADR-0010's precedent for pure-mechanics libraries,
braind keeps everything that is policy rather than parsing in its own code, covered by
direct tests: the frontmatter split, the book-detection rule, per-field normalization
and leniency, and the generated-note serialization contract (data-model.md).

See [Book Reviews](../../specs/010-books/research.md) (R1) and [ADR-0010](0010-adopt-standard-oidc-libraries.md).
