// Package output is the contract between a CLI built on agent-cli-core and the
// LLM harness that runs it: one response envelope, stable exit codes,
// untrusted-content marking and bounded output.
//
// # Envelope
//
// Every command writes one envelope. Success:
//
//	{"ok":true,"data":...,"meta":{"truncated":false,"next_offset":null,"count":12,"request_id":"..."}}
//
// Failure:
//
//	{"ok":false,"error":{"code":"policy_denied","message":"...","hint":"..."}}
//
// Build one with Success, Failure or FromError and emit it with Write. The
// process exit code always agrees with the envelope: use Envelope.ExitCode or
// ExitOf.
//
// # Exit codes and categories
//
// Exit codes 0 to 9 are fixed (see ExitCode). Errors reach them through a
// Category: any error that implements CategoryError maps to its exit code
// without the producing package importing this one's callers. An error with
// no category is a general error (exit 1).
//
// # Untrusted content
//
// Free text written by other people can contain instructions aimed at the
// model. The tool decides which fields are free text and wraps each in an
// Untrusted value. In JSON it carries "untrusted": true; in table and text
// output it is wrapped in explicit delimiters that include the author and
// timestamp. This is a mitigation, not a guarantee.
//
// # Bounds
//
// Output is capped (DefaultMaxBytes unless Bounds.MaxBytes says otherwise).
// When data is an array, whole items are dropped from the end; when data is a
// string, it is cut at a character boundary. Either way the result stays
// valid JSON and valid UTF-8, meta.truncated is set, and meta.next_offset
// tells the caller where to resume (an item index for arrays, a byte offset
// for strings) through Bounds.Offset.
//
// Object data is not cut unless Bounds.ArrayField names a top-level array in
// it: the array is then bounded like array data and every other field is kept.
// Meta.NextPageToken carries an upstream opaque continuation token.
//
// # Redaction
//
// Write scrubs error messages and hints with the shared redactor so they
// cannot carry tokens or request bodies.
package output
