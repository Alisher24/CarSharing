package invoices

import (
	"strconv"
	"strings"

	"github.com/Alisher24/CarSharing/backend/internal/billing"
	"github.com/Alisher24/CarSharing/backend/internal/completion"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
)

// Letter is what a person receives about one finished ride: the subject the letter is sent under and
// its plain text. The address is not part of it, because an invoice does not know the address of the
// account that owes it; whoever read that account fills it in.
//
// The text is plain text and stays plain text: no markup is written here and none is interpreted by
// whoever shows it.
type Letter struct {
	Subject string
	Text    string
}

// The wording of the letter. Every phrase a check or the letter itself depends on is declared once
// here rather than spelled where it is used.
const (
	letterSubject = "Поездка завершена"

	letterFinished   = "Поездка завершена."
	letterIssuedAt   = "Счёт выставлен: "
	letterReason     = "Причина: "
	letterTotal      = "Итог: "
	letterPayment    = "Оплата: "
	letterDriving    = "Движение: "
	letterPaused     = "Пауза: "
	letterMinuteRate = " мин по "
	letterPerMinute  = " за минуту = "

	letterUserFinished   = "поездку завершил пользователь"
	letterEnergyDepleted = "закончился запас энергии или топлива"
	letterUnknownReason  = "причина не указана"

	letterPaid           = "счёт оплачен"
	letterPending        = "счёт ожидает оплаты, исход оплаты сообщим отдельно"
	letterFailed         = "попытка оплаты не прошла, счёт остаётся неоплаченным"
	letterUnknownPayment = "состояние оплаты неизвестно"
)

// LetterOf renders the letter one stored invoice produces. It is a pure function of that invoice: it
// reads no environment, opens no transaction and sends nothing, so what a person receives is checked
// by a unit test rather than through an assembled stack.
//
// Nothing is counted here. The minutes, the rates and the total are the ones the invoice states, and
// each amount is rendered from the whole number of tyiyn it stores: a sum that passed through a
// floating-point number would no longer be the sum the invoice published, and the letter is the third
// place that sum appears.
//
// The two lines are always written, including the mode the ride never entered, because an invoice
// states two of them whatever the ride did. The moment the letter names is the moment the invoice was
// issued at, which is the moment the ending was recorded: a ride that ran out before anything noticed
// it is dated by the record of its ending rather than by a second reading of the ride.
func LetterOf(ride Invoice) Letter {
	lines := []string{
		letterFinished,
		letterIssuedAt + timestamp.Format(ride.IssuedAt),
		letterReason + reasonText(ride),
		"",
		lineText(letterDriving, ride.Driving),
		lineText(letterPaused, ride.Paused),
		"",
		letterTotal + somText(ride.TotalTyiyn),
		letterPayment + paymentText(ride.Payment),
	}
	return Letter{Subject: letterSubject, Text: strings.Join(lines, "\n")}
}

// reasonText is why the ride ended, and which sources ran out when that is why. The sources are named
// in the order the invoice stored them, which is the order of the vehicle's own profile.
func reasonText(ride Invoice) string {
	switch ride.Completion {
	case completion.UserFinished:
		return letterUserFinished
	case completion.EnergyDepleted:
		if len(ride.Exhausted) == 0 {
			return letterEnergyDepleted
		}
		names := make([]string, 0, len(ride.Exhausted))
		for _, source := range ride.Exhausted {
			names = append(names, sourceName(source))
		}
		return letterEnergyDepleted + ": " + strings.Join(names, ", ")
	default:
		return letterUnknownReason
	}
}

// sourceName is one energy source as the interface names it, in the middle of a sentence.
func sourceName(kind fleet.SourceKind) string {
	switch kind {
	case fleet.SourceBattery:
		return "батарея"
	case fleet.SourceGasoline:
		return "бензин"
	case fleet.SourceDiesel:
		return "дизель"
	case fleet.SourceLPG:
		return "сжиженный газ"
	case fleet.SourceCNG:
		return "сжатый газ"
	default:
		return "неизвестный источник"
	}
}

// lineText is one mode of the ride as the invoice priced it: the minutes begun in it, the rate those
// minutes were charged at, and their product. All three are printed as the invoice states them, so a
// reader can multiply what the letter says and arrive at what it says the line cost.
func lineText(label string, line Line) string {
	return label + strconv.FormatInt(line.Minutes, 10) + letterMinuteRate +
		somText(billing.AmountTyiyn(line.Rate)) + letterPerMinute + somText(line.AmountTyiyn)
}

// paymentText is where the payment of the invoice stands at the moment the letter is written. It
// states one of the three states the invoice can hold and never an outcome that has not happened: a
// letter that promised a payment would be wrong the moment that payment was refused.
func paymentText(status PaymentStatus) string {
	switch status {
	case PaidPayment:
		return letterPaid
	case PendingPayment:
		return letterPending
	case FailedPayment:
		return letterFailed
	default:
		return letterUnknownPayment
	}
}
