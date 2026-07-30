# slack — amenbo's official Slack notification plugin

Report to a Slack channel what your AI did in a project while you were away from it.

```
AI created AMB-T-42 — Ship the thing
AI moved AMB-T-42 to in_progress — Ship the thing
AI finished AMB-T-42 — Ship the thing
```

Three things decide what arrives.

- **Only the AI's writes.** Every event names who drove it, and a write you drove yourself is
  one you were present for — a channel repeating it back to you is noise. What is worth a
  notification is the work that happened while nobody was watching it.
- **The channel is the setting.** `webhook_url` is a secret setting, and a setting belongs to a
  project, so the value itself is which channel a project reports to. Point two projects at two
  webhooks and they report to two channels; there is no channel name anywhere in the plugin.
- **Which events, you choose.** `events` is a list you tick, from everything amenbo fires. Its
  default is the four above, and both the choice and the channel are answered per project.

## Use

```sh
amenbo plugin install slack
amenbo plugin config set slack webhook_url https://hooks.slack.com/services/…
amenbo plugin enable slack            # installing never runs anything; this is the consent
```

Run these from the folder amenbo is bound to: the setting and the switch are that project's.

### What can be reported

Every event amenbo fires is on offer, and each one has its own sentence — one line. The four in
**bold** are what a channel gets until you say otherwise.

| Event | The message |
|---|---|
| **`task.created`** | `AI created <task>` |
| **`task.status_changed`** | `AI moved <task> to <status>` |
| **`task.done`** | `AI finished <task>` |
| **`task.rejected`** | `AI decided against <task>` |
| `task.assigned` | `AI assigned <task> to <who>` |
| `task.moved` | `AI moved <task> into <project>` |
| `task.deleted` | `AI deleted <task>` |
| `decision.accepted` | `AI accepted <decision>` |
| `decision.rejected` | `AI rejected <decision>` |
| `comment.added` | `AI added comment #<n>` |
| `comment.removed` | `AI took back a comment on <task>` |

`<task>` and `<decision>` are the record's ref and its title, read back from the store. Two of them
cannot be: a **deleted** task is gone, so its title comes off the vanished record the event carries
in its place, and a comment **added** names only itself — nothing on the wire says which task it
hangs on, and there is nothing to ask for one with. A comment **taken back** does name its task, so
that one is read back after all.

### A burst arrives as one message

Deleting a project, or clearing a pile of tasks, fires tens of events in a moment — and to you that
was one act. So a line waits while amenbo says anything is still queued for this plugin, and the run
that sees nothing behind it sends everything waiting as one message:

```
AI created AMB-T-42 — Ship the thing
AI moved AMB-T-42 to in_progress — Ship the thing
AI finished AMB-T-42 — Ship the thing
```

A quiet channel is unaffected: one event with nothing behind it is one message, as before. And a
batch is never a message held indefinitely — amenbo delivers as fast as it can, so what is waiting is
waiting on the events already queued in front of it, nothing else.

### Settings

| key | what it does |
|---|---|
| `webhook_url` | The Slack [incoming webhook](https://api.slack.com/messaging/webhooks) to post to. Secret, and required — a plugin with nowhere to post does nothing, so `enable` is refused until it holds a value. |
| `events` | What to report, ticked from the table above. Defaults to the four in bold. |

```sh
amenbo plugin config set slack events task.done,task.rejected     # only the terminals
```

**Choosing none is an answer, and it is honoured** — the plugin stays enabled and reports nothing,
which is a different thing from never having answered. Clearing the setting (an empty value) puts
the default back.

Being declared secret is what keeps the webhook out of an `export` and out of the `config` object on
stdin: amenbo hands it over in the environment instead, and this plugin reads it from there. It never
reads a secret file of its own. The choice is not a secret, so it arrives on stdin with the event.

## Why nothing arrived

A hook is fire-and-forget, so its failure surfaces nowhere you were listening. The execution
log is what answers:

```sh
amenbo plugin log slack
```

One line per run, and the diagnostics of any run that did not end cleanly. The webhook URL is
never in them — a log a channel can be posted to from is a leak.

## The contract, as this plugin reads it

A plugin is just an executable. amenbo starts it, writes one JSON document to its stdin, and
reads back what it wrote and how it exited. This one has a single face, the **observation
hook**: amenbo fires it with no arguments when an event fires, and nobody is waiting for an
answer. There is nothing here worth invoking on purpose, so an argument is refused rather than
answered.

```json
{ "v": 1, "event": "task.done", "id": 42, "actor": "ai", "at": "2026-07-22T09:00:00Z" }
```

**`v` is the contract version**, and it is first on the wire. New fields are added silently, so
a plugin ignores what it does not recognise; `v` moves only when an existing field's meaning
breaks — which is why a document announcing another version is dropped rather than guessed at.

**A payload names a record and carries none of it.** So the title in a message is read back,
and the way to read is the one every author already has:

```sh
amenbo task show 42 --json --actor ai
```

amenbo names the store to open and the window to read it through in the environment, because
neither can be worked out from where a plugin stands. This plugin passes both on untouched and
adds nothing of its own. The facet is declared because every operation that uses one requires
it; what it would otherwise settle — how far the reader reaches — the window has already
settled. See [`amenbo.go`](amenbo.go).

**A message goes out for every event it takes.** The send and the read are not the same
failure: a webhook that will not take the message means nothing was reported, while a title
that could not be read back costs the message its title and nothing else. So a message that
lost its title still goes out, naming the task by its number, and the run still ends non-zero
so the fault lands in the log instead of quietly shortening every message from then on.

**An event delivered twice is reported once.** A runner that died after this plugin took an event in,
but before amenbo could take that row off the queue, delivers it again — and nobody reading the
channel could tell that second line from a real one. So what has been taken in is written down, keyed
by what happened, to which record, at which moment, and a key already there stops the run before
anything is read, held or sent. The record is a bounded tail in the plugin's own installed directory,
which `plugin uninstall` takes away with everything else. **What is not stopped is you doing the same
thing twice**: two writes are two moments, and both are reported. See [`taken.go`](taken.go).

**How much is behind you is the runner's to say, and batching is the plugin's to do.** A plugin cannot
see its own queue — it is launched once per event — so the runner hands over how many are still
queued after this one. Zero is what a flush waits for, and zero is not a promise that nothing more is
coming: an event written a moment later is delivered like any other, so a batch may be followed by a
second one. That is one message becoming two, never a message lost. What is waiting is written down
before anything is sent, because a launch that does not come back would otherwise take it along. See
[`pending.go`](pending.go).

**Nothing is retried.** amenbo drops a failed event and never retries it; two sides retrying one send
is how one message becomes three, with nobody able to say why. A flush that Slack refused leaves its
lines still owed instead, so the next one carries them — they are late, not lost. See
[`hook.go`](hook.go) and [`slack.go`](slack.go).

## Build

A single static Go binary, no runtime and no dependencies:

```sh
make build     # → ./slack
make test      # gofmt, go vet, go test
```

To try a build before there is a release to install from, hand-install it into a throwaway
amenbo base:

```sh
make install AMENBO_BASE="$AMENBO_HOME"
amenbo plugin config set slack webhook_url <url>
amenbo plugin enable slack
```

That lays the binary down beside [`dev/manifest.json`](dev/manifest.json), a stand-in for the
entry the catalog holds: it carries the same fields, so the two can be read against each other.
Its `url` and `checksum` name a release that is not cut yet, and nothing about a hand-install
is verified anyway — the real install route resolves the catalog entry and checks the asset's
provenance before anything lands on disk.

### Releases

The distributables are baked in CI, not on a machine: pushing a `v*` tag runs
[`release.yml`](.github/workflows/release.yml), which bakes every asset key the catalog entry
publishes, uploads them, and prints their digests in the run summary for the entry to quote.
`make dist` is the same build, to check one before tagging it.

```sh
make dist      # → dist/slack-v1-*.tar.gz + sha256 digests
```

Everything cross-compiles from one runner — the plugin is pure Go over HTTP, so no asset needs a
C toolchain or a Mac. **A release is not a distribution:** nothing installs from those bytes
until the catalog entry points at them, and the signature that blesses an asset is the
catalog's, made on merge.

## License

Apache-2.0. See [LICENSE](LICENSE).
