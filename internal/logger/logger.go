package logger

import (
	"os"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/rafidoth/gotemp/internal/config"
)

type LoggerService struct {
	nrApp *newrelic.Application
}

func NewLoggerService(cfg *config.ObservabilityConfig) *LoggerService {
	lService := &LoggerService{}

	if cfg.NewRelic.LicenseKey == "" {
		return lService
	}

	var configOptions []newrelic.ConfigOption
	configOptions = append(configOptions,
		newrelic.ConfigAppName(cfg.ServiceName),
		newrelic.ConfigLicense(cfg.NewRelic.LicenseKey),
		newrelic.ConfigAppLogForwardingEnabled(cfg.NewRelic.AppLogForwardingEnabled),
		newrelic.ConfigDistributedTracerEnabled(cfg.NewRelic.DistributedTracingEnabled),
	)

	// Add debug logging only if explicitly enabled
	if cfg.NewRelic.DebugLogging {
		configOptions = append(configOptions, newrelic.ConfigDebugLogger(os.Stdout))
	}

	app, err := newrelic.NewApplication(configOptions...)
	if err != nil {
		return lService
	}

	lService.nrApp = app
	return lService
}

// Shutdown shuts down New Relic
func (ls *LoggerService) Shutdown() {
	if ls.nrApp != nil {
		ls.nrApp.Shutdown(10 * time.Second)
	}
}

// GetApplication returns the New Relic application instance
func (ls *LoggerService) GetApplication() *newrelic.Application {
	return ls.nrApp
}
