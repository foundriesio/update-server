# Style

Conventions for this repository: the Markdown docs, code comments, and
commits.

## Markdown Docs

Applies to `README.md`, `docs/`, and `contrib/README.md`.

### Line Length

Hard-wrap prose at 80 columns. After editing any line, re-flow its whole
paragraph or blockquote so no line runs long or short. Never wrap inside
code blocks, tables, or headings. A line dominated by a link may exceed 80
rather than splitting the link.

### Admonitions

Use GitHub alert syntax, keyword uppercase, marker on its own line:

```
> [!NOTE]
> `tuf-init` requires `auth-init` to have been run first — the TUF
> role keys are encrypted at rest with the HMAC secret it creates.
```

Not `> **Note:** ...` or `> [!Note]`. Levels in use: `[!NOTE]`,
`[!IMPORTANT]`, `[!WARNING]`.

### Lists

Use `-` for unordered lists, not `*`. Indent continuation lines two spaces:

```
- The Factory's offline keys tarball (typically `offline-creds.tgz`),
  which contains the offline root key used to sign the rotation.
```

### Headings

Title Case for section headings (`## Repoint Existing Devices`). When a
heading names a literal file, directory, or command, backtick it and keep
its real case: `` ## `auth-config-*.json` ``, `` ## `run-local.sh` ``.

### Terminology

- **Factory** is capitalized when it names a FoundriesFactory instance:
  "your Factory PKI", "all valid Factory devices". Lowercase only for the
  generic noun.
- First reference on a page is **FoundriesFactory®** with the mark;
  plain FoundriesFactory afterwards.

### Inline Code

Backtick commands, flags, file paths, and literal values in prose:
`pki-init`, `--factory`, `datadir/certs/tls.crt`. Do not leave a flag or
path bare because it appears mid-sentence.

### Em-Dashes

- Use a spaced em-dash ( — ) for asides and definitions, including link
  lists: `- [Production guide](./production.md) — running behind a
  TLS-terminating proxy`.
- Do not use an em-dash to attach a remedy to an error description; quote
  the literal instruction instead. Ambiguous: "fails with an error naming
  the stubbed asset — run `git lfs pull` and rebuild". Clear: "fails with
  an error naming the stubbed asset: \"run `git lfs pull` and rebuild\"".

### Links and Anchors

- Link the first mention of a thing rather than appending "... see
  [link](file)".
- Keep cross-page pointers to a minimum: a guide should read as one flow,
  not a chain of detours. Prefer stating the fact inline; link for depth.
- When renaming a heading, update every anchor that points at it
  (`git grep '#old-anchor'`), including links from other pages.

### Keep Prose Current and Unrepeated

- When a flow changes, delete text the change obsoletes (flags, steps,
  rationale) rather than leaving it beside the new content.
- State a fact once per page; later sections reference it implicitly
  instead of restating it.
- Prefer two sentences over one sentence glued with parentheses or dashes.

## Code Comments

Comments explain **intent — the "why"** behind the code. They never narrate
what the code does; that is recoverable by reading the code.

- **Minimal.** Use the fewest words that convey the intent. Prefer a single
  line. Delete comments that only echo the code.
- **Capture only non-obvious context** — load-bearing intent, invariants,
  and constraints a reader could not reconstruct from the code itself.
- Stay technically accurate; invent no behavior.

## Commit Messages

Commit messages follow the same spirit as code comments: describe the
**intent** of the change — why it was made. Keep the body minimal and free
of contrastive phrasing. Use a concise, imperative subject line.
**Hard-wrap the body at ~72 columns, composed via `git commit -F <file>`**
— a single-line `git commit -m` body is left unwrapped and shell-parses
backticks / `$` / `!` (a source of mangled history); reserve `-m` for the
subject.

## Trailers

Every commit — and every annotated tag — carries the trailers the
repository's history uses: a `Signed-off-by: <Name> <email>` (DCO) trailer
first, then any `Co-Authored-By: <Name> <email>` trailers. Check recent
history with `git log` before committing and mirror the names, emails, and
order it uses.
