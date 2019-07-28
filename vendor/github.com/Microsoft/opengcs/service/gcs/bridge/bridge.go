// Package bridge defines the bridge struct, which implements the control loop
// and functions of the GCS's bridge client.
package bridge

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Microsoft/opengcs/internal/log"
	"github.com/Microsoft/opengcs/internal/oc"
	"github.com/Microsoft/opengcs/internal/runtime/hcsv2"
	"github.com/Microsoft/opengcs/service/gcs/core"
	"github.com/Microsoft/opengcs/service/gcs/gcserr"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/stdio"
	"github.com/Microsoft/opengcs/service/libs/commonutils"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
	"golang.org/x/sys/unix"
)

// UnknownMessage represents the default handler logic for an unmatched request
// type sent from the bridge.
func UnknownMessage(r *Request) (RequestResponse, error) {
	return nil, gcserr.WrapHresult(errors.Errorf("bridge: function not supported, header type: %v", r.Header.Type), gcserr.HrNotImpl)
}

// UnknownMessageHandler creates a default HandlerFunc out of the
// UnknownMessage handler logic.
func UnknownMessageHandler() Handler {
	return HandlerFunc(UnknownMessage)
}

// Handler responds to a bridge request.
type Handler interface {
	ServeMsg(*Request) (RequestResponse, error)
}

// HandlerFunc is an adapter to use functions as handlers.
type HandlerFunc func(*Request) (RequestResponse, error)

// ServeMsg calls f(w, r).
func (f HandlerFunc) ServeMsg(r *Request) (RequestResponse, error) {
	return f(r)
}

// Mux is a protocol multiplexer for request response pairs
// following the bridge protocol.
type Mux struct {
	mu sync.Mutex
	m  map[prot.MessageIdentifier]map[prot.ProtocolVersion]Handler
}

// NewBridgeMux creates a default bridge multiplexer.
func NewBridgeMux() *Mux {
	return &Mux{m: make(map[prot.MessageIdentifier]map[prot.ProtocolVersion]Handler)}
}

// Handle registers the handler for the given message id and protocol version.
func (mux *Mux) Handle(id prot.MessageIdentifier, ver prot.ProtocolVersion, handler Handler) {
	mux.mu.Lock()
	defer mux.mu.Unlock()

	if handler == nil {
		panic("bridge: nil handler")
	}

	if _, ok := mux.m[id]; !ok {
		mux.m[id] = make(map[prot.ProtocolVersion]Handler)
	}

	if _, ok := mux.m[id][ver]; ok {
		logrus.WithFields(logrus.Fields{
			"message-type":     id.String(),
			"protocol-version": ver,
		}).Warn("opengcs::bridge - overwriting bridge handler")
	}

	mux.m[id][ver] = handler
}

// HandleFunc registers the handler function for the given message id and protocol version.
func (mux *Mux) HandleFunc(id prot.MessageIdentifier, ver prot.ProtocolVersion, handler func(*Request) (RequestResponse, error)) {
	if handler == nil {
		panic("bridge: nil handler func")
	}

	mux.Handle(id, ver, HandlerFunc(handler))
}

// Handler returns the handler to use for the given request type.
func (mux *Mux) Handler(r *Request) Handler {
	mux.mu.Lock()
	defer mux.mu.Unlock()

	if r == nil {
		panic("bridge: nil request to handler")
	}

	var m map[prot.ProtocolVersion]Handler
	var ok bool
	if m, ok = mux.m[r.Header.Type]; !ok {
		return UnknownMessageHandler()
	}

	var h Handler
	if h, ok = m[r.Version]; !ok {
		return UnknownMessageHandler()
	}

	return h
}

// ServeMsg dispatches the request to the handler whose
// type matches the request type.
func (mux *Mux) ServeMsg(r *Request) (RequestResponse, error) {
	h := mux.Handler(r)
	return h.ServeMsg(r)
}

// Request is the bridge request that has been sent.
type Request struct {
	// Context is the request context received from the bridge.
	Context context.Context
	// Header is the wire format message header that preceeded the message for
	// this request.
	Header *prot.MessageHeader
	// ContainerID is the id of the container that this message cooresponds to.
	ContainerID string
	// ActivityID is the id of the specific activity for this request.
	ActivityID string
	// Message is the portion of the request that follows the `Header`. This is
	// a json encoded string that MUST contain `prot.MessageBase`.
	Message []byte
	// Version is the version of the protocol that `Header` and `Message` were
	// sent in.
	Version prot.ProtocolVersion
}

// RequestResponse is the base response for any bridge message request.
type RequestResponse interface {
	Base() *prot.MessageResponseBase
}

type bridgeResponse struct {
	// ctx is the context created on request read
	ctx      context.Context
	header   *prot.MessageHeader
	response interface{}
}

// Bridge defines the bridge client in the GCS. It acts in many ways analogous
// to go's `http` package and multiplexer.
//
// It has two fundamentally different dispatch options:
//
// 1. Request/Response where using the `Handler` a request
//    of a given type will be dispatched to the apprpriate handler
//    and an appropriate response will respond to exactly that request that
//    caused the dispatch.
//
// 2. `PublishNotification` where a notification that was not initiated
//    by a request from any client can be written to the bridge at any time
//    in any order.
type Bridge struct {
	// Handler to invoke when messages are received.
	Handler Handler
	// EnableV4 enables the v4+ bridge and the schema v2+ interfaces.
	EnableV4 bool

	// responseChan is the response channel used for both request/response
	// and publish notification workflows.
	responseChan chan bridgeResponse

	coreint   core.Core
	hostState *hcsv2.Host

	quitChan chan bool
	// hasQuitPending when != 0 will cause no more requests to be Read.
	hasQuitPending uint32

	protVer prot.ProtocolVersion
}

// AssignHandlers creates and assigns the appropriate bridge
// events to be listen for and intercepted on `mux` before forwarding
// to `gcs` for handling.
func (b *Bridge) AssignHandlers(mux *Mux, gcs core.Core, host *hcsv2.Host) {
	b.coreint = gcs
	b.hostState = host

	// These are PvInvalid because they will be called previous to any protocol
	// negotiation so they respond only when the protocols are not known.
	if b.EnableV4 {
		mux.HandleFunc(prot.ComputeSystemNegotiateProtocolV1, prot.PvInvalid, b.negotiateProtocolV2)
	} else {
		mux.HandleFunc(prot.ComputeSystemCreateV1, prot.PvInvalid, b.createContainer)
	}

	// v3 specific handlers
	mux.HandleFunc(prot.ComputeSystemExecuteProcessV1, prot.PvV3, b.execProcess)
	mux.HandleFunc(prot.ComputeSystemShutdownForcedV1, prot.PvV3, b.killContainer)
	mux.HandleFunc(prot.ComputeSystemShutdownGracefulV1, prot.PvV3, b.shutdownContainer)
	mux.HandleFunc(prot.ComputeSystemSignalProcessV1, prot.PvV3, b.signalProcess)
	mux.HandleFunc(prot.ComputeSystemGetPropertiesV1, prot.PvV3, b.getProperties)
	mux.HandleFunc(prot.ComputeSystemWaitForProcessV1, prot.PvV3, b.waitOnProcess)
	mux.HandleFunc(prot.ComputeSystemResizeConsoleV1, prot.PvV3, b.resizeConsole)
	mux.HandleFunc(prot.ComputeSystemModifySettingsV1, prot.PvV3, b.modifySettings)

	if b.EnableV4 {
		// v4 specific handlers
		mux.HandleFunc(prot.ComputeSystemStartV1, prot.PvV4, b.startContainerV2)
		mux.HandleFunc(prot.ComputeSystemCreateV1, prot.PvV4, b.createContainerV2)
		mux.HandleFunc(prot.ComputeSystemExecuteProcessV1, prot.PvV4, b.execProcessV2)
		mux.HandleFunc(prot.ComputeSystemShutdownForcedV1, prot.PvV4, b.killContainerV2)
		mux.HandleFunc(prot.ComputeSystemShutdownGracefulV1, prot.PvV4, b.shutdownContainerV2)
		mux.HandleFunc(prot.ComputeSystemSignalProcessV1, prot.PvV4, b.signalProcessV2)
		mux.HandleFunc(prot.ComputeSystemGetPropertiesV1, prot.PvV4, b.getPropertiesV2)
		mux.HandleFunc(prot.ComputeSystemWaitForProcessV1, prot.PvV4, b.waitOnProcessV2)
		mux.HandleFunc(prot.ComputeSystemResizeConsoleV1, prot.PvV4, b.resizeConsoleV2)
		mux.HandleFunc(prot.ComputeSystemModifySettingsV1, prot.PvV4, b.modifySettingsV2)
	}
}

// ListenAndServe connects to the bridge transport, listens for
// messages and dispatches the appropriate handlers to handle each
// event in an asynchronous manner.
func (b *Bridge) ListenAndServe(bridgeIn io.ReadCloser, bridgeOut io.WriteCloser) error {
	requestChan := make(chan *Request)
	requestErrChan := make(chan error)
	b.responseChan = make(chan bridgeResponse)
	responseErrChan := make(chan error)
	b.quitChan = make(chan bool)

	defer close(b.quitChan)
	defer bridgeOut.Close()
	defer close(responseErrChan)
	defer close(b.responseChan)
	defer close(requestChan)
	defer close(requestErrChan)
	defer bridgeIn.Close()

	// Receive bridge requests and schedule them to be processed.
	go func() {
		var recverr error
		for {
			if atomic.LoadUint32(&b.hasQuitPending) == 0 {
				header := &prot.MessageHeader{}
				if err := binary.Read(bridgeIn, binary.LittleEndian, header); err != nil {
					if err == io.ErrUnexpectedEOF || err == os.ErrClosed {
						break
					}
					recverr = errors.Wrap(err, "bridge: failed reading message header")
					break
				}
				message := make([]byte, header.Size-prot.MessageHeaderSize)
				if _, err := io.ReadFull(bridgeIn, message); err != nil {
					if err == io.ErrUnexpectedEOF || err == os.ErrClosed {
						break
					}
					recverr = errors.Wrap(err, "bridge: failed reading message payload")
					break
				}

				base := prot.MessageBase{}
				if err := json.Unmarshal(message, &base); err != nil {
					// TODO: JTERRY75 - This should fail the request but right
					// now we still forward to the method and let them return
					// this error. Unify the JSON part previous to invoking a
					// request.
				}

				ctx, span := trace.StartSpan(context.Background(), "opengcs::bridge::request")
				span.AddAttributes(
					trace.Int64Attribute("message-id", int64(header.ID)),
					trace.StringAttribute("message-type", header.Type.String()),
					trace.StringAttribute("activityID", base.ActivityID),
					trace.StringAttribute("cid", base.ContainerID))

				log.G(ctx).WithField("message", string(message)).Debug("request read message")

				requestChan <- &Request{
					Context:     ctx,
					Header:      header,
					ContainerID: base.ContainerID,
					ActivityID:  base.ActivityID,
					Message:     message,
					Version:     b.protVer,
				}
			}
		}
		requestErrChan <- recverr
	}()
	// Process each bridge request async and create the response writer.
	go func() {
		for req := range requestChan {
			go func(r *Request) {
				br := bridgeResponse{
					ctx: r.Context,
					header: &prot.MessageHeader{
						Type: prot.GetResponseIdentifier(r.Header.Type),
						ID:   r.Header.ID,
					},
				}
				resp, err := b.Handler.ServeMsg(r)
				if resp == nil {
					resp = &prot.MessageResponseBase{}
				}
				resp.Base().ActivityID = r.ActivityID
				if err != nil {
					span := trace.FromContext(r.Context)
					if span != nil {
						oc.SetSpanStatus(span, err)
					}
					setErrorForResponseBase(resp.Base(), err)
				}
				br.response = resp
				b.responseChan <- br
			}(req)
		}
	}()
	// Process each bridge response sync. This channel is for request/response and publish workflows.
	go func() {
		var resperr error
		for resp := range b.responseChan {
			responseBytes, err := json.Marshal(resp.response)
			if err != nil {
				resperr = errors.Wrapf(err, "bridge: failed to marshal JSON for response \"%v\"", resp.response)
				break
			}
			resp.header.Size = uint32(len(responseBytes) + prot.MessageHeaderSize)
			if err := binary.Write(bridgeOut, binary.LittleEndian, resp.header); err != nil {
				resperr = errors.Wrap(err, "bridge: failed writing message header")
				break
			}

			if _, err := bridgeOut.Write(responseBytes); err != nil {
				resperr = errors.Wrap(err, "bridge: failed writing message payload")
				break
			}

			s := trace.FromContext(resp.ctx)
			if s != nil {
				log.G(resp.ctx).WithField("message", string(responseBytes)).Debug("request write response")
				s.End()
			}
		}
		responseErrChan <- resperr
	}()

	select {
	case err := <-requestErrChan:
		return err
	case err := <-responseErrChan:
		return err
	case <-b.quitChan:
		// The request loop needs to exit so that the teardown process begins.
		// Set the request loop to stop processing new messages
		atomic.StoreUint32(&b.hasQuitPending, 1)
		// Wait for the request loop to process its last message. Its possible
		// that if it lost the race with the hasQuitPending it could be stuck in
		// a pending read from bridgeIn. Wait 2 seconds and kill the connection.
		var err error
		select {
		case err = <-requestErrChan:
		case <-time.After(time.Second * 5):
			// Timeout expired first. Close the connection to unblock the read
			if cerr := bridgeIn.Close(); cerr != nil {
				err = errors.Wrap(cerr, "bridge: failed to close bridgeIn")
			}
			<-requestErrChan
		}
		<-responseErrChan
		return err
	}
}

// PublishNotification writes a specific notification to the bridge.
func (b *Bridge) PublishNotification(n *prot.ContainerNotification) {
	ctx, span := trace.StartSpan(context.Background(), "opengcs::bridge::PublishNotification")
	span.AddAttributes(trace.StringAttribute("notification", fmt.Sprintf("%+v", n)))
	// DONT defer span.End() here. Publish is odd because bridgeResponse calls
	// `End` on the `ctx` after the response is sent.

	resp := bridgeResponse{
		ctx: ctx,
		header: &prot.MessageHeader{
			Type: prot.ComputeSystemNotificationV1,
			ID:   0,
		},
		response: n,
	}
	b.responseChan <- resp
}

func (b *Bridge) createContainer(r *Request) (RequestResponse, error) {
	var request prot.ContainerCreate
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON in message \"%s\"", r.Message)
	}

	logrus.WithFields(logrus.Fields{
		"activityID": request.ActivityID,
		"cid":        request.ContainerID,
	}).Info("opengcs::bridge::createContainer")

	// The request contains a JSON string field which is equivalent to a
	// CreateContainerInfo struct.
	var settings prot.VMHostedContainerSettings
	if err := commonutils.UnmarshalJSONWithHresult([]byte(request.ContainerConfig), &settings); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON for ContainerConfig \"%s\"", request.ContainerConfig)
	}
	if err := b.coreint.CreateContainer(request.ContainerID, settings); err != nil {
		return nil, err
	}

	response := &prot.ContainerCreateResponse{
		SelectedProtocolVersion: uint32(prot.PvV3),
	}

	waitFn, err := b.coreint.WaitContainer(request.ContainerID)
	if err != nil {
		logrus.Error(err)
	}

	go func() {
		nt := waitFn()
		notification := &prot.ContainerNotification{
			MessageBase: prot.MessageBase{
				ContainerID: request.ContainerID,
				ActivityID:  request.ActivityID,
			},
			Type:       nt,
			Operation:  prot.AoNone,
			Result:     0,
			ResultInfo: "",
		}
		b.PublishNotification(notification)
	}()

	// Set our protocol selected version before return.
	b.protVer = prot.PvV3
	return response, nil
}

func (b *Bridge) execProcess(r *Request) (RequestResponse, error) {
	var request prot.ContainerExecuteProcess
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message)
	}

	logrus.WithFields(logrus.Fields{
		"activityID": request.ActivityID,
		"cid":        request.ContainerID,
	}).Info("opengcs::bridge::execProcess")

	// The request contains a JSON string field which is equivalent to an
	// ExecuteProcessInfo struct.
	var params prot.ProcessParameters
	if err := commonutils.UnmarshalJSONWithHresult([]byte(request.Settings.ProcessParameters), &params); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON for ProcessParameters \"%s\"", request.Settings.ProcessParameters)
	}

	var conSettings stdio.ConnectionSettings
	if params.CreateStdInPipe {
		conSettings.StdIn = &request.Settings.VsockStdioRelaySettings.StdIn
	}
	if params.CreateStdOutPipe {
		conSettings.StdOut = &request.Settings.VsockStdioRelaySettings.StdOut
	}
	if params.CreateStdErrPipe {
		conSettings.StdErr = &request.Settings.VsockStdioRelaySettings.StdErr
	}

	var pid int
	// If this is the exec of the init process for V1 we need to return the
	// error of the exec previous to any ContainerExited notification. So we
	// signal when we are done writing to the bridge.
	var execInitErrorDone chan<- struct{}

	defer func() {
		if execInitErrorDone != nil {
			execInitErrorDone <- struct{}{}
		}
	}()
	var err error
	if params.IsExternal {
		pid, err = b.coreint.RunExternalProcess(params, conSettings)
	} else {
		pid, execInitErrorDone, err = b.coreint.ExecProcess(request.ContainerID, params, conSettings)
	}

	if err != nil {
		return nil, err
	}

	return &prot.ContainerExecuteProcessResponse{
		ProcessID: uint32(pid),
	}, nil
}

func (b *Bridge) killContainer(r *Request) (RequestResponse, error) {
	return b.signalContainer(r, unix.SIGKILL)
}

func (b *Bridge) shutdownContainer(r *Request) (RequestResponse, error) {
	return b.signalContainer(r, unix.SIGTERM)
}

// signalContainer is not a handler func. This is because the actual signal is
// implied based on the message type.
func (b *Bridge) signalContainer(r *Request, signal syscall.Signal) (RequestResponse, error) {
	var request prot.MessageBase
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message)
	}

	logrus.WithFields(logrus.Fields{
		"activityID": request.ActivityID,
		"cid":        request.ContainerID,
		"signal":     signal,
	}).Info("opengcs::bridge::signalContainer")

	err := b.coreint.SignalContainer(request.ContainerID, signal)
	if err != nil {
		return nil, err
	}

	return &prot.MessageResponseBase{}, nil
}

func (b *Bridge) signalProcess(r *Request) (RequestResponse, error) {
	var request prot.ContainerSignalProcess
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message)
	}

	logrus.WithFields(logrus.Fields{
		"activityID": request.ActivityID,
		"cid":        request.ContainerID,
		"pid":        request.ProcessID,
		"signal":     request.Options.Signal,
	}).Info("opengcs::bridge::signalProcess")

	if err := b.coreint.SignalProcess(int(request.ProcessID), request.Options); err != nil {
		return nil, err
	}

	return &prot.MessageResponseBase{}, nil
}

func (b *Bridge) getProperties(r *Request) (RequestResponse, error) {
	var request prot.ContainerGetProperties
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message)
	}

	logrus.WithFields(logrus.Fields{
		"activityID": request.ActivityID,
		"cid":        request.ContainerID,
	}).Info("opengcs::bridge::getProperties")

	properties, err := b.coreint.GetProperties(request.ContainerID, request.Query)
	if err != nil {
		return nil, err
	}

	propertyJSON := []byte("{}")
	if properties != nil {
		var err error
		propertyJSON, err = json.Marshal(properties)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to marshal properties into JSON: %v", properties)
		}
	}

	return &prot.ContainerGetPropertiesResponse{
		Properties: string(propertyJSON),
	}, nil
}

func (b *Bridge) waitOnProcess(r *Request) (RequestResponse, error) {
	var request prot.ContainerWaitForProcess
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message)
	}

	logrus.WithFields(logrus.Fields{
		"activityID": request.ActivityID,
		"cid":        request.ContainerID,
		"pid":        request.ProcessID,
		"timeout-ms": request.TimeoutInMs,
	}).Info("opengcs::bridge::waitOnProcess")

	exitCodeChan, doneChan, err := b.coreint.WaitProcess(int(request.ProcessID))
	if err != nil {
		return nil, err
	}

	// If we timed out or if we got the exit code. Acknowledge we no longer want to wait.
	defer close(doneChan)

	select {
	case exitCode := <-exitCodeChan:
		return &prot.ContainerWaitForProcessResponse{
			ExitCode: uint32(exitCode),
		}, nil
	case <-time.After(time.Duration(request.TimeoutInMs) * time.Millisecond):
		return nil, gcserr.NewHresultError(gcserr.HvVmcomputeTimeout)
	}
}

func (b *Bridge) resizeConsole(r *Request) (RequestResponse, error) {
	var request prot.ContainerResizeConsole
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message)
	}

	logrus.WithFields(logrus.Fields{
		"activityID": request.ActivityID,
		"cid":        request.ContainerID,
		"pid":        request.ProcessID,
		"height":     request.Height,
		"width":      request.Width,
	}).Info("opengcs::bridge::resizeConsole")

	err := b.coreint.ResizeConsole(int(request.ProcessID), request.Height, request.Width)
	if err != nil {
		return nil, err
	}

	return &prot.MessageResponseBase{
		ActivityID: request.ActivityID,
	}, nil
}

func (b *Bridge) modifySettings(r *Request) (RequestResponse, error) {
	request, err := prot.UnmarshalContainerModifySettings(r.Message)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message)
	}

	logrus.WithFields(logrus.Fields{
		"activityID": request.ActivityID,
		"cid":        request.ContainerID,
	}).Info("opengcs::bridge::modifySettings")

	err = b.coreint.ModifySettings(request.ContainerID, request.Request.(*prot.ResourceModificationRequestResponse))
	if err != nil {
		return nil, err
	}

	return &prot.MessageResponseBase{}, nil
}

// setErrorForResponseBase modifies the passed-in MessageResponseBase to
// contain information pertaining to the given error.
func setErrorForResponseBase(response *prot.MessageResponseBase, errForResponse error) {
	errorMessage := errForResponse.Error()
	stackString := ""
	fileName := ""
	lineNumber := -1
	functionName := ""
	if stack := gcserr.BaseStackTrace(errForResponse); stack != nil {
		bottomFrame := stack[0]
		stackString = fmt.Sprintf("%+v", stack)
		fileName = fmt.Sprintf("%s", bottomFrame)
		lineNumberStr := fmt.Sprintf("%d", bottomFrame)
		var err error
		lineNumber, err = strconv.Atoi(lineNumberStr)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"line-number":   lineNumberStr,
				logrus.ErrorKey: err,
			}).Error("opengcs::bridge::setErrorForResponseBase - failed to parse line number, using -1 instead")
			lineNumber = -1
		}
		functionName = fmt.Sprintf("%n", bottomFrame)
	}
	hresult, err := gcserr.GetHresult(errForResponse)
	if err != nil {
		// Default to using the generic failure HRESULT.
		hresult = gcserr.HrFail
	}
	response.Result = int32(hresult)
	response.ErrorMessage = errorMessage
	newRecord := prot.ErrorRecord{
		Result:       int32(hresult),
		Message:      errorMessage,
		StackTrace:   stackString,
		ModuleName:   "gcs",
		FileName:     fileName,
		Line:         uint32(lineNumber),
		FunctionName: functionName,
	}
	response.ErrorRecords = append(response.ErrorRecords, newRecord)
}
