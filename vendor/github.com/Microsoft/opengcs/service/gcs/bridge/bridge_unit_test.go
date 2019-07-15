package bridge

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/Microsoft/opengcs/service/gcs/gcserr"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func Test_Bridge_Mux_New(t *testing.T) {
	m := NewBridgeMux()
	if m == nil {
		t.Error("Failed to create bridge mux")
	}
}

func Test_Bridge_Mux_New_Success(t *testing.T) {
	m := NewBridgeMux()
	if m.m == nil {
		t.Error("Bridge mux map is not initialized")
	}
}

type thandler struct {
	set  bool
	resp RequestResponse
	err  error
}

func (h *thandler) ServeMsg(r *Request) (RequestResponse, error) {
	h.set = true
	return h.resp, h.err
}

func TestBridgeMux_Handle_NilHandler_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("The code did not panic on nil handler")
		}
	}()

	m := NewBridgeMux()
	m.Handle(prot.ComputeSystemCreateV1, prot.PvInvalid, nil)
}

func TestBridgeMux_Handle_NilMap_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("The code did not panic on nil map")
		}
	}()

	m := &Mux{} // Caller didn't use NewBridgeMux (not supported).
	th := &thandler{}
	m.Handle(prot.ComputeSystemCreateV1, prot.PvInvalid, th)
}

func Test_Bridge_Mux_Handle_Succeeds(t *testing.T) {
	th := &thandler{}
	m := NewBridgeMux()
	m.Handle(prot.ComputeSystemCreateV1, prot.PvInvalid, th)

	var verMap map[prot.ProtocolVersion]Handler
	var ok bool
	if verMap, ok = m.m[prot.ComputeSystemCreateV1]; !ok {
		t.Error("The handler type map not successfully added.")
	}

	var hOut Handler
	if hOut, ok = verMap[prot.PvInvalid]; !ok {
		t.Error("The handler was not successfully added.")
	}

	// Is it the correct handler?
	hOut.ServeMsg(nil)

	if !th.set {
		t.Error("The handler added was not the same handler.")
	}
}

func TestBridgeMux_HandleFunc_NilHandleFunc_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("The code did not panic on nil handler")
		}
	}()

	m := NewBridgeMux()
	m.HandleFunc(prot.ComputeSystemCreateV1, prot.PvInvalid, nil)
}

func TestBridgeMux_HandleFunc_NilMap_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("The code did not panic on nil handler")
		}
	}()

	hIn := func(*Request) (RequestResponse, error) {
		return nil, nil
	}

	m := &Mux{} // Caller didn't use NewBridgeMux (not supported).
	m.HandleFunc(prot.ComputeSystemCreateV1, prot.PvInvalid, hIn)
}

func Test_Bridge_Mux_HandleFunc_Succeeds(t *testing.T) {
	var set bool
	hIn := func(*Request) (RequestResponse, error) {
		set = true
		return nil, nil
	}

	m := NewBridgeMux()
	m.HandleFunc(prot.ComputeSystemCreateV1, prot.PvInvalid, hIn)

	var verMap map[prot.ProtocolVersion]Handler
	var ok bool
	if verMap, ok = m.m[prot.ComputeSystemCreateV1]; !ok {
		t.Error("The handler type map not successfully added.")
	}

	var hOut Handler
	if hOut, ok = verMap[prot.PvInvalid]; !ok {
		t.Error("The handler was not successfully added.")
	}

	// Is it the correct handler?
	hOut.ServeMsg(nil)

	if !set {
		t.Error("The handler added was not the same handler.")
	}
}

func Test_Bridge_Mux_Handler_NilRequest_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("The code did not panic on nil request to handler")
		}
	}()

	var set bool
	hIn := func(*Request) (RequestResponse, error) {
		set = true
		return nil, nil
	}

	m := NewBridgeMux()
	m.HandleFunc(prot.ComputeSystemCreateV1, prot.PvInvalid, hIn)
	m.Handler(nil)
	if set {
		t.Fatal("should not be set on nil request")
	}
}

func verifyResponseIsDefaultHandler(t *testing.T, resp RequestResponse) {
	if resp == nil {
		t.Fatal("The response is nil")
	}

	base := resp.Base()
	if base.Result != int32(gcserr.HrNotImpl) {
		t.Fatal("The default handler did not set a -1 error result.")
	}
	if len(base.ErrorRecords) != 1 {
		t.Fatal("The default handler did not set an error record.")
	}
	if !strings.Contains(base.ErrorRecords[0].Message, "bridge: function not supported") {
		t.Fatal("The default handler did not return the not supported message")
	}
}

func Test_Bridge_Mux_Handler_NotAdded_Default(t *testing.T) {
	// Testing specifically that if we have a bridge with no handlers that
	// for the incomming request we get the default handler.

	m := NewBridgeMux()

	req := &Request{
		Header: &prot.MessageHeader{
			Type: prot.ComputeSystemCreateV1,
			Size: 0,
			ID:   prot.SequenceID(1),
		},
	}

	hOut := m.Handler(req)
	resp, err := hOut.ServeMsg(req)
	if resp != nil {
		t.Fatalf("expected nil response got: %+v", resp)
	}
	if err == nil {
		t.Fatal("expected valid error got: nil")
	}
}

func Test_Bridge_Mux_Handler_Added_NotMatched(t *testing.T) {
	// Testing specifically that if we have at least one handler of a different
	// type than the incomming request that we still get the default handler
	// and that the other handler does not get called.

	m := NewBridgeMux()
	th := &thandler{}

	// Add at least one handler for a different request type.
	m.Handle(prot.ComputeSystemCreateV1, prot.PvInvalid, th)

	req := &Request{
		Header: &prot.MessageHeader{
			Type: prot.ComputeSystemExecuteProcessV1,
			Size: 0,
			ID:   prot.SequenceID(1),
		},
	}

	// Handle the request of a different type.
	hOut := m.Handler(req)
	respChan := make(chan bridgeResponse, 1) // We need to allocate the space because we are running ServeMsg sync.
	defer close(respChan)

	resp, err := hOut.ServeMsg(req)
	if resp != nil {
		t.Fatalf("expected nil response got: %+v", resp)
	}
	if err == nil {
		t.Fatal("expected valid error got: nil")
	}
	if th.set {
		t.Error("Handler did not call the appropriate handler for a match request")
	}
}

func Test_Bridge_Mux_Handler_Success(t *testing.T) {
	m := NewBridgeMux()
	th := &thandler{
		resp: &prot.ContainerCreateResponse{},
	}

	m.Handle(prot.ComputeSystemCreateV1, prot.PvInvalid, th)

	req := &Request{
		Header: &prot.MessageHeader{
			Type: prot.ComputeSystemCreateV1,
			Size: 0,
			ID:   prot.SequenceID(1),
		},
	}

	hOut := m.Handler(req)
	respChan := make(chan bridgeResponse, 1) // We need to allocate the space because we are running ServeMsg sync.
	defer close(respChan)

	resp, err := hOut.ServeMsg(req)
	if resp == nil {
		t.Fatal("expected valid response got: nil")
	}
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}
	if !th.set {
		t.Error("Handler did not call the appropriate handler for a match request")
	}
}

func Test_Bridge_Mux_ServeMsg_NotAdded_Default(t *testing.T) {
	// Testing specifically that if we have a bridge with no handlers that
	// calling ServeMsg we get the default handler.

	m := NewBridgeMux()

	req := &Request{
		Header: &prot.MessageHeader{
			Type: prot.ComputeSystemCreateV1,
			Size: 0,
			ID:   prot.SequenceID(1),
		},
	}

	respChan := make(chan bridgeResponse, 1) // We need to allocate the space because we are running ServeMsg sync.
	defer close(respChan)

	resp, err := m.ServeMsg(req)
	if resp != nil {
		t.Fatalf("expected nil response, got: %+v", resp)
	}
	if err == nil {
		t.Fatal("expected error got: nil")
	}
}

func Test_Bridge_Mux_ServeMsg_Added_NotMatched(t *testing.T) {
	// Testing specifically that if we have at least one handler of a different
	// type than the incomming request that calling ServeMsg we get the default
	// handler.

	m := NewBridgeMux()
	th := &thandler{}

	// Add at least one handler for a different request type.
	m.Handle(prot.ComputeSystemCreateV1, prot.PvInvalid, th)

	req := &Request{
		Header: &prot.MessageHeader{
			Type: prot.ComputeSystemExecuteProcessV1,
			Size: 0,
			ID:   prot.SequenceID(1),
		},
	}

	// Handle the request of a different type.
	respChan := make(chan bridgeResponse, 1) // We need to allocate the space because we are running ServeMsg sync.
	defer close(respChan)

	resp, err := m.ServeMsg(req)
	if resp != nil {
		t.Fatalf("expected nil response, got: %+v", resp)
	}
	if err == nil {
		t.Fatal("expected error got: nil")
	}
	if th.set {
		t.Error("Handler did not call the appropriate handler for a match request")
	}
}

func Test_Bridge_Mux_ServeMsg_Success(t *testing.T) {
	m := NewBridgeMux()
	th := &thandler{
		resp: &prot.ContainerCreateResponse{},
	}

	m.Handle(prot.ComputeSystemCreateV1, prot.PvInvalid, th)

	req := &Request{
		Header: &prot.MessageHeader{
			Type: prot.ComputeSystemCreateV1,
			Size: 0,
			ID:   prot.SequenceID(1),
		},
	}

	respChan := make(chan bridgeResponse, 1) // We need to allocate the space because we are running ServeMsg sync.
	defer close(respChan)

	resp, err := m.ServeMsg(req)
	if resp == nil {
		t.Fatal("expected valid response got: nil")
	}
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}
	if !th.set {
		t.Error("Handler did not call the appropriate handler for a match request")
	}
}

type errorTransport struct {
	e error
}

func (e *errorTransport) Dial(_ uint32) (transport.Connection, error) {
	return nil, e.e
}

func serverSend(conn io.Writer, messageType prot.MessageIdentifier, messageID prot.SequenceID, i interface{}) error {
	body := make([]byte, 0)
	if i != nil {
		var err error
		body, err = json.Marshal(i)
		if err != nil {
			return errors.Wrap(err, "Failed to json marshal to server.")
		}
	}

	header := prot.MessageHeader{
		Type: messageType,
		ID:   messageID,
		Size: uint32(len(body) + prot.MessageHeaderSize),
	}

	// Send the header.
	if err := binary.Write(conn, binary.LittleEndian, header); err != nil {
		return errors.Wrap(err, "bridge_test: failed to write message header")
	}
	// Send the body.
	if _, err := conn.Write(body); err != nil {
		return errors.Wrap(err, "bridge_test: failed to write the message body")
	}
	return nil
}

func serverRead(conn io.Reader) (*prot.MessageHeader, []byte, error) {
	header := &prot.MessageHeader{}
	// Read the header.
	if err := binary.Read(conn, binary.LittleEndian, header); err != nil {
		return nil, nil, errors.Wrap(err, "bridge_test: failed to read message header")
	}
	message := make([]byte, header.Size-prot.MessageHeaderSize)
	// Read the body.
	if _, err := io.ReadFull(conn, message); err != nil {
		return nil, nil, errors.Wrap(err, "bridge_test: failed to read the message body")
	}

	return header, message, nil
}

type loopbackConnection struct {
	// Format is client-read, server-write, server-read, client-write
	pipes [4]*os.File
}

func (lc *loopbackConnection) close() {
	for i := 3; i >= 0; i-- {
		lc.pipes[i].Close()
	}
}

func (lc *loopbackConnection) CRead() io.ReadCloser {
	return lc.pipes[0]
}

func (lc *loopbackConnection) CWrite() io.WriteCloser {
	return lc.pipes[3]
}

func (lc *loopbackConnection) SRead() io.ReadCloser {
	return lc.pipes[2]
}

func (lc *loopbackConnection) SWrite() io.WriteCloser {
	return lc.pipes[1]
}

func newLoopbackConnection() *loopbackConnection {
	l := new(loopbackConnection)
	l.pipes[0], l.pipes[1], _ = os.Pipe()
	l.pipes[2], l.pipes[3], _ = os.Pipe()
	return l
}

func Test_Bridge_ListenAndServe_UnknownMessageHandler_Success(t *testing.T) {
	// Turn off logging so as not to spam output.
	logrus.SetOutput(ioutil.Discard)

	lc := newLoopbackConnection()
	defer lc.close()

	b := &Bridge{
		Handler: UnknownMessageHandler(),
	}

	go func() {
		if err := b.ListenAndServe(lc.SRead(), lc.SWrite()); err != nil {
			t.Error(err)
		}
	}()
	defer func() {
		b.quitChan <- true
	}()

	message := &prot.ContainerResizeConsole{
		MessageBase: prot.MessageBase{
			ContainerID: "01234567-89ab-cdef-0123-456789abcdef",
			ActivityID:  "00000000-0000-0000-0000-000000000001",
		},
	}
	if err := serverSend(lc.CWrite(), prot.ComputeSystemResizeConsoleV1, prot.SequenceID(1), message); err != nil {
		t.Error("Failed to send message to server")
		return
	}
	header, body, err := serverRead(lc.CRead())
	if err != nil {
		t.Error("Failed to read message response from server")
		return
	}
	response := &prot.MessageResponseBase{}
	if err := json.Unmarshal(body, response); err != nil {
		t.Error("Failed to unmarshal response body from server")
		return
	}

	// Verify
	if header.Type != prot.ComputeSystemResponseResizeConsoleV1 {
		t.Error("Response header was not resize console response.")
	}
	if header.ID != prot.SequenceID(1) {
		t.Error("Response header had wrong sequence id")
	}
	verifyResponseIsDefaultHandler(t, response)
	if response.ActivityID != message.ActivityID {
		t.Fatal("Response had invalid activity id")
	}
}

func Test_Bridge_ListenAndServe_CorrectHandler_Success(t *testing.T) {
	// Turn off logging so as not to spam output.
	logrus.SetOutput(ioutil.Discard)

	lc := newLoopbackConnection()
	defer lc.close()

	mux := NewBridgeMux()
	message := &prot.ContainerResizeConsole{
		MessageBase: prot.MessageBase{
			ContainerID: "01234567-89ab-cdef-0123-456789abcdef",
			ActivityID:  "00000000-0000-0000-0000-000000000010",
		},
	}
	resizeFn := func(r *Request) (RequestResponse, error) {
		// Verify the request is as expected.
		if r.Header.Type != prot.ComputeSystemResizeConsoleV1 {
			return nil, errors.New("bridge_test: wrong request type")
		}
		if r.Header.ID != prot.SequenceID(1) {
			return nil, errors.New("bridge_test: wrong sequence id")
		}

		rBody := prot.ContainerResizeConsole{}

		if err := json.Unmarshal(r.Message, &rBody); err != nil {
			return nil, errors.New("failed to unmarshal body")
		}
		if message.ContainerID != rBody.ContainerID {
			return nil, errors.New("containerID of source and handler func not equal")
		}

		return &prot.MessageResponseBase{
			Result:     1,
			ActivityID: rBody.ActivityID,
		}, nil
	}
	mux.HandleFunc(prot.ComputeSystemResizeConsoleV1, prot.PvV3, resizeFn)
	b := &Bridge{
		Handler: mux,
		protVer: prot.PvV3,
	}

	go func() {
		if err := b.ListenAndServe(lc.SRead(), lc.SWrite()); err != nil {
			t.Error(err)
		}
	}()
	defer func() {
		b.quitChan <- true
	}()

	if err := serverSend(lc.CWrite(), prot.ComputeSystemResizeConsoleV1, prot.SequenceID(1), message); err != nil {
		t.Error("Failed to send message to server")
		return
	}
	header, body, err := serverRead(lc.CRead())
	if err != nil {
		t.Error("Failed to read message response from server")
		return
	}
	response := &prot.MessageResponseBase{}
	if err := json.Unmarshal(body, response); err != nil {
		t.Error("Failed to unmarshal response body from server")
		return
	}
	// Verify.
	if header.Type != prot.ComputeSystemResponseResizeConsoleV1 {
		t.Error("response header was not resize console response.")
	}
	if header.ID != prot.SequenceID(1) {
		t.Error("response header had wrong sequence id")
	}
	if response.ActivityID != message.ActivityID {
		t.Error("response body did not have same activity id")
	}
	if response.Result != 1 {
		t.Error("response result was not 1 as expected")
	}
}

func Test_Bridge_ListenAndServe_HandlersAreAsync_Success(t *testing.T) {
	// Turn off logging so as not to spam output.
	logrus.SetOutput(ioutil.Discard)

	lc := newLoopbackConnection()
	defer lc.close()

	mux := NewBridgeMux()

	orderWg := sync.WaitGroup{}
	orderWg.Add(1)

	firstFn := func(r *Request) (RequestResponse, error) {
		// Wait for the second request to come in.
		orderWg.Wait()
		return &prot.MessageResponseBase{
			Result: 1,
		}, nil
	}
	secondFn := func(r *Request) (RequestResponse, error) {
		defer orderWg.Done()
		return &prot.MessageResponseBase{
			Result: 10,
		}, nil
	}
	mux.HandleFunc(prot.ComputeSystemResizeConsoleV1, prot.PvV3, firstFn)
	mux.HandleFunc(prot.ComputeSystemModifySettingsV1, prot.PvV3, secondFn)

	b := &Bridge{
		Handler: mux,
		protVer: prot.PvV3,
	}

	go func() {
		if err := b.ListenAndServe(lc.SRead(), lc.SWrite()); err != nil {
			t.Error(err)
		}
	}()
	defer func() {
		b.quitChan <- true
	}()

	if err := serverSend(lc.CWrite(), prot.ComputeSystemResizeConsoleV1, prot.SequenceID(0), nil); err != nil {
		t.Error("Failed to send first message to server")
		return
	}
	if err := serverSend(lc.CWrite(), prot.ComputeSystemModifySettingsV1, prot.SequenceID(1), nil); err != nil {
		t.Error("Failed to send second message to server")
		return
	}

	headerFirst, _, errFirst := serverRead(lc.CRead())
	if errFirst != nil {
		t.Error("Failed to read first response from server")
		return
	}
	headerSecond, _, errSecond := serverRead(lc.CRead())
	if errSecond != nil {
		t.Error("Failed to read first response from server")
		return
	}
	// headerFirst should match the 2nd request.
	if headerFirst.Type != prot.ComputeSystemResponseModifySettingsV1 {
		t.Error("Incorrect response type for 2nd request")
	}
	if headerFirst.ID != prot.SequenceID(1) {
		t.Error("Incorrect response order for 2nd request")
	}
	// headerSecond should match the 1st request.
	if headerSecond.Type != prot.ComputeSystemResponseResizeConsoleV1 {
		t.Error("Incorrect response for 1st request")
	}
	if headerSecond.ID != prot.SequenceID(0) {
		t.Error("Incorrect response order for 1st request")
	}
}
