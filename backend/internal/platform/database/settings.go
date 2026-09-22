package database

// Settings are the address and credential a process uses to reach PostgreSQL.
type Settings struct {
	Host     string
	Port     uint16
	Name     string
	User     string
	Password string
}
