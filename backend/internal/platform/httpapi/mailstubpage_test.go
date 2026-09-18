package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Alisher24/CarSharing/backend/internal/mailstub"
)

// What every page of the inbox states about itself: it is HTML nobody stores, it is not sniffed into
// another type, its own stylesheet travels inside it, and it reaches no source outside itself.
func TestEveryPageOfTheInboxStatesWhatItIs(t *testing.T) {
	handler := mailstubInboxRouter(t, newFakeMailBox(), []byte(cursorKey))
	for _, path := range []string{inboxPath, "/nowhere"} {
		answer := mailstubCall(t, handler, http.MethodGet, path, "", "", nil)
		if contentType := answer.Header().Get(contentTypeHeader); contentType != htmlMediaType {
			t.Errorf("%s answers Content-Type %q", path, contentType)
		}
		if cache := answer.Header().Get(cacheControlHeader); cache != noStoreCacheControl {
			t.Errorf("%s answers Cache-Control %q", path, cache)
		}
		if options := answer.Header().Get(contentTypeOptionsHeader); options != nosniffContentTypeOption {
			t.Errorf("%s answers X-Content-Type-Options %q", path, options)
		}
		if policy := answer.Header().Get(contentSecurityHeader); policy != contentSecurityPolicy {
			t.Errorf("%s answers Content-Security-Policy %q", path, policy)
		}
		if requestID := answer.Header().Get(requestIDHeader); requestID == "" {
			t.Errorf("%s answers no request identifier", path)
		}
		// The policy admits the page's own stylesheet and no external source, so the stylesheet is
		// inside the page: a page that fetched one would be the request the policy forbids.
		if styles := answer.Body.String(); !strings.Contains(styles, string(inboxStyles)) {
			t.Errorf("%s does not carry the stylesheet of the pages", path)
		}
	}
}

// The list is the whole point of the page: the letters the box holds, newest first, each of them
// stating when it was accepted, who it is for and what it is about, with a way to open it.
func TestTheInboxPageListsTheLettersItHolds(t *testing.T) {
	box := newFakeMailBox()
	handler := mailstubInboxRouter(t, box, []byte(cursorKey))
	accepted := twoLettersAccepted(t, box)

	answer := mailstubCall(t, handler, http.MethodGet, inboxPath, "", "", nil)
	if answer.Code != http.StatusOK {
		t.Fatalf("the list answered %d %s", answer.Code, answer.Body.String())
	}
	page := answer.Body.String()
	for _, stored := range accepted {
		for _, stated := range []string{
			storedMoment(stored.AcceptedAt),
			stored.To,
			`<a href="` + inboxLetterPath(stored.ID) + `">` + stored.Subject + `</a>`,
		} {
			if !strings.Contains(page, stated) {
				t.Errorf("the list does not state %q", stated)
			}
		}
	}
	// The order is the collection's: the letter delivered last is the one at the top.
	if newest, older := strings.Index(page, accepted[1].Subject), strings.Index(page, accepted[0].Subject); newest > older {
		t.Errorf("the list shows the older letter first")
	}
	// The machine-readable view of the same box is one click away, and so is the list itself.
	if !strings.Contains(page, `href="`+inboxMessagesLink+`"`) {
		t.Errorf("the list does not link to the collection of the contract")
	}
	if !strings.Contains(page, `href="`+inboxPath+`"`) {
		t.Errorf("the list does not offer to be read again")
	}
}

// An empty box is not an empty screen: it says so and states where its letters come from.
func TestAnEmptyInboxSaysSoRatherThanShowingNothing(t *testing.T) {
	handler := mailstubInboxRouter(t, newFakeMailBox(), []byte(cursorKey))
	answer := mailstubCall(t, handler, http.MethodGet, inboxPath, "", "", nil)
	if answer.Code != http.StatusOK {
		t.Fatalf("an empty box answered %d %s", answer.Code, answer.Body.String())
	}
	for _, stated := range []string{inboxNoLetters, inboxNoLettersHint} {
		if !strings.Contains(answer.Body.String(), stated) {
			t.Errorf("an empty box does not state %q", stated)
		}
	}
}

// The page of one letter shows the letter as the stub stored it: who it is for, what it is about,
// when it arrived, which letter it is, the key it was delivered under, and its text.
func TestOneLetterPageShowsTheLetterAsItWasStored(t *testing.T) {
	box := newFakeMailBox()
	handler := mailstubInboxRouter(t, box, []byte(cursorKey))
	accepted := twoLettersAccepted(t, box)
	stored := accepted[0]

	answer := mailstubCall(t, handler, http.MethodGet, inboxLetterPath(stored.ID), "", "", nil)
	if answer.Code != http.StatusOK {
		t.Fatalf("the letter answered %d %s", answer.Code, answer.Body.String())
	}
	page := answer.Body.String()
	for _, stated := range []string{
		stored.To,
		stored.Subject,
		stored.Text,
		stored.ID,
		stored.DeliveryKey,
		storedMoment(stored.AcceptedAt),
		inboxMessagesLink + "/" + stored.ID,
	} {
		if !strings.Contains(page, stated) {
			t.Errorf("the letter does not state %q", stated)
		}
	}
	// The key is the reason a repeated delivery adds no second letter, and the page names it as such
	// rather than leaving a reader to find it in the table.
	if !strings.Contains(page, inboxDeliveryKeyLabel) {
		t.Errorf("the letter does not name the key it states")
	}
}

// The stub decides nothing about what a letter says and the page executes nothing of it: markup in a
// subject or a text is shown as the characters it is, which is a check rather than a promise.
func TestMarkupInALetterIsShownRatherThanExecuted(t *testing.T) {
	box := newFakeMailBox()
	handler := mailstubInboxRouter(t, box, []byte(cursorKey))
	stored := markupLetterAccepted(t, box)

	answer := mailstubCall(t, handler, http.MethodGet, inboxLetterPath(stored.ID), "", "", nil)
	if answer.Code != http.StatusOK {
		t.Fatalf("the letter answered %d %s", answer.Code, answer.Body.String())
	}
	page := answer.Body.String()
	for _, raw := range []string{"<script>", "</script>", "<b>", "<img src=x onerror=alert(1)>"} {
		if strings.Contains(page, raw) {
			t.Errorf("the page carries %q as markup", raw)
		}
	}
	// The markup travels as the characters it is: the subject keeps its quotes escaped and the text
	// keeps its tags escaped inside the block that preserves its line breaks.
	for _, shown := range []string{
		"&lt;script&gt;alert(&#39;письмо&#39;)&lt;/script&gt;",
		"&lt;b&gt;жирный&lt;/b&gt; и &lt;img src=x onerror=alert(1)&gt;",
	} {
		if !strings.Contains(page, shown) {
			t.Errorf("the page does not show %q as the text it is", shown)
		}
	}
	// The same letter is listed by its subject, which is escaped there too.
	list := mailstubCall(t, handler, http.MethodGet, inboxPath, "", "", nil)
	if !strings.Contains(list.Body.String(), "&lt;script&gt;alert(&#39;письмо&#39;)&lt;/script&gt;") {
		t.Errorf("the list does not show the subject as the text it is")
	}
}

// markupLetterAccepted delivers one letter whose subject and text carry markup, through the delivery
// operation, and answers it as the box stored it.
func markupLetterAccepted(t *testing.T, box *fakeMailBox) mailstub.Message {
	t.Helper()
	internal := mailstubInternalRouter(t, box, newFakeActions(t), true)
	body := `{"to":"rider@example.test","subject":"<script>alert('письмо')</script>",` +
		`"text":"<b>жирный</b> и <img src=x onerror=alert(1)>"}`
	answer := mailstubCall(t, internal, http.MethodPost, mailstubDeliveryPath, body, deliveryToken,
		map[string]string{deliveryKeyHeader: deliveredKey(t, otherInvoice)})
	if answer.Code != http.StatusCreated {
		t.Fatalf("the letter answered %d %s", answer.Code, answer.Body.String())
	}
	return box.stored[deliveredKey(t, otherInvoice)]
}

// A letter the box does not hold answers the same page an unknown address does, and the box does not
// say which of the two it was.
func TestAnUnknownLetterAndAnUnknownPageReadAlike(t *testing.T) {
	handler := mailstubInboxRouter(t, newFakeMailBox(), []byte(cursorKey))
	absent := mailstubCall(t, handler, http.MethodGet, inboxLetterPath(storedLetterID(9)), "", "", nil)
	if absent.Code != http.StatusNotFound || !strings.Contains(absent.Body.String(), inboxNoSuchLetter) {
		t.Fatalf("a letter that is not there answered %d %s", absent.Code, absent.Body.String())
	}
	// A page the surface does not serve is refused as a page rather than as an operation of the
	// contract, so a mistyped address reads as a refusal and not as emptiness.
	unknown := mailstubCall(t, handler, http.MethodGet, "/messages", "", "", nil)
	if unknown.Code != http.StatusNotFound || !strings.Contains(unknown.Body.String(), inboxNoSuchPage) {
		t.Fatalf("an address that is not a page answered %d %s", unknown.Code, unknown.Body.String())
	}
	for _, path := range []string{"/messages/", "/messages/one/two", "/inbox"} {
		answer := mailstubCall(t, handler, http.MethodGet, path, "", "", nil)
		if answer.Code != http.StatusNotFound {
			t.Errorf("%s answered %d", path, answer.Code)
		}
	}
}

// The box is read and never changed: a page answers GET and HEAD, and every other method is refused
// with the methods that are served rather than with an unknown resource.
func TestTheInboxPagesRefuseEveryOtherMethod(t *testing.T) {
	handler := mailstubInboxRouter(t, newFakeMailBox(), []byte(cursorKey))
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			answer := mailstubCall(t, handler, method, inboxPath, "", "", nil)
			if answer.Code != http.StatusMethodNotAllowed {
				t.Fatalf("%s answered %d %s", method, answer.Code, answer.Body.String())
			}
			if allow := answer.Header().Get("Allow"); allow != http.MethodGet+", "+http.MethodHead {
				t.Errorf("%s states Allow %q", method, allow)
			}
			if !strings.Contains(answer.Body.String(), inboxWrongMethod) {
				t.Errorf("%s answers no page stating the refusal", method)
			}
		})
	}
}

// A cursor this box did not issue is refused with a page that leads back to the list, rather than
// with a list drawn from a position nobody stated.
func TestAnUnreadableCursorIsRefusedAsAPage(t *testing.T) {
	box := newFakeMailBox()
	handler := mailstubInboxRouter(t, box, []byte(cursorKey))
	accepted := twoLettersAccepted(t, box)

	answer := mailstubCall(t, handler, http.MethodGet, inboxPath+"?cursor=not-a-cursor", "", "", nil)
	if answer.Code != http.StatusOK {
		t.Fatalf("an unreadable cursor answered %d %s", answer.Code, answer.Body.String())
	}
	page := answer.Body.String()
	if !strings.Contains(page, inboxUnreadableCursor) {
		t.Errorf("the page does not state what happened:\n%s", page)
	}
	if strings.Contains(page, accepted[0].Subject) {
		t.Errorf("the page lists the box anyway")
	}
	if !strings.Contains(page, `href="`+inboxPath+`"`) {
		t.Errorf("the page offers no way back to the list")
	}
	// The collection under the page refuses the same cursor as the contract says it does.
	refused := mailstubCall(t, handler, http.MethodGet, mailstubMessagesPath+"?cursor=not-a-cursor", "", "", nil)
	if refused.Code != http.StatusBadRequest || answeredCode(t, refused) != "INVALID_CURSOR" {
		t.Fatalf("the collection answered %d %s", refused.Code, refused.Body.String())
	}
}

// The list is a page of the collection: twenty letters at a time, and the link to the page after it
// appears when there is one, carrying the cursor the collection issues and staying on this surface.
// The last page states no link.
func TestTheInboxPageContinuesThroughTheCursorsOfTheCollection(t *testing.T) {
	box := newFakeMailBox()
	handler := mailstubInboxRouter(t, box, []byte(cursorKey))
	accepted := lettersAccepted(t, box, mailstub.PageSize+2)

	first := mailstubCall(t, handler, http.MethodGet, inboxPath, "", "", nil)
	if first.Code != http.StatusOK {
		t.Fatalf("the first page answered %d %s", first.Code, first.Body.String())
	}
	page := first.Body.String()
	if rows := strings.Count(page, `class="inbox-moment"`); rows != mailstub.PageSize {
		t.Fatalf("the first page lists %d letters", rows)
	}
	newest, oldest := accepted[len(accepted)-1], accepted[0]
	if !strings.Contains(page, newest.Subject) {
		t.Errorf("the first page does not carry the newest letter")
	}
	if strings.Contains(page, oldest.Subject) {
		t.Errorf("the first page carries a letter of the last page")
	}
	next := nextPageLink(t, page)
	if next == "" {
		t.Fatalf("the first page states no link to the next one")
	}
	// The link leads to the next page of this surface, not to the JSON of the same box: a reader who
	// follows it reads letters and a way further on, as the check below reads them.
	if !strings.HasPrefix(next, inboxPath+"?") {
		t.Fatalf("the first page continues at %q", next)
	}

	second := mailstubCall(t, handler, http.MethodGet, next, "", "", nil)
	if second.Code != http.StatusOK {
		t.Fatalf("the second page answered %d %s", second.Code, second.Body.String())
	}
	if !strings.Contains(second.Body.String(), oldest.Subject) {
		t.Errorf("the second page is not the page of this surface that carries the oldest letter")
	}
	if rows := strings.Count(second.Body.String(), `class="inbox-moment"`); rows != 2 {
		t.Errorf("the second page lists %d letters", rows)
	}
	if nextPageLink(t, second.Body.String()) != "" {
		t.Errorf("the last page states a link to a page that is not there")
	}
}

// A reader who reloads a page they reached by a cursor is shown that page again rather than the first
// one: the footer offers the address that was read, not only its path.
func TestAReloadedPageStaysWhereTheReaderWas(t *testing.T) {
	box := newFakeMailBox()
	handler := mailstubInboxRouter(t, box, []byte(cursorKey))
	lettersAccepted(t, box, mailstub.PageSize+2)
	next := nextPageLink(t, mailstubCall(t, handler, http.MethodGet, inboxPath, "", "", nil).Body.String())

	answer := mailstubCall(t, handler, http.MethodGet, next, "", "", nil)
	if answer.Code != http.StatusOK {
		t.Fatalf("the second page answered %d %s", answer.Code, answer.Body.String())
	}
	// The footer's refresh link is the first of the two the footer states, and it is the address the
	// page was read at, cursor included.
	if !strings.Contains(answer.Body.String(), `href="`+strings.ReplaceAll(next, "&", "&amp;")+`"`) {
		t.Errorf("the page offers no way to read itself again:\n%s", answer.Body.String())
	}
}

// nextPageLink is the address the list offers for the page after it, as the page states it.
func nextPageLink(t *testing.T, page string) string {
	t.Helper()
	at := strings.Index(page, `class="inbox-next"`)
	if at < 0 {
		return ""
	}
	href := page[at:]
	open := strings.Index(href, `href="`)
	if open < 0 {
		t.Fatalf("the page states no address for the next page")
	}
	href = href[open+len(`href="`):]
	return href[:strings.Index(href, `"`)]
}

// The pages are added beside the contract rather than instead of it: both reads answer the JSON the
// operations declare, and a path of the contract is answered by the boundary even when it is the
// refusal the contract states.
func TestTheContractOfTheInboxStillAnswersItsOwnJSON(t *testing.T) {
	box := newFakeMailBox()
	handler := mailstubInboxRouter(t, box, []byte(cursorKey))
	accepted := twoLettersAccepted(t, box)

	collection := mailstubCall(t, handler, http.MethodGet, mailstubMessagesPath, "", "", nil)
	if collection.Code != http.StatusOK {
		t.Fatalf("the collection answered %d %s", collection.Code, collection.Body.String())
	}
	page := answeredCollection(t, collection)
	if len(page.Items) != len(accepted) {
		t.Errorf("the collection answers %d letters", len(page.Items))
	}
	letter := mailstubCall(t, handler, http.MethodGet, mailstubMessagesPath+"/"+accepted[0].ID, "", "", nil)
	if letter.Code != http.StatusOK {
		t.Fatalf("one letter answered %d %s", letter.Code, letter.Body.String())
	}
	if strings.Contains(letter.Header().Get(contentTypeHeader), "html") {
		t.Errorf("the letter is answered as a page")
	}
	absent := mailstubCall(t, handler, http.MethodGet, mailstubMessagesPath+"/"+storedLetterID(9), "", "", nil)
	if absent.Code != http.StatusNotFound || answeredCode(t, absent) != "RESOURCE_NOT_FOUND" {
		t.Fatalf("a letter that is not there answered %d %s", absent.Code, absent.Body.String())
	}
}

// lettersAccepted fills the box with as many letters as a check asks for, through the delivery
// operation, and answers them in the order they were delivered. What each letter says carries a
// closing bracket, so that a check about one of them reads that letter rather than the beginning of
// another number.
func lettersAccepted(t *testing.T, box *fakeMailBox, count int) []mailstub.Message {
	t.Helper()
	internal := mailstubInternalRouter(t, box, newFakeActions(t), true)
	accepted := make([]mailstub.Message, 0, count)
	for position := 1; position <= count; position++ {
		invoice := fmt.Sprintf("01994342-6ba7-7000-8000-0000000001%02d", position)
		body := fmt.Sprintf(`{"to":"rider-%d@example.test","subject":"Письмо [%d]","text":"Текст [%d]"}`,
			position, position, position)
		answer := mailstubCall(t, internal, http.MethodPost, mailstubDeliveryPath, body,
			deliveryToken, map[string]string{deliveryKeyHeader: deliveredKey(t, invoice)})
		if answer.Code != http.StatusCreated {
			t.Fatalf("letter %d answered %d %s", position, answer.Code, answer.Body.String())
		}
		accepted = append(accepted, box.stored[deliveredKey(t, invoice)])
	}
	return accepted
}
