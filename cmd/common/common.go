package common

import (
	"log"
	"os"
	"path/filepath"
)

func InitLogger() *os.File {
	homeDir, _ := os.UserHomeDir()
	logDir := filepath.Join(homeDir, ".flying_nimbus", "logs")
	os.MkdirAll(logDir, 0o755)

	logPath := filepath.Join(logDir, "flying_nimbus.log")

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		log.Fatal("Failed to open log file:", err)
	}

	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	return logFile
}
