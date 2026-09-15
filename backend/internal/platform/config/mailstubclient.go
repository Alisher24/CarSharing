package config

// MailstubAPIURLVariable names the address the processes that call the mail stub reach it on. It is
// the callers' setting alone: the stub itself is told nothing about where it is called from.
const MailstubAPIURLVariable = "MAILSTUB_API_URL"

// defaultMailstubAPIURL is the address the mail stub is reached on in the assembled stack, which is
// the name of the service that serves it. The internal listener is published nowhere, so a process
// outside that network names the address it uses.
const defaultMailstubAPIURL = "http://mailstub:8080"

// MailstubClient is what the process that delivers letters is told: the address it reaches the mail
// stub on and the token of the delivery capability. It carries no other credential, because a
// process that sends a letter has no business arming the loss of an answer.
func MailstubClient() (InternalClient, error) {
	return mailstubClient(MailstubDeliveryTokenFileVariable)
}

// MailstubDemoClient is what the demonstration control for the mail stub is told: the address of the
// same service and the token of its demonstration capability. The two clients are told apart by the
// capability they carry rather than by the address they call, because both are the mail stub.
func MailstubDemoClient() (InternalClient, error) {
	return mailstubClient(MailstubDemoTokenFileVariable)
}

// mailstubClient reads one caller's credential and the address it calls, which is the shape every
// client of an internal surface is given.
func mailstubClient(tokenVariable string) (InternalClient, error) {
	return internalClientOf(tokenVariable, MailstubAPIURLVariable, defaultMailstubAPIURL)
}
