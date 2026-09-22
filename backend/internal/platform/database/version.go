package database

// InitialVersion is the version a stored row carries when it is created. Every record of the
// installation counts its own revisions from it, so a reader can tell a row that has just been written
// from one that has been changed, and no module states the number again.
const InitialVersion int64 = 1
