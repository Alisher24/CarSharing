package httpapi

import (
	"bytes"
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
	"github.com/Alisher24/CarSharing/backend/internal/platform/httpheader"
	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
)

// inboxMessagesLink is the collection of the contract, which every page of this surface offers as the
// machine-readable view of the same box. The contract states the path once; this is the one spelling
// of it inside the page surface.
const inboxMessagesLink = "/api/v1/messages"

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
//
// The pages are given the same handlers the operations of the inbox are served by, so the two
// surfaces cannot come to differ about the size of a page, its order or the cursor that continues it.
func inboxPagesBeside(
	operations http.Handler, served mailstubInboxHandlers, contract contractPrefixes,
) http.Handler {
	pages := newInboxPages(served)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if contract.owns(r.URL.Path) {
			operations.ServeHTTP(w, r)
			return
		}
		pages.serve(w, r)
	})
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

func refusalAnswer(status int, message string) inboxAnswer {
	return inboxAnswer{refusal: &pageRefusal{status: status, message: message}}
}

// inboxPages serves the pages of the read-only box over the box itself. A page is read through the
// same handlers the collection is read through, so the page size, the order and the cursor of a page
// are the collection's rather than a second copy of them.
type inboxPages struct {
	served mailstubInboxHandlers
	routes []inboxRoute
}

func newInboxPages(served mailstubInboxHandlers) *inboxPages {
	pages := &inboxPages{served: served}
	pages.routes = []inboxRoute{
		{path: inboxPath, draw: pages.letterList},
		{path: inboxMessagePath, draw: pages.oneLetter, letter: true},
	}
	for index := range pages.routes {
		// A route whose pattern cannot be matched would answer nothing at all, so the table states
		// where it is wrong as the process starts rather than in front of a reader.
		inboxPatternParts(pages.routes[index].path)
	}
	return pages
}

// serve answers one request that is not the contract's. A page of this surface is drawn; everything
// else is refused with the same headers, so a caller asking this listener for a page always receives
// one.
func (p *inboxPages) serve(w http.ResponseWriter, r *http.Request) {
	r = withRequestIdentity(w, r)
	route := p.routeOf(r)
	if route == nil {
		p.drawRefusal(w, refusalAnswer(http.StatusNotFound, inboxNoSuchPage), nil)
		return
	}
	page := &p.routes[route.index]
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
		p.drawRefusal(w, refusalAnswer(http.StatusMethodNotAllowed, inboxWrongMethod), page)
		return
	}
	p.draw(w, r, page, page.draw(pageRequestOf(r, route)))
}

// routeOf matches a request path against the table of routes and answers what it captured, or nil
// when no page of this surface serves the path.
func (p *inboxPages) routeOf(r *http.Request) *pageRoute {
	for index := range p.routes {
		captured, fits := inboxRouteMatch(p.routes[index].path, r.URL.Path)
		if !fits {
			continue
		}
		return &pageRoute{index: index, captured: captured}
	}
	return nil
}

// draw answers one page. The markup is rendered into a buffer before anything is written, so a
// template that fails partway through leaves a stated refusal rather than half a page under a
// success.
func (p *inboxPages) draw(w http.ResponseWriter, r *http.Request, page *inboxRoute, answer inboxAnswer) {
	if answer.refusal != nil {
		p.drawRefusal(w, answer, page)
		return
	}
	document, err := p.frame(r, page, answer.content, answer.data)
	if err != nil {
		slog.ErrorContext(r.Context(), "a page of the mail box could not be drawn", "error", err)
		p.drawRefusal(w, refusalAnswer(http.StatusInternalServerError, inboxUndrawable), page)
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
func (p *inboxPages) drawRefusal(w http.ResponseWriter, answer inboxAnswer, page *inboxRoute) {
	document, err := p.frame(nil, page, inboxTemplates.Lookup(inboxRefusalTemplate), answer.refusal.message)
	writeInboxPageHeaders(w)
	if err != nil {
		// A page that cannot be drawn still states the status rather than answering with nothing.
		w.WriteHeader(answer.refusal.status)
		return
	}
	w.WriteHeader(answer.refusal.status)
	_, _ = w.Write(document)
}

// frame draws one page: the content of the page filled in with what it read, inside the layout every
// page of this surface shares. The footer leads to the list from a letter page and reloads the page
// from the list itself, which is the way back a reader of either is left with.
func (p *inboxPages) frame(
	r *http.Request, page *inboxRoute, content *template.Template, data any,
) ([]byte, error) {
	document := inboxDocument{
		Title:   inboxTitle,
		Summary: inboxSummary,
		Styles:  inboxStyles,
		Labels:  inboxPageLabels(),
		JSON:    inboxMessagesLink,
		Current: inboxPath,
		Back:    inboxPath,
	}
	if r != nil {
		request := pageRequestOf(r, nil)
		document.JSON = inboxJSONPath(page.letter, request.identifier)
		// The list of a page that was read with a cursor is refreshed at the page it shows rather
		// than at the first one, so a reader who reloads continues where they were.
		document.Current = r.URL.RequestURI()
	}
	if page != nil {
		document.Letter = page.letter
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
func (p *inboxPages) letterList(request pageRequest) inboxAnswer {
	limit := cursor.PageSize
	after, err := p.served.positionOf(presentedCursor(request.query), limit)
	if err != nil {
		return refusalAnswer(http.StatusOK, inboxUnreadableCursor)
	}
	read, err := p.served.pageOf(request.ctx, after, limit)
	if err != nil {
		slog.ErrorContext(request.ctx, "the mail box could not be read", "error", err)
		return refusalAnswer(http.StatusServiceUnavailable, inboxUnavailable)
	}
	return inboxAnswer{
		content: inboxTemplates.Lookup(inboxLettersTemplate),
		data: pageLetters{
			Letters: newLetterRows(read.messages),
			Next:    nextLettersLink(read.next, limit),
		},
	}
}

// oneLetter reads one letter of the box exactly as the stub stored it. A letter that is not there is
// a page that says so rather than a failure: the reader asked for a page and receives one.
func (p *inboxPages) oneLetter(request pageRequest) inboxAnswer {
	stored, err := p.served.inbox.ByID(request.ctx, request.identifier)
	if errors.Is(err, mailstub.ErrMessageNotFound) {
		return refusalAnswer(http.StatusNotFound, inboxNoSuchLetter)
	}
	if err != nil {
		slog.ErrorContext(request.ctx, "one letter could not be read", "error", err)
		return refusalAnswer(http.StatusServiceUnavailable, inboxUnavailable)
	}
	return inboxAnswer{
		content: inboxTemplates.Lookup(inboxLetterTemplate),
		data: pageLetter{
			letterRow:   newLetterRow(stored),
			DeliveryKey: stored.DeliveryKey,
			Text:        stored.Text,
		},
	}
}

// nextLettersLink is the address of the page after the one read, or empty when the box holds no more
// letters. It is the address of the list itself read with the cursor the collection issued for the
// same page under the same scope, so the next page is a page of this surface rather than the JSON of
// the same box.
func nextLettersLink(next *string, limit int) template.URL {
	if next == nil {
		return ""
	}
	query := url.Values{cursorParameter: {*next}, limitParameter: {strconv.Itoa(limit)}}
	return template.URL(inboxPath + "?" + query.Encode())
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
	w.Header().Set(httpheader.ContentType, htmlMediaType)
	w.Header().Set(cacheControlHeader, noStoreCacheControl)
	w.Header().Set(contentTypeOptionsHeader, nosniffContentTypeOption)
	w.Header().Set(contentSecurityHeader, contentSecurityPolicy)
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

func newLetterRows(stored []mailstub.Message) []letterRow {
	letters := make([]letterRow, 0, len(stored))
	for _, letter := range stored {
		letters = append(letters, newLetterRow(letter))
	}
	return letters
}

// inboxLetterPath is the address of one letter's page, built from the pattern that page is served at
// rather than from a second copy of the route.
func inboxLetterPath(identifier string) string {
	return strings.Replace(inboxMessagePath, "{id}", url.PathEscape(identifier), 1)
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
