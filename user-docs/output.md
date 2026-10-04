# output

The response contract of a CLI built on `agent-cli-core`: one envelope, fixed exit codes, untrusted-content marking and bounded output. Every command of your CLI should end by calling `output.Write` and then exiting with the code from the envelope.

```go
import "github.com/stainedhead/agent-cli-core/output"
```

## Getting started

```go
func run(w io.Writer, opts output.Options) int {
    data, err := listThings()
    env := output.Success(data, &output.Meta{RequestID: reqID})
    if err != nil {
        env = output.FromError(err)
    }
    if werr := output.Write(w, env, opts); werr != nil {
        env = output.FromError(werr)
        _ = output.Write(w, env, output.Options{})
    }
    return int(env.ExitCode())
}
```

`os.Exit(run(os.Stdout, opts))` then always agrees with the envelope.

## The envelope

Success: `{"ok":true,"data":...,"meta":{"truncated":false,"next_offset":null,"count":12,"request_id":"..."}}`.
Failure: `{"ok":false,"error":{"code":"policy_denied","message":"...","hint":"..."}}`.

`meta.count` is the number of items when `data` is an array. `meta.request_id` is omitted when you leave it empty.

## Exit codes and errors

| Code | Constant | Category (`error.code`) |
|---|---|---|
| 0 | `ExitOK` | `ok` |
| 1 | `ExitGeneral` | `general` |
| 2 | `ExitUsage` | `usage` |
| 3 | `ExitAuth` | `auth` |
| 4 | `ExitForbidden` | `forbidden` |
| 5 | `ExitNotFound` | `not_found` |
| 6 | `ExitPolicyDenied` | `policy_denied` |
| 7 | `ExitConflict` | `conflict` |
| 8 | `ExitRateLimited` | `rate_limited` |
| 9 | `ExitValidation` | `validation` |

Make your own errors map to a code by implementing `output.CategoryError` (`Category() output.Category`). Add `Hint() string` to supply `error.hint`. `output.FromError(err)` and `output.ExitOf(err)` find the category anywhere in a wrapped error chain, and `FromError` finds the hint the same way, including inside `errors.Join`; an error with no category is `general` (exit 1).

```go
type notFound struct{ id string }

func (e notFound) Error() string              { return "item " + e.id + " was not found" }
func (e notFound) Category() output.Category { return output.CategoryNotFound }
```

## Marking free text as untrusted

You decide which fields hold text written by other people. Wrap them in `output.Untrusted`:

```go
data := map[string]any{
    "number":  "T-1001",
    "comment": output.Untrusted{Value: body, Author: "carol", Timestamp: when},
}
```

In JSON the field carries `"untrusted": true`. In `table` and `text` output it is wrapped in `<<<UNTRUSTED author="..." timestamp="...">>>` ... `<<<END UNTRUSTED>>>` delimiters. Fields you do not wrap are not marked. This reduces prompt-injection risk; it does not remove it.

## Output size

`Options.Bounds.MaxBytes` caps one write (default 32768 bytes). If `data` is an array, whole items are dropped from the end; if it is a string, it is cut on a character boundary. The result stays valid JSON and UTF-8, `meta.truncated` is `true`, and `meta.next_offset` says where to resume. Pass that value as `Options.Bounds.Offset` on the next call (your CLI would expose it as a flag such as `--offset`) and repeat until `truncated` is `false`.

Return large results as an array or a string so they can be paged. Data of any other shape (an object, a number) cannot be cut: if it does not fit, `Write` returns `output.ErrBoundTooSmall`, as it does when a single array item is larger than `MaxBytes`. For an object that holds a list, such as `{items, total}`, set `Bounds.ArrayField`.

### Bounding a list inside an object (`Bounds.ArrayField`)

Set `Bounds.ArrayField` to the name of a top-level key whose value is an array. Whole items are dropped from the end of that array; every other field, and the order of all fields, is kept. It works for structs and maps.

```go
opts := output.Options{Bounds: output.Bounds{MaxBytes: 4096, Offset: 0, ArrayField: "items"}}
```

- `Offset` skips that many items; `meta.count` is the items kept; `meta.next_offset` is the absolute index of the first dropped item, exactly as for array data, so you pass it back as `Offset`.
- A `null` array counts as empty and stays `null`. `Offset` equal to the length gives an empty page; an `Offset` past the end returns `output.ErrOffsetOutOfRange`.
- The data must be an object with that key holding an array, otherwise `Write` returns `output.ErrArrayField` (exit 2). One item larger than the budget returns `output.ErrBoundTooSmall`.
- Only top-level keys, not paths. Leave it empty for the previous behavior. Failure envelopes ignore it.

### Continuation token (`Meta.NextPageToken`)

When a service pages with its own opaque token, put it in `Meta.NextPageToken`. It is written as `meta.next_page_token` (omitted when empty), is never cut, counts toward the byte budget, and appears as `next_page_token=` in the table and text footer. It stays set when output is truncated, so the order is: resume with `next_offset` while `truncated` is `true`, and use the token for the next vendor page once `truncated` is `false`. Only put a value there that is safe to show; the library does not treat it as a secret.

## Formats

`output.ParseFormat` turns a `--format` value into `FormatJSON` (default), `FormatTable` or `FormatText`. Tables align the columns of an array of objects; text prints `key: value` lines. Both end with a `-- count=... truncated=...` line.

## Secrets

Error messages and hints are scrubbed of bearer tokens, JWTs, `token=`/`secret=` pairs and long opaque strings before they are written. Ordinary words such as "basic" or "bearer" in prose are kept, and a 40-hex git commit id is not treated as a key. Pass any secret your process holds in `Options.Secrets` (or build the envelope with `output.FromErrorWithSecrets(err, secrets...)` / `output.FailureWithSecrets`) to have it removed by value as well. Never put a token in `data`.

## Troubleshooting

- `max-bytes is too small for the smallest unit of output`: raise `MaxBytes` or return smaller items.
- `offset is past the end of the data`: use the `next_offset` from the previous page, with the same query.
- A message shows `[redacted]`: the library removed something that looked like a secret.
