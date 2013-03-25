package main

import (
	"./ircd"
	"encoding/json"
	"flag"
	"os"
)

var (
	// Normal execution parameters
	config  = flag.String("config", "/etc/ircd.conf", "The configuration file to use")
	logfile = flag.String("log", "/var/log/ircd.log", "The file to which logs are written")

	// Flags
	silent = flag.Bool("silent", false, "Don't write logs to the console")

	// Other execution modes
	genconf   = flag.Bool("genconf", false, "Genereate a configuration file and exit")
	checkconf = flag.Bool("checkconf", false, "Check the configuration file and exit")
)

func main() {
	flag.Parse()

	if *genconf {
		conf, err := os.Create(*config)
		if err != nil {
			ircd.Error.Fatalf("Opening config file %q for writing: %s", *config, err)
		}
		b, err := json.MarshalIndent(ircd.DefaultConfiguration, "", "    ")
		if err != nil {
			ircd.Error.Fatalf("Failed to marshal default configuration!: %s", err)
		}
		if _, err = conf.Write(b); err != nil {
			ircd.Error.Fatalf("Writing default configuration to %q: %s", *config, err)
		}
		ircd.Info.Printf("Configuration file written to %q", *config)
		os.Exit(0)
	}

	if err := ircd.SetFile(*logfile); err != nil {
		ircd.Error.Fatalf("Opening logfile: %s", err)
	}
	if !*silent {
		ircd.ShowInConsole()
	}

	if err := ircd.LoadConfigFile(*config); err != nil {
		ircd.Error.Fatalf("Loading config: %s", err)
	}

	if *checkconf {
		if !ircd.CheckConfig() {
			ircd.Error.Fatalf("Invalid configuration")
		}
		ircd.Info.Printf("Configuration successfully checked.")
	}

	ircd.Start()
}
