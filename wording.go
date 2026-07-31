package main

import "strings"

// This file is what a line says, and the only place it is spelled. Everything else in this plugin
// decides *whether* there is a line and *which record* it is about; the words themselves live here,
// once per language, so adding a language is adding a row and nothing else.
//
// The store is what settles which row is read. amenbo already knows the language the user reads in
// and the name their AI goes by, so this plugin asks rather than answers, and keeps no language
// setting of its own — one question, answered once, in the place the user already answered it
// (see readPreferences).

// fallbackLanguage is the row every other one is read against: the one that must be complete. A
// language code this build has never heard of falls back to it whole, and a row that has not been
// filled in yet falls back to it phrase by phrase — so a translation can arrive in pieces without
// any of them being the piece that leaves a message blank.
const fallbackLanguage = "en"

// The parts a sentence is put together from. A wording says where they go; it never says what is
// in them. Three of the four hold something this plugin must not translate — the name the user
// gave their AI, the record's own ref, a project's slug or an assignee's facet — and the fourth is
// an event name off the wire.
const (
	slotWho   = "{who}"
	slotWhat  = "{what}"
	slotState = "{state}"
	slotEvent = "{event}"
)

// titleJoin sets a title apart from the sentence that leads to it. It is punctuation rather than
// wording, and what it separates is the user's own text, so it is not a language's to choose: a
// title is quoted the same way in every message this plugin sends.
const titleJoin = " — "

// say is one event's sentence in the two forms it may need. `full` is the one that has somewhere
// to put the second thing the event names — the status a task moved to, who it went to, which
// project it went into, the task a comment hangs on — and `bare` is what is said when that did not
// arrive. An event that never names a second thing fills `bare` alone.
type say struct {
	full string
	bare string
}

// wording is one language's side of every message: a sentence per event, a sentence for an event
// this build has no wording for, and a word per status.
type wording struct {
	// says is keyed by the event, so a translator sees the same eleven keys the manifest
	// subscribes to and the user chooses among.
	says map[string]say
	// unknown is what a twelfth event is reported as. It cannot be reached through hook, which
	// filters on the catalog first, but naming the event beats an empty message for a caller
	// that hands one over.
	unknown string
	// statuses is amenbo's own word for each state a task can be in. A channel that invented its
	// own would be naming a state the user cannot find in the app, so these are taken from
	// amenbo's dictionary rather than translated afresh.
	statuses map[string]string
}

// wordings is every language this build can write a line in, keyed by amenbo's language code.
//
// What is *not* here is as deliberate as what is: a task's title, a project's slug, an assignee's
// facet and a record's ref are the user's own data and travel through a sentence untouched. So do
// the diagnostics on stderr and everything in `help` — those are read by whoever is installing the
// plugin, not by the channel.
var wordings = map[string]wording{
	"en": {
		says: map[string]say{
			eventTaskCreated:      {bare: "{who} created {what}"},
			eventStatusChanged:    {full: "{who} moved {what} to {state}", bare: "{who} moved {what}"},
			eventTaskDone:         {bare: "{who} finished {what}"},
			eventTaskRejected:     {bare: "{who} decided against {what}"},
			eventTaskAssigned:     {full: "{who} assigned {what} to {state}", bare: "{who} assigned {what}"},
			eventTaskMoved:        {full: "{who} moved {what} into {state}", bare: "{who} moved {what} to another project"},
			eventTaskDeleted:      {bare: "{who} deleted {what}"},
			eventDecisionAccepted: {bare: "{who} accepted {what}"},
			eventDecisionRejected: {bare: "{who} rejected {what}"},
			eventCommentAdded:     {full: "{who} added a comment on {what}", bare: "{who} added {what}"},
			eventCommentRemoved:   {full: "{who} took back a comment on {what}", bare: "{who} took back {what}"},
		},
		unknown: "{who} acted on {what} ({event})",
		statuses: map[string]string{
			"todo":        "To do",
			"in_progress": "In progress",
			"done":        "Done",
			"blocked":     "Blocked",
			"rejected":    "Rejected",
		},
	},
	"ja": {
		says: map[string]say{
			eventTaskCreated:      {bare: "{who} が {what} を作成しました"},
			eventStatusChanged:    {full: "{who} が {what} を{state}に変更しました", bare: "{who} が {what} の状態を変更しました"},
			eventTaskDone:         {bare: "{who} が {what} を完了しました"},
			eventTaskRejected:     {bare: "{who} が {what} をやらないと決めました"},
			eventTaskAssigned:     {full: "{who} が {what} を {state} に割り当てました", bare: "{who} が {what} を割り当てました"},
			eventTaskMoved:        {full: "{who} が {what} を {state} へ移動しました", bare: "{who} が {what} を別のプロジェクトへ移動しました"},
			eventTaskDeleted:      {bare: "{who} が {what} を削除しました"},
			eventDecisionAccepted: {bare: "{who} が {what} を採択しました"},
			eventDecisionRejected: {bare: "{who} が {what} を却下しました"},
			eventCommentAdded:     {full: "{who} が {what} にコメントしました", bare: "{who} が {what} を追加しました"},
			eventCommentRemoved:   {full: "{who} が {what} のコメントを取り消しました", bare: "{who} が {what} を取り消しました"},
		},
		unknown: "{who} が {what} に対して操作しました（{event}）",
		statuses: map[string]string{
			"todo":        "未着手",
			"in_progress": "進行中",
			"done":        "完了",
			"blocked":     "ブロック",
			"rejected":    "却下",
		},
	},
}

// sentence is the message: what the AI did, to which record, in the language the store reads in and
// under the name it gives its AI.
func sentence(how preferences, in input, about subject) string {
	said := strings.NewReplacer(
		slotWho, how.aiDisplayName,
		slotWhat, about.name,
		slotState, stateWord(how.language, in.Event, in.New),
		slotEvent, in.Event,
	).Replace(saying(how.language, in.Event, elaborated(in)))
	if about.title == "" {
		return said
	}
	return said + titleJoin + about.title
}

// elaborated says which of a sentence's two forms this event calls for — whether the second thing
// it would name arrived. For a comment that is the task it hangs on, which an amenbo old enough
// carries none of; for every other event it is the state the record moved to.
func elaborated(in input) bool {
	switch in.Event {
	case eventCommentAdded, eventCommentRemoved:
		return in.Parent != nil
	default:
		return in.New != ""
	}
}

// saying picks the sentence to fill in. A language with no row of its own, and a row with nothing
// under this event, are the same case to a reader in a channel: they get the English one, which is
// the row that is always complete.
func saying(language, event string, full bool) string {
	form, known := wordings[language].says[event]
	if !known {
		form, known = wordings[fallbackLanguage].says[event]
	}
	if !known {
		return unknownSaying(language)
	}
	if full && form.full != "" {
		return form.full
	}
	return form.bare
}

// unknownSaying is what a twelfth event is reported as.
func unknownSaying(language string) string {
	if said := wordings[language].unknown; said != "" {
		return said
	}
	return wordings[fallbackLanguage].unknown
}

// stateWord is what goes where a sentence names the second thing.
//
// Only one of the three is a word amenbo owns: a status. The other two — the facet a task was
// assigned to, the slug of the project it moved into — are the store's own values, and a channel
// that translated them would be naming something the user cannot search for. So they pass through,
// and a status this build does not have a word for passes through as well: the value off the wire
// says more than nothing does.
func stateWord(language, event, newState string) string {
	if event != eventStatusChanged || newState == "" {
		return newState
	}
	if word := wordings[language].statuses[newState]; word != "" {
		return word
	}
	if word := wordings[fallbackLanguage].statuses[newState]; word != "" {
		return word
	}
	return newState
}
