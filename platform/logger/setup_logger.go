package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"

	"github.com/rs/zerolog"
)

type InterceptWriter struct {
	wrapped zerolog.LevelWriter
}

func (w InterceptWriter) Write(p []byte) (n int, err error) {
	// Parse JSON log
	var data map[string]interface{}
	if err := json.Unmarshal(p, &data); err == nil {
		fmt.Printf("[Intercepted Log] %v\n", data)
		// Example: access fields
		if user, ok := data["user"]; ok {
			fmt.Println("User is:", user)
		}
		if code, ok := data["code"]; ok {
			fmt.Println("Code is:", code)
		}
	}
	return w.wrapped.Write(p)
}

type LokiClient struct {
	PushIntveralSeconds int
	// This will also trigger the send event
	MaxBatchSize int
	Values       map[string][][]string
	LokiEndpoint string
	BatchCount   int
}

func (l *LokiClient) Bgrun() {
	lastRunTimestamp := 0
	isWorking := true
	for {
		if time.Now().Second()-lastRunTimestamp > l.PushIntveralSeconds || l.BatchCount > l.MaxBatchSize {
			// log.Debug().Msg("Running background log push")
			for k := range l.Values {
				if len(l.Values) > 0 {
					prevLogs := l.Values[k]
					l.Values[k] = [][]string{}
					err := pushToLoki(prevLogs, l.LokiEndpoint, k)
					if err != nil && isWorking {
						isWorking = false
						// log.Error().Msgf("Logs are currently not being forwarded to loki due to an error: %v", err)
					}
					if err == nil && !isWorking {
						isWorking = true
						// I will not accept PR comments about this log message tyvm
						// log.Info().Msgf("Logs are now being published again. The loki instance seems to be reachable once more!")
					}
				}
			}
			lastRunTimestamp = time.Now().Second()
			l.BatchCount = 0
		}
	}
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

type lokiLogEvent struct {
	Streams []lokiStream
}

/*
This function contains *no* error handling/logging because this:
a) should not crash the application
b) would mean that every run of this creates further logs that cannot be published
=> The error will be returned and the problem will be logged ONCE by the handling function
*/

func pushToLoki(logs [][]string, lokiEndpoint string, logLevel string) error {
	// log.Debug().Msg("Preparing to push logs to Loki")
	lokiPushPath := "/loki/api/v1/push"

	data, err := json.Marshal(lokiLogEvent{
		Streams: []lokiStream{
			{
				Stream: map[string]string{
					"service": "golang_backend",
					"level":   logLevel,
				},
				Values: logs,
			},
		},
	})

	if err != nil {
		log.Error().Msgf("Failed to marshal logs: %v", err)
		return err
	}

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%v%v", lokiEndpoint, lokiPushPath), bytes.NewBuffer(data))

	if err != nil {
		log.Error().Msgf("Failed to create HTTP request: %v", err)
		return err
	}

	ctx, cancel := context.WithTimeout(req.Context(), 100*time.Millisecond)

	defer cancel()

	req = req.WithContext(ctx)

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		log.Error().Msgf("Failed to send logs to Loki: %v", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 && resp.StatusCode != 200 && resp.StatusCode != 202 && resp.StatusCode != 204 && resp.StatusCode != 203 {
		log.Error().Msgf("Failed to push logs to loki: %v", resp.StatusCode)
		return fmt.Errorf("failed to push logs to loki: %v", resp.StatusCode)
	}

	// log.Debug().Msg("Successfully pushed logs to Loki")
	return nil
}

var lokiClient LokiClient

type LokiLogsWriter struct{}

func (l LokiLogsWriter) Write(p []byte) (n int, err error) {
	// Parse JSON log
	var data map[string]interface{}
	if err := json.Unmarshal(p, &data); err != nil {
		return 0, err
	}
	lokiClient.Values[data["level"].(string)] = append(lokiClient.Values[data["level"].(string)], []string{strconv.FormatInt(time.Now().UnixNano(), 10), string(p)})
	lokiClient.BatchCount++
	// fmt.Printf("[Intercepted Log] %v\n", data)
	return len(p), nil
}

func CustomZerologMiddleware(c *fiber.Ctx) error {
	start := time.Now()
	err := c.Next()
	duration := time.Since(start)
	durationMs := duration.Seconds() * 1000

	// Skip logging for /favicon.ico
	if c.Path() == "/favicon.ico" || c.Response().StatusCode() == 404 {
		return err
	}

	log.Info().
		Str("method", c.Method()).
		Str("url", c.OriginalURL()).
		Int("status", c.Response().StatusCode()).
		Float64("latency_ms", durationMs).
		Str("ip", c.IP()).
		Msg("Handled request")

	return err
}

func SetupLogger() *zerolog.Logger {
	// log.Debug().Msg("Setting up logger")
	// lokiClient = LokiClient{
	// 	PushIntveralSeconds: 5,
	// 	MaxBatchSize:        10,
	// 	Values:              map[string][][]string{},
	// 	LokiEndpoint:        os.Getenv("LOKI_ENDPOINT"),
	// 	BatchCount:          0,
	// }

	environment := os.Getenv("ENVIRONMENT")
	// Set global log level
	if environment == "PRODUCTION" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	// Enable development mode for local development (pretty console output)
	// zerolog.SetGlobalLevel(zerolog.DebugLevel)
	// log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	// Configure timestamp format
	zerolog.TimeFieldFormat = time.RFC3339

	// Set up output to file

	// logFile, err := os.OpenFile("./tmp/application.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	// if err != nil {
	// 	if err == os.ErrNotExist {
	// 		logfile, err := os.Create("./tmp/application.log")
	// 		if err != nil {
	// 			log.Fatal().Err(err).Msg("Failed to create log file")
	// 		}
	// 		logFile = logfile
	// 	}
	// 	// log.Fatal().Err(err).Msg("Failed to open log file")
	// }

	// set up log output to telegraf

	consoler_logger := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		NoColor:    false,
		TimeFormat: time.RFC3339,
	}

	// Use multi-writer for both console and file
	// loki_writer := LokiLogsWriter{}
	// go lokiClient.Bgrun()
	multi := zerolog.MultiLevelWriter(consoler_logger)
	logger := zerolog.New(multi).With().Timestamp().Caller().Logger()

	log.Logger = logger
	// Make log.Ctx(ctx) usable everywhere: without this, a context that carries no
	// logger falls through to zerolog's disabled (no-op) logger and silently drops
	// every log.Ctx(ctx)... line (e.g. the per-request logs in the repositories).
	// DefaultContextLogger is the fallback zerolog.Ctx returns when the context has
	// none, so those calls now write through the configured logger.
	zerolog.DefaultContextLogger = &logger
	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
		if strings.Contains(file, "fiberzerolog") {
			return ""
		}
		return file + ":" + strconv.Itoa(line)
	}

	log.Debug().Msg("Logger setup complete")
	// Or just file
	// log.Logger = zerolog.New(logFile).With().Timestamp().Caller().Logger()
	// logger.Err(fmt.Errorf("test error log")).Msg("Test error log")
	return &logger
}
