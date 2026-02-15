package output

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	apperr "github.com/sportwhiz/gdcli/internal/errors"
)

type Envelope struct {
	Command      string           `json:"command"`
	TimestampUTC string           `json:"timestamp_utc"`
	RequestID    string           `json:"request_id"`
	Result       any              `json:"result,omitempty"`
	Error        *apperr.AppError `json:"error,omitempty"`
}

type Writer struct {
	Out io.Writer
}

func NewWriter(out io.Writer) *Writer {
	return &Writer{Out: out}
}

func (w *Writer) EmitJSON(command, reqID string, result any, err *apperr.AppError) error {
	env := Envelope{
		Command:      command,
		TimestampUTC: time.Now().UTC().Format(time.RFC3339),
		RequestID:    reqID,
		Result:       normalize(result),
		Error:        err,
	}
	enc := json.NewEncoder(w.Out)
	enc.SetEscapeHTML(false)
	return enc.Encode(env)
}

func (w *Writer) EmitNDJSON(command, reqID string, records []any) error {
	enc := json.NewEncoder(w.Out)
	enc.SetEscapeHTML(false)
	for _, r := range records {
		env := Envelope{
			Command:      command,
			TimestampUTC: time.Now().UTC().Format(time.RFC3339),
			RequestID:    reqID,
			Result:       normalize(r),
		}
		if err := enc.Encode(env); err != nil {
			return err
		}
	}
	return nil
}

func normalize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(t))
		for _, k := range keys {
			out[k] = normalize(t[k])
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = normalize(t[i])
		}
		return out
	default:
		return v
	}
}

func LogErr(errOut io.Writer, format string, args ...any) {
	fmt.Fprintf(errOut, format+"\n", args...)
}
