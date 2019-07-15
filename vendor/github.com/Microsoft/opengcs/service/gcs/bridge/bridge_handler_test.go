package bridge

import (
	"context"
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

func createRequest(t *testing.T, id prot.MessageIdentifier, ver prot.ProtocolVersion, message interface{}) *Request {
	r := &Request{
		Context: context.Background(),
		Version: ver,
	}

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
	return r
}

func verifyResponseError(t *testing.T, resp RequestResponse, err error) {
	if resp != nil {
		t.Fatalf("response was returned on expected error: %+v", resp)
	}
	if err == nil {
		t.Fatal("expected valid error, got: nil")
	}
}

func verifyResponseJSONError(t *testing.T, resp RequestResponse, err error) {
	verifyResponseError(t, resp, err)
	if !strings.Contains(err.Error(), "failed to unmarshal JSON") {
		t.Fatalf("response error %v, was not a json marshal error", err)
	}
}

func verifyResponseSuccess(t *testing.T, resp RequestResponse, err error) {
	if resp == nil {
		t.Fatal("expected valid response, got: nil")
	}
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func newMessageBase() prot.MessageBase {
	const chars = "abcdefghijklmnopqrstuvwxyz"
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	f := func() string {
		b := make([]byte, 10)
		for i := 0; i < len(b); i++ {
			b[i] = chars[r.Intn(len(chars))]
		}
		return string(b)
	}

	base := prot.MessageBase{
		ContainerID: f(),
		ActivityID:  f(),
	}
	return base
}

func newMessageUVMBase() prot.MessageBase {
	b := newMessageBase()
	b.ContainerID = hcsv2.UVMContainerID
	return b
}

func Test_NegotiateProtocol_DuplicateCall_Failure(t *testing.T) {
	req := createRequest(t, prot.ComputeSystemNegotiateProtocolV1, prot.PvInvalid, nil)

	tb := new(Bridge)
	resp, err := tb.negotiateProtocolV2(req)

	verifyResponseJSONError(t, resp, err)
}

func Test_NegotiateProtocol_InvalidJson_Failure(t *testing.T) {
	req := createRequest(t, prot.ComputeSystemNegotiateProtocolV1, prot.PvInvalid, nil)

	tb := new(Bridge)
	resp, err := tb.negotiateProtocolV2(req)

	verifyResponseJSONError(t, resp, err)
}

func Test_NegotiateProtocol_InvalidRange_Low_Failure(t *testing.T) {
	r := &prot.NegotiateProtocol{
		MessageBase:    newMessageBase(),
		MinimumVersion: 3,
		MaximumVersion: 3,
	}

	req := createRequest(t, prot.ComputeSystemNegotiateProtocolV1, prot.PvInvalid, r)

	tb := new(Bridge)
	resp, err := tb.negotiateProtocolV2(req)

	verifyResponseError(t, resp, err)
}

func Test_NegotiateProtocol_InvalidRange_High_Failure(t *testing.T) {
	r := &prot.NegotiateProtocol{
		MessageBase:    newMessageBase(),
		MinimumVersion: uint32(prot.PvMax) + 1,
		MaximumVersion: uint32(prot.PvMax) + 1,
	}

	req := createRequest(t, prot.ComputeSystemNegotiateProtocolV1, prot.PvInvalid, r)

	tb := new(Bridge)
	resp, err := tb.negotiateProtocolV2(req)

	verifyResponseError(t, resp, err)
}

func Test_NegotiateProtocol_ValidRange_Success(t *testing.T) {
	r := &prot.NegotiateProtocol{
		MessageBase:    newMessageBase(),
		MinimumVersion: 4,
		MaximumVersion: uint32(prot.PvMax) + 1,
	}

	req := createRequest(t, prot.ComputeSystemNegotiateProtocolV1, prot.PvInvalid, r)

	tb := new(Bridge)
	resp, err := tb.negotiateProtocolV2(req)

	verifyResponseSuccess(t, resp, err)

	npr := resp.(*prot.NegotiateProtocolResponse)
	if npr.Version != uint32(prot.PvMax) {
		t.Errorf("Invalid version number selected for response: %v", npr.Version)
	}
	// verify that the bridge global was updated
	if tb.protVer != prot.PvMax {
		t.Error("The global bridge protocol version was not updated after a call to negotiate protocol")
	}
}

func Test_CreateContainer_InvalidJson_Failure(t *testing.T) {
	req := createRequest(t, prot.ComputeSystemCreateV1, prot.PvInvalid, nil)

	tb := new(Bridge)
	resp, err := tb.createContainer(req)

	verifyResponseJSONError(t, resp, err)
}

func Test_CreateContainer_InvalidHostedJson_Failure(t *testing.T) {
	r := &prot.ContainerCreate{
		MessageBase: newMessageBase(),
	}

	req := createRequest(t, prot.ComputeSystemCreateV1, prot.PvInvalid, r)

	tb := new(Bridge)
	resp, err := tb.createContainer(req)

	verifyResponseJSONError(t, resp, err)
}

func Test_CreateContainer_CoreCreateContainerFails_Failure(t *testing.T) {
	r := &prot.ContainerCreate{
		MessageBase:     newMessageBase(),
		ContainerConfig: "{}", // Just unmarshal to defaults
	}

	req := createRequest(t, prot.ComputeSystemCreateV1, prot.PvInvalid, r)

	tb := &Bridge{
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	resp, err := tb.createContainer(req)

	verifyResponseError(t, resp, err)
}

func createContainerConfig() (*prot.ContainerCreate, prot.VMHostedContainerSettings) {
	hs := prot.VMHostedContainerSettings{
		Layers:          []prot.Layer{{Path: "0"}, {Path: "1"}, {Path: "2"}},
		SandboxDataPath: "3",
		MappedVirtualDisks: []prot.MappedVirtualDisk{
			{
				ContainerPath:     "/path/inside/container",
				Lun:               4,
				CreateInUtilityVM: true,
				ReadOnly:          false,
			},
		},
		NetworkAdapters: []prot.NetworkAdapter{
			{
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
	req := createRequest(t, prot.ComputeSystemCreateV1, prot.PvInvalid, r)

	mc := &mockcore.MockCore{Behavior: mockcore.SingleSuccess}
	mc.WaitContainerWg.Add(1)

	tb := &Bridge{coreint: mc}
	resp, err := tb.createContainer(req)

	verifyResponseSuccess(t, resp, err)
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
	req := createRequest(t, prot.ComputeSystemCreateV1, prot.PvInvalid, r)

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

	resp, err := b.createContainer(req)
	verifyResponseSuccess(t, resp, err)
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
	req := createRequest(t, prot.ComputeSystemStartV1, prot.PvV4, nil)

	b := new(Bridge)
	resp, err := b.startContainerV2(req)

	verifyResponseJSONError(t, resp, err)
}

func Test_StartContainer_Success(t *testing.T) {
	r := newMessageBase()
	req := createRequest(t, prot.ComputeSystemStartV1, prot.PvV4, r)

	b := new(Bridge)
	b.responseChan = make(chan bridgeResponse)
	defer close(b.responseChan)

	resp, err := b.startContainerV2(req)
	verifyResponseSuccess(t, resp, err)
}

func Test_ExecProcess_InvalidJson_Failure(t *testing.T) {
	req := createRequest(t, prot.ComputeSystemExecuteProcessV1, prot.PvV3, nil)

	tb := new(Bridge)
	resp, err := tb.execProcess(req)

	verifyResponseJSONError(t, resp, err)
}

func Test_ExecProcess_InvalidProcessParameters_Failure(t *testing.T) {
	r := &prot.ContainerExecuteProcess{
		MessageBase: newMessageBase(),
		Settings: prot.ExecuteProcessSettings{
			ProcessParameters: "",
		},
	}

	req := createRequest(t, prot.ComputeSystemExecuteProcessV1, prot.PvV3, r)

	tb := new(Bridge)
	resp, err := tb.execProcess(req)

	verifyResponseJSONError(t, resp, err)
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

	req := createRequest(t, prot.ComputeSystemExecuteProcessV1, prot.PvV3, r)

	tb := &Bridge{
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	resp, err := tb.execProcess(req)

	verifyResponseError(t, resp, err)
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

	req := createRequest(t, prot.ComputeSystemExecuteProcessV1, prot.PvV3, r)
	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	tb := &Bridge{
		coreint: mc,
	}
	resp, err := tb.execProcess(req)

	verifyResponseSuccess(t, resp, err)
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

	req := createRequest(t, prot.ComputeSystemExecuteProcessV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	resp, err := tb.execProcess(req)

	verifyResponseError(t, resp, err)
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

	req := createRequest(t, prot.ComputeSystemExecuteProcessV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	tb := &Bridge{
		hostState: uvm,
		coreint:   mc,
	}
	resp, err := tb.execProcess(req)

	verifyResponseSuccess(t, resp, err)
	if r.ContainerID != mc.LastExecProcess.ID {
		t.Fatal("last exec process did not have the same container ID")
	}
	if !reflect.DeepEqual(pp, mc.LastExecProcess.Params) {
		t.Fatal("last exec process did not have equal params structs")
	}
}

func Test_KillContainer_InvalidJson_Failure(t *testing.T) {
	req := createRequest(t, prot.ComputeSystemShutdownForcedV1, prot.PvV3, nil)

	tb := new(Bridge)
	resp, err := tb.killContainer(req)

	verifyResponseJSONError(t, resp, err)
}

func Test_KillContainer_CoreFails_Failure(t *testing.T) {
	r := newMessageBase()
	req := createRequest(t, prot.ComputeSystemShutdownForcedV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	resp, err := tb.killContainer(req)

	verifyResponseError(t, resp, err)
}

func Test_KillContainer_CoreSucceeds_Success(t *testing.T) {
	r := newMessageBase()
	req := createRequest(t, prot.ComputeSystemShutdownForcedV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	tb := &Bridge{hostState: uvm, coreint: mc}
	resp, err := tb.killContainer(req)

	verifyResponseSuccess(t, resp, err)
	if r.ContainerID != mc.LastSignalContainer.ID {
		t.Fatal("last signal container did not have the same container ID")
	}
	if mc.LastSignalContainer.Signal != unix.SIGKILL {
		t.Fatal("last signal container did not have equal signal values")
	}
}

func Test_ShutdownContainer_InvalidJson_Failure(t *testing.T) {
	req := createRequest(t, prot.ComputeSystemShutdownGracefulV1, prot.PvV3, nil)

	tb := new(Bridge)
	resp, err := tb.shutdownContainer(req)

	verifyResponseJSONError(t, resp, err)
}

func Test_ShutdownContainer_CoreFails_Failure(t *testing.T) {
	r := newMessageBase()
	req := createRequest(t, prot.ComputeSystemShutdownGracefulV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	resp, err := tb.shutdownContainer(req)

	verifyResponseError(t, resp, err)
}

func Test_ShutdownContainer_CoreSucceeds_Success(t *testing.T) {
	r := newMessageBase()
	req := createRequest(t, prot.ComputeSystemShutdownGracefulV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	tb := &Bridge{hostState: uvm, coreint: mc}
	resp, err := tb.shutdownContainer(req)

	verifyResponseSuccess(t, resp, err)
	if r.ContainerID != mc.LastSignalContainer.ID {
		t.Fatal("last signal container did not have the same container ID")
	}
	if mc.LastSignalContainer.Signal != unix.SIGTERM {
		t.Fatal("last signal container did not have equal signal values")
	}
}

func Test_SignalProcess_InvalidJson_Failure(t *testing.T) {
	req := createRequest(t, prot.ComputeSystemSignalProcessV1, prot.PvV3, nil)

	tb := new(Bridge)
	resp, err := tb.signalProcess(req)

	verifyResponseJSONError(t, resp, err)
}

func Test_SignalProcess_CoreFails_Failure(t *testing.T) {
	r := &prot.ContainerSignalProcess{
		MessageBase: newMessageBase(),
		ProcessID:   20,
		Options: prot.SignalProcessOptions{
			Signal: 10,
		},
	}

	req := createRequest(t, prot.ComputeSystemSignalProcessV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	resp, err := tb.signalProcess(req)

	verifyResponseError(t, resp, err)
}

func Test_SignalProcess_CoreSucceeds_Success(t *testing.T) {
	r := &prot.ContainerSignalProcess{
		MessageBase: newMessageBase(),
		ProcessID:   20,
		Options: prot.SignalProcessOptions{
			Signal: 10,
		},
	}

	req := createRequest(t, prot.ComputeSystemSignalProcessV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	tb := &Bridge{hostState: uvm, coreint: mc}
	resp, err := tb.signalProcess(req)

	verifyResponseSuccess(t, resp, err)
	if uint32(mc.LastSignalProcess.Pid) != r.ProcessID {
		t.Fatal("last signal process did not have the same pid")
	}
	if !reflect.DeepEqual(r.Options, mc.LastSignalProcess.Options) {
		t.Fatal("last signal process did not have equal options structs")
	}
}

func Test_GetProperties_InvalidJson_Failure(t *testing.T) {
	req := createRequest(t, prot.ComputeSystemGetPropertiesV1, prot.PvV3, nil)

	tb := new(Bridge)
	resp, err := tb.getProperties(req)

	verifyResponseJSONError(t, resp, err)
}

func Test_GetProperties_CoreFails_Failure(t *testing.T) {
	r := &prot.ContainerGetProperties{
		MessageBase: newMessageBase(),
		Query:       "",
	}

	req := createRequest(t, prot.ComputeSystemGetPropertiesV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	resp, err := tb.getProperties(req)

	verifyResponseError(t, resp, err)
}

func Test_GetProperties_CoreSucceeds_Success(t *testing.T) {
	r := &prot.ContainerGetProperties{
		MessageBase: newMessageBase(),
		Query:       "{\"PropertyTypes\":[\"ProcessList\"]}",
	}

	req := createRequest(t, prot.ComputeSystemGetPropertiesV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	tb := &Bridge{hostState: uvm, coreint: mc}
	resp, err := tb.getProperties(req)

	verifyResponseSuccess(t, resp, err)
	if mc.LastGetProperties.ID != r.ContainerID {
		t.Fatal("last get properties did not have the same container ID")
	}
	cgpr, ok := resp.(*prot.ContainerGetPropertiesResponse)
	if !ok {
		t.Fatalf("get properties returned the wrong response type: %T", resp)
	}

	var properties prot.Properties
	json.Unmarshal([]byte(cgpr.Properties), &properties)
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
	req := createRequest(t, prot.ComputeSystemWaitForProcessV1, prot.PvV3, nil)

	tb := new(Bridge)
	resp, err := tb.waitOnProcess(req)

	verifyResponseJSONError(t, resp, err)
}

func Test_WaitOnProcess_CoreFails_Failure(t *testing.T) {
	r := &prot.ContainerWaitForProcess{
		MessageBase: newMessageBase(),
		ProcessID:   20,
		TimeoutInMs: 1000,
	}

	req := createRequest(t, prot.ComputeSystemWaitForProcessV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	resp, err := tb.waitOnProcess(req)

	verifyResponseError(t, resp, err)
}

func Test_WaitOnProcess_CoreSucceeds_Timeout_Error(t *testing.T) {
	r := &prot.ContainerWaitForProcess{
		MessageBase: newMessageBase(),
		ProcessID:   20,
		TimeoutInMs: 10,
	}

	req := createRequest(t, prot.ComputeSystemWaitForProcessV1, prot.PvV3, r)

	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	mc.LastWaitProcessReturnContext = &mockcore.WaitProcessReturnContext{
		ExitCodeChan: make(chan int, 1),
		DoneChan:     make(chan bool, 1),
	}

	// Do not write the exit code so that the timeout occurs.
	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{hostState: uvm, coreint: mc}
	resp, err := tb.waitOnProcess(req)

	verifyResponseError(t, resp, err)

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

	req := createRequest(t, prot.ComputeSystemWaitForProcessV1, prot.PvV3, r)

	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	mc.LastWaitProcessReturnContext = &mockcore.WaitProcessReturnContext{
		ExitCodeChan: make(chan int, 1),
		DoneChan:     make(chan bool, 1),
	}

	// Immediately write the exit code so the waitOnProcess doesnt block.
	mc.LastWaitProcessReturnContext.ExitCodeChan <- 2980

	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{hostState: uvm, coreint: mc}
	resp, err := tb.waitOnProcess(req)

	verifyResponseSuccess(t, resp, err)
	if uint32(mc.LastWaitProcess.Pid) != r.ProcessID {
		t.Fatal("last wait process did not have same pid")
	}
	// Verify that the caller bridge calls done in the success case
	// to acknowledge the exit code response.
	<-mc.LastWaitProcessReturnContext.DoneChan
}

func Test_ResizeConsole_InvalidJson_Failure(t *testing.T) {
	req := createRequest(t, prot.ComputeSystemResizeConsoleV1, prot.PvV3, nil)

	tb := new(Bridge)
	resp, err := tb.resizeConsole(req)

	verifyResponseJSONError(t, resp, err)
}

func Test_ResizeConsole_CoreFails_Failure(t *testing.T) {
	r := &prot.ContainerResizeConsole{
		MessageBase: newMessageBase(),
		ProcessID:   20,
		Width:       20,
		Height:      20,
	}

	req := createRequest(t, prot.ComputeSystemResizeConsoleV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	resp, err := tb.resizeConsole(req)

	verifyResponseError(t, resp, err)
}

func Test_ResizeConsole_CoreSucceeds_Success(t *testing.T) {
	r := &prot.ContainerResizeConsole{
		MessageBase: newMessageBase(),
		ProcessID:   20,
		Width:       640,
		Height:      480,
	}

	req := createRequest(t, prot.ComputeSystemResizeConsoleV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	tb := &Bridge{hostState: uvm, coreint: mc}
	resp, err := tb.resizeConsole(req)

	verifyResponseSuccess(t, resp, err)
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
	req := createRequest(t, prot.ComputeSystemModifySettingsV1, prot.PvV3, nil)

	tb := new(Bridge)
	resp, err := tb.modifySettings(req)

	verifyResponseJSONError(t, resp, err)
}

func Test_ModifySettings_VirtualDisk_InvalidSettingsJson_Failure(t *testing.T) {
	r := &prot.ContainerModifySettings{
		MessageBase: newMessageBase(),
		Request: &prot.ResourceModificationRequestResponse{
			ResourceType: prot.PtMappedVirtualDisk,
		},
	}

	req := createRequest(t, prot.ComputeSystemModifySettingsV1, prot.PvV3, r)

	tb := new(Bridge)
	resp, err := tb.modifySettings(req)

	verifyResponseJSONError(t, resp, err)
}

func Test_ModifySettings_MappedDirectory_InvalidSettingsJson_Failure(t *testing.T) {
	r := &prot.ContainerModifySettings{
		MessageBase: newMessageBase(),
		Request: &prot.ResourceModificationRequestResponse{
			ResourceType: prot.PtMappedDirectory,
		},
	}

	req := createRequest(t, prot.ComputeSystemModifySettingsV1, prot.PvV3, r)

	tb := new(Bridge)
	resp, err := tb.modifySettings(req)

	verifyResponseJSONError(t, resp, err)
}

func Test_ModifySettings_CoreFails_Failure(t *testing.T) {
	r := &prot.ContainerModifySettings{
		MessageBase: newMessageBase(),
		Request: &prot.ResourceModificationRequestResponse{
			ResourceType: prot.PtMappedDirectory,
			Settings:     &prot.MappedDirectory{}, // Default values.
		},
	}

	req := createRequest(t, prot.ComputeSystemModifySettingsV1, prot.PvV3, r)

	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint: &mockcore.MockCore{
			Behavior: mockcore.Error,
		},
	}
	resp, err := tb.modifySettings(req)

	verifyResponseError(t, resp, err)
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

	req := createRequest(t, prot.ComputeSystemModifySettingsV1, prot.PvV3, r)

	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint:   mc,
	}
	resp, err := tb.modifySettings(req)

	verifyResponseSuccess(t, resp, err)
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

	req := createRequest(t, prot.ComputeSystemModifySettingsV1, prot.PvV3, r)

	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	tb := &Bridge{
		coreint: mc,
	}
	resp, err := tb.modifySettings(req)

	verifyResponseSuccess(t, resp, err)
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

	req := createRequest(t, prot.ComputeSystemModifySettingsV1, prot.PvV3, r)

	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	uvm := hcsv2.NewHost(nil, nil)
	tb := &Bridge{
		hostState: uvm,
		coreint:   mc,
	}
	resp, err := tb.modifySettings(req)

	verifyResponseSuccess(t, resp, err)
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

	req := createRequest(t, prot.ComputeSystemModifySettingsV1, prot.PvV3, r)

	mc := &mockcore.MockCore{Behavior: mockcore.Success}
	tb := &Bridge{
		coreint: mc,
	}
	resp, err := tb.modifySettings(req)

	verifyResponseError(t, resp, err)
}
