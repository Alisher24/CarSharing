package config

import "os"

// The profile a process is started as. Only the demo profile seeds data a deployment must not
// carry, so the command that writes it refuses every other value.
const (
	EnvironmentVariable   = "APP_ENV"
	ProductionEnvironment = "production"
	DemoEnvironment       = "demo"
)

// Environment is the installation profile every process must agree on.
type Environment struct {
	Name string
}

func loadEnvironment() Environment {
	name := os.Getenv(EnvironmentVariable)
	if name == "" {
		name = ProductionEnvironment
	}
	return Environment{Name: name}
}
