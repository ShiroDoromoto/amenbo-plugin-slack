# slack — amenbo's official Slack notification plugin

Report to a Slack channel what your AI did in a project while you were away from it, and which
tasks have run out of days.

```
*amenbo-plugin-slack*
AI created AMB-T-42 — Ship the thing
AI moved AMB-T-42 to In progress — Ship the thing
AI finished AMB-T-42 — Ship the thing
```

Three things decide what arrives.

- **Only the AI's writes, and the days nobody wrote.** Every event names who drove it, and a write
  you drove yourself is one you were present for — a channel repeating it back to you is noise. What
  is worth a notification is what happened while nobody was watching: the AI's work, and a due date
  arriving, which is nobody's write at all.
- **The channel is the setting.** `webhook_url` is a secret setting, and a setting belongs to a
  project, so the value itself is which channel a project reports to. Point two projects at two
  webhooks and they report to two channels; there is no channel name anywhere in the plugin. And
  every message says which project it came from, so pointing two of them at one channel still
  reads.
- **Which events, you choose.** `events` is a list you tick, from everything amenbo fires. Its
  default is the four above plus the two due dates, and both the choice and the channel are answered
  per project.

## Use

```sh
amenbo plugin install slack
printf %s 'https://hooks.slack.com/services/…' | amenbo plugin config set slack webhook_url -
amenbo plugin enable slack            # installing never runs anything; this is the consent
```

Run these from the folder amenbo is bound to: the setting and the switch are that project's. The
webhook goes in through `-`, which reads it from stdin — written as an argument it would sit in the
shell's history and in anything reading the process list, and a webhook is a credential.

Switching it on is where the URL is first read: a value that is not shaped like a webhook is said
so there, rather than at the first event that fails to post ([below](#whether-the-webhook-is-one)).

### Getting the webhook URL

The webhook is Slack's, not amenbo's: it is made in the workspace, points at one channel, and is the
whole of what this plugin is ever told about where to post. From
[Your Apps](https://api.slack.com/apps), create an app in the workspace that should hear about the
project, turn on **Incoming Webhooks**, add one to the channel it should post to, and copy the URL it
hands back — that URL *is* the channel, so a webhook for the wrong channel is a plugin reporting to
the wrong room with nothing else set wrong. Slack's own walkthrough is [Sending messages using
incoming webhooks](https://api.slack.com/messaging/webhooks).

Anyone holding the URL can post to that channel, which is why it is declared secret here: it leaves
amenbo only into this plugin's environment, and never into an export, a log or a read-back.

### One channel per project

A setting belongs to the project it was written in and there is no tier under it, so which channel a
project reports to is answered in that project and nowhere else. Pointing a second project at a
second channel is two commands run where that project is bound; the install is the machine's and is
already behind you:

```sh
cd ~/work/other-project
printf %s 'https://hooks.slack.com/services/…' | amenbo plugin config set slack webhook_url -
amenbo plugin enable slack
```

Nothing carries over between them, and that is the point twice: a project you have not answered for
holds no webhook, so `enable` there is refused rather than quietly borrowing another project's
channel — and the gate is per project too, so switching the plugin off in one leaves the others
reporting.

What the plugin keeps between runs is split the same way. A store holds every project at once, so
the lines a burst is holding and the record of what has already been taken in are kept under the
project they came from — otherwise the run that flushed a batch would post whatever was waiting into
whichever channel it happened to be launched for.

### Which project a message came from

Every message leads with the project's name, in bold:

```
*amenbo-plugin-slack*
AI created AMB-T-42 — Ship the thing
```

The name is the one amenbo holds for the project the plugin was launched for — not a folder name and
not a guess — and it is read at the moment of sending: once for the message, rather than once for
every line, since every line in a batch comes from the same project. It is said at the top for the
same reason: repeating it down the left of a batch would say nothing the first line has not.

A name that cannot be read costs the message its heading and nothing else: the message still goes
out, and the run ends non-zero so the reason is in `amenbo plugin log slack`.

### What can be reported

Every event amenbo fires is on offer, and each one has its own sentence — one line. The six in
**bold** are what a channel gets until you say otherwise.

| Event | The message |
|---|---|
| **`task.created`** | `<AI> created <task>` |
| **`task.status_changed`** | `<AI> moved <task> to <status>` |
| **`task.done`** | `<AI> finished <task>` |
| **`task.rejected`** | `<AI> decided against <task>` |
| `task.assigned` | `<AI> assigned <task> to <who>` |
| `task.moved` | `<AI> moved <task> into <project>` |
| `task.deleted` | `<AI> deleted <task>` |
| `decision.accepted` | `<AI> accepted <decision>` |
| `decision.rejected` | `<AI> rejected <decision>` |
| `comment.added` | `<AI> added a comment on <task>` |
| `comment.removed` | `<AI> took back a comment on <task>` |
| **`task.due`** | `<task> is due` |
| **`task.due_tomorrow`** | `<task> is due tomorrow` |

`<task>` and `<decision>` are the record's ref and its title, read back from the store. One of them
cannot be: a **deleted** task is gone, so its title comes off the vanished record the event carries
in its place. A comment is read back by the task it hangs on rather than by itself — its own number
belongs to a timeline nobody in the channel is looking at — and an amenbo old enough not to send
that task falls back to naming the comment by its number.

`<AI>` and `<who>` are the names your AI and you go by, and the sentences above are the English
ones — all of it is read back from amenbo too. See below.

### The two that nobody did

The last two rows have no `<AI>` in them, and that is the whole of what makes them different. A due
date is not something anyone wrote: the day came. So the rule above — only the AI's writes — has
nothing to catch there, and it is the same rule that says they belong in the channel, since a day
arriving is the event nobody is at the desk for.

They are amenbo's own, not this plugin's. Something in the machine's schedule wakes amenbo up, it
reads the due dates against that machine's calendar day, and it fires one event per task, once a day
— `task.due` for what is due today or already past, `task.due_tomorrow` for what has one day left.
The two are the red and the yellow of the app's own chips, so a channel and a screen never disagree
about which a task is in. There is no hour to choose and none to guess at, and a task left open is
named again the next day.

A day can name a dozen tasks at once, so a message keeps the two kinds apart rather than
interleaving them in whatever order they were delivered:

```
*amenbo-plugin-slack*
AI finished AMB-T-42 — Ship the thing

AMB-T-9 is due — Renew the certificate
AMB-T-11 is due — Pay the invoice

AMB-T-12 is due tomorrow — Book the room
```

What was done comes first and reads in the order it happened; then what is already standing, then
what arrives tomorrow. Being quiet at night is the channel's own business — Slack has a Do Not
Disturb, and amenbo does not override it by holding a notification until morning.

### The language a message is in

A message is written in the language amenbo is set to, and the two who appear in it — your AI, and
you when it hands something over — are called what you named them. None of it is a setting here:
you have answered all three already, in **Settings** in the app, and this plugin reads them back
rather than asking a second time.

```sh
amenbo config set language ja
amenbo config set ai_name そらまめ
amenbo config set human_name 山田
```

```
*amenbo-plugin-slack*
そらまめ が AMB-T-42 を完了しました — Ship the thing
```

All nineteen languages amenbo offers are here: `de` `en` `es` `fr` `hi` `id` `it` `ja` `ko` `nl`
`pl` `pt-BR` `ru` `th` `tr` `uk` `vi` `zh-Hans` `zh-Hant`.

**What is translated is the sentence, and one word inside it.** The word is a task's status, and it
is worded the way the app words it — a channel calling a state something amenbo does not would be
naming something you cannot go and find. Everything else in a line is yours and arrives as you
wrote it: the title, the ref, the name of the project a task moved into. So are the diagnostics in
`amenbo plugin log slack` and this README — those are read by whoever is installing the plugin, not
by the channel.

Who a task was **assigned** to arrives as `ai` or `human` — a contract value, not a name — so it is
said by the name that party goes by, the same one the sentence's subject is said by. Setting a name
is optional: until you set one, amenbo answers with a name of its own, in the language it is set
to. A name is reached for and never invented here, so `ai` and `human` reach a line only when the
settings could not be read at all — the same read whose failure costs the line its language.

**Nothing here can cost you a notification.** A language this build has never heard of — one amenbo
adds after this release — is reported in English, as is a status with no word yet; and settings that
could not be read at all cost a line its language and nothing else, the message still going out with
the run ending non-zero so the reason is in the log.

### A burst arrives as one message

Deleting a project, clearing a pile of tasks, or a day's due dates coming round, fires tens of events
in a moment — and to you that was one act, or no act at all. So a line waits while amenbo says
anything is still queued for this project, and the run that sees nothing behind it sends everything
waiting as one message:

```
*amenbo-plugin-slack*
AI created AMB-T-42 — Ship the thing
AI moved AMB-T-42 to In progress — Ship the thing
AI finished AMB-T-42 — Ship the thing
```

A quiet channel is unaffected: one event with nothing behind it is one message, as before. While the
sends are getting through, what is waiting is waiting on the events queued in front of it and no
longer — amenbo delivers as fast as it can.

While they are not getting through, the lines stay owed and pile up, so the hold stops at **200
messages** and the oldest fall off. `amenbo plugin log slack` says how many were dropped, and when.
Losing the oldest lines is the cheaper loss: a webhook that has been revoked refuses every flush, and
a batch left to grow becomes a message longer than Slack will take — at which point fixing the
webhook would no longer fix the channel.

Two projects on one machine keep out of each other's way: the count amenbo hands over is this
project's queue, and the lines held are this project's too, so each channel flushes on its own quiet
moment and neither holds the other's lines back.

### Settings

| key | what it does |
|---|---|
| `webhook_url` | The Slack [incoming webhook](https://api.slack.com/messaging/webhooks) to post to. Secret, and required — a plugin with nowhere to post does nothing, so `enable` is refused until it holds a value. |
| `events` | What to report, ticked from the table above. Defaults to the six in bold. |

```sh
amenbo plugin config set slack events task.done,task.rejected     # only the terminals
```

**Choosing none is an answer, and it is honoured** — the plugin stays enabled and reports nothing,
which is a different thing from never having answered. Clearing the setting (an empty value) puts
the default back.

The short of both is in the manifest too — a `help` line under each field, and the webhook's shape as
a `placeholder` in the empty box — so the setting form answers there for whoever never opens this
file. Reword one and reword the other, and the catalog entry's translations with them.

The placeholder elides the parts that vary (`T…/B…/…`) instead of spelling a plausible URL out. A
full-shaped webhook is what a secret scanner blocks a push over, and here it would be blocking a
string nobody can post to.

Being declared secret is what keeps the webhook out of an `export` and out of the `config` object on
stdin: amenbo hands it over in the environment instead, and this plugin reads it from there. It never
reads a secret file of its own. The choice is not a secret, so it arrives on stdin with the event.

### Whether the webhook is one

A pasted URL is right or wrong long before an event fires, so the settings form asks this plugin
twice — once by itself, and once when you press:

| | When | What it does |
|---|---|---|
| the check | enabling, and after a save while enabled | reads the URL's shape. **Never posts** |
| **Send a test message** | you press it | posts one line to the channel |

```sh
amenbo --actor ai plugin run slack config check    # {"v":1,"ok":true,…}
amenbo --actor ai plugin run slack config test     # one line, into the channel
```

They are split by what they may cost. The check runs unasked and is held to two seconds, so a check
that posted would put a line in the channel every time you saved — it reads `https://hooks.slack.com`
and `/services/` and three parts after it, and stops there. **A check that does not say yes leaves
the plugin disabled**, which is why it says nothing about whether the webhook still works: a URL
revoked yesterday still has the shape, and only a real post can tell you otherwise. That post is the
button.

The verdict's sentences go on the settings form and nowhere else — they quote none of what you
pasted, the CLI's refusal names the box and not the text, and `amenbo agent --json` carries neither.
The test message is a line in a channel like any other, so it is written in the language you read
amenbo in and headed by the project you pressed in.

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
reads back what it wrote and how it exited. Its face is the **observation hook**: amenbo fires
it with no arguments when an event fires, and nobody is waiting for an answer.

Two calls stand beside it, `config check` and `config test`, and they are the settings form's
rather than a terminal's — [above](#whether-the-webhook-is-one). The manifest declares them under
`settings`; amenbo raises them down the road every other run takes, so they arrive with the same
injected settings and land in the same execution log. Any other argument is refused.

```json
{ "v": 1, "event": "task.done", "id": 42, "actor": "ai", "at": "2026-07-22T09:00:00Z" }
```

**`v` is the contract version**, and it is first on the wire. New fields are added silently, so
a plugin ignores what it does not recognise; `v` moves only when an existing field's meaning
breaks — which is why a document announcing another version is dropped rather than guessed at.

**A payload names a record and carries none of it**, and it says nothing about how to word what it
names. So both are read back, and the way to read is the one every author already has:

```sh
amenbo task show 42 --json --actor ai
amenbo project show AMB-P-1 --json --actor ai    # the heading, on the run that sends
amenbo config --json --actor ai                  # the language and the AI's name, per line worded
```

amenbo names the store to open and the window to read it through in the environment, because
neither can be worked out from where a plugin stands. This plugin passes both on untouched and
adds nothing of its own. The facet is declared because every operation that uses one requires
it; what it would otherwise settle — how far the reader reaches — the window has already
settled. See [`amenbo.go`](amenbo.go).

**A message goes out for every event it takes.** The send and the reads are not the same
failure: a webhook that will not take the message means nothing was reported, while a title
that could not be read back costs the message its title and nothing else — settings that could not
be read cost it the language, and a project that could not be read costs it the heading. So a
message that lost any of them still goes out, naming the task by its number and worded in English,
and the run still ends non-zero so the fault lands in the log instead of quietly shortening every
message from then on.

**An event delivered twice is reported once.** A runner that died after this plugin took an event in,
but before amenbo could take that row off the queue, delivers it again — and nobody reading the
channel could tell that second line from a real one. So what has been taken in is written down, keyed
by what happened, to which record, at which moment, and a key already there stops the run before
anything is read, held or sent. The record is a bounded tail in the plugin's own installed directory,
under the project it is about, which `plugin uninstall` takes away with everything else. **What is
not stopped is you doing the same thing twice**: two writes are two moments, and both are reported.
See [`taken.go`](taken.go).

**How much is behind you is the runner's to say, and batching is the plugin's to do.** A plugin cannot
see its own queue — it is launched once per event — so the runner hands over how many are still
queued after this one for the project this launch fires for. Zero is what a flush waits for, and it is
a zero this project can act on: the count, the lines held and the channel they go to are all the one
project's. Zero is not a promise that nothing more is coming: an event written a moment later is
delivered like any other, so a batch may be followed by a second one. That is one message becoming
two, never a message lost. What is waiting is written down before anything is sent, because a launch
that does not come back would otherwise take it along — and it is written down under the project it
belongs to, the window amenbo hands over for reading records back being what tells one project's
state from another's. A build that kept that state for the whole store left behind files no project
can claim, so the first run under the split drops them rather than delivering them to a guess. See
[`pending.go`](pending.go).

**Nothing is retried.** amenbo drops a failed event and never retries it; two sides retrying one send
is how one message becomes three, with nobody able to say why. A flush that Slack refused leaves its
lines still owed instead, so the next one carries them — they are late, not lost. What is owed is
bounded for the same reason it is kept: a refusal that never stops being one would otherwise pile
lines up for good, so the oldest fall off past 200 and the log says how many. See [`hook.go`](hook.go)
and [`slack.go`](slack.go).

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

A local sink is the way to watch a build post without a channel taking the lines — and it will not
get past `enable`, since it is not shaped like a Slack webhook and the check says so. Hand the URL
over as `AMENBO_CONFIG_WEBHOOK_URL` instead, which is where the plugin reads it from anyway, and the
gate is not in the way of a build nobody has released yet.

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
make dist      # → dist/slack-<version>-*.tar.gz + sha256 digests
```

The version in those names is the tag being released, so nothing has to be bumped by hand for a
release to be named after itself; run off a commit no tag stands on, the build calls itself `dev`.

Everything cross-compiles from one runner — the plugin is pure Go over HTTP, so no asset needs a
C toolchain or a Mac. **A release is not a distribution:** nothing installs from those bytes
until the catalog entry points at them, and the signature that blesses an asset is the
catalog's, made on merge.

## License

Apache-2.0. See [LICENSE](LICENSE).
