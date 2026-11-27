package vercel

import (
	"errors"
	"fmt"
	"io"

	"github.com/fxamacker/cbor/v2"
)

// cborStreamType asks for a CBOR Sequence of [TYPE, PAYLOAD] items, which keeps
// the output bytes of a command exact.
const cborStreamType = "application/vnd.vercel.sandbox.command-stream+cbor-seq"

const (
	itemStdout  = 1
	itemStderr  = 2
	itemFailure = 3
	itemCommand = 128
)

type cborStream struct {
	body    io.ReadCloser
	dec     *cbor.Decoder
	started bool
	done    bool
}

func newCBORStream(body io.ReadCloser) *cborStream {
	return &cborStream{body: body, dec: cbor.NewDecoder(body)}
}

func (s *cborStream) Close() error { return s.body.Close() }

type commandPayload struct {
	ID       string `json:"id"`
	ExitCode *int   `json:"exitCode"`
}

// Next returns the next event. A stream that ends before the command did is
// io.ErrUnexpectedEOF.
func (s *cborStream) Next() (Event, error) {
	if s.done {
		return Event{}, io.EOF
	}
	var item []cbor.RawMessage
	if err := s.dec.Decode(&item); err != nil {
		if errors.Is(err, io.EOF) {
			return Event{}, io.ErrUnexpectedEOF
		}
		return Event{}, fmt.Errorf("decode command stream: %w", err)
	}
	if len(item) != 2 {
		return Event{}, fmt.Errorf("decode command stream: item has %d fields", len(item))
	}
	var kind int
	if err := cbor.Unmarshal(item[0], &kind); err != nil {
		return Event{}, fmt.Errorf("decode command stream: %w", err)
	}
	return s.event(kind, item[1])
}

func (s *cborStream) event(kind int, payload cbor.RawMessage) (Event, error) {
	switch kind {
	case itemStdout, itemStderr:
		var data []byte
		if err := cbor.Unmarshal(payload, &data); err != nil {
			return Event{}, fmt.Errorf("decode command output: %w", err)
		}
		if kind == itemStderr {
			return Event{Kind: EventStderr, Data: data}, nil
		}
		return Event{Kind: EventStdout, Data: data}, nil
	case itemFailure:
		var failure struct {
			Message string `json:"message"`
		}
		if err := cbor.Unmarshal(payload, &failure); err != nil {
			return Event{}, fmt.Errorf("decode command failure: %w", err)
		}
		return Event{Kind: EventError, Err: failure.Message}, nil
	case itemCommand:
		var cmd commandPayload
		if err := cbor.Unmarshal(payload, &cmd); err != nil {
			return Event{}, fmt.Errorf("decode command status: %w", err)
		}
		if cmd.ExitCode == nil && !s.started {
			s.started = true
			return Event{Kind: EventStarted, CmdID: cmd.ID}, nil
		}
		s.done = true
		return Event{Kind: EventEnd, CmdID: cmd.ID, ExitCode: cmd.ExitCode}, nil
	}
	// A type this version does not know is skipped, not an error.
	return Event{}, nil
}
