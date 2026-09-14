package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/genxcare/api/pkg/logging"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":     slog.LevelDebug,
		"DEBUG":     slog.LevelDebug,
		" warn ":    slog.LevelWarn,
		"warning":   slog.LevelWarn,
		"error":     slog.LevelError,
		"info":      slog.LevelInfo,
		"":          slog.LevelInfo,
		"gibberish": slog.LevelInfo,
	}

	for input, want := range cases {
		if got := logging.ParseLevel(input); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestNewEmitsJSONWithService(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, logging.Options{Level: "info", ServiceName: "genxcare-api"})

	logger.Info("started", slog.Int("port", 8080))

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("log output is not JSON: %v (%q)", err, buf.String())
	}
	if record["service"] != "genxcare-api" {
		t.Errorf("service = %v, want genxcare-api", record["service"])
	}
	if record["msg"] != "started" {
		t.Errorf("msg = %v, want started", record["msg"])
	}
}

func TestNewRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, logging.Options{Level: "warn"})

	logger.Info("should be dropped")

	if buf.Len() != 0 {
		t.Errorf("expected info record to be filtered at warn level, got %q", buf.String())
	}
}

func TestFromContext(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, logging.Options{})

	ctx := logging.WithLogger(context.Background(), logger)
	if logging.FromContext(ctx) != logger {
		t.Error("FromContext did not return the stored logger")
	}
	if logging.FromContext(context.Background()) != slog.Default() {
		t.Error("FromContext should fall back to the default logger")
	}
}
