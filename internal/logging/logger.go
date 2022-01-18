// Package logging provides a shared logger and log utilities to be used in all internal packages.
package logging

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/handlers"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/term"
)

var (
	Level                                   = zap.NewAtomicLevel()
	L, _                                    = NewLogger(Level)
	S                   *zap.SugaredLogger  = L.Sugar()
	defaultStderrWriter zapcore.WriteSyncer = os.Stderr
	defaultStdoutWriter zapcore.WriteSyncer = os.Stdout
)

func SetLevel(level string) error {
	return Level.UnmarshalText([]byte(level))
}

func NewLogger(level zapcore.LevelEnabler) (*zap.Logger, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		writer := zapcore.Lock(filtered(defaultStderrWriter))
		encoder := zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
			MessageKey: "message",

			LevelKey:    "level",
			EncodeLevel: zapcore.CapitalColorLevelEncoder,

			TimeKey:    "time",
			EncodeTime: zapcore.ISO8601TimeEncoder,

			CallerKey:    "caller",
			EncodeCaller: zapcore.ShortCallerEncoder,
		})

		return zap.New(zapcore.NewCore(encoder, writer, level), zap.AddCaller()), nil
	}

	return zap.New(
		zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
			zapcore.Lock(filtered(defaultStdoutWriter)),
			level,
		),
		zap.AddCaller(),
	), nil
}

func ZapLogFormatter(_ io.Writer, params handlers.LogFormatterParams) {
	L.Info("handled request",
		zap.String("method", params.Request.Method),
		zap.String("path", params.URL.Path),
		zap.Int("status", params.StatusCode),
		zap.Int("size", params.Size))
}

func StandardErrorLog() *log.Logger {
	errorLog, err := zap.NewStdLogAt(L, zapcore.ErrorLevel)
	if err != nil {
		return nil
	}

	return errorLog
}

// TODO: Remove the filtered writer after Go stops writing header
// values to errors, as it's cpu expensive to search every log line.
// https://github.com/golang/go/pull/48979
func filtered(logger zapcore.WriteSyncer) zapcore.WriteSyncer {
	return &filteredWriterSyncer{
		dest: logger,
	}
}

type filteredWriterSyncer struct {
	dest zapcore.WriteSyncer
}

var strInvalidHeaderFieldValue = []byte("invalid header field value")

func (w *filteredWriterSyncer) Write(b []byte) (int, error) {
	if idx := bytes.Index(b, strInvalidHeaderFieldValue); idx >= 0 {
		idx += len(strInvalidHeaderFieldValue)

		forKeyIdx := bytes.Index(b, []byte("for key"))
		if forKeyIdx > idx {
			return w.dest.Write(append(b[:idx+1], b[forKeyIdx:]...))
		}

		if b[0] != '{' {
			// not json; free to truncate.
			return w.dest.Write(b[:idx])
		}

		// we can't see where the end is. parse the message so you can truncate it. :/
		m := map[string]interface{}{}
		if err := json.Unmarshal(b, &m); err != nil {
			S.Error("Had some trouble parsing log line that needs to be filtered. Omitting log entry")
			// on error write nothing, just to be safe.
			return 0, nil // nolint
		}

		if msg, ok := m["msg"]; ok {
			if smsg, ok := msg.(string); ok {
				if idx := strings.Index(smsg, string(strInvalidHeaderFieldValue)); idx >= 0 {
					m["msg"] = smsg[:idx+len(strInvalidHeaderFieldValue)]

					newBytes, err := json.Marshal(m)
					if err == nil {
						return w.dest.Write(newBytes)
					}
				}
			}
		}
		// write nothing, just to be safe.
		return 0, nil
	}

	return w.dest.Write(b)
}

func (w *filteredWriterSyncer) Sync() error {
	return w.dest.Sync()
}

var ctxLoggerKey = "logger"

// UserAwareLoggerMiddleware saves a request-specific logger to the context
func UserAwareLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if userID := c.GetString("userID"); userID != "" {
			logger := L.With(
				zapcore.Field{
					Key:    "userID",
					Type:   zapcore.StringType,
					String: userID,
				})
			c.Set(ctxLoggerKey, logger.Sugar())
		}

		c.Next()
	}
}

// Logger gets the request-specific logger from the context
func Logger(c *gin.Context) *zap.SugaredLogger {
	if loggerInf, ok := c.Get(ctxLoggerKey); ok {
		if logger, ok := loggerInf.(*zap.SugaredLogger); ok {
			return logger
		}
	}

	return S
}
