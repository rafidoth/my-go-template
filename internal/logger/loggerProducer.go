package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/newrelic/go-agent/v3/integrations/logcontext-v2/zerologWriter"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/rafidoth/gotemp/internal/config"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
)

func NewLoggerWithService(cfg *config.ObservabilityConfig, loggerService *LoggerService) zerolog.Logger {

	//  Reads the desired log level (debug, info, warn, error) from the config and sets
	//  it on the logger. Defaults to "info" if the level string is unrecognized.
	var logLevel zerolog.Level
	level := cfg.GetLogLevel()

	switch level {
	case "debug":
		logLevel = zerolog.DebugLevel
	case "info":
		logLevel = zerolog.InfoLevel
	case "warn":
		logLevel = zerolog.WarnLevel
	case "error":
		logLevel = zerolog.ErrorLevel
	default:
		logLevel = zerolog.InfoLevel
	}

	//Sets the global zerolog time format to "2006-01-02 15:04:05" and enables stack
	//trace marshaling via pkgerrors for richer error output.
	zerolog.TimeFieldFormat = "2006-01-02 15:04:05"
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	var writer io.Writer

	// Setup base writer
	var baseWriter io.Writer
	if cfg.IsProduction() && cfg.Logging.Format == "json" {
		// In production, write to stdout
		baseWriter = os.Stdout

		// in production also forwarding the logs to new relic using its zerologWriter
		if loggerService != nil && loggerService.nrApp != nil {
			nrWriter := zerologWriter.New(baseWriter, loggerService.nrApp)
			writer = nrWriter
		} else {
			writer = baseWriter
		}
	} else {
		// Development mode - use console writer
		consoleWriter := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "2006-01-02 15:04:05"}
		writer = consoleWriter
	}

	// Note: New Relic log forwarding is now handled automatically by zerologWriter integration
	logger := zerolog.New(writer).
		Level(logLevel).
		With().
		Timestamp().
		Str("service", cfg.ServiceName).
		Str("environment", cfg.Environment).
		Logger()

	// Include stack traces for errors in development
	if !cfg.IsProduction() {
		logger = logger.With().Stack().Logger()
	}

	return logger
}

// WithTraceContext enriches an existing zerolog.Logger with New Relic distributed
// tracing metadata extracted from the given transaction.
//
// This is intended to be called within HTTP handlers or background tasks where a
// New Relic transaction is active. It pulls the trace ID and span ID from the
// transaction's metadata and attaches them as structured fields ("trace.id" and
// "span.id") on the returned logger. This allows log entries to be correlated with
// specific traces and spans in the New Relic UI, enabling end-to-end request tracing
// across services.
//
// If the provided transaction is nil (e.g., New Relic is not configured), the
// original logger is returned unchanged, making this safe to call unconditionally.
func WithTraceContext(logger zerolog.Logger, txn *newrelic.Transaction) zerolog.Logger {
	if txn == nil {
		return logger
	}

	// Get trace metadata from transaction
	metadata := txn.GetTraceMetadata()

	return logger.With().
		Str("trace.id", metadata.TraceID).
		Str("span.id", metadata.SpanID).
		Logger()
}

// NewPgxLogger creates a zerolog.Logger specifically tailored for logging database
// operations from the pgx PostgreSQL driver.
//
// Every log entry produced by this logger includes a "component" field set to
// "database", making it easy to filter database-related logs from application logs.
// The caller specifies the minimum log level (e.g., zerolog.DebugLevel to see all
// queries, or zerolog.WarnLevel to only see slow/problematic ones).
func NewPgxLogger(level zerolog.Level) zerolog.Logger {
	writer := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006-01-02 15:04:05",
		FormatFieldValue: func(i any) string {
			switch v := i.(type) {
			case string:
				// SQL statements longer than 200 characters are truncated with "..." to
				// keep log output concise and readable.
				// Clean and format SQL for better readability
				if len(v) > 200 {
					// Truncate very long SQL statements
					return v[:200] + "..."
				}
				return v
			case []byte:
				// Attempts to unmarshal as JSON and pretty-prints the result
				// with indentation. This is useful for logging query parameters or result rows that
				// pgx may emit as raw JSON bytes. Falls back to plain string conversion if the
				// bytes are not valid JSON.
				var obj interface{}
				if err := json.Unmarshal(v, &obj); err == nil {
					pretty, _ := json.MarshalIndent(obj, "", "    ")
					return "\n" + string(pretty)
				}
				return string(v)
			default:
				// fallback to just print it
				return fmt.Sprintf("%v", v)
			}
		},
	}

	return zerolog.New(writer).
		Level(level).
		With().
		Timestamp().
		Str("component", "database").
		Logger()
}

// GetPgxTraceLogLevel converts a zerolog.Level into the corresponding integer value
// used by pgx's tracelog package for controlling database query log verbosity.
//
// pgx's tracelog uses its own integer-based log level system (defined in
// github.com/jackc/pgx/v5/tracelog) rather than zerolog levels directly. This
// function bridges the two systems so that the application can use a single log
// level configuration (from zerolog) and have it consistently applied to database
// query tracing as well.
//
// Mapping:
//   - zerolog.DebugLevel -> 6 (tracelog.LogLevelDebug) — logs all queries and results
//   - zerolog.InfoLevel  -> 4 (tracelog.LogLevelInfo)  — logs queries without detailed results
//   - zerolog.WarnLevel  -> 3 (tracelog.LogLevelWarn)  — logs only slow or problematic queries
//   - zerolog.ErrorLevel -> 2 (tracelog.LogLevelError) — logs only query errors
//   - Any other level    -> 0 (tracelog.LogLevelNone)  — disables query logging entirely
func GetPgxTraceLogLevel(level zerolog.Level) int {
	switch level {
	case zerolog.DebugLevel:
		return 6 // tracelog.LogLevelDebug
	case zerolog.InfoLevel:
		return 4 // tracelog.LogLevelInfo
	case zerolog.WarnLevel:
		return 3 // tracelog.LogLevelWarn
	case zerolog.ErrorLevel:
		return 2 // tracelog.LogLevelError
	default:
		return 0 // tracelog.LogLevelNone
	}
}
