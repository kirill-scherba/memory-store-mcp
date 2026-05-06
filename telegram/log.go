// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package telegram

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// BotLogger provides dual logging: stderr + file.
type BotLogger struct {
	mu       sync.Mutex
	logFile  *os.File
	logger   *log.Logger
	enabled  bool
}

var defaultLogger *BotLogger

// InitBotLogger initializes the global bot logger with file output.
// Log file is created at logPath (or ~/.config/memory-store-mcp/telegram.log if empty).
// Rotates at maxSize bytes (default 10MB). Keeps keepFiles rotated copies.
func InitBotLogger(logPath string, maxSize int64, keepFiles int) error {
	if logPath == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return fmt.Errorf("cannot determine config dir: %w", err)
		}
		logDir := filepath.Join(configDir, "memory-store-mcp")
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return fmt.Errorf("cannot create log dir: %w", err)
		}
		logPath = filepath.Join(logDir, "telegram.log")
	}

	if maxSize <= 0 {
		maxSize = 10 * 1024 * 1024 // 10 MB
	}
	if keepFiles <= 0 {
		keepFiles = 3
	}

	// Rotate if needed
	if fi, err := os.Stat(logPath); err == nil && fi.Size() > maxSize {
		rotateLog(logPath, keepFiles)
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("cannot open log file: %w", err)
	}

	// Write to both stderr and file
	multi := io.MultiWriter(os.Stderr, f)

	defaultLogger = &BotLogger{
		logFile: f,
		logger:  log.New(multi, "", log.LstdFlags|log.Lmicroseconds),
		enabled: true,
	}

	defaultLogger.Logf("📋 Bot logger initialized: %s", logPath)
	return nil
}

// Close closes the log file.
func (bl *BotLogger) Close() {
	bl.mu.Lock()
	defer bl.mu.Unlock()
	if bl.logFile != nil {
		bl.logFile.Close()
		bl.logFile = nil
	}
}

// Logf writes a formatted log message to both stderr and the log file.
func (bl *BotLogger) Logf(format string, args ...interface{}) {
	bl.mu.Lock()
	defer bl.mu.Unlock()
	if bl.logger != nil {
		bl.logger.Output(2, fmt.Sprintf(format, args...))
	}
}

// LogLLM logs the full LLM interaction: request messages and response.
func (bl *BotLogger) LogLLM(systemPrompt, userMessage, llmResponse, parsedResult string) {
	bl.Logf("\n========== LLM REQUEST ==========")
	bl.Logf("SYSTEM: %.500s", systemPrompt)
	bl.Logf("USER:   %s", userMessage)
	bl.Logf("RAW:    %s", llmResponse)
	bl.Logf("PARSED: %s", parsedResult)
	bl.Logf("========== LLM END ==========")
}

// LogUserMessage logs the incoming user message.
func (bl *BotLogger) LogUserMessage(chatID int64, text string) {
	bl.Logf("[USER:%d] %s", chatID, text)
}

// LogBotResponse logs the outgoing bot response.
func (bl *BotLogger) LogBotResponse(chatID int64, text string) {
	bl.Logf("[BOT:%d] %s", chatID, text)
}

// Logf is a package-level helper that uses the default logger.
func Logf(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Logf(format, args...)
		return
	}
	log.Printf(format, args...)
}

// LogLLM is a package-level helper.
func LogLLM(systemPrompt, userMessage, llmResponse, parsedResult string) {
	if defaultLogger != nil {
		defaultLogger.LogLLM(systemPrompt, userMessage, llmResponse, parsedResult)
	}
}

// LogUserMessage is a package-level helper.
func LogUserMessage(chatID int64, text string) {
	if defaultLogger != nil {
		defaultLogger.LogUserMessage(chatID, text)
		return
	}
	log.Printf("[USER:%d] %s", chatID, text)
}

// LogBotResponse is a package-level helper.
func LogBotResponse(chatID int64, text string) {
	if defaultLogger != nil {
		defaultLogger.LogBotResponse(chatID, text)
		return
	}
	log.Printf("[BOT:%d] %s", chatID, text)
}

// rotateLog rotates the log file: .1 -> .2 -> .3, current -> .1
func rotateLog(logPath string, keepFiles int) {
	for i := keepFiles - 1; i > 0; i-- {
		oldPath := fmt.Sprintf("%s.%d", logPath, i)
		newPath := fmt.Sprintf("%s.%d", logPath, i+1)
		if _, err := os.Stat(oldPath); err == nil {
			os.Rename(oldPath, newPath)
		}
	}
	if _, err := os.Stat(logPath); err == nil {
		os.Rename(logPath, fmt.Sprintf("%s.1", logPath))
	}
}

// LogClose closes the default logger's file.
func LogClose() {
	if defaultLogger != nil {
		defaultLogger.Close()
	}
}
// TimeStr returns a compact timestamp string.
func TimeStr() string {
	return time.Now().Format("15:04:05.000")
}