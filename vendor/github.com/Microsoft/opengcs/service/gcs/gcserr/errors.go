package gcserr

import (
	"fmt"
	"io"

	"github.com/pkg/errors"
)

// Hresult is a type corresponding to the HRESULT error type used on Windows.
type Hresult int32

const (
	// HrNotImpl is the HRESULT for a not implemented function.
	HrNotImpl = Hresult(-2147467263) // 0x80004001
	// HrFail is the HRESULT for an invocation failure.
	HrFail = Hresult(-2147467259) // 0x80004005
	// HrErrNotFound is the HRESULT for an invalid process id.
	HrErrNotFound = Hresult(-2147023728) // 0x80070490
	// HvVmcomputeTimeout is the HRESULT for operations that timed out.
	HvVmcomputeTimeout = Hresult(-1070137079) // 0xC0370109
	// HrVmcomputeInvalidJSON is the HRESULT for failing to unmarshal a json
	// string.
	HrVmcomputeInvalidJSON = Hresult(-1070137075) // 0xC037010D
	// HrVmcomputeSystemNotFound is the HRESULT for:
	//
	// A virtual machine or container with the specified identifier does not
	// exist.
	HrVmcomputeSystemNotFound = Hresult(-1070137074) // 0xC037010E
	// HrVmcomputeSystemAlreadyExists is the HRESULT for:
	//
	// A virtual machine or container with the specified identifier already exists.
	HrVmcomputeSystemAlreadyExists = Hresult(-1070137073) // 0xC037010F
	// HrVmcomputeUnsupportedProtocolVersion is the HRESULT for an invalid
	// protocol version range specified at negotiation.
	HrVmcomputeUnsupportedProtocolVersion = Hresult(-1070137076) // 0xC037010C
	// HrVmcomputeUnknownMessage is the HRESULT for unknown message types sent
	// from the HCS.
	HrVmcomputeUnknownMessage = Hresult(-1070137077) // 0xC037010B
)

// StackTracer is an interface originating (but not exported) from the
// github.com/pkg/errors package. It defines something which can return a stack
// trace.
type StackTracer interface {
	StackTrace() errors.StackTrace
}

// BaseStackTrace gets the earliest errors.StackTrace in the given error's cause
// stack. This will be the stack trace which reaches closest to the error's
// actual origin. It returns nil if no stack trace is found in the cause stack.
func BaseStackTrace(e error) errors.StackTrace {
	type causer interface {
		Cause() error
	}
	cause := e
	var tracer StackTracer
	for cause != nil {
		serr, ok := cause.(StackTracer)
		if ok {
			tracer = serr
		}
		cerr, ok := cause.(causer)
		if !ok {
			break
		}
		cause = cerr.Cause()
	}
	if tracer == nil {
		return nil
	}
	return tracer.StackTrace()
}

type baseHresultError struct {
	hresult Hresult
}

func (e *baseHresultError) Error() string {
	return fmt.Sprintf("HRESULT: 0x%x", uint32(e.Hresult()))
}
func (e *baseHresultError) Hresult() Hresult {
	return e.hresult
}

type wrappingHresultError struct {
	cause   error
	hresult Hresult
}

func (e *wrappingHresultError) Error() string {
	return fmt.Sprintf("HRESULT 0x%x", uint32(e.Hresult())) + ": " + e.Cause().Error()
}
func (e *wrappingHresultError) Hresult() Hresult {
	return e.hresult
}
func (e *wrappingHresultError) Cause() error {
	return e.cause
}
func (e *wrappingHresultError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			fmt.Fprintf(s, "%+v\n", e.Cause())
			return
		}
		fallthrough
	case 's':
		io.WriteString(s, e.Error())
	case 'q':
		fmt.Fprintf(s, "%q", e.Error())
	}
}
func (e *wrappingHresultError) StackTrace() errors.StackTrace {
	type stackTracer interface {
		StackTrace() errors.StackTrace
	}
	serr, ok := e.Cause().(stackTracer)
	if !ok {
		return nil
	}
	return serr.StackTrace()
}

// NewHresultError produces a new error with the given HRESULT.
func NewHresultError(hresult Hresult) error {
	return &baseHresultError{hresult: hresult}
}

// WrapHresult produces a new error with the given HRESULT and wrapping the
// given error.
func WrapHresult(e error, hresult Hresult) error {
	return &wrappingHresultError{
		cause:   e,
		hresult: hresult,
	}
}

// GetHresult interates through the error's cause stack (similiarly to how the
// Cause function in github.com/pkg/errors operates). At the first error it
// encounters which implements the Hresult() method, it return's that error's
// HRESULT. This allows errors higher up in the cause stack to shadow the
// HRESULTs of errors lower down.
func GetHresult(e error) (Hresult, error) {
	type hresulter interface {
		Hresult() Hresult
	}
	type causer interface {
		Cause() error
	}
	cause := e
	for cause != nil {
		herr, ok := cause.(hresulter)
		if ok {
			return herr.Hresult(), nil
		}
		cerr, ok := cause.(causer)
		if !ok {
			break
		}
		cause = cerr.Cause()
	}
	return -1, errors.Errorf("no HRESULT found in cause stack for error %s", e)
}
