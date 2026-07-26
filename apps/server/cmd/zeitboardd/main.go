package main

import (
	"flag"
	"log"

	"non24.app/server/internal/daemon"
)

const maxLogBytes = 10 << 20

func main() {
	configPath := flag.String("config", "", "path to JSON config file")
	serviceName := flag.String("service-name", "ZeitBoardServer", "Windows service name")
	logPath := flag.String("log", "", "optional daemon log file")
	checkConfig := flag.Bool("check-config", false, "validate configuration and exit")
	flag.Parse()

	if *logPath != "" {
		writer, err := daemon.OpenRotatingLog(*logPath, maxLogBytes)
		if err != nil {
			log.Fatalf("open log file: %v", err)
		}
		log.SetOutput(writer)
		defer writer.Close()
	}

	if *checkConfig {
		if err := daemon.ValidateConfig(*configPath); err != nil {
			log.Fatalf("validate config: %v", err)
		}
		log.Printf("configuration is valid")
		return
	}

	if err := daemon.Run(*configPath, *serviceName); err != nil {
		log.Fatalf("run daemon: %v", err)
	}
}
