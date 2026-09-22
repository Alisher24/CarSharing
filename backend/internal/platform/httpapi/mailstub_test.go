package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	mailstubapi "github.com/Alisher24/CarSharing/backend/internal/contracts/mailstubapi"
	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
	"github.com/Alisher24/CarSharing/backend/internal/mailstub"
	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/Alisher24/CarSharing/backend/internal/platform/httpheader"
	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
)

// The two credentials a mail stub check is served with, and the key every delivery below is sent
// under. The invoice is the one the key names, so a check that changes one of them is about the
// field it changed.
const (
	deliveryToken = "delivery-capability-token"
	mailDemoToken = "mail-demo-capability-token"
	invoiceID     = "01994342-6ba7-7000-8000-0000000000a1"
	otherInvoice  = "01994342-6ba7-7000-8000-0000000000a2"
	actionID      = "11111111-1111-4111-8111-111111111111"
)

// The moment the fake box answers with, so a receipt states a value a check can name rather than the
// moment the check happened to run at.
var acceptedAt = time.Date(2026, time.September, 15, 8, 32, 11, 123456000, time.UTC)

// A letter and the two requests that carry it: the one the box stores, and the one a check delivers
// when it changes a field of it.
func deliveredKey(t *testing.T, invoice string) string {
	t.Helper()
	key, err := mailstub.InvoiceKey(invoice)
	if err != nil {
		t.Fatal(err)
	}
	return key.String()
}

const deliveredLetter = `{"to":"rider@example.test","subject":"Поездка завершена","text":"Итог: 12,34 сома"}`

// fakeMailBox is the box the handler checks are answered from: it applies the two rules the delivery
// operation is about — one letter per key, and the same key with another letter a conflict — without
// a database, which is what the handler itself is checked for.
type fakeMailBox struct {
	mutex   sync.Mutex
	stored  map[string]mailstub.Message
	letters []mailstub.Message
	failure error
	dropped bool
}

func newFakeMailBox() *fakeMailBox {
	return &fakeMailBox{stored: map[string]mailstub.Message{}}
}

func (b *fakeMailBox) Accept(
	_ context.Context, key mailstub.Key, request mailstub.Request,
) (mailstub.Receipt, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if b.failure != nil {
		return mailstub.Receipt{}, b.failure
	}
	if stored, held := b.stored[key.String()]; held {
		if stored.To != request.To || stored.Subject != request.Subject || stored.Text != request.Text {
			return mailstub.Receipt{}, mailstub.ErrDeliveryConflict
		}
		// A repeat answers the receipt of the delivery that really arrived, so it loses nothing: the
		// demand for a lost answer belongs to the delivery that stores a letter.
		return mailstub.Receipt{Message: stored, Replayed: true}, nil
	}
	stored := mailstub.Message{
		ID:          storedLetterID(len(b.letters) + 1),
		DeliveryKey: key.String(),
		To:          request.To,
		Subject:     request.Subject,
		Text:        request.Text,
		AcceptedAt:  acceptedMomentOf(len(b.letters) + 1),
	}
	b.stored[key.String()] = stored
	b.letters = append(b.letters, stored)
	return mailstub.Receipt{Message: stored, Dropped: b.dropped}, nil
}

// acceptedMomentOf is the moment the fake box accepted the letter it stores in one position. A box
// accepts letters one after another, so each of them is a moment later than the one before: a check
// about the order the collection publishes reads a box whose order is decided by the moment rather
// than by the identifier alone.
func acceptedMomentOf(position int) time.Time {
	return acceptedAt.Add(time.Duration(position) * time.Second)
}

// storedLetterID is the identifier the fake box gives the letter it stores in one position. Every
// letter gets one of its own, so a check about one letter reads the letter it names.
func storedLetterID(position int) string {
	return fmt.Sprintf("01994342-6ba7-7000-8000-0000000000%02d", position)
}

func (b *fakeMailBox) ByID(_ context.Context, id string) (mailstub.Message, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if b.failure != nil {
		return mailstub.Message{}, b.failure
	}
	for _, stored := range b.letters {
		if stored.ID == id {
			return stored, nil
		}
	}
	return mailstub.Message{}, mailstub.ErrMessageNotFound
}

func (b *fakeMailBox) ReadPage(_ context.Context, after *cursor.Position, limit int) (mailstub.Page, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if b.failure != nil {
		return mailstub.Page{}, b.failure
	}
	remaining := make([]mailstub.Message, len(b.letters))
	copy(remaining, b.letters)
	// The order the collection publishes, which is the order the store reads a page in.
	sort.Slice(remaining, func(one, other int) bool {
		if !remaining[one].AcceptedAt.Equal(remaining[other].AcceptedAt) {
			return remaining[one].AcceptedAt.After(remaining[other].AcceptedAt)
		}
		return remaining[one].ID > remaining[other].ID
	})
	if after != nil {
		for index, stored := range remaining {
			if stored.ID == after.ID {
				remaining = remaining[index+1:]
				break
			}
		}
	}
	if len(remaining) <= limit {
		return mailstub.Page{Messages: remaining}, nil
	}
	page := remaining[:limit]
	last := page[len(page)-1]
	return mailstub.Page{
		Messages: page,
		Next:     &cursor.Position{Moment: last.AcceptedAt, ID: last.ID},
	}, nil
}

// fakeMailActions is the memory of the demonstration actions a handler check is answered from.
type fakeMailActions struct {
	mutex    sync.Mutex
	asked    []string
	answered map[string]idempotency.Fingerprint
	failure  error
}

func newFakeMailActions() *fakeMailActions {
	return &fakeMailActions{answered: map[string]idempotency.Fingerprint{}}
}

func (a *fakeMailActions) Apply(
	_ context.Context, actionID string, fingerprint idempotency.Fingerprint,
) (mailstub.Result, bool, error) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if a.failure != nil {
		return mailstub.Result{}, false, a.failure
	}
	if held, answered := a.answered[actionID]; answered {
		if held != fingerprint {
			return mailstub.Result{}, false, mailstub.ErrActionConflict
		}
		return mailstub.Result{ActionID: actionID, ServerTime: acceptedAt}, true, nil
	}
	a.answered[actionID] = fingerprint
	a.asked = append(a.asked, actionID)
	return mailstub.Result{ActionID: actionID, ServerTime: acceptedAt}, false, nil
}

// The internal listener of the mail stub as the process assembles it, over the fakes above.
func mailstubInternalRouter(t *testing.T, box *fakeMailBox, actions *fakeMailActions, demonstrating bool) http.Handler {
	t.Helper()
	handler, err := NewMailstubInternalListener(MailstubInternalDependencies{
		Deliveries:    box,
		Actions:       actions,
		Probe:         func(context.Context) error { return nil },
		Tokens:        MailstubTokens{Delivery: deliveryToken, Demo: mailDemoToken},
		Demonstrating: demonstrating,
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

// The inbox of the mail stub as the process assembles it, over one box and the key its cursors are
// signed with.
func mailstubInboxRouter(t *testing.T, box *fakeMailBox, key []byte) http.Handler {
	t.Helper()
	signer, err := cursor.NewSigner(key)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewMailstubInboxListener(MailstubInboxDependencies{Inbox: box, Cursors: signer})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

// mailstubCall sends one request and answers what the listener replied, together with the response
// headers, so a check can assert about the receipt as well as the status.
func mailstubCall(
	t *testing.T, handler http.Handler, method, path, body, token string, headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set(httpheader.ContentType, httpheader.JSON)
	}
	if token != "" {
		request.Header.Set(httpheader.Authorization, "Bearer "+token)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// What one error answer says, so a check asserts the code the contract declares rather than the
// status alone.
func answeredCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not the error contract: %s", recorder.Body.String())
	}
	return body.Code
}

// The first delivery of a letter stores it and answers its receipt; a repeat of the same key and
// letter answers the receipt of that first delivery, marked as a repeat.
func TestADeliveryIsAnsweredWithTheReceiptOfTheLetterItStored(t *testing.T) {
	box := newFakeMailBox()
	handler := mailstubInternalRouter(t, box, newFakeActions(t), true)

	first := mailstubCall(t, handler, http.MethodPost, mailstubDeliveryPath, deliveredLetter,
		deliveryToken, map[string]string{deliveryKeyHeader: deliveredKey(t, invoiceID)})
	if first.Code != http.StatusCreated {
		t.Fatalf("the first delivery answered %d %s", first.Code, first.Body.String())
	}
	if replayed := first.Header().Get("Idempotency-Replayed"); replayed != "false" {
		t.Errorf("a first delivery states Idempotency-Replayed %q", replayed)
	}
	receipt := deliveredReceipt(t, first)
	if receipt.Id != storedLetterID(1) || receipt.AcceptedAt != timestamp.Format(acceptedMomentOf(1)) {
		t.Errorf("the receipt is %+v", receipt)
	}

	repeat := mailstubCall(t, handler, http.MethodPost, mailstubDeliveryPath, deliveredLetter,
		deliveryToken, map[string]string{deliveryKeyHeader: deliveredKey(t, invoiceID)})
	if repeat.Code != http.StatusCreated {
		t.Fatalf("the repeat answered %d %s", repeat.Code, repeat.Body.String())
	}
	if replayed := repeat.Header().Get("Idempotency-Replayed"); replayed != "true" {
		t.Errorf("a repeat states Idempotency-Replayed %q", replayed)
	}
	if again := deliveredReceipt(t, repeat); again != receipt {
		t.Errorf("the repeat states the receipt %+v", again)
	}
}

// The same key with another letter is a conflict rather than a repeat, and nothing about the stored
// letter changes.
func TestAKeyThatHoldsAnotherLetterIsRefused(t *testing.T) {
	box := newFakeMailBox()
	handler := mailstubInternalRouter(t, box, newFakeActions(t), true)
	key := deliveredKey(t, invoiceID)
	mailstubCall(t, handler, http.MethodPost, mailstubDeliveryPath, deliveredLetter,
		deliveryToken, map[string]string{deliveryKeyHeader: key})

	conflict := mailstubCall(t, handler, http.MethodPost, mailstubDeliveryPath,
		`{"to":"rider@example.test","subject":"Другое письмо","text":"Другой текст"}`,
		deliveryToken, map[string]string{deliveryKeyHeader: key})
	if conflict.Code != http.StatusConflict || answeredCode(t, conflict) != "DELIVERY_CONFLICT" {
		t.Fatalf("the conflict answered %d %s", conflict.Code, conflict.Body.String())
	}
	if stored := box.stored[key]; stored.Subject != "Поездка завершена" {
		t.Errorf("the stored letter became %+v", stored)
	}
	// Another invoice is another key and therefore another letter.
	second := mailstubCall(t, handler, http.MethodPost, mailstubDeliveryPath, deliveredLetter,
		deliveryToken, map[string]string{deliveryKeyHeader: deliveredKey(t, otherInvoice)})
	if second.Code != http.StatusCreated {
		t.Fatalf("a second letter answered %d %s", second.Code, second.Body.String())
	}
}

// A delivery key of a shape this build does not write is a malformed header: it names no invoice the
// letter could be about, so nothing is stored.
func TestADeliveryKeyOfAnotherShapeIsRefused(t *testing.T) {
	box := newFakeMailBox()
	handler := mailstubInternalRouter(t, box, newFakeActions(t), true)
	for _, raw := range []string{
		"invoice:" + invoiceID,
		"receipt:" + invoiceID + ":issued",
		"invoice:11111111-1111-4111-8111-111111111111:issued",
	} {
		refused := mailstubCall(t, handler, http.MethodPost, mailstubDeliveryPath, deliveredLetter,
			deliveryToken, map[string]string{deliveryKeyHeader: raw})
		if refused.Code != http.StatusBadRequest || answeredCode(t, refused) != "INVALID_HEADER" {
			t.Errorf("%q answered %d %s", raw, refused.Code, refused.Body.String())
		}
	}
	if len(box.letters) != 0 {
		t.Errorf("%d letters were stored", len(box.letters))
	}
}

// The token of a delivery is checked before the letter is read, and a missing credential, a wrong one
// and the token of the other capability are one answer.
func TestTheDeliveryCredentialIsCheckedBeforeTheLetter(t *testing.T) {
	box := newFakeMailBox()
	handler := mailstubInternalRouter(t, box, newFakeActions(t), true)
	for _, refused := range []struct {
		name, token, body string
	}{
		{name: "no credential", token: "", body: deliveredLetter},
		{name: "a wrong credential", token: "not-the-token", body: deliveredLetter},
		{name: "the other capability", token: mailDemoToken, body: deliveredLetter},
		{name: "malformed JSON with no credential", token: "", body: "{"},
	} {
		t.Run(refused.name, func(t *testing.T) {
			answer := mailstubCall(t, handler, http.MethodPost, mailstubDeliveryPath, refused.body,
				refused.token, map[string]string{deliveryKeyHeader: deliveredKey(t, invoiceID)})
			if answer.Code != http.StatusUnauthorized ||
				answeredCode(t, answer) != "INTERNAL_AUTHENTICATION_REQUIRED" {
				t.Fatalf("the delivery answered %d %s", answer.Code, answer.Body.String())
			}
		})
	}
}

// A delivery of the mail stub's own demonstration token is refused there: the token that arms the
// loss is not the token that delivers a letter.
func TestTheDemonstrationControlRefusesTheDeliveryToken(t *testing.T) {
	handler := mailstubInternalRouter(t, newFakeMailBox(), newFakeActions(t), true)
	body := `{"action_id":"` + actionID + `","action":"drop_next_response_after_accept"}`
	refused := mailstubCall(t, handler, http.MethodPost, mailDemoActionPath, body, deliveryToken, nil)
	if refused.Code != http.StatusUnauthorized ||
		answeredCode(t, refused) != "INTERNAL_AUTHENTICATION_REQUIRED" {
		t.Fatalf("the demonstration control answered %d %s", refused.Code, refused.Body.String())
	}
}

// The action arms the loss once: a repeat of the identifier reproduces the stored answer and asks the
// box for nothing a second time.
func TestTheDemonstrationActionArmsTheLossOnce(t *testing.T) {
	actions := newFakeActions(t)
	handler := mailstubInternalRouter(t, newFakeMailBox(), actions, true)
	body := `{"action_id":"` + actionID + `","action":"drop_next_response_after_accept"}`

	first := mailstubCall(t, handler, http.MethodPost, mailDemoActionPath, body, mailDemoToken, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("the action answered %d %s", first.Code, first.Body.String())
	}
	if replayed := first.Header().Get("Idempotency-Replayed"); replayed != "false" {
		t.Errorf("a first action states Idempotency-Replayed %q", replayed)
	}
	repeat := mailstubCall(t, handler, http.MethodPost, mailDemoActionPath, body, mailDemoToken, nil)
	if repeat.Code != http.StatusOK || repeat.Body.String() != first.Body.String() {
		t.Fatalf("the repeat answered %d %s", repeat.Code, repeat.Body.String())
	}
	if replayed := repeat.Header().Get("Idempotency-Replayed"); replayed != "true" {
		t.Errorf("a repeated action states Idempotency-Replayed %q", replayed)
	}
	if len(actions.asked) != 1 {
		t.Errorf("the box was asked to arm the loss %d times", len(actions.asked))
	}
}

// An action the identifier already answered with another request is a conflict, and an attempt that
// is still running asks the caller to come back.
func TestTheDemonstrationActionReportsAConflictAndBusy(t *testing.T) {
	busy := newFakeActions(t)
	busy.failure = mailstub.ErrActionInProgress
	handler := mailstubInternalRouter(t, newFakeMailBox(), busy, true)
	body := `{"action_id":"` + actionID + `","action":"drop_next_response_after_accept"}`
	answer := mailstubCall(t, handler, http.MethodPost, mailDemoActionPath, body, mailDemoToken, nil)
	if answer.Code != http.StatusConflict || answeredCode(t, answer) != "IDEMPOTENCY_IN_PROGRESS" {
		t.Fatalf("a running action answered %d %s", answer.Code, answer.Body.String())
	}
	if wait := answer.Header().Get("Retry-After"); wait != "1" {
		t.Errorf("a running action asks for %q", wait)
	}

	conflicting := newFakeActions(t)
	conflicting.failure = mailstub.ErrActionConflict
	handler = mailstubInternalRouter(t, newFakeMailBox(), conflicting, true)
	answer = mailstubCall(t, handler, http.MethodPost, mailDemoActionPath, body, mailDemoToken, nil)
	if answer.Code != http.StatusConflict || answeredCode(t, answer) != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("a conflicting action answered %d %s", answer.Code, answer.Body.String())
	}
}

// Outside a demonstration the action is not part of the specification this listener serves, so it
// answers as an unknown resource while the delivery keeps working.
func TestTheDemonstrationActionIsAbsentOutsideADemonstration(t *testing.T) {
	handler := mailstubInternalRouter(t, newFakeMailBox(), newFakeActions(t), false)
	body := `{"action_id":"` + actionID + `","action":"drop_next_response_after_accept"}`
	absent := mailstubCall(t, handler, http.MethodPost, mailDemoActionPath, body, mailDemoToken, nil)
	if absent.Code != http.StatusNotFound || answeredCode(t, absent) != "RESOURCE_NOT_FOUND" {
		t.Fatalf("the action answered %d %s", absent.Code, absent.Body.String())
	}
	delivery := mailstubCall(t, handler, http.MethodPost, mailstubDeliveryPath, deliveredLetter,
		deliveryToken, map[string]string{deliveryKeyHeader: deliveredKey(t, invoiceID)})
	if delivery.Code != http.StatusCreated {
		t.Fatalf("the delivery answered %d %s", delivery.Code, delivery.Body.String())
	}
}

// A delivery whose answer a demonstration asked to lose stores the letter and closes the connection
// instead of answering: the client sees a broken transfer, which is the state a retry recovers from.
func TestADeliveryWhoseAnswerIsLostClosesTheConnection(t *testing.T) {
	box := newFakeMailBox()
	box.dropped = true
	server := httptest.NewServer(mailstubInternalRouter(t, box, newFakeActions(t), true))
	defer server.Close()

	request, err := http.NewRequest(http.MethodPost, server.URL+mailstubDeliveryPath,
		strings.NewReader(deliveredLetter))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(httpheader.ContentType, httpheader.JSON)
	request.Header.Set(httpheader.Authorization, "Bearer "+deliveryToken)
	request.Header.Set(deliveryKeyHeader, deliveredKey(t, invoiceID))

	response, err := server.Client().Do(request)
	if err == nil {
		response.Body.Close()
		t.Fatalf("the delivery answered %d instead of losing its answer", response.StatusCode)
	}
	if len(box.letters) != 1 {
		t.Fatalf("the box holds %d letters", len(box.letters))
	}
}

// Readiness answers with the mail schema rather than with the process: a probe that cannot read the
// box is a container that is not ready.
func TestMailReadinessFollowsTheBox(t *testing.T) {
	box := newFakeMailBox()
	handler, err := NewMailstubInternalListener(MailstubInternalDependencies{
		Deliveries: box,
		Actions:    newFakeActions(t),
		Probe:      func(context.Context) error { return errors.New("the mail schema is unreachable") },
		Tokens:     MailstubTokens{Delivery: deliveryToken, Demo: mailDemoToken},
	})
	if err != nil {
		t.Fatal(err)
	}
	unready := mailstubCall(t, handler, http.MethodGet, ReadyPath, "", "", nil)
	if unready.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness answered %d %s", unready.Code, unready.Body.String())
	}

	handler, err = NewMailstubInternalListener(MailstubInternalDependencies{
		Deliveries: box,
		Actions:    newFakeActions(t),
		Probe:      func(context.Context) error { return nil },
		Tokens:     MailstubTokens{Delivery: deliveryToken, Demo: mailDemoToken},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ready := mailstubCall(t, handler, http.MethodGet, ReadyPath, "", "", nil); ready.Code != http.StatusOK {
		t.Fatalf("readiness answered %d %s", ready.Code, ready.Body.String())
	}
}

// Each listener serves exactly the paths the contract marks as its own: a delivery through the inbox
// listener and a read of the box through the internal one are refused. The internal listener refuses
// as the contract does, and the inbox listener refuses as the page surface it also serves: the path
// belongs to no page there, and the answer a person reads is a page.
func TestEachMailListenerServesOnlyItsOwnPaths(t *testing.T) {
	internal := mailstubInternalRouter(t, newFakeMailBox(), newFakeActions(t), true)
	absent := mailstubCall(t, internal, http.MethodGet, mailstubMessagesPath, "", "", nil)
	if absent.Code != http.StatusNotFound || answeredCode(t, absent) != "RESOURCE_NOT_FOUND" {
		t.Fatalf("the internal listener answered a box read with %d %s", absent.Code, absent.Body.String())
	}

	inbox := mailstubInboxRouter(t, newFakeMailBox(), []byte(cursorKey))
	absent = mailstubCall(t, inbox, http.MethodPost, mailstubDeliveryPath, deliveredLetter, deliveryToken,
		map[string]string{deliveryKeyHeader: deliveredKey(t, invoiceID)})
	if absent.Code != http.StatusNotFound || !strings.Contains(absent.Body.String(), inboxNoSuchPage) {
		t.Fatalf("the inbox answered a delivery with %d %s", absent.Code, absent.Body.String())
	}
}

// The inbox is anonymous and read-only: the collection pages forward with a cursor of its own, the
// last page states no cursor, and a cursor of another limit, another key or another shape is refused.
func TestTheInboxPagesForwardUnderItsOwnKey(t *testing.T) {
	box := newFakeMailBox()
	handler := mailstubInboxRouter(t, box, []byte(cursorKey))
	accepted := twoLettersAccepted(t, box)

	first := mailstubCall(t, handler, http.MethodGet, mailstubMessagesPath+"?limit=1", "", "", nil)
	if first.Code != http.StatusOK {
		t.Fatalf("the collection answered %d %s", first.Code, first.Body.String())
	}
	page := answeredCollection(t, first)
	// Two letters accepted at one moment are published by identifier, newest first, so the second
	// delivery is the one the first page carries.
	if len(page.Items) != 1 || page.Items[0].Id != accepted[1].ID || page.NextCursor == nil {
		t.Fatalf("the first page is %+v", page)
	}
	second := mailstubCall(t, handler,
		http.MethodGet, mailstubMessagesPath+"?limit=1&cursor="+*page.NextCursor, "", "", nil)
	if second.Code != http.StatusOK {
		t.Fatalf("the second page answered %d %s", second.Code, second.Body.String())
	}
	continued := answeredCollection(t, second)
	if len(continued.Items) != 1 || continued.Items[0].Id != accepted[0].ID {
		t.Fatalf("the second page is %+v", continued)
	}
	if continued.NextCursor != nil {
		t.Errorf("the last page states the cursor %v", *continued.NextCursor)
	}

	for _, refused := range []struct {
		name  string
		query string
	}{
		{name: "another limit", query: "?limit=2&cursor=" + *page.NextCursor},
		{name: "another shape", query: "?limit=1&cursor=not-a-cursor"},
		{name: "another key", query: "?limit=1&cursor=" + cursorIssuedForAnotherKey(t, accepted[1])},
	} {
		t.Run(refused.name, func(t *testing.T) {
			answer := mailstubCall(t, handler, http.MethodGet, mailstubMessagesPath+refused.query, "", "", nil)
			if answer.Code != http.StatusBadRequest || answeredCode(t, answer) != "INVALID_CURSOR" {
				t.Fatalf("the cursor answered %d %s", answer.Code, answer.Body.String())
			}
		})
	}
}

// One letter is read whole, text included, and it is read without a credential: the box is anonymous.
func TestTheInboxAnswersOneLetterWithoutACredential(t *testing.T) {
	box := newFakeMailBox()
	handler := mailstubInboxRouter(t, box, []byte(cursorKey))
	accepted := twoLettersAccepted(t, box)

	answer := mailstubCall(t, handler, http.MethodGet, mailstubMessagesPath+"/"+accepted[0].ID, "", "", nil)
	if answer.Code != http.StatusOK {
		t.Fatalf("the letter answered %d %s", answer.Code, answer.Body.String())
	}
	var letter mailstubapi.Message
	if err := json.Unmarshal(answer.Body.Bytes(), &letter); err != nil {
		t.Fatal(err)
	}
	if letter.Id != accepted[0].ID || string(letter.To) != accepted[0].To ||
		letter.Subject != accepted[0].Subject || letter.Text != accepted[0].Text {
		t.Errorf("the letter is %+v", letter)
	}
	absent := mailstubCall(t, handler, http.MethodGet,
		mailstubMessagesPath+"/"+storedLetterID(len(accepted)+1), "", "", nil)
	if absent.Code != http.StatusNotFound || answeredCode(t, absent) != "RESOURCE_NOT_FOUND" {
		t.Fatalf("a missing letter answered %d %s", absent.Code, absent.Body.String())
	}
}

// The box has no operation that changes it: a method the contract does not declare is answered as a
// method that is not allowed, with the methods that are, rather than as an unknown resource.
func TestTheInboxRefusesEveryOtherMethod(t *testing.T) {
	handler := mailstubInboxRouter(t, newFakeMailBox(), []byte(cursorKey))
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			answer := mailstubCall(t, handler, method, mailstubMessagesPath, "", "", nil)
			if answer.Code != http.StatusMethodNotAllowed ||
				answeredCode(t, answer) != "METHOD_NOT_ALLOWED" {
				t.Fatalf("%s answered %d %s", method, answer.Code, answer.Body.String())
			}
			if allow := answer.Header().Get("Allow"); allow != http.MethodGet {
				t.Errorf("%s states Allow %q", method, allow)
			}
		})
	}
}

// A read carries no body: a request that sends one is refused rather than served, which is what keeps
// a create from being smuggled through an operation that has none.
func TestAReadOfTheInboxRefusesABody(t *testing.T) {
	handler := mailstubInboxRouter(t, newFakeMailBox(), []byte(cursorKey))
	answer := mailstubCall(t, handler, http.MethodGet, mailstubMessagesPath, "{}", "",
		map[string]string{httpheader.ContentType: httpheader.JSON})
	if answer.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("a read with a body answered %d %s", answer.Code, answer.Body.String())
	}
}

// The key every inbox cursor is signed with, and a letter of the box the collection reads.
const cursorKey = "mailstub-cursor-signing-key-of-one-installation"

// inboxLetters are the two letters the box holds in the checks below: the invoice each is about and
// what it says, which is what a read of the box is compared with.
var inboxLetters = []struct {
	invoice string
	subject string
	text    string
}{
	{invoice: invoiceID, subject: "Первое письмо", text: "Первый текст"},
	{invoice: otherInvoice, subject: "Второе письмо", text: "Второй текст"},
}

// twoLettersAccepted fills the box through the delivery operation, so the letters the collection
// reads are letters the stub really accepted, and answers them in the order they were delivered.
func twoLettersAccepted(t *testing.T, box *fakeMailBox) []mailstub.Message {
	t.Helper()
	internal := mailstubInternalRouter(t, box, newFakeActions(t), true)
	accepted := make([]mailstub.Message, 0, len(inboxLetters))
	for position, letter := range inboxLetters {
		body := `{"to":"rider@example.test","subject":"` + letter.subject +
			`","text":"` + letter.text + `"}`
		answer := mailstubCall(t, internal, http.MethodPost, mailstubDeliveryPath, body,
			deliveryToken, map[string]string{deliveryKeyHeader: deliveredKey(t, letter.invoice)})
		if answer.Code != http.StatusCreated {
			t.Fatalf("the letter answered %d %s", answer.Code, answer.Body.String())
		}
		stored := mailstub.Message{
			ID:         deliveredReceipt(t, answer).Id,
			To:         "rider@example.test",
			Subject:    letter.subject,
			Text:       letter.text,
			AcceptedAt: acceptedMomentOf(position + 1),
		}
		accepted = append(accepted, stored)
	}
	return accepted
}

// cursorIssuedForAnotherKey signs a cursor for one letter with the key of another installation, which
// is what a box that shared a public cursor key would accept.
func cursorIssuedForAnotherKey(t *testing.T, letter mailstub.Message) string {
	t.Helper()
	signer, err := cursor.NewSigner([]byte("mailstub-cursor-signing-key-of-another-installation"))
	if err != nil {
		t.Fatal(err)
	}
	issued, err := signer.Issue(
		cursor.Position{Moment: acceptedAt, ID: letter.ID},
		cursor.AnonymousOperationOn(getMessagesOperation, cursor.Parameter{Name: "limit", Value: "1"}))
	if err != nil {
		t.Fatal(err)
	}
	return issued
}

// The path of the delivery operation, the two paths of the inbox, and the demonstration control,
// stated once here so a check and the listener it calls name the same ones.
const (
	mailstubDeliveryPath = "/internal/v1/messages"
	mailstubMessagesPath = "/api/v1/messages"
)

// The two headers the delivery operation declares are named on both sides of the wire: the client of
// the stub fills them in (internal/mailstub) and the boundary of the process that serves the delivery
// reads them. They are two declarations of one name, and this is the check that holds them together.
func TestTheHeadersOfADeliveryAreNamedOnce(t *testing.T) {
	for _, header := range []struct {
		name     string
		client   string
		boundary string
	}{
		{name: "the delivery key", client: mailstub.DeliveryKeyHeader, boundary: deliveryKeyHeader},
		{name: "the request identifier", client: httpheader.RequestID, boundary: httpheader.RequestID},
	} {
		if header.client != header.boundary {
			t.Errorf("%s is %q to the sender and %q to the receiver", header.name, header.client, header.boundary)
		}
	}
}

// newFakeActions is the memory one handler check is answered from.
func newFakeActions(t *testing.T) *fakeMailActions {
	t.Helper()
	return newFakeMailActions()
}

// deliveredReceipt reads the receipt one 201 carries.
func deliveredReceipt(t *testing.T, recorder *httptest.ResponseRecorder) mailstubapi.DeliveryReceipt {
	t.Helper()
	var receipt mailstubapi.DeliveryReceipt
	if err := json.Unmarshal(recorder.Body.Bytes(), &receipt); err != nil {
		t.Fatalf("the receipt could not be read: %s", recorder.Body.String())
	}
	return receipt
}

// answeredCollection reads the page one 200 carries.
func answeredCollection(t *testing.T, recorder *httptest.ResponseRecorder) mailstubapi.MessageCollection {
	t.Helper()
	var page mailstubapi.MessageCollection
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatalf("the page could not be read: %s", recorder.Body.String())
	}
	return page
}
