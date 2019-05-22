package bridge

import (
	"encoding/json"
	"io/ioutil"
	"math/rand"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Microsoft/opengcs/internal/runtime/hcsv2"
	"github.com/Microsoft/opengcs/service/gcs/core/mockcore"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

type testResponseWriter struct {
	header         *prot.MessageHeader
	response       interface{}
	err            error
	errActivityID  string
	respWriteCount int
}

func (w *testResponseWriter) Header() *prot.MessageHeader {
	return w.header
}

func (w *testResponseWriter) Write(r interface{}) {
	w.response = r
	w.respWriteCount++
}

func (w *testResponseWriter) Error(activityID string, err error) {
	if activityID == "" {
		activityID = "00000000-0000-0000-0000-000000000000"
	}

	w.errActivityID = activityID
	w.err = err
	w.respWriteCount++
}

func createRequest(t *testing.T, id prot.MessageIdentifier, ver prot.ProtocolVersion, message interface{}) *Request {
	r := &Request{}

	bytes := make([]byte, 0)
	if message != nil {
		var err error
		bytes, err = json.Marshal(message)
		if err != nil {
			t.Fatalf("failed to marshal message for request: (%s)", err)
		}
	}
	hdr := &prot.MessageHeader{
		Type: id,
		Size: uint32(prot.MessageHeaderSize + len(bytes)),
		ID:   0,
	}

	r.Header = hdr
	r.Message = bytes
	r.Version = ver
	return r
}

func createResponseWriter(r *Request) *testResponseWriter {
	hdr := &prot.MessageHeader{
		Type: prot.GetResponseIdentifier(r.Header.Type),
		ID:   r.Header.ID,
	}

	return &testResponseWriter{header: hdr}
}

func setupRequestResponse(t *testing.T, id prot.MessageIdentifier, ver prot.ProtocolVersion, message interface{}) (*Request, *testResponseWriter) {
	r := createRequest(t, id, ver, message)
	rw := createResponseWriter(r)
	return r, rw
}

func verifyResponseWriteCount(t *testing.T, rw *testResponseWriter) {
	if rw.respWriteCount != 1 {
		t.Fatalf("response was written (%d) times != 1", rw.respWriteCount)
	}
}

func verifyResponseError(t *testing.T, rw *testResponseWriter) {
	verifyResponseWriteCount(t, rw)
	if rw.response != nil {
		t.Fatal("response wrote a response not an error")
	}
	if rw.err == nil {
		t.Fatal("response did not write an error")
	}
}

func verifyResponseJSONError(t *testing.T, rw *testResponseWriter) {
	verifyResponseError(t, rw)
	if !strings.Contains(rw.err.Error(), "failed to unmarshal JSON") {
		t.Fatal("response error was not a json marshal error")
	}
}

func verifyResponseSuccess(t *testing.T, rw *testResponseWriter) {
	verifyResponseWriteCount(t, rw)
	if rw.err != nil {
		t.Fatalf("response was an error response: %v", rw.err)
	}
	if rw.response == nil {
		t.Fatal("response was a success but no message was included")
	}
}

func verifyActivityIDEmptyGUID(t *testing.T, rw *testResponseWriter) {
	if rw.err == nil {
		t.Fatal("we only expect an empty activity ID on error cases")
	}

	if "00000000-0000-0000-0000-000000000000" != rw.errActivityID {
		t.Fatalf("response activity ID (%s) was not equal to the empty guid '00000000-0000-0000-0000-000000000000'", rw.errActivityID)
	}
}

func verifyActivityID(t *testing.T, req *prot.MessageBase, rw *testResponseWriter) {
	var respActivityID string
	if rw.err != nil {
		respActivityID = rw.errActivityID
	} else {
		rwv := reflect.ValueOf(rw.response)
		respActivityID = rwv.Elem().FieldByName("ActivityID").String()
	}

	if req.ActivityID != respActivityID {
		t.Fatalf("response activity ID (%s) was not equal to request (%s) for 'Error' case", req.ActivityID, rw.errActivityID)
	}
}

func newMessageBase() *prot.MessageBase {
	const chars = "abcdefghijklmnopqrstuvwxyz"
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	f := func() string {
		b := make([]byte, 10)
		for i := 0; i < len(b); i++ {
			b[i] = chars[r.Intn(len(chars))]
		}
		return string(b)
	}

	base := &prot.MessageBase{
		ContainerID: f(),
		ActivityID:  f(),
	}
	return base
}

func newMessageUVMBase() *prot.MessageBase {
	b := newMessageBase()
	b.ContainerID = hcsv2.UVMContainerID
	return b
}

func Test_NegotiateProtocol_DuplicateCall_Failure(t *testing.T) {
	req, rw := setupRequestResponse(t, prot.ComputeSystemNegotiateProtocolV1, prot.PvInvalid, nil)

	tb := new(Bridge)
	tb.negotiateProtocolV2(rw, req)

	verifyResponseJSONError(t, rw)
	verifyActivityIDEmptyGUID(t, rw)
}

func Test_NegotiateProtocol_InvalidJson_Failure(t *testing.T) {
	req, rw := setupRequestResponse(t, prot.ComputeSystemNegotiateProtocolV1, prot.PvInvalid, nil)

	tb := new(Bridge)
	tb.negotiateProtocolV2(rw, req)

	verifyResponseJSONError(t, rw)
	verifyActivityIDEmptyGUID(t, rw)
}

func Test_NegotiateProtocol_InvalidRange_Low_Failure(t *testing.T) {
	r := &prot.NegotiateProtocol{
		MessageBase:    newMessageBase(),
		MinimumVersion: 3,
		MaximumVersion: 3,
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemNegotiateProtocolV1, prot.PvInvalid, r)

	tb := new(Bridge)
	tb.negotiateProtocolV2(rw, req)

	verifyResponseError(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
}

func Test_NegotiateProtocol_InvalidRange_High_Failure(t *testing.T) {
	r := &prot.NegotiateProtocol{
		MessageBase:    newMessageBase(),
		MinimumVersion: uint32(prot.PvMax) + 1,
		MaximumVersion: uint32(prot.PvMax) + 1,
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemNegotiateProtocolV1, prot.PvInvalid, r)

	tb := new(Bridge)
	tb.negotiateProtocolV2(rw, req)

	verifyResponseError(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
}

func Test_NegotiateProtocol_ValidRange_Success(t *testing.T) {
	r := &prot.NegotiateProtocol{
		MessageBase:    newMessageBase(),
		MinimumVersion: 4,
		MaximumVersion: uint32(prot.PvMax) + 1,
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemNegotiateProtocolV1, prot.PvInvalid, r)

	tb := new(Bridge)
	tb.negotiateProtocolV2(rw, req)

	verifyResponseSuccess(t, rw)
	verifyActivityID(t, r.MessageBase, rw)

	resp := rw.response.(*prot.NegotiateProtocolResponse)
	if resp.Version != uint32(prot.PvMax) {
		t.Errorf("Invalid version number selected for response: %v", resp.Version)
	}
	// verify that the bridge global was updated
	if tb.protVer != prot.PvMax {
		t.Error("The global bridge protocol version was not updated after a call to negotiate protocol")
	}
}

func Test_CreateContainer_InvalidJson_Failure(t *testing.T) {
	req, rw := setupRequestResponse(t, prot.ComputeSystemCreateV1, prot.PvInvalid, nil)

	tb := new(Bridge)
	tb.createContainer(rw, req)

	verifyResponseJSONError(t, rw)
	verifyActivityIDEmptyGUID(t, rw)
}

func Test_CreateContainer_InvalidHostedJson_Failure(t *testing.T) {
	r := &prot.ContainerCreate{
		MessageBase: newMessageBase(),
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemCreateV1, prot.PvInvalid, r)

	tb := new(Bridge)
	tb.createContainer(rw, req)

	verifyResponseJSONError(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
}

func Test_CreateContainer_CoreCreateContainerFails_Failure(t *testing.T) {
	r := &prot.ContainerCreate{
		MessageBase:     newMessageBase(),
		ContainerConfig: "{}", // Just unmarshal to defaults
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemCreateV1, prot.PvInvalid, r)

	tb := &Bridge{
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	tb.createContainer(rw, req)

	verifyResponseError(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
}

func createContainerConfig() (*prot.ContainerCreate, prot.VMHostedContainerSettings) {
	hs := prot.VMHostedContainerSettings{
		Layers:          []prot.Layer{prot.Layer{Path: "0"}, prot.Layer{Path: "1"}, prot.Layer{Path: "2"}},
		SandboxDataPath: "3",
		MappedVirtualDisks: []prot.MappedVirtualDisk{
			prot.MappedVirtualDisk{
				ContainerPath:     "/path/inside/container",
				Lun:               4,
				CreateInUtilityVM: true,
				ReadOnly:          false,
			},
		},
		NetworkAdapters: []prot.NetworkAdapter{
			prot.NetworkAdapter{
				AdapterInstanceID:  "00000000-0000-0000-0000-000000000000",
				FirewallEnabled:    false,
				NatEnabled:         true,
				AllocatedIPAddress: "192.168.0.0",
				HostIPAddress:      "192.168.0.1",
				HostIPPrefixLength: 16,
				HostDNSServerList:  "0.0.0.0 1.1.1.1 8.8.8.8",
				HostDNSSuffix:      "microsoft.com",
				EnableLowMetric:    true,
			},
		},
	}

	hsb, _ := json.Marshal(hs)
	r := &prot.ContainerCreate{
		MessageBase:     newMessageBase(),
		ContainerConfig: string(hsb),
	}

	return r, hs
}

func Test_CreateContainer_Success_WaitContainer_Failure(t *testing.T) {
	logrus.SetOutput(ioutil.Discard)

	r, hs := createContainerConfig()
	req, rw := setupRequestResponse(t, prot.ComputeSystemCreateV1, prot.PvInvalid, r)

	mc := &mockcore.MockCore{Behavior: mockcore.SingleSuccess}
	mc.WaitContainerWg.Add(1)

	tb := &Bridge{coreint: mc}
	tb.createContainer(rw, req)

	verifyResponseSuccess(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
	if r.ContainerID != mc.LastCreateContainer.ID {
		t.Fatal("last create container did not have the same container ID")
	}
	if !reflect.DeepEqual(hs, mc.LastCreateContainer.Settings) {
		t.Fatal("request/response structs are not equal")
	}

	// Verify that wait was called. This also tests that if we dont exit in the
	// error case here we would panic when PublishNotification tries to write to
	// the responseChan.
	mc.WaitContainerWg.Wait()
}

func Test_CreateContainer_Success_WaitContainer_Success(t *testing.T) {
	r, hs := createContainerConfig()
	req, rw := setupRequestResponse(t, prot.ComputeSystemCreateV1, prot.PvInvalid, r)

	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	mc.WaitContainerWg.Add(1)
	b := &Bridge{coreint: mc}
	b.responseChan = make(chan bridgeResponse)
	defer close(b.responseChan)

	publishWg := sync.WaitGroup{}
	publishWg.Add(1)
	go func() {
		defer publishWg.Done()

		response := <-b.responseChan

		cn := response.response.(*prot.ContainerNotification)
		if cn.ContainerID != r.ContainerID {
			t.Fatal("publish response had invalid container ID")
		}
		if cn.ActivityID != r.ActivityID {
			t.Fatal("publish response had invalid activity ID")
		}
		if cn.Type != prot.NtUnexpectedExit {
			t.Fatal("publish response had invalid type")
		}
		if cn.Operation != prot.AoNone {
			t.Fatal("publish response had invalid operation")
		}
		if cn.Result != 0 {
			t.Fatal("publish response had invalid result")
		}
	}()

	b.createContainer(rw, req)
	verifyResponseSuccess(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
	if r.ContainerID != mc.LastCreateContainer.ID {
		t.Fatal("last create container did not have the same container ID")
	}
	if !reflect.DeepEqual(hs, mc.LastCreateContainer.Settings) {
		t.Fatal("last create container did not have equal settings structs")
	}
	// verify that the bridge global was updated
	if b.protVer != prot.PvV3 {
		t.Error("The global bridge protocol version was not updated after a call to create container")
	}

	mc.WaitContainerWg.Wait()
	if r.ContainerID != mc.LastWaitContainer.ID {
		t.Fatal("last wait container did not have the same container ID")
	}

	// Wait for the publish to take place on the exited notification.
	publishWg.Wait()
}

func Test_StartContainer_InvalidJson_Failure(t *testing.T) {
	req, rw := setupRequestResponse(t, prot.ComputeSystemStartV1, prot.PvV4, nil)

	b := new(Bridge)
	b.startContainerV2(rw, req)

	verifyResponseJSONError(t, rw)
	verifyActivityIDEmptyGUID(t, rw)
}

func Test_StartContainer_Success(t *testing.T) {
	r := newMessageBase()
	req, rw := setupRequestResponse(t, prot.ComputeSystemStartV1, prot.PvV4, r)

	b := new(Bridge)
	b.responseChan = make(chan bridgeResponse)
	defer close(b.responseChan)

	b.startContainerV2(rw, req)
	verifyResponseSuccess(t, rw)
	verifyActivityID(t, r, rw)
}

func Test_ExecProcess_InvalidJson_Failure(t *testing.T) {
	req, rw := setupRequestResponse(t, prot.ComputeSystemExecuteProcessV1, prot.PvV3, nil)

	tb := new(Bridge)
	tb.execProcess(rw, req)

	verifyResponseJSONError(t, rw)
	verifyActivityIDEmptyGUID(t, rw)
}

func Test_ExecProcess_InvalidProcessParameters_Failure(t *testing.T) {
	r := &prot.ContainerExecuteProcess{
		MessageBase: newMessageBase(),
		Settings: prot.ExecuteProcessSettings{
			ProcessParameters: "",
		},
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemExecuteProcessV1, prot.PvV3, r)

	tb := new(Bridge)
	tb.execProcess(rw, req)

	verifyResponseJSONError(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
}

func Test_ExecProcess_External_CoreFails_Failure(t *testing.T) {
	pp := prot.ProcessParameters{
		IsExternal: true,
	}
	ppbytes, _ := json.Marshal(pp)
	r := &prot.ContainerExecuteProcess{
		MessageBase: newMessageBase(),
		Settings: prot.ExecuteProcessSettings{
			ProcessParameters: string(ppbytes),
		},
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemExecuteProcessV1, prot.PvV3, r)

	tb := &Bridge{
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	tb.execProcess(rw, req)

	verifyResponseError(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
}

func Test_ExecProcess_External_CoreSucceeds_Success(t *testing.T) {
	pp := prot.ProcessParameters{
		IsExternal: true,
	}
	ppbytes, _ := json.Marshal(pp)
	r := &prot.ContainerExecuteProcess{
		MessageBase: newMessageBase(),
		Settings: prot.ExecuteProcessSettings{
			ProcessParameters: string(ppbytes),
		},
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemExecuteProcessV1, prot.PvV3, r)
	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	tb := &Bridge{
		coreint: mc,
	}
	tb.execProcess(rw, req)

	verifyResponseSuccess(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
	if !reflect.DeepEqual(pp, mc.LastRunExternalProcess.Params) {
		t.Fatal("last run external process did not have equal params structs")
	}
}

func Test_ExecProcess_Container_CoreFails_Failure(t *testing.T) {
	r := &prot.ContainerExecuteProcess{
		MessageBase: newMessageBase(),
		Settings: prot.ExecuteProcessSettings{
			ProcessParameters: "{}", // Default
		},
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemExecuteProcessV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	tb.execProcess(rw, req)

	verifyResponseError(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
}

func Test_ExecProcess_Container_CoreSucceeds_Success(t *testing.T) {
	pp := prot.ProcessParameters{
		CommandLine: "test",
	}
	ppbytes, _ := json.Marshal(pp)
	r := &prot.ContainerExecuteProcess{
		MessageBase: newMessageBase(),
		Settings: prot.ExecuteProcessSettings{
			ProcessParameters: string(ppbytes),
		},
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemExecuteProcessV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	tb := &Bridge{
		hostState: uvm,
		coreint:   mc,
	}
	tb.execProcess(rw, req)

	verifyResponseSuccess(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
	if r.ContainerID != mc.LastExecProcess.ID {
		t.Fatal("last exec process did not have the same container ID")
	}
	if !reflect.DeepEqual(pp, mc.LastExecProcess.Params) {
		t.Fatal("last exec process did not have equal params structs")
	}
}

func Test_KillContainer_InvalidJson_Failure(t *testing.T) {
	req, rw := setupRequestResponse(t, prot.ComputeSystemShutdownForcedV1, prot.PvV3, nil)

	tb := new(Bridge)
	tb.killContainer(rw, req)

	verifyResponseJSONError(t, rw)
	verifyActivityIDEmptyGUID(t, rw)
}

func Test_KillContainer_CoreFails_Failure(t *testing.T) {
	r := newMessageBase()
	req, rw := setupRequestResponse(t, prot.ComputeSystemShutdownForcedV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	tb.killContainer(rw, req)

	verifyResponseError(t, rw)
	verifyActivityID(t, r, rw)
}

func Test_KillContainer_CoreSucceeds_Success(t *testing.T) {
	r := newMessageBase()
	req, rw := setupRequestResponse(t, prot.ComputeSystemShutdownForcedV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	tb := &Bridge{hostState: uvm, coreint: mc}
	tb.killContainer(rw, req)

	verifyResponseSuccess(t, rw)
	verifyActivityID(t, r, rw)
	if r.ContainerID != mc.LastSignalContainer.ID {
		t.Fatal("last signal container did not have the same container ID")
	}
	if mc.LastSignalContainer.Signal != unix.SIGKILL {
		t.Fatal("last signal container did not have equal signal values")
	}
}

func Test_ShutdownContainer_InvalidJson_Failure(t *testing.T) {
	req, rw := setupRequestResponse(t, prot.ComputeSystemShutdownGracefulV1, prot.PvV3, nil)

	tb := new(Bridge)
	tb.shutdownContainer(rw, req)

	verifyResponseJSONError(t, rw)
	verifyActivityIDEmptyGUID(t, rw)
}

func Test_ShutdownContainer_CoreFails_Failure(t *testing.T) {
	r := newMessageBase()
	req, rw := setupRequestResponse(t, prot.ComputeSystemShutdownGracefulV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	tb.shutdownContainer(rw, req)

	verifyResponseError(t, rw)
	verifyActivityID(t, r, rw)
}

func Test_ShutdownContainer_CoreSucceeds_Success(t *testing.T) {
	r := newMessageBase()
	req, rw := setupRequestResponse(t, prot.ComputeSystemShutdownGracefulV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	tb := &Bridge{hostState: uvm, coreint: mc}
	tb.shutdownContainer(rw, req)

	verifyResponseSuccess(t, rw)
	verifyActivityID(t, r, rw)
	if r.ContainerID != mc.LastSignalContainer.ID {
		t.Fatal("last signal container did not have the same container ID")
	}
	if mc.LastSignalContainer.Signal != unix.SIGTERM {
		t.Fatal("last signal container did not have equal signal values")
	}
}

func Test_SignalProcess_InvalidJson_Failure(t *testing.T) {
	req, rw := setupRequestResponse(t, prot.ComputeSystemSignalProcessV1, prot.PvV3, nil)

	tb := new(Bridge)
	tb.signalProcess(rw, req)

	verifyResponseJSONError(t, rw)
	verifyActivityIDEmptyGUID(t, rw)
}

func Test_SignalProcess_CoreFails_Failure(t *testing.T) {
	r := &prot.ContainerSignalProcess{
		MessageBase: newMessageBase(),
		ProcessID:   20,
		Options: prot.SignalProcessOptions{
			Signal: 10,
		},
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemSignalProcessV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	tb.signalProcess(rw, req)

	verifyResponseError(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
}

func Test_SignalProcess_CoreSucceeds_Success(t *testing.T) {
	r := &prot.ContainerSignalProcess{
		MessageBase: newMessageBase(),
		ProcessID:   20,
		Options: prot.SignalProcessOptions{
			Signal: 10,
		},
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemSignalProcessV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	tb := &Bridge{hostState: uvm, coreint: mc}
	tb.signalProcess(rw, req)

	verifyResponseSuccess(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
	if uint32(mc.LastSignalProcess.Pid) != r.ProcessID {
		t.Fatal("last signal process did not have the same pid")
	}
	if !reflect.DeepEqual(r.Options, mc.LastSignalProcess.Options) {
		t.Fatal("last signal process did not have equal options structs")
	}
}

func Test_GetProperties_InvalidJson_Failure(t *testing.T) {
	req, rw := setupRequestResponse(t, prot.ComputeSystemGetPropertiesV1, prot.PvV3, nil)

	tb := new(Bridge)
	tb.getProperties(rw, req)

	verifyResponseJSONError(t, rw)
	verifyActivityIDEmptyGUID(t, rw)
}

func Test_GetProperties_CoreFails_Failure(t *testing.T) {
	r := &prot.ContainerGetProperties{
		MessageBase: newMessageBase(),
		Query:       "",
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemGetPropertiesV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	tb.getProperties(rw, req)

	verifyResponseError(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
}

func Test_GetProperties_CoreSucceeds_Success(t *testing.T) {
	r := &prot.ContainerGetProperties{
		MessageBase: newMessageBase(),
		Query:       "{\"PropertyTypes\":[\"ProcessList\"]}",
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemGetPropertiesV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	tb := &Bridge{hostState: uvm, coreint: mc}
	tb.getProperties(rw, req)

	verifyResponseSuccess(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
	if mc.LastGetProperties.ID != r.ContainerID {
		t.Fatal("last get properties did not have the same container ID")
	}
	response, ok := rw.response.(*prot.ContainerGetPropertiesResponse)
	if !ok {
		t.Fatalf("get properties returned the wrong response type: %T", rw.response)
	}

	var properties prot.Properties
	json.Unmarshal([]byte(response.Properties), &properties)
	numProcesses := len(properties.ProcessList)
	if numProcesses != 1 {
		t.Fatalf("get properties returned an incorrect number of processes: %d", numProcesses)
	}
	pid := properties.ProcessList[0].ProcessID
	if pid != 101 {
		t.Fatalf("get properties returned a process with an incorrect pid: %d", pid)
	}
}

func Test_WaitOnProcess_InvalidJson_Failure(t *testing.T) {
	req, rw := setupRequestResponse(t, prot.ComputeSystemWaitForProcessV1, prot.PvV3, nil)

	tb := new(Bridge)
	tb.waitOnProcess(rw, req)

	verifyResponseJSONError(t, rw)
	verifyActivityIDEmptyGUID(t, rw)
}

func Test_WaitOnProcess_CoreFails_Failure(t *testing.T) {
	r := &prot.ContainerWaitForProcess{
		MessageBase: newMessageBase(),
		ProcessID:   20,
		TimeoutInMs: 1000,
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemWaitForProcessV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	tb.waitOnProcess(rw, req)

	verifyResponseError(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
}

func Test_WaitOnProcess_CoreSucceeds_Timeout_Error(t *testing.T) {
	r := &prot.ContainerWaitForProcess{
		MessageBase: newMessageBase(),
		ProcessID:   20,
		TimeoutInMs: 10,
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemWaitForProcessV1, prot.PvV3, r)

	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	mc.LastWaitProcessReturnContext = &mockcore.WaitProcessReturnContext{
		ExitCodeChan: make(chan int, 1),
		DoneChan:     make(chan bool, 1),
	}

	// Do not write the exit code so that the timeout occurs.
	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{hostState: uvm, coreint: mc}
	tb.waitOnProcess(rw, req)

	verifyResponseError(t, rw)
	verifyActivityID(t, r.MessageBase, rw)

	// Verify that the caller bridge calls done in the timeout case
	// to acknowledge the response.
	<-mc.LastWaitProcessReturnContext.DoneChan
}

func Test_WaitOnProcess_CoreSucceeds_Success(t *testing.T) {
	r := &prot.ContainerWaitForProcess{
		MessageBase: newMessageBase(),
		ProcessID:   20,
		TimeoutInMs: 1000,
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemWaitForProcessV1, prot.PvV3, r)

	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	mc.LastWaitProcessReturnContext = &mockcore.WaitProcessReturnContext{
		ExitCodeChan: make(chan int, 1),
		DoneChan:     make(chan bool, 1),
	}

	// Immediately write the exit code so the waitOnProcess doesnt block.
	mc.LastWaitProcessReturnContext.ExitCodeChan <- 2980

	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{hostState: uvm, coreint: mc}
	tb.waitOnProcess(rw, req)

	verifyResponseSuccess(t, rw)
	if uint32(mc.LastWaitProcess.Pid) != r.ProcessID {
		t.Fatal("last wait process did not have same pid")
	}
	// Verify that the caller bridge calls done in the success case
	// to acknowledge the exit code response.
	<-mc.LastWaitProcessReturnContext.DoneChan
}

func Test_ResizeConsole_InvalidJson_Failure(t *testing.T) {
	req, rw := setupRequestResponse(t, prot.ComputeSystemResizeConsoleV1, prot.PvV3, nil)

	tb := new(Bridge)
	tb.resizeConsole(rw, req)

	verifyResponseJSONError(t, rw)
	verifyActivityIDEmptyGUID(t, rw)
}

func Test_ResizeConsole_CoreFails_Failure(t *testing.T) {
	r := &prot.ContainerResizeConsole{
		MessageBase: newMessageBase(),
		ProcessID:   20,
		Width:       20,
		Height:      20,
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemResizeConsoleV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	tb.resizeConsole(rw, req)

	verifyResponseError(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
}

func Test_ResizeConsole_CoreSucceeds_Success(t *testing.T) {
	r := &prot.ContainerResizeConsole{
		MessageBase: newMessageBase(),
		ProcessID:   20,
		Width:       640,
		Height:      480,
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemResizeConsoleV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	tb := &Bridge{hostState: uvm, coreint: mc}
	tb.resizeConsole(rw, req)

	verifyResponseSuccess(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
	if uint32(mc.LastResizeConsole.Pid) != r.ProcessID {
		t.Fatal("last resize console did not have same pid")
	}
	if mc.LastResizeConsole.Width != r.Width {
		t.Fatal("last resize console did not have same width")
	}
	if mc.LastResizeConsole.Height != r.Height {
		t.Fatal("last resize console did not have same height")
	}
}

func Test_ModifySettings_InvalidJson_Failure(t *testing.T) {
	req, rw := setupRequestResponse(t, prot.ComputeSystemModifySettingsV1, prot.PvV3, nil)

	tb := new(Bridge)
	tb.modifySettings(rw, req)

	verifyResponseJSONError(t, rw)
	verifyActivityIDEmptyGUID(t, rw)
}

func Test_ModifySettings_VirtualDisk_InvalidSettingsJson_Failure(t *testing.T) {
	r := &prot.ContainerModifySettings{
		MessageBase: newMessageBase(),
		Request: &prot.ResourceModificationRequestResponse{
			ResourceType: prot.PtMappedVirtualDisk,
		},
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemModifySettingsV1, prot.PvV3, r)

	tb := new(Bridge)
	tb.modifySettings(rw, req)

	verifyResponseJSONError(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
}

func Test_ModifySettings_MappedDirectory_InvalidSettingsJson_Failure(t *testing.T) {
	r := &prot.ContainerModifySettings{
		MessageBase: newMessageBase(),
		Request: &prot.ResourceModificationRequestResponse{
			ResourceType: prot.PtMappedDirectory,
		},
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemModifySettingsV1, prot.PvV3, r)

	tb := new(Bridge)
	tb.modifySettings(rw, req)

	verifyResponseJSONError(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
}

func Test_ModifySettings_CoreFails_Failure(t *testing.T) {
	r := &prot.ContainerModifySettings{
		MessageBase: newMessageBase(),
		Request: &prot.ResourceModificationRequestResponse{
			ResourceType: prot.PtMappedDirectory,
			Settings:     &prot.MappedDirectory{}, // Default values.
		},
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemModifySettingsV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	tb.modifySettings(rw, req)

	verifyResponseError(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
}

func Test_ModifySettings_CoreSucceeds_Success(t *testing.T) {
	r := &prot.ContainerModifySettings{
		MessageBase: newMessageBase(),
		Request: &prot.ResourceModificationRequestResponse{
			ResourceType: prot.PtMappedDirectory,
			RequestType:  prot.RtAdd,
			Settings: &prot.MappedDirectory{
				ReadOnly: true,
			},
		},
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemModifySettingsV1, prot.PvV3, r)

	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint:   mc,
	}
	tb.modifySettings(rw, req)

	verifyResponseSuccess(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
	if r.ContainerID != mc.LastModifySettings.ID {
		t.Fatal("last modify settings did not have the same container ID")
	}
	if !reflect.DeepEqual(r.Request, mc.LastModifySettings.Request) {
		t.Fatal("last modify settings did not have equal requests struct")
	}
}

/*
// TODO: jterry75 - Enable V2 unit tests
func Test_ModifySettings_V2_Success(t *testing.T) {
	r := &prot.ContainerModifySettings{
		MessageBase: newMessageUVMBase(),
		V2Request: &prot.ModifySettingRequest{
			ResourceType: prot.MrtMappedDirectory,
			RequestType:  prot.MreqtAdd,
			Settings: &prot.MappedDirectory{
				ReadOnly: true,
			},
		},
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemModifySettingsV1, prot.PvV3, r)

	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	tb := &Bridge{
		coreint: mc,
	}
	tb.modifySettings(rw, req)

	verifyResponseSuccess(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
	if r.ContainerID != mc.LastModifySettings.ID {
		t.Fatal("last modify settings did not have the same container ID")
	}
	v1Request := prot.ResourceModificationRequestResponse{}
	v1Request.ResourceType = prot.PropertyType(r.V2Request.ResourceType)
	v1Request.RequestType = prot.RequestType(r.V2Request.RequestType)
	v1Request.Settings = r.V2Request.Settings
	if !reflect.DeepEqual(v1Request, mc.LastModifySettings.Request) {
		t.Fatal("last modify settings did not have equal requests struct")
	}
}
*/

func Test_ModifySettings_BothV1V2_Success(t *testing.T) {
	r := &prot.ContainerModifySettings{
		MessageBase: newMessageBase(),
		Request: &prot.ResourceModificationRequestResponse{
			ResourceType: prot.PtMappedVirtualDisk,
			RequestType:  prot.RtRemove,
			Settings: &prot.MappedVirtualDisk{
				ReadOnly: true,
			},
		},
		V2Request: &prot.ModifySettingRequest{
			ResourceType: prot.MrtMappedDirectory,
			RequestType:  prot.MreqtAdd,
			Settings: &prot.MappedDirectoryV2{
				ReadOnly: true,
			},
		},
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemModifySettingsV1, prot.PvV3, r)

	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint:   mc,
	}
	tb.modifySettings(rw, req)

	verifyResponseSuccess(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
	if r.ContainerID != mc.LastModifySettings.ID {
		t.Fatal("last modify settings did not have the same container ID")
	}
	if !reflect.DeepEqual(r.Request, mc.LastModifySettings.Request) {
		t.Fatal("last modify settings did not have equal requests struct")
	}
}

func Test_ModifySettings_NeitherV1V2_Fails(t *testing.T) {
	r := &prot.ContainerModifySettings{
		MessageBase: newMessageBase(),
	}

	req, rw := setupRequestResponse(t, prot.ComputeSystemModifySettingsV1, prot.PvV3, r)

	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	tb := &Bridge{
		coreint: mc,
	}
	tb.modifySettings(rw, req)

	verifyResponseError(t, rw)
	verifyActivityID(t, r.MessageBase, rw)
}
