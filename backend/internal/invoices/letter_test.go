package invoices

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/billing"
	"github.com/Alisher24/CarSharing/backend/internal/completion"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
)

// The moment and the two accounts every letter below is written from.
var letterMoment = time.Date(2026, time.September, 15, 8, 32, 11, 123456000, time.UTC)

// rideOf is the invoice one letter is rendered from: ninety seconds of driving begun as two minutes at
// 1234 tyiyn and a pause nobody entered, which is 2468 tyiyn. Every check below changes one field of
// it, so what it observes is the change rather than the whole letter.
func rideOf() Invoice {
	return Invoice{
		ID:       "01994342-6ba7-7000-8000-000000000001",
		RentalID: "01994342-6ba7-7000-8000-000000000002",
		UserID:   draftedFor,

		IssuedAt:      letterMoment,
		Currency:      "KGS",
		BillingPolicy: "started_minute",
		Completion:    completion.UserFinished,

		Driving: Line{
			Mode:        billing.Driving,
			Duration:    90_000_000,
			Minutes:     2,
			Rate:        1234,
			AmountTyiyn: 2468,
		},
		Paused: Line{
			Mode:        billing.Paused,
			Duration:    0,
			Minutes:     0,
			Rate:        321,
			AmountTyiyn: 0,
		},
		TotalTyiyn: 2468,
		Payment:    PendingPayment,
	}
}

// An ordinary ending is named in words and mentions no source: nothing ran out, and a letter that
// listed sources would describe an ending that did not happen.
func TestAnOrdinaryEndingIsNamedWithoutSources(t *testing.T) {
	letter := LetterOf(rideOf())
	if letter.Subject != "Поездка завершена" {
		t.Errorf("the subject is %q", letter.Subject)
	}
	if !strings.Contains(letter.Text, "поездку завершил пользователь") {
		t.Errorf("the letter does not name the reason:\n%s", letter.Text)
	}
	for _, source := range []string{"батарея", "бензин", "дизель", "сжатый газ"} {
		if strings.Contains(letter.Text, source) {
			t.Errorf("the letter names the source %q:\n%s", source, letter.Text)
		}
	}
}

// A ride that ran out is named by its reason and by the sources that were empty, in the order the
// invoice stored them.
func TestADepletedEndingNamesTheExhaustedSourcesInOrder(t *testing.T) {
	ride := rideOf()
	ride.Completion = completion.EnergyDepleted
	ride.Exhausted = []fleet.SourceKind{fleet.SourceBattery, fleet.SourceGasoline}
	letter := LetterOf(ride)
	if !strings.Contains(letter.Text, "закончился запас энергии или топлива: батарея, бензин") {
		t.Errorf("the letter does not name the sources in order:\n%s", letter.Text)
	}
}

// Both lines of the invoice are written whatever the ride did, and the minutes and the rate of each
// are printed exactly as the invoice stores them.
func TestBothLinesAreWrittenAsTheInvoiceStatesThem(t *testing.T) {
	letter := LetterOf(rideOf())
	if !strings.Contains(letter.Text, "Движение: 2 мин по 12,34 сома за минуту = 24,68 сома") {
		t.Errorf("the driving line is not the invoice's:\n%s", letter.Text)
	}
	if !strings.Contains(letter.Text, "Пауза: 0 мин по 3,21 сома за минуту = 0,00 сома") {
		t.Errorf("the paused line of a ride nobody paused is not written:\n%s", letter.Text)
	}
}

// The total is the one the invoice states, written from its whole tyiyn rather than from a sum of the
// two lines computed here.
func TestTheTotalIsTheOneTheInvoiceStates(t *testing.T) {
	ride := rideOf()
	ride.TotalTyiyn = 9007199254740993
	letter := LetterOf(ride)
	if !strings.Contains(letter.Text, "Итог: 90 071 992 547 409,93 сома") {
		t.Errorf("the total is not the invoice's:\n%s", letter.Text)
	}
	// The same amount rendered through a floating-point number is what the letter must not say: the
	// two differ in the last digits, which is exactly what a check has to refuse.
	rounded := strconv.FormatFloat(float64(ride.TotalTyiyn)/tyiynInSom, 'f', 2, 64)
	if strings.Contains(letter.Text, rounded+" сома") {
		t.Errorf("the total was rounded through a floating-point number: %s", rounded)
	}
}

// A ride that cost nothing says so, and its letter claims no payment that is not there: the invoice is
// settled by the moment it was issued, which is the state it states.
func TestAZeroInvoiceIsWrittenAsZero(t *testing.T) {
	ride := rideOf()
	ride.Driving = Line{Mode: billing.Driving, Rate: 1234}
	ride.Paused = Line{Mode: billing.Paused, Rate: 321}
	ride.TotalTyiyn = 0
	status, _ := FirstPayment(ride.TotalTyiyn, ride.IssuedAt)
	ride.Payment = status
	letter := LetterOf(ride)
	if !strings.Contains(letter.Text, "Итог: 0,00 сома") {
		t.Errorf("the total is not zero:\n%s", letter.Text)
	}
	if !strings.Contains(letter.Text, "Оплата: "+letterPaid) {
		t.Errorf("a ride that cost nothing does not state that nothing is owed:\n%s", letter.Text)
	}
	if strings.Contains(letter.Text, letterPending) {
		t.Errorf("a ride that cost nothing waits for a payment:\n%s", letter.Text)
	}
}

// The three states an invoice can hold produce three different letters, and each of them is true at
// the moment it is written: none of them promises an outcome that has not happened.
func TestEachPaymentStateIsWrittenTruthfully(t *testing.T) {
	written := map[string]PaymentStatus{}
	for _, status := range []PaymentStatus{PendingPayment, FailedPayment, PaidPayment} {
		ride := rideOf()
		ride.Payment = status
		letter := LetterOf(ride)
		if !strings.Contains(letter.Text, "Оплата: ") {
			t.Fatalf("the letter of a %s invoice says nothing about the payment:\n%s", status, letter.Text)
		}
		if previous, twice := written[letter.Text]; twice {
			t.Errorf("the states %s and %s produce one letter", previous, status)
		}
		written[letter.Text] = status
	}
	// A pending invoice is the one a letter written at the moment of the ending carries, and it says
	// that the outcome is still to come rather than stating one.
	pending := rideOf()
	if !strings.Contains(LetterOf(pending).Text, "Оплата: "+letterPending) {
		t.Errorf("a pending invoice claims an outcome:\n%s", LetterOf(pending).Text)
	}
}

// The letter is a pure function of the invoice: the same invoice produces the same text, and nothing
// outside the value it was given takes part in it.
func TestTheLetterIsAFunctionOfTheInvoiceAlone(t *testing.T) {
	ride := rideOf()
	once, twice := LetterOf(ride), LetterOf(rideOf())
	if once != twice {
		t.Errorf("two letters about one invoice differ:\n%s\n%s", once.Text, twice.Text)
	}
}

// A letter is rendered again by every attempt that delivers it, so what it states about the payment
// is the state the invoice was issued with: an invoice whose payment has moved since then produces
// the same letter as it did at the ending, which is what keeps a retry a repeat rather than a
// delivery under one key with another content.
func TestTheLetterOfAnInvoiceIsTheOneItWasIssuedWith(t *testing.T) {
	atTheEnding := rideOf()
	settled := atTheEnding
	settled.Payment = PaidPayment
	paidAt := letterMoment.Add(time.Second)
	settled.PaidAt = &paidAt
	settled.PaymentVersion = 2

	if LetterOf(settled) == LetterOf(atTheEnding) {
		t.Fatal("a payment transition does not change the letter of the invoice it settled")
	}
	if LetterOf(settled.AsIssued()) != LetterOf(atTheEnding.AsIssued()) {
		t.Errorf("the letter of an invoice changed with its payment:\n%s\n%s",
			LetterOf(settled.AsIssued()).Text, LetterOf(atTheEnding.AsIssued()).Text)
	}
}

// The moment the letter names is the moment the invoice was issued at, written the way the contract
// writes an instant.
func TestTheLetterNamesTheMomentTheInvoiceWasIssuedAt(t *testing.T) {
	if !strings.Contains(LetterOf(rideOf()).Text, "Счёт выставлен: 2026-09-15T08:32:11.123456Z") {
		t.Errorf("the letter does not name the moment:\n%s", LetterOf(rideOf()).Text)
	}
}

// A status or a reason this build does not know is written as unknown rather than as one of the states
// it does: a letter that guessed would be wrong about a ride nobody can explain.
func TestAnUnknownStateIsWrittenAsUnknown(t *testing.T) {
	ride := rideOf()
	ride.Payment = PaymentStatus("reversed")
	ride.Completion = completion.Reason("driver_vanished")
	letter := LetterOf(ride)
	if !strings.Contains(letter.Text, letterUnknownPayment) || !strings.Contains(letter.Text, letterUnknownReason) {
		t.Errorf("an unknown state is not written as unknown:\n%s", letter.Text)
	}
}
