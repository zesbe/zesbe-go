package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
)

var (
	// Log is the global logger instance
	Log zerolog.Logger

	// logFile keeps reference to the log file for cleanup
	logFile *os.File
)

// Config holds logger configuration
type Config struct {
	Level      string
	Pretty     bool
	FilePath   string
	MaxSize    int64 // Max file size in bytes before rotation
	MaxBackups int   // Max number of backup files
}

// DefaultConfig returns default logger configuration
func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	return Config{
		Level:      "info",
		Pretty:     false,
		FilePath:   filepath.Join(home, ".zesbe-go", "logs", "zesbe.log"),
		MaxSize:    10 * 1024 * 1024, // 10MB
		MaxBackups: 5,
	}
}

// Init initializes the global logger
func Init(cfg Config) error {
	// Parse log level
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}

	// Create log directory if needed
	if cfg.FilePath != "" {
		dir := filepath.Dir(cfg.FilePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create log directory: %w", err)
		}

		// Rotate if needed
		rotateIfNeeded(cfg)

		// Open log file
		logFile, err = os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
	}

	// Configure output writers
	var writers []io.Writer
	if logFile != nil {
		writers = append(writers, logFile)
	}

	// Create multi-writer
	var output io.Writer
	if len(writers) > 0 {
		output = io.MultiWriter(writers...)
	} else {
		output = os.Stderr
	}

	// Configure zerolog
	zerolog.TimeFieldFormat = time.RFC3339
	zerolog.SetGlobalLevel(level)

	if cfg.Pretty {
		output = zerolog.ConsoleWriter{Out: output, TimeFormat: time.RFC3339}
	}

	Log = zerolog.New(output).With().Timestamp().Caller().Logger()

	return nil
}

// rotateIfNeeded rotates log file if it exceeds max size
func rotateIfNeeded(cfg Config) {
	info, err := os.Stat(cfg.FilePath)
	if err != nil {
		return
	}

	if info.Size() < cfg.MaxSize {
		return
	}

	// Rotate existing backups
	for i := cfg.MaxBackups - 1; i > 0; i-- {
		oldPath := fmt.Sprintf("%s.%d", cfg.FilePath, i)
		newPath := fmt.Sprintf("%s.%d", cfg.FilePath, i+1)
		os.Rename(oldPath, newPath)
	}

	// Move current file to .1
	os.Rename(cfg.FilePath, cfg.FilePath+".1")
}

// Close closes the log file
func Close() {
	if logFile != nil {
		logFile.Close()
	}
}

// Debug logs a debug message
func Debug(msg string) {
	Log.Debug().Msg(msg)
}

// Debugf logs a formatted debug message
func Debugf(format string, args ...interface{}) {
	Log.Debug().Msgf(format, args...)
}

// Info logs an info message
func Info(msg string) {
	Log.Info().Msg(msg)
}

// Infof logs a formatted info message
func Infof(format string, args ...interface{}) {
	Log.Info().Msgf(format, args...)
}

// Warn logs a warning message
func Warn(msg string) {
	Log.Warn().Msg(msg)
}

// Warnf logs a formatted warning message
func Warnf(format string, args ...interface{}) {
	Log.Warn().Msgf(format, args...)
}

// Error logs an error message
func Error(msg string, err error) {
	Log.Error().Err(err).Msg(msg)
}

// Errorf logs a formatted error message
func Errorf(format string, args ...interface{}) {
	Log.Error().Msgf(format, args...)
}

// Fatal logs a fatal message and exits
func Fatal(msg string, err error) {
	Log.Fatal().Err(err).Msg(msg)
}

// WithField returns a logger with a single field
func WithField(key string, value interface{}) zerolog.Logger {
	return Log.With().Interface(key, value).Logger()
}

// WithFields returns a logger with multiple fields
func WithFields(fields map[string]interface{}) zerolog.Logger {
	ctx := Log.With()
	for k, v := range fields {
		ctx = ctx.Interface(k, v)
	}
	return ctx.Logger()
}

// APIRequest logs an API request
func APIRequest(provider, model, endpoint string) {
	Log.Info().
		Str("provider", provider).
		Str("model", model).
		Str("endpoint", endpoint).
		Msg("API request")
}

// APIResponse logs an API response
func APIResponse(provider string, status int, duration time.Duration, tokens int) {
	Log.Info().
		Str("provider", provider).
		Int("status", status).
		Dur("duration", duration).
		Int("tokens", tokens).
		Msg("API response")
}

// ToolExecution logs a tool execution
func ToolExecution(tool string, success bool, duration time.Duration) {
	event := Log.Info().
		Str("tool", tool).
		Bool("success", success).
		Dur("duration", duration)
	event.Msg("tool execution")
}
