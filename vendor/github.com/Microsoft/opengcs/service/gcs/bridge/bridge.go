// Package bridge defines the bridge struct, which implements the control loop
// and functions of the GCS's bridge client.
package bridge

import (
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

	"github.com/Microsoft/opengcs/internal/runtime/hcsv2"
	"github.com/Microsoft/opengcs/service/gcs/core"
	"github.com/Microsoft/opengcs/service/gcs/gcserr"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/stdio"
	"github.com/Microsoft/opengcs/service/libs/commonutils"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// UnknownMessage represents the default handler logic for an unmatched request
// type sent from the bridge.
func UnknownMessage(w ResponseWriter, r *Request) {
	w.Error("", gcserr.WrapHresult(errors.Errorf("bridge: function not supported, header type: %v", r.Header.Type), gcserr.HrNotImpl))
}

// UnknownMessageHandler creates a default HandlerFunc out of the
// UnknownMessage handler logic.
func UnknownMessageHandler() Handler {
	return HandlerFunc(UnknownMessage)
}

// Handler responds to a bridge request.
type Handler interface {
	ServeMsg(ResponseWriter, *Request)
}

// HandlerFunc is an adapter to use functions as handlers.
type HandlerFunc func(ResponseWriter, *Request)

// ServeMsg calls f(w, r).
func (f HandlerFunc) ServeMsg(w ResponseWriter, r *Request) {
	f(w, r)
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
func (mux *Mux) HandleFunc(id prot.MessageIdentifier, ver prot.ProtocolVersion, handler func(ResponseWriter, *Request)) {
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
func (mux *Mux) ServeMsg(w ResponseWriter, r *Request) {
	h := mux.Handler(r)
	h.ServeMsg(w, r)
}

// Request is the bridge request that has been sent.
type Request struct {
	Header  *prot.MessageHeader
	Message []byte
	Version prot.ProtocolVersion
}

// ResponseWriter is the dispatcher used to construct the Bridge response.
type ResponseWriter interface {
	// Header is the request header that was requested.
	Header() *prot.MessageHeader
	// Write a successful response message.
	Write(interface{})
	// Error writes the provided error as a response to the message correlated
	// with the activity ID passed. If the activity ID is the empty string it
	// will be translated to an empty guid.
	Error(string, error)
}

type bridgeResponse struct {
	header   *prot.MessageHeader
	response interface{}
}

type requestResponseWriter struct {
	header      *prot.MessageHeader
	respChan    chan bridgeResponse
	respWritten bool
}

func (w *requestResponseWriter) Header() *prot.MessageHeader {
	return w.header
}

func (w *requestResponseWriter) Write(r interface{}) {
	logrus.WithFields(logrus.Fields{
		"message-type": w.header.Type.String(),
		"message-id":   w.header.ID,
	}).Info("opengcs::bridge::requestResponseWriter")

	w.respChan <- bridgeResponse{header: w.header, response: r}
	w.respWritten = true
}

func (w *requestResponseWriter) Error(activityID string, err error) {
	if activityID == "" {
		activityID = "00000000-0000-0000-0000-000000000000"
	}

	resp := &prot.MessageResponseBase{ActivityID: activityID}
	setErrorForResponseBase(resp, err)
	logrus.WithFields(logrus.Fields{
		"activityID":    activityID,
		"message-type":  w.header.Type.String(),
		"message-id":    w.header.ID,
		logrus.ErrorKey: err,
	}).Error("opengcs::bridge::requestResponseWriter - error")
	w.Write(resp)
}

// Bridge defines the bridge client in the GCS. It acts in many
// ways analogous to go's `http` package and multiplexer.
//
// It has two fundamentally different dispatch options:
//
// 1. Request/Response where using the `Handler` a request
//    of a given type will be dispatched to the apprpriate handler
//    and an appropriate `ResponseWriter` will respond to exactly
//    that request that caused the dispatch.
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
				logrus.WithFields(logrus.Fields{
					"message-type": header.Type.String(),
					"message-id":   header.ID,
					"message":      string(message),
				}).Debug("opengcs::bridge - read message")
				requestChan <- &Request{header, message, b.protVer}
			}
		}
		requestErrChan <- recverr
	}()
	// Process each bridge request async and create the response writer.
	go func() {
		for req := range requestChan {
			go func(r *Request) {
				wr := &requestResponseWriter{
					header: &prot.MessageHeader{
						Type: prot.GetResponseIdentifier(r.Header.Type),
						ID:   r.Header.ID,
					},
					respChan: b.responseChan,
				}
				b.Handler.ServeMsg(wr, r)
				if !wr.respWritten {
					logrus.WithFields(logrus.Fields{
						"message-type": r.Header.Type.String(),
						"message-id":   r.Header.ID,
					}).Error("opengcs::bridge - request didn't write response")
				}
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
			logrus.WithFields(logrus.Fields{
				"message-type": resp.header.Type.String(),
				"message-id":   resp.header.ID,
				"message":      string(responseBytes),
			}).Debug("opengcs::bridge - response sent")
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
	if n == nil {
		panic("bridge: cannot publish nil notification")
	}

	logrus.WithFields(logrus.Fields{
		"activityID": n.ActivityID,
		"cid":        n.ContainerID,
		"type":       n.Type,
		"operation":  n.Operation,
		"result":     n.Result,
	}).Info("opengcs::bridge::PublishNotification")

	resp := bridgeResponse{
		header: &prot.MessageHeader{
			Type: prot.ComputeSystemNotificationV1,
			ID:   0,
		},
		response: n,
	}
	b.responseChan <- resp
}

func (b *Bridge) createContainer(w ResponseWriter, r *Request) {
	var request prot.ContainerCreate
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		w.Error("", errors.Wrapf(err, "failed to unmarshal JSON in message \"%s\"", r.Message))
		return
	}

	logrus.WithFields(logrus.Fields{
		"activityID": request.ActivityID,
		"cid":        request.ContainerID,
	}).Info("opengcs::bridge::createContainer")

	// The request contains a JSON string field which is equivalent to a
	// CreateContainerInfo struct.
	var settings prot.VMHostedContainerSettings
	if err := commonutils.UnmarshalJSONWithHresult([]byte(request.ContainerConfig), &settings); err != nil {
		w.Error(request.ActivityID, errors.Wrapf(err, "failed to unmarshal JSON for ContainerConfig \"%s\"", request.ContainerConfig))
		return
	}
	if err := b.coreint.CreateContainer(request.ContainerID, settings); err != nil {
		w.Error(request.ActivityID, err)
		return
	}

	response := &prot.ContainerCreateResponse{
		MessageResponseBase: &prot.MessageResponseBase{
			ActivityID: request.ActivityID,
		},
		SelectedProtocolVersion: uint32(prot.PvV3),
	}

	waitFn, err := b.coreint.WaitContainer(request.ContainerID)
	if err != nil {
		logrus.Error(err)
	}

	go func() {
		nt := waitFn()
		notification := &prot.ContainerNotification{
			MessageBase: &prot.MessageBase{
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
	w.Write(response)
}

func (b *Bridge) execProcess(w ResponseWriter, r *Request) {
	var request prot.ContainerExecuteProcess
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		w.Error("", errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message))
		return
	}

	logrus.WithFields(logrus.Fields{
		"activityID": request.ActivityID,
		"cid":        request.ContainerID,
	}).Info("opengcs::bridge::execProcess")

	// The request contains a JSON string field which is equivalent to an
	// ExecuteProcessInfo struct.
	var params prot.ProcessParameters
	if err := commonutils.UnmarshalJSONWithHresult([]byte(request.Settings.ProcessParameters), &params); err != nil {
		w.Error(request.ActivityID, errors.Wrapf(err, "failed to unmarshal JSON for ProcessParameters \"%s\"", request.Settings.ProcessParameters))
		return
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
		w.Error(request.ActivityID, err)
		return
	}

	response := &prot.ContainerExecuteProcessResponse{
		MessageResponseBase: &prot.MessageResponseBase{
			ActivityID: request.ActivityID,
		},
		ProcessID: uint32(pid),
	}
	w.Write(response)
}

func (b *Bridge) killContainer(w ResponseWriter, r *Request) {
	b.signalContainer(w, r, unix.SIGKILL)
}

func (b *Bridge) shutdownContainer(w ResponseWriter, r *Request) {
	b.signalContainer(w, r, unix.SIGTERM)
}

// signalContainer is not a handler func. This is because the actual signal is
// implied based on the message type.
func (b *Bridge) signalContainer(w ResponseWriter, r *Request, signal syscall.Signal) {
	var request prot.MessageBase
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		w.Error("", errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message))
		return
	}

	log := logrus.WithFields(logrus.Fields{
		"activityID": request.ActivityID,
		"cid":        request.ContainerID,
		"signal":     signal,
	})
	log.Info("opengcs::bridge::signalContainer")

	err := b.coreint.SignalContainer(request.ContainerID, signal)
	if err != nil {
		w.Error(request.ActivityID, err)
		return
	}

	response := &prot.MessageResponseBase{
		ActivityID: request.ActivityID,
	}
	w.Write(response)
}

func (b *Bridge) signalProcess(w ResponseWriter, r *Request) {
	var request prot.ContainerSignalProcess
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		w.Error("", errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message))
		return
	}

	logrus.WithFields(logrus.Fields{
		"activityID": request.ActivityID,
		"cid":        request.ContainerID,
		"pid":        request.ProcessID,
		"signal":     request.Options.Signal,
	}).Info("opengcs::bridge::signalProcess")

	if err := b.coreint.SignalProcess(int(request.ProcessID), request.Options); err != nil {
		w.Error(request.ActivityID, err)
		return
	}

	response := &prot.MessageResponseBase{
		ActivityID: request.ActivityID,
	}
	w.Write(response)
}

func (b *Bridge) getProperties(w ResponseWriter, r *Request) {
	var request prot.ContainerGetProperties
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		w.Error("", errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message))
		return
	}

	logrus.WithFields(logrus.Fields{
		"activityID": request.ActivityID,
		"cid":        request.ContainerID,
	}).Info("opengcs::bridge::getProperties")

	properties, err := b.coreint.GetProperties(request.ContainerID, request.Query)
	if err != nil {
		w.Error(request.ActivityID, err)
		return
	}

	propertyJSON := []byte("{}")
	if properties != nil {
		var err error
		propertyJSON, err = json.Marshal(properties)
		if err != nil {
			w.Error(request.ActivityID, errors.Wrapf(err, "failed to marshal properties into JSON: %v", properties))
			return
		}
	}

	response := &prot.ContainerGetPropertiesResponse{
		MessageResponseBase: &prot.MessageResponseBase{
			ActivityID: request.ActivityID,
		},
		Properties: string(propertyJSON),
	}
	w.Write(response)
}

func (b *Bridge) waitOnProcess(w ResponseWriter, r *Request) {
	var request prot.ContainerWaitForProcess
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		w.Error("", errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message))
		return
	}

	logrus.WithFields(logrus.Fields{
		"activityID": request.ActivityID,
		"cid":        request.ContainerID,
		"pid":        request.ProcessID,
		"timeout-ms": request.TimeoutInMs,
	}).Info("opengcs::bridge::waitOnProcess")

	exitCodeChan, doneChan, err := b.coreint.WaitProcess(int(request.ProcessID))
	if err != nil {
		w.Error(request.ActivityID, err)
		return
	}
	defer close(doneChan)

	select {
	case exitCode := <-exitCodeChan:
		response := &prot.ContainerWaitForProcessResponse{
			MessageResponseBase: &prot.MessageResponseBase{
				ActivityID: request.ActivityID,
			},
			ExitCode: uint32(exitCode),
		}
		w.Write(response)
	case <-time.After(time.Duration(request.TimeoutInMs) * time.Millisecond):
		w.Error(request.ActivityID, gcserr.NewHresultError(gcserr.HvVmcomputeTimeout))
	}

	// If we timed out or if we got the exit code. Acknowledge we no longer want to wait.
	doneChan <- true
}

func (b *Bridge) resizeConsole(w ResponseWriter, r *Request) {
	var request prot.ContainerResizeConsole
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		w.Error("", errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message))
		return
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
		w.Error(request.ActivityID, err)
		return
	}

	response := &prot.MessageResponseBase{
		ActivityID: request.ActivityID,
	}
	w.Write(response)
}

func (b *Bridge) modifySettings(w ResponseWriter, r *Request) {
	request, err := prot.UnmarshalContainerModifySettings(r.Message)
	if err != nil {
		aid := ""
		if request != nil {
			aid = request.ActivityID
		}
		w.Error(aid, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message))
		return
	}

	logrus.WithFields(logrus.Fields{
		"activityID": request.ActivityID,
		"cid":        request.ContainerID,
	}).Info("opengcs::bridge::modifySettings")

	err = b.coreint.ModifySettings(request.ContainerID, request.Request.(*prot.ResourceModificationRequestResponse))
	if err != nil {
		w.Error(request.ActivityID, err)
		return
	}

	response := &prot.MessageResponseBase{
		ActivityID: request.ActivityID,
	}
	w.Write(response)
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
