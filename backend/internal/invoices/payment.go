package invoices

// PaymentStatus is what is owed on an invoice. Every status the contract publishes is declared here,
// because the storage admits all three and the read of one has to name what it read; the transitions
// between them belong to the task that owns payment.
type PaymentStatus string

const (
	// PendingPayment is an invoice nothing has settled yet. An invoice this build has just issued
	// states it, whatever its amount: a zero invoice is settled by the transition that reads it, and
	// this task does not perform that transition.
	PendingPayment PaymentStatus = "pending"

	// FailedPayment is an attempt that was refused. The invoice is unchanged and may be attempted
	// again.
	FailedPayment PaymentStatus = "failed"

	// PaidPayment is an invoice that is settled.
	PaidPayment PaymentStatus = "paid"
)

// Known reports whether a status is one this vocabulary declares, which is what a row read from
// storage is judged by before it is published under a shape that states one.
func (s PaymentStatus) Known() bool {
	return s == PendingPayment || s == FailedPayment || s == PaidPayment
}
