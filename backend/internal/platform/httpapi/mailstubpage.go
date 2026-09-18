package httpapi

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/mailstub"
	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
)

// The pages of the human inbox. A page answers the first two; a path the contract owns goes to the
// boundary, which knows those paths and answers them as the JSON operations they are.
const (
	inboxPath        = "/"
	inboxMessagePath = "/messages/{id}"

	// inboxMessagesLink is the collection of the contract, which every page of this surface offers as
	// the machine-readable view of the same box. The contract states the path once; this is the one
	// spelling of it inside the page surface.
	inboxMessagesLink = "/api/v1/messages"
)

// The headers every page of the inbox carries. The policy admits no source outside the page itself
// and only its own inline stylesheet, so the browser of a demonstration reaches nothing beyond this
// listener — a header rather than a habit of the markup.
const (
	htmlMediaType = "text/html; charset=utf-8"

	contentTypeOptionsHeader = "X-Content-Type-Options"
	contentSecurityHeader    = "Content-Security-Policy"
	nosniffContentTypeOption = "nosniff"

	contentSecurityPolicy = "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; " +
		"form-action 'none'"
)

// The words of the pages, stated together because they are one vocabulary: a link and the page it
// opens name the same thing. The markup states them through pageLabels rather than spelling any of
// them a second time.
const (
	inboxTitle   = "Ящик почтовой заглушки"
	inboxSummary = "Локальный ящик заглушки: письма, которые она приняла. Только чтение и только на " +
		"этом адресе."

	inboxMomentLabel      = "Момент приёма"
	inboxRecipientLabel   = "Получатель"
	inboxSubjectLabel     = "Тема"
	inboxIdentifierLabel  = "Идентификатор"
	inboxDeliveryKeyLabel = "Ключ доставки"
	inboxTextLabel        = "Текст"

	inboxNoLetters     = "Писем пока нет"
	inboxNoLettersHint = "Письмо приходит, когда поездка закончена и по ней выставлен счёт."
	inboxNextPageLink  = "Дальше"
	inboxRefreshLink   = "Обновить"
	inboxBackToList    = "К списку писем"
	inboxJSONLink      = "JSON"

	inboxNoSuchPage       = "Такой страницы в ящике нет."
	inboxNoSuchLetter     = "Такого письма в ящике нет."
	inboxUnreadableCursor = "Ссылка на эту страницу не читается: она подписана не этим ящиком или " +
		"пришла из другой страницы."
	inboxWrongMethod = "Ящик только читают: его страницы отвечают на GET и HEAD."
	inboxUnavailable = "Ящик сейчас недоступен: заглушка не может прочитать свою схему."
	inboxUndrawable  = "Страницу не удалось собрать."
)

// storedZone names the zone every moment of the box is stored in. A page states a moment as it is
// stored and names the zone beside it rather than converting: the reader's zone is not the box's to
// guess, and the image carries no zone database to convert into.
const storedZone = "UTC"

// inboxPagesBeside serves the human inbox beside the contract. A path the contract owns passes
// through untouched, because a page there would answer an operation that declares JSON — including
// its own refusals; every other path is answered as a page, including the unknown one, so that a
// person who mistypes an address reads what happened instead of an empty screen.
func inboxPagesBeside(
	operations http.Handler, inbox MailInbox, cursors *cursor.Signer, contract contractPrefixes,
) http.Handler {
	pages := newInboxPages(inbox, cursors)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if contract.owns(r.URL.Path) {
			operations.ServeHTTP(w, r)
			return
		}
		pages.serve(w, r)
	})
}

// inboxPage is one page of this surface: the pattern it answers, and what it reads to draw itself.
// The table below is the single declaration of what the surface serves, so another page is one entry
// beside these rather than an edit to a growing conditional.
type inboxPage struct {
	path string
	draw func(pageVisit) inboxAnswer

	// letter reports whether the page shows one letter rather than the list, which is what the footer
	// of the layout offers a way back from.
	letter bool
}

// pageVisit is one in-flight page request: the context it is read under, the query its address carried,
// and what the pattern of its page captured from the address.
type pageVisit struct {
	ctx        context.Context
	query      url.Values
	identifier string
}

// inboxAnswer is what a page read: the template that shows it and what to fill it with, or the
// refusal it has to state instead. A page that could not reach the box refuses with the status the
// failure carries rather than showing an empty one, so an unreachable box never reads as a box that
// holds no letters.
type inboxAnswer struct {
	content *template.Template
	data    any
	refusal *pageRefusal
}

// pageRefusal is a page that states why nothing is shown, together with the status that reason
// carries: a letter that is not there and a link that cannot be read are refusals of different kinds,
// and neither is a failure of the page that answers them.
type pageRefusal struct {
	status  int
	message string
}

// lettersAnswer and letterAnswer are the two pages this surface draws, named where they are returned
// so that a page always answers with the template that belongs to it.
func lettersAnswer(letters pageLetters) inboxAnswer {
	return inboxAnswer{content: inboxTemplates.Lookup(inboxLettersTemplate), data: letters}
}

func letterAnswer(letter pageLetter) inboxAnswer {
	return inboxAnswer{content: inboxTemplates.Lookup(inboxLetterTemplate), data: letter}
}

func refusalAnswer(status int, message string) inboxAnswer {
	return inboxAnswer{refusal: &pageRefusal{status: status, message: message}}
}

// inboxPages serves the pages of the read-only box over the box itself. A page is read through the
// same handlers the collection is read through, so the page size, the order and the cursor of a page
// are the collection's rather than a second copy of them.
type inboxPages struct {
	handlers mailstubInboxHandlers
	pages    []inboxPage
}

func newInboxPages(inbox MailInbox, cursors *cursor.Signer) *inboxPages {
	served := &inboxPages{handlers: mailstubInboxHandlers{inbox: inbox, cursors: cursors}}
	served.pages = []inboxPage{
		{path: inboxPath, draw: served.letterList},
		{path: inboxMessagePath, draw: served.oneLetter, letter: true},
	}
	for index := range served.pages {
		// A page whose pattern cannot be matched would answer nothing at all, so the table states
		// where it is wrong as the process starts rather than in front of a reader.
		inboxPatternParts(served.pages[index].path)
	}
	return served
}

// serve answers one request that is not the contract's. A page of this surface is drawn; everything
// else is refused with the same headers, so a caller asking this listener for a page always receives
// one.
func (p *inboxPages) serve(w http.ResponseWriter, r *http.Request) {
	r = withRequestIdentity(w, r)
	page, request := p.match(r)
	if page == nil {
		p.drawRefusal(w, &pageRefusal{status: http.StatusNotFound, message: inboxNoSuchPage}, nil)
		return
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
		p.drawRefusal(w, &pageRefusal{status: http.StatusMethodNotAllowed, message: inboxWrongMethod}, page)
		return
	}
	p.draw(w, request, page, page.draw(pageVisitOf(request)))
}

// match resolves a request to the page that answers it: the first pattern of the table the path fits,
// together with the request that carries what the pattern captured.
func (p *inboxPages) match(r *http.Request) (*inboxPage, *http.Request) {
	for index := range p.pages {
		page := &p.pages[index]
		captured, fits := inboxRoute(page.path, r.URL.Path)
		if !fits {
			continue
		}
		return page, r.WithContext(context.WithValue(r.Context(), inboxPathValuesKey{}, captured))
	}
	return nil, r
}

// inboxRoute matches one request path against one pattern of this surface and answers what the
// pattern captured. A literal segment is compared as it is written and `{name}` captures one whole
// segment, so the two must have as many segments as each other: `/messages/one/two` fits nothing and
// is the unknown page it is, rather than the page of a letter named `one/two`.
func inboxRoute(pattern, path string) (map[string]string, bool) {
	parts, captures := inboxPatternParts(pattern)
	segments := strings.Split(path, "/")
	if len(segments) != len(parts) {
		return nil, false
	}
	values := make(map[string]string, len(captures))
	for index, part := range parts {
		switch {
		case isInboxCapture(part):
			if segments[index] == "" {
				return nil, false
			}
			values[inboxCaptureName(part)] = segments[index]
		case segments[index] != part:
			return nil, false
		}
	}
	return values, true
}

// inboxPatternParts splits one pattern into the segments it is matched segment by segment, and names
// the segments that capture. A pattern that is not a sequence of literals and `{name}` captures is a
// defect of this build, so it stops the process where the table is declared.
func inboxPatternParts(pattern string) ([]string, []string) {
	parts := strings.Split(pattern, "/")
	captures := make([]string, 0, len(parts))
	for _, part := range parts {
		if !isInboxCapture(part) {
			if strings.ContainsAny(part, "{}") {
				panic("the inbox page pattern " + pattern + " is neither a literal nor a capture")
			}
			continue
		}
		name := inboxCaptureName(part)
		if name == "" {
			panic("the inbox page pattern " + pattern + " names no capture")
		}
		captures = append(captures, name)
	}
	return parts, captures
}

// isInboxCapture reports whether one segment of a pattern captures rather than matching a literal. A
// segment that only opens or only closes a brace is neither, and inboxPatternParts refuses it.
func isInboxCapture(part string) bool {
	return strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}")
}

// inboxCaptureName is the name one capturing segment gives what it captures.
func inboxCaptureName(part string) string {
	return strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
}

// inboxPathValuesKey carries what the pattern of a page captured from the request path.
type inboxPathValuesKey struct{}

// inboxIdentifierCapture is the name the one-letter pattern gives the identifier it captures.
const inboxIdentifierCapture = "id"

// pageVisitOf reads one page request: the context it is answered in, the query its address carried
// and what its pattern captured.
func pageVisitOf(r *http.Request) pageVisit {
	captured, _ := r.Context().Value(inboxPathValuesKey{}).(map[string]string)
	return pageVisit{ctx: r.Context(), query: r.URL.Query(), identifier: captured[inboxIdentifierCapture]}
}

// draw answers one page. The markup is rendered into a buffer before anything is written, so a
// template that fails partway through leaves a stated refusal rather than half a page under a
// success.
func (p *inboxPages) draw(w http.ResponseWriter, r *http.Request, page *inboxPage, answer inboxAnswer) {
	if answer.refusal != nil {
		p.drawRefusal(w, answer.refusal, page)
		return
	}
	document, err := p.frame(r, page.letter, answer.content, answer.data)
	if err != nil {
		slog.ErrorContext(r.Context(), "a page of the mail box could not be drawn", "error", err)
		p.drawRefusal(w, &pageRefusal{status: http.StatusInternalServerError, message: inboxUndrawable}, page)
		return
	}
	writeInboxPageHeaders(w)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(document); err != nil {
		slog.ErrorContext(r.Context(), "a page of the mail box was not delivered", "error", err)
	}
}

// drawRefusal answers with a page rather than with emptiness, so a mistyped address, an unreadable
// cursor and an unreachable box each read as what happened. A refusal is a page of the same surface,
// framed by the layout that leads back to the list; one asked for by a page that could not be served
// keeps that page's own way back.
func (p *inboxPages) drawRefusal(w http.ResponseWriter, refusal *pageRefusal, page *inboxPage) {
	document, err := p.frame(nil, page != nil && page.letter, inboxTemplates.Lookup(inboxRefusalTemplate),
		refusal.message)
	writeInboxPageHeaders(w)
	if err != nil {
		// A page that cannot be drawn still states the status rather than answering with nothing.
		w.WriteHeader(refusal.status)
		return
	}
	w.WriteHeader(refusal.status)
	_, _ = w.Write(document)
}

// frame draws one page: the content of the page filled in with what it read, inside the layout every
// page of this surface shares. The footer leads to the list from a letter page and reloads the page
// from the list itself, which is the way back a reader of either is left with.
func (p *inboxPages) frame(
	r *http.Request, letter bool, content *template.Template, data any,
) ([]byte, error) {
	document := inboxDocument{
		Title:   inboxTitle,
		Summary: inboxSummary,
		Styles:  inboxStyles,
		Labels:  inboxPageLabels(),
		JSON:    inboxMessagesLink,
		Current: inboxPath,
		Letter:  letter,
	}
	if r != nil {
		document.JSON = inboxJSONPath(letter, pageVisitOf(r).identifier)
		document.Current = r.URL.Path
	}
	if content != nil {
		var body bytes.Buffer
		if err := inboxTemplates.ExecuteTemplate(&body, content.Name(), pageContent{
			Labels: document.Labels,
			Data:   data,
		}); err != nil {
			return nil, err
		}
		document.Content = template.HTML(body.String())
	}
	var rendered bytes.Buffer
	if err := inboxTemplates.ExecuteTemplate(&rendered, inboxLayoutTemplate, document); err != nil {
		return nil, err
	}
	return rendered.Bytes(), nil
}

// letterList reads one page of the box: the letters it holds, and the address of the page after it
// when there is one. The page size, the order and the cursor are the collection's, because both
// surfaces read one box.
func (p *inboxPages) letterList(from pageVisit) inboxAnswer {
	limit := mailstub.PageSize
	after, err := p.handlers.positionOf(presentedCursor(from.query), limit)
	if err != nil {
		return refusalAnswer(http.StatusOK, inboxUnreadableCursor)
	}
	page, err := p.handlers.inbox.ReadPage(from.ctx, after, limit)
	if err != nil {
		slog.ErrorContext(from.ctx, "the mail box could not be read", "error", err)
		return refusalAnswer(http.StatusServiceUnavailable, inboxUnavailable)
	}
	return lettersAnswer(pageLetters{
		Letters: newLetterRows(page.Messages),
		Next:    p.nextPageOf(page, limit),
	})
}

// oneLetter reads one letter of the box exactly as the stub stored it. A letter that is not there is
// a page that says so rather than a failure: the reader asked for a page and receives one.
func (p *inboxPages) oneLetter(from pageVisit) inboxAnswer {
	stored, err := p.handlers.inbox.ByID(from.ctx, from.identifier)
	if errors.Is(err, mailstub.ErrMessageNotFound) {
		return refusalAnswer(http.StatusNotFound, inboxNoSuchLetter)
	}
	if err != nil {
		slog.ErrorContext(from.ctx, "one letter could not be read", "error", err)
		return refusalAnswer(http.StatusServiceUnavailable, inboxUnavailable)
	}
	return letterAnswer(pageLetter{
		letterRow:   newLetterRow(stored),
		DeliveryKey: stored.DeliveryKey,
		Text:        stored.Text,
	})
}

// nextPageOf is the address of the page after the one read, or empty when the box holds no more
// letters. The cursor is the one the collection issues for the same page under the same scope, so
// both surfaces continue through one position.
func (p *inboxPages) nextPageOf(page mailstub.Page, limit int) template.URL {
	if page.Next == nil {
		return ""
	}
	issued, err := p.handlers.cursors.Issue(
		cursor.Position{CreatedAt: page.Next.AcceptedAt, ID: page.Next.ID},
		p.handlers.inboxScopeOf(limit))
	if err != nil {
		slog.Error("the cursor of the page after this one could not be issued", "error", err)
		return ""
	}
	query := url.Values{cursorParameter: {issued}, limitParameter: {strconv.Itoa(limit)}}
	return template.URL(inboxMessagesLink + "?" + query.Encode())
}

// presentedCursor reads the cursor a page request carries, which is the parameter the collection
// declares: a page of the box continues through the position the collection publishes.
func presentedCursor(query url.Values) *string {
	presented := query.Get(cursorParameter)
	if presented == "" {
		return nil
	}
	return &presented
}

// inboxJSONPath is the machine-readable view of one page: the collection for the list, and the letter
// itself for a letter.
func inboxJSONPath(letter bool, identifier string) string {
	if !letter {
		return inboxMessagesLink
	}
	return inboxMessagesLink + "/" + url.PathEscape(identifier)
}

// writeInboxPageHeaders states what every page of this surface is: HTML that no intermediary stores
// and that reaches no source outside itself.
func writeInboxPageHeaders(w http.ResponseWriter) {
	w.Header().Set(contentTypeHeader, htmlMediaType)
	w.Header().Set(cacheControlHeader, noStoreCacheControl)
	w.Header().Set(contentTypeOptionsHeader, nosniffContentTypeOption)
	w.Header().Set(contentSecurityHeader, contentSecurityPolicy)
}

// inboxDocument is the shape every page of the inbox is drawn in: the layout the pages share, filled
// in with the page's own title, summary and content, and the labels both are read by.
type inboxDocument struct {
	Title   string
	Summary string
	Styles  template.CSS
	Content template.HTML
	Labels  pageLabels
	JSON    string
	Current string
	Letter  bool
}

// pageContent is what a page's own template is filled in with: what the page read, and the labels it
// states around it.
type pageContent struct {
	Labels pageLabels
	Data   any
}

// letterRow is one letter as a page shows it, and the two addresses it is reached by: its own page,
// and the machine-readable view of the same letter.
type letterRow struct {
	ID         string
	AcceptedAt string
	To         string
	Subject    string
	Link       string
	JSONLink   string
}

func newLetterRow(stored mailstub.Message) letterRow {
	return letterRow{
		ID:         stored.ID,
		AcceptedAt: storedMoment(stored.AcceptedAt),
		To:         stored.To,
		Subject:    stored.Subject,
		Link:       inboxLetterPath(stored.ID),
		JSONLink:   inboxMessagesLink + "/" + url.PathEscape(stored.ID),
	}
}

// inboxLetterPath is the address of one letter's page, built from the pattern that page is served at
// rather than from a second copy of the route.
func inboxLetterPath(identifier string) string {
	return strings.Replace(inboxMessagePath, "{id}", url.PathEscape(identifier), 1)
}

func newLetterRows(stored []mailstub.Message) []letterRow {
	letters := make([]letterRow, 0, len(stored))
	for _, letter := range stored {
		letters = append(letters, newLetterRow(letter))
	}
	return letters
}

// storedMoment states a moment the way the box stores it, with the zone it is stored in named beside
// it. The spelling is the contract's own format for an instant, so a page and the JSON of the same
// letter never name one moment in two ways.
func storedMoment(acceptedAt time.Time) string {
	return timestamp.Format(acceptedAt) + " " + storedZone
}

// pageLetters is the list of letters and the address of the page after it, which is empty when the
// box holds no more.
type pageLetters struct {
	Letters []letterRow
	Next    template.URL
}

// pageLetter is one letter as its own page shows it: everything the list shows, and the two values
// only a whole letter carries — the key it was delivered under, so a reader can see why a repeated
// delivery adds no second letter, and the text exactly as the box stored it.
type pageLetter struct {
	letterRow
	DeliveryKey string
	Text        string
}
