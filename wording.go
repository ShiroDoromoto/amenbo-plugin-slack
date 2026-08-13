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
	// test is the line the settings form's test message carries. It is the one sentence here
	// that no event produces — a person pressed a button — but it lands in the same channel as
	// every other, so it is worded here rather than left in the language the author writes in.
	test string
	// statuses is amenbo's own word for each state a task can be in. A channel that invented its
	// own would be naming a state the user cannot find in the app, so these are taken from
	// amenbo's dictionary rather than translated afresh.
	statuses map[string]string
}

// wordings is every language this build can write a line in, keyed by amenbo's language code and
// ordered by it. All nineteen amenbo offers are here — the code has to be spelled the way the store
// spells it, `pt-BR` and not `pt-br`, or the row is one nothing ever reads.
//
// The statuses are not translated here: they are amenbo's own words, taken from its dictionary, so
// that a state reads the same in a channel as in the app the reader would go and look it up in. The
// sentences around them are this file's.
//
// What is *not* here is as deliberate as what is: a task's title, a project's slug, an assignee's
// facet and a record's ref are the user's own data and travel through a sentence untouched. So do
// the diagnostics on stderr and everything in `help` — those are read by whoever is installing the
// plugin, not by the channel.
var wordings = map[string]wording{
	"de": {
		says: map[string]say{
			eventTaskCreated:      {bare: "{who} hat {what} erstellt"},
			eventStatusChanged:    {full: "{who} hat {what} auf {state} gesetzt", bare: "{who} hat den Status von {what} geändert"},
			eventTaskDone:         {bare: "{who} hat {what} erledigt"},
			eventTaskRejected:     {bare: "{who} hat entschieden, {what} nicht zu machen"},
			eventTaskAssigned:     {full: "{who} hat {what} an {state} übergeben", bare: "{who} hat {what} zugewiesen"},
			eventTaskMoved:        {full: "{who} hat {what} nach {state} verschoben", bare: "{who} hat {what} in ein anderes Projekt verschoben"},
			eventTaskDeleted:      {bare: "{who} hat {what} gelöscht"},
			eventDecisionAccepted: {bare: "{who} hat {what} angenommen"},
			eventDecisionRejected: {bare: "{who} hat {what} abgelehnt"},
			eventCommentAdded:     {full: "{who} hat {what} kommentiert", bare: "{who} hat {what} hinzugefügt"},
			eventCommentRemoved:   {full: "{who} hat einen Kommentar zu {what} zurückgenommen", bare: "{who} hat {what} zurückgenommen"},
		},
		unknown: "{who} hat etwas an {what} gemacht ({event})",
		test:    "Testnachricht von amenbo — dieses Projekt meldet hierher.",
		statuses: map[string]string{
			"todo":        "Offen",
			"in_progress": "In Arbeit",
			"done":        "Erledigt",
			"blocked":     "Blockiert",
			"rejected":    "Verworfen",
		},
	},
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
		test:    "Test message from amenbo — this project reports here.",
		statuses: map[string]string{
			"todo":        "To do",
			"in_progress": "In progress",
			"done":        "Done",
			"blocked":     "Blocked",
			"rejected":    "Rejected",
		},
	},
	"es": {
		says: map[string]say{
			eventTaskCreated:      {bare: "{who} creó {what}"},
			eventStatusChanged:    {full: "{who} cambió {what} a {state}", bare: "{who} cambió el estado de {what}"},
			eventTaskDone:         {bare: "{who} terminó {what}"},
			eventTaskRejected:     {bare: "{who} decidió no hacer {what}"},
			eventTaskAssigned:     {full: "{who} asignó {what} a {state}", bare: "{who} asignó {what}"},
			eventTaskMoved:        {full: "{who} movió {what} a {state}", bare: "{who} movió {what} a otro proyecto"},
			eventTaskDeleted:      {bare: "{who} eliminó {what}"},
			eventDecisionAccepted: {bare: "{who} aceptó {what}"},
			eventDecisionRejected: {bare: "{who} rechazó {what}"},
			eventCommentAdded:     {full: "{who} comentó en {what}", bare: "{who} añadió {what}"},
			eventCommentRemoved:   {full: "{who} retiró un comentario de {what}", bare: "{who} retiró {what}"},
		},
		unknown: "{who} hizo algo en {what} ({event})",
		test:    "Mensaje de prueba de amenbo: este proyecto informa aquí.",
		statuses: map[string]string{
			"todo":        "Pendiente",
			"in_progress": "En curso",
			"done":        "Hecho",
			"blocked":     "Bloqueado",
			"rejected":    "Descartado",
		},
	},
	"fr": {
		says: map[string]say{
			eventTaskCreated:      {bare: "{who} a créé {what}"},
			eventStatusChanged:    {full: "{who} a fait passer {what} à {state}", bare: "{who} a changé l'état de {what}"},
			eventTaskDone:         {bare: "{who} a terminé {what}"},
			eventTaskRejected:     {bare: "{who} a décidé de ne pas faire {what}"},
			eventTaskAssigned:     {full: "{who} a affecté {what} à {state}", bare: "{who} a affecté {what}"},
			eventTaskMoved:        {full: "{who} a déplacé {what} vers {state}", bare: "{who} a déplacé {what} vers un autre projet"},
			eventTaskDeleted:      {bare: "{who} a supprimé {what}"},
			eventDecisionAccepted: {bare: "{who} a accepté {what}"},
			eventDecisionRejected: {bare: "{who} a rejeté {what}"},
			eventCommentAdded:     {full: "{who} a commenté {what}", bare: "{who} a ajouté {what}"},
			eventCommentRemoved:   {full: "{who} a retiré un commentaire sur {what}", bare: "{who} a retiré {what}"},
		},
		unknown: "{who} est intervenu sur {what} ({event})",
		test:    "Message de test d'amenbo — ce projet rend compte ici.",
		statuses: map[string]string{
			"todo":        "À faire",
			"in_progress": "En cours",
			"done":        "Terminée",
			"blocked":     "Bloquée",
			"rejected":    "Écartée",
		},
	},
	"hi": {
		says: map[string]say{
			eventTaskCreated:      {bare: "{who} ने {what} बनाया"},
			eventStatusChanged:    {full: "{who} ने {what} को {state} किया", bare: "{who} ने {what} की स्थिति बदली"},
			eventTaskDone:         {bare: "{who} ने {what} पूरा किया"},
			eventTaskRejected:     {bare: "{who} ने {what} न करने का तय किया"},
			eventTaskAssigned:     {full: "{who} ने {what} {state} को सौंपा", bare: "{who} ने {what} सौंपा"},
			eventTaskMoved:        {full: "{who} ने {what} को {state} में डाला", bare: "{who} ने {what} को दूसरे प्रोजेक्ट में डाला"},
			eventTaskDeleted:      {bare: "{who} ने {what} मिटाया"},
			eventDecisionAccepted: {bare: "{who} ने {what} स्वीकार किया"},
			eventDecisionRejected: {bare: "{who} ने {what} अस्वीकार किया"},
			eventCommentAdded:     {full: "{who} ने {what} पर टिप्पणी की", bare: "{who} ने {what} जोड़ा"},
			eventCommentRemoved:   {full: "{who} ने {what} पर की टिप्पणी वापस ली", bare: "{who} ने {what} वापस लिया"},
		},
		unknown: "{who} ने {what} पर कुछ किया ({event})",
		test:    "amenbo से परीक्षण संदेश — यह प्रोजेक्ट यहीं बताएगा।",
		statuses: map[string]string{
			"todo":        "करना है",
			"in_progress": "चल रहा है",
			"done":        "पूरा",
			"blocked":     "रुका हुआ",
			"rejected":    "अस्वीकृत",
		},
	},
	"id": {
		says: map[string]say{
			eventTaskCreated:      {bare: "{who} membuat {what}"},
			eventStatusChanged:    {full: "{who} mengubah {what} menjadi {state}", bare: "{who} mengubah status {what}"},
			eventTaskDone:         {bare: "{who} menyelesaikan {what}"},
			eventTaskRejected:     {bare: "{who} memutuskan tidak mengerjakan {what}"},
			eventTaskAssigned:     {full: "{who} menugaskan {what} ke {state}", bare: "{who} menugaskan {what}"},
			eventTaskMoved:        {full: "{who} memindahkan {what} ke {state}", bare: "{who} memindahkan {what} ke proyek lain"},
			eventTaskDeleted:      {bare: "{who} menghapus {what}"},
			eventDecisionAccepted: {bare: "{who} menerima {what}"},
			eventDecisionRejected: {bare: "{who} menolak {what}"},
			eventCommentAdded:     {full: "{who} mengomentari {what}", bare: "{who} menambahkan {what}"},
			eventCommentRemoved:   {full: "{who} menarik komentar pada {what}", bare: "{who} menarik {what}"},
		},
		unknown: "{who} melakukan sesuatu pada {what} ({event})",
		test:    "Pesan uji dari amenbo — proyek ini melapor ke sini.",
		statuses: map[string]string{
			"todo":        "Akan dikerjakan",
			"in_progress": "Sedang dikerjakan",
			"done":        "Selesai",
			"blocked":     "Terhambat",
			"rejected":    "Ditolak",
		},
	},
	"it": {
		says: map[string]say{
			eventTaskCreated:      {bare: "{who} ha creato {what}"},
			eventStatusChanged:    {full: "{who} ha portato {what} a {state}", bare: "{who} ha cambiato lo stato di {what}"},
			eventTaskDone:         {bare: "{who} ha completato {what}"},
			eventTaskRejected:     {bare: "{who} ha deciso di non fare {what}"},
			eventTaskAssigned:     {full: "{who} ha assegnato {what} a {state}", bare: "{who} ha assegnato {what}"},
			eventTaskMoved:        {full: "{who} ha spostato {what} in {state}", bare: "{who} ha spostato {what} in un altro progetto"},
			eventTaskDeleted:      {bare: "{who} ha eliminato {what}"},
			eventDecisionAccepted: {bare: "{who} ha accettato {what}"},
			eventDecisionRejected: {bare: "{who} ha respinto {what}"},
			eventCommentAdded:     {full: "{who} ha commentato {what}", bare: "{who} ha aggiunto {what}"},
			eventCommentRemoved:   {full: "{who} ha ritirato un commento su {what}", bare: "{who} ha ritirato {what}"},
		},
		unknown: "{who} è intervenuto su {what} ({event})",
		test:    "Messaggio di prova da amenbo — questo progetto riferisce qui.",
		statuses: map[string]string{
			"todo":        "Da fare",
			"in_progress": "In corso",
			"done":        "Fatta",
			"blocked":     "Bloccata",
			"rejected":    "Scartata",
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
		test:    "amenbo からのテスト送信です。このプロジェクトはここへ報告します。",
		statuses: map[string]string{
			"todo":        "未着手",
			"in_progress": "進行中",
			"done":        "完了",
			"blocked":     "ブロック",
			"rejected":    "却下",
		},
	},
	"ko": {
		says: map[string]say{
			eventTaskCreated:      {bare: "{who}이(가) {what}을(를) 만들었습니다"},
			eventStatusChanged:    {full: "{who}이(가) {what}을(를) {state}(으)로 바꿨습니다", bare: "{who}이(가) {what}의 상태를 바꿨습니다"},
			eventTaskDone:         {bare: "{who}이(가) {what}을(를) 완료했습니다"},
			eventTaskRejected:     {bare: "{who}이(가) {what}을(를) 하지 않기로 했습니다"},
			eventTaskAssigned:     {full: "{who}이(가) {what}을(를) {state}에게 맡겼습니다", bare: "{who}이(가) {what}의 담당을 정했습니다"},
			eventTaskMoved:        {full: "{who}이(가) {what}을(를) {state}(으)로 옮겼습니다", bare: "{who}이(가) {what}을(를) 다른 프로젝트로 옮겼습니다"},
			eventTaskDeleted:      {bare: "{who}이(가) {what}을(를) 삭제했습니다"},
			eventDecisionAccepted: {bare: "{who}이(가) {what}을(를) 채택했습니다"},
			eventDecisionRejected: {bare: "{who}이(가) {what}을(를) 기각했습니다"},
			eventCommentAdded:     {full: "{who}이(가) {what}에 댓글을 남겼습니다", bare: "{who}이(가) {what}을(를) 남겼습니다"},
			eventCommentRemoved:   {full: "{who}이(가) {what}의 댓글을 거뒀습니다", bare: "{who}이(가) {what}을(를) 거뒀습니다"},
		},
		unknown: "{who}이(가) {what}에 무언가 했습니다 ({event})",
		test:    "amenbo에서 보낸 테스트 메시지입니다. 이 프로젝트는 여기로 보고합니다.",
		statuses: map[string]string{
			"todo":        "할 일",
			"in_progress": "진행 중",
			"done":        "완료",
			"blocked":     "막힘",
			"rejected":    "기각",
		},
	},
	"nl": {
		says: map[string]say{
			eventTaskCreated:      {bare: "{who} heeft {what} aangemaakt"},
			eventStatusChanged:    {full: "{who} heeft {what} op {state} gezet", bare: "{who} heeft de status van {what} gewijzigd"},
			eventTaskDone:         {bare: "{who} heeft {what} afgerond"},
			eventTaskRejected:     {bare: "{who} heeft besloten {what} niet te doen"},
			eventTaskAssigned:     {full: "{who} heeft {what} aan {state} toegewezen", bare: "{who} heeft {what} toegewezen"},
			eventTaskMoved:        {full: "{who} heeft {what} naar {state} verplaatst", bare: "{who} heeft {what} naar een ander project verplaatst"},
			eventTaskDeleted:      {bare: "{who} heeft {what} verwijderd"},
			eventDecisionAccepted: {bare: "{who} heeft {what} aangenomen"},
			eventDecisionRejected: {bare: "{who} heeft {what} afgewezen"},
			eventCommentAdded:     {full: "{who} heeft op {what} gereageerd", bare: "{who} heeft {what} toegevoegd"},
			eventCommentRemoved:   {full: "{who} heeft een reactie op {what} ingetrokken", bare: "{who} heeft {what} ingetrokken"},
		},
		unknown: "{who} heeft iets met {what} gedaan ({event})",
		test:    "Testbericht van amenbo — dit project meldt hier.",
		statuses: map[string]string{
			"todo":        "Te doen",
			"in_progress": "Bezig",
			"done":        "Klaar",
			"blocked":     "Geblokkeerd",
			"rejected":    "Afgewezen",
		},
	},
	"pl": {
		says: map[string]say{
			eventTaskCreated:      {bare: "{who} utworzył(a) {what}"},
			eventStatusChanged:    {full: "{who} zmienił(a) {what} na {state}", bare: "{who} zmienił(a) status {what}"},
			eventTaskDone:         {bare: "{who} ukończył(a) {what}"},
			eventTaskRejected:     {bare: "{who} postanowił(a) nie robić {what}"},
			eventTaskAssigned:     {full: "{who} przypisał(a) {what} do {state}", bare: "{who} przypisał(a) {what}"},
			eventTaskMoved:        {full: "{who} umieścił(a) {what} w {state}", bare: "{who} umieścił(a) {what} w innym projekcie"},
			eventTaskDeleted:      {bare: "{who} usunął(-ęła) {what}"},
			eventDecisionAccepted: {bare: "{who} przyjął(-ęła) {what}"},
			eventDecisionRejected: {bare: "{who} odrzucił(a) {what}"},
			eventCommentAdded:     {full: "{who} skomentował(a) {what}", bare: "{who} dodał(a) {what}"},
			eventCommentRemoved:   {full: "{who} wycofał(a) komentarz do {what}", bare: "{who} wycofał(a) {what}"},
		},
		unknown: "{who} zrobił(a) coś z {what} ({event})",
		test:    "Wiadomość testowa z amenbo — ten projekt raportuje tutaj.",
		statuses: map[string]string{
			"todo":        "Do zrobienia",
			"in_progress": "W toku",
			"done":        "Gotowe",
			"blocked":     "Zablokowane",
			"rejected":    "Odrzucone",
		},
	},
	"pt-BR": {
		says: map[string]say{
			eventTaskCreated:      {bare: "{who} criou {what}"},
			eventStatusChanged:    {full: "{who} mudou {what} para {state}", bare: "{who} mudou o status de {what}"},
			eventTaskDone:         {bare: "{who} concluiu {what}"},
			eventTaskRejected:     {bare: "{who} decidiu não fazer {what}"},
			eventTaskAssigned:     {full: "{who} atribuiu {what} a {state}", bare: "{who} definiu o responsável de {what}"},
			eventTaskMoved:        {full: "{who} moveu {what} para {state}", bare: "{who} moveu {what} para outro projeto"},
			eventTaskDeleted:      {bare: "{who} excluiu {what}"},
			eventDecisionAccepted: {bare: "{who} aceitou {what}"},
			eventDecisionRejected: {bare: "{who} recusou {what}"},
			eventCommentAdded:     {full: "{who} comentou em {what}", bare: "{who} adicionou {what}"},
			eventCommentRemoved:   {full: "{who} retirou um comentário de {what}", bare: "{who} retirou {what}"},
		},
		unknown: "{who} fez algo em {what} ({event})",
		test:    "Mensagem de teste do amenbo — este projeto reporta aqui.",
		statuses: map[string]string{
			"todo":        "A fazer",
			"in_progress": "Em andamento",
			"done":        "Concluída",
			"blocked":     "Bloqueada",
			"rejected":    "Recusada",
		},
	},
	"ru": {
		says: map[string]say{
			eventTaskCreated:      {bare: "{who} создал {what}"},
			eventStatusChanged:    {full: "{who} перевёл {what} в статус {state}", bare: "{who} изменил статус {what}"},
			eventTaskDone:         {bare: "{who} завершил {what}"},
			eventTaskRejected:     {bare: "{who} решил не делать {what}"},
			eventTaskAssigned:     {full: "{who} назначил {state} ответственным за {what}", bare: "{who} назначил ответственного за {what}"},
			eventTaskMoved:        {full: "{who} переместил {what} в {state}", bare: "{who} переместил {what} в другой проект"},
			eventTaskDeleted:      {bare: "{who} удалил {what}"},
			eventDecisionAccepted: {bare: "{who} принял {what}"},
			eventDecisionRejected: {bare: "{who} отклонил {what}"},
			eventCommentAdded:     {full: "{who} прокомментировал {what}", bare: "{who} добавил {what}"},
			eventCommentRemoved:   {full: "{who} отозвал комментарий к {what}", bare: "{who} отозвал {what}"},
		},
		unknown: "{who} что-то сделал с {what} ({event})",
		test:    "Тестовое сообщение от amenbo — этот проект отчитывается сюда.",
		statuses: map[string]string{
			"todo":        "К выполнению",
			"in_progress": "В работе",
			"done":        "Готово",
			"blocked":     "Заблокировано",
			"rejected":    "Отклонено",
		},
	},
	"th": {
		says: map[string]say{
			eventTaskCreated:      {bare: "{who} สร้าง {what}"},
			eventStatusChanged:    {full: "{who} เปลี่ยน {what} เป็น {state}", bare: "{who} เปลี่ยนสถานะของ {what}"},
			eventTaskDone:         {bare: "{who} ทำ {what} เสร็จแล้ว"},
			eventTaskRejected:     {bare: "{who} ตัดสินใจไม่ทำ {what}"},
			eventTaskAssigned:     {full: "{who} มอบหมาย {what} ให้ {state}", bare: "{who} มอบหมาย {what}"},
			eventTaskMoved:        {full: "{who} ย้าย {what} ไปที่ {state}", bare: "{who} ย้าย {what} ไปโปรเจกต์อื่น"},
			eventTaskDeleted:      {bare: "{who} ลบ {what}"},
			eventDecisionAccepted: {bare: "{who} รับ {what} แล้ว"},
			eventDecisionRejected: {bare: "{who} ตีตก {what}"},
			eventCommentAdded:     {full: "{who} แสดงความเห็นใน {what}", bare: "{who} เพิ่ม {what}"},
			eventCommentRemoved:   {full: "{who} ถอนความเห็นใน {what}", bare: "{who} ถอน {what}"},
		},
		unknown: "{who} ทำบางอย่างกับ {what} ({event})",
		test:    "ข้อความทดสอบจาก amenbo — โปรเจกต์นี้จะรายงานมาที่นี่",
		statuses: map[string]string{
			"todo":        "รอทำ",
			"in_progress": "กำลังทำ",
			"done":        "เสร็จแล้ว",
			"blocked":     "ติดขัด",
			"rejected":    "ตีตก",
		},
	},
	"tr": {
		says: map[string]say{
			eventTaskCreated:      {bare: "{who}, {what} kaydını oluşturdu"},
			eventStatusChanged:    {full: "{who}, {what} kaydını {state} durumuna geçirdi", bare: "{who}, {what} kaydının durumunu değiştirdi"},
			eventTaskDone:         {bare: "{who}, {what} kaydını bitirdi"},
			eventTaskRejected:     {bare: "{who}, {what} kaydını yapmamaya karar verdi"},
			eventTaskAssigned:     {full: "{who}, {what} kaydını {state} kullanıcısına verdi", bare: "{who}, {what} kaydının sorumlusunu belirledi"},
			eventTaskMoved:        {full: "{who}, {what} kaydını {state} projesine taşıdı", bare: "{who}, {what} kaydını başka bir projeye taşıdı"},
			eventTaskDeleted:      {bare: "{who}, {what} kaydını sildi"},
			eventDecisionAccepted: {bare: "{who}, {what} kararını kabul etti"},
			eventDecisionRejected: {bare: "{who}, {what} kararını reddetti"},
			eventCommentAdded:     {full: "{who}, {what} kaydına yorum yaptı", bare: "{who}, {what} ekledi"},
			eventCommentRemoved:   {full: "{who}, {what} kaydındaki yorumu geri aldı", bare: "{who}, {what} geri aldı"},
		},
		unknown: "{who}, {what} üzerinde bir işlem yaptı ({event})",
		test:    "amenbo'dan test mesajı — bu proje buraya bildiriyor.",
		statuses: map[string]string{
			"todo":        "Yapılacak",
			"in_progress": "Sürüyor",
			"done":        "Bitti",
			"blocked":     "Engelli",
			"rejected":    "Reddedildi",
		},
	},
	"uk": {
		says: map[string]say{
			eventTaskCreated:      {bare: "{who} створив {what}"},
			eventStatusChanged:    {full: "{who} перевів {what} у статус {state}", bare: "{who} змінив статус {what}"},
			eventTaskDone:         {bare: "{who} завершив {what}"},
			eventTaskRejected:     {bare: "{who} вирішив не робити {what}"},
			eventTaskAssigned:     {full: "{who} призначив {state} відповідальним за {what}", bare: "{who} призначив відповідального за {what}"},
			eventTaskMoved:        {full: "{who} перемістив {what} до {state}", bare: "{who} перемістив {what} до іншого проєкту"},
			eventTaskDeleted:      {bare: "{who} видалив {what}"},
			eventDecisionAccepted: {bare: "{who} прийняв {what}"},
			eventDecisionRejected: {bare: "{who} відхилив {what}"},
			eventCommentAdded:     {full: "{who} прокоментував {what}", bare: "{who} додав {what}"},
			eventCommentRemoved:   {full: "{who} відкликав коментар до {what}", bare: "{who} відкликав {what}"},
		},
		unknown: "{who} щось зробив з {what} ({event})",
		test:    "Тестове повідомлення від amenbo — цей проєкт звітує сюди.",
		statuses: map[string]string{
			"todo":        "До виконання",
			"in_progress": "У роботі",
			"done":        "Готово",
			"blocked":     "Заблоковано",
			"rejected":    "Відхилено",
		},
	},
	"vi": {
		says: map[string]say{
			eventTaskCreated:      {bare: "{who} đã tạo {what}"},
			eventStatusChanged:    {full: "{who} đã chuyển {what} sang {state}", bare: "{who} đã đổi trạng thái của {what}"},
			eventTaskDone:         {bare: "{who} đã hoàn thành {what}"},
			eventTaskRejected:     {bare: "{who} đã quyết định không làm {what}"},
			eventTaskAssigned:     {full: "{who} đã giao {what} cho {state}", bare: "{who} đã giao {what}"},
			eventTaskMoved:        {full: "{who} đã chuyển {what} sang dự án {state}", bare: "{who} đã chuyển {what} sang dự án khác"},
			eventTaskDeleted:      {bare: "{who} đã xoá {what}"},
			eventDecisionAccepted: {bare: "{who} đã chấp nhận {what}"},
			eventDecisionRejected: {bare: "{who} đã bác {what}"},
			eventCommentAdded:     {full: "{who} đã bình luận về {what}", bare: "{who} đã thêm {what}"},
			eventCommentRemoved:   {full: "{who} đã rút lại bình luận về {what}", bare: "{who} đã rút lại {what}"},
		},
		unknown: "{who} đã tác động đến {what} ({event})",
		test:    "Tin nhắn thử từ amenbo — dự án này sẽ báo về đây.",
		statuses: map[string]string{
			"todo":        "Cần làm",
			"in_progress": "Đang làm",
			"done":        "Xong",
			"blocked":     "Bị chặn",
			"rejected":    "Bị bác",
		},
	},
	"zh-Hans": {
		says: map[string]say{
			eventTaskCreated:      {bare: "{who} 创建了 {what}"},
			eventStatusChanged:    {full: "{who} 把 {what} 改为{state}", bare: "{who} 修改了 {what} 的状态"},
			eventTaskDone:         {bare: "{who} 完成了 {what}"},
			eventTaskRejected:     {bare: "{who} 决定不做 {what}"},
			eventTaskAssigned:     {full: "{who} 把 {what} 分配给了 {state}", bare: "{who} 分配了 {what}"},
			eventTaskMoved:        {full: "{who} 把 {what} 移动到了 {state}", bare: "{who} 把 {what} 移动到了另一个项目"},
			eventTaskDeleted:      {bare: "{who} 删除了 {what}"},
			eventDecisionAccepted: {bare: "{who} 采纳了 {what}"},
			eventDecisionRejected: {bare: "{who} 否决了 {what}"},
			eventCommentAdded:     {full: "{who} 评论了 {what}", bare: "{who} 添加了 {what}"},
			eventCommentRemoved:   {full: "{who} 撤回了对 {what} 的评论", bare: "{who} 撤回了 {what}"},
		},
		unknown: "{who} 对 {what} 做了操作（{event}）",
		test:    "来自 amenbo 的测试消息——这个项目会报告到这里。",
		statuses: map[string]string{
			"todo":        "待办",
			"in_progress": "进行中",
			"done":        "已完成",
			"blocked":     "受阻",
			"rejected":    "已否决",
		},
	},
	"zh-Hant": {
		says: map[string]say{
			eventTaskCreated:      {bare: "{who} 建立了 {what}"},
			eventStatusChanged:    {full: "{who} 把 {what} 改成{state}", bare: "{who} 修改了 {what} 的狀態"},
			eventTaskDone:         {bare: "{who} 完成了 {what}"},
			eventTaskRejected:     {bare: "{who} 決定不做 {what}"},
			eventTaskAssigned:     {full: "{who} 把 {what} 指派給了 {state}", bare: "{who} 指派了 {what}"},
			eventTaskMoved:        {full: "{who} 把 {what} 移動到了 {state}", bare: "{who} 把 {what} 移動到了另一個專案"},
			eventTaskDeleted:      {bare: "{who} 刪除了 {what}"},
			eventDecisionAccepted: {bare: "{who} 採納了 {what}"},
			eventDecisionRejected: {bare: "{who} 否決了 {what}"},
			eventCommentAdded:     {full: "{who} 評論了 {what}", bare: "{who} 新增了 {what}"},
			eventCommentRemoved:   {full: "{who} 撤回了對 {what} 的評論", bare: "{who} 撤回了 {what}"},
		},
		unknown: "{who} 對 {what} 做了操作（{event}）",
		test:    "來自 amenbo 的測試訊息——這個專案會報告到這裡。",
		statuses: map[string]string{
			"todo":        "待辦",
			"in_progress": "進行中",
			"done":        "已完成",
			"blocked":     "受阻",
			"rejected":    "已否決",
		},
	},
}

// sentence is the message: what the AI did, to which record, in the language the store reads in and
// under the name it gives its AI.
func sentence(how preferences, in input, about subject) string {
	said := strings.NewReplacer(
		slotWho, how.aiDisplayName,
		slotWhat, about.name,
		slotState, stateWord(how, in.Event, in.New),
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

// stateWord is what goes where a sentence names the second thing. Two of the three arrive as a
// value off the wire and leave as something a reader recognises; the third arrives that way and
// stays, because it already is one.
//
//   - a **status** is a word amenbo owns, so it is said the way amenbo says it
//   - an **assignee** arrives as the facet, `ai` or `human`, which names one of the two the line
//     is already about — so it is said by the name they go by, the same one the sentence's subject
//     is said by
//   - a **project** arrives as its slug, which is the store's own value and the one the user would
//     go and search for, so it passes through untouched
func stateWord(how preferences, event, newState string) string {
	switch {
	case newState == "":
		return newState
	case event == eventStatusChanged:
		return statusWord(how.language, newState)
	case event == eventTaskAssigned:
		return whoseName(how, newState)
	}
	return newState
}

// testLine is the whole of a test message — the one line this plugin sends that describes no
// event. A language with no row of its own, or a row this build filled in only in part, falls
// back to English the same way a sentence does: a channel getting the line in English says what
// it was sent to say, and an empty message does not.
func testLine(language string) string {
	if line := wordings[language].test; line != "" {
		return line
	}
	return wordings[fallbackLanguage].test
}

// statusWord is amenbo's own word for a state. A status this build has no word for passes through
// as it arrived: the value off the wire says more than nothing does, so a state amenbo adds later
// still reports.
func statusWord(language, status string) string {
	if word := wordings[language].statuses[status]; word != "" {
		return word
	}
	if word := wordings[fallbackLanguage].statuses[status]; word != "" {
		return word
	}
	return status
}

// whoseName resolves the facet a task was handed to into the name that facet goes by. This is not
// a translation — the name it lands on is the user's own text, and travels on untouched like every
// other thing of theirs in a line. It is the same resolution the subject of the sentence already
// gets, and doing it in one place and not the other is what left a line calling the same party two
// different things.
//
// A facet neither of the two, and a store that answered with no name at all, are said as they
// arrived: what this reaches for is a name amenbo gave, never one made up here. The second is rare
// on purpose — amenbo names both parties whether or not the user has chosen anything — so `human`
// in a line means the settings could not be read, which is the same failure that costs it its
// language.
func whoseName(how preferences, facet string) string {
	switch facet {
	case actorAI:
		return how.aiDisplayName
	case actorHuman:
		if how.humanDisplayName != "" {
			return how.humanDisplayName
		}
	}
	return facet
}
