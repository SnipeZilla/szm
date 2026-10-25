//go:build windows && (amd64 || arm64)

package netmon

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// sessionName is the trace session's name, as "logman query -ets" lists it.
const sessionName = "SnipeZilla Server Manager network"

// Microsoft-Windows-Kernel-Network {7DD42A49-5329-4832-8DFD-43D979153A88}
var kernelNetwork = windows.GUID{Data1: 0x7DD42A49, Data2: 0x5329, Data3: 0x4832, Data4: [8]byte{0x8D, 0xFD, 0x43, 0xD9, 0x79, 0x15, 0x3A, 0x88}}

var (
	advapi32           = windows.NewLazySystemDLL("advapi32.dll")
	procStartTraceW    = advapi32.NewProc("StartTraceW")
	procControlTraceW  = advapi32.NewProc("ControlTraceW")
	procEnableTraceEx2 = advapi32.NewProc("EnableTraceEx2")
	procOpenTraceW     = advapi32.NewProc("OpenTraceW")
	procProcessTrace   = advapi32.NewProc("ProcessTrace")
	procCloseTrace     = advapi32.NewProc("CloseTrace")
)

const (
	wnodeFlagTracedGUID         = 0x00020000
	eventTraceRealTimeMode      = 0x00000100
	eventTraceControlQuery      = 0
	eventTraceControlStop       = 1
	eventControlCodeEnable      = 1
	traceLevelInformation       = 4
	processTraceModeRealTime    = 0x00000100
	processTraceModeEventRecord = 0x10000000
	invalidProcessTraceHandle   = ^uint64(0)
	errorNoSystemResources      = syscall.Errno(1450)
)

// WNODE_HEADER
type wnodeHeader struct {
	BufferSize        uint32
	ProviderID        uint32
	HistoricalContext uint64
	TimeStamp         int64
	GUID              windows.GUID
	ClientContext     uint32
	Flags             uint32
}

// EVENT_TRACE_PROPERTIES
type eventTraceProperties struct {
	Wnode               wnodeHeader
	BufferSize          uint32
	MinimumBuffers      uint32
	MaximumBuffers      uint32
	MaximumFileSize     uint32
	LogFileMode         uint32
	FlushTimer          uint32
	EnableFlags         uint32
	AgeLimit            int32
	NumberOfBuffers     uint32
	FreeBuffers         uint32
	EventsLost          uint32
	BuffersWritten      uint32
	LogBuffersLost      uint32
	RealTimeBuffersLost uint32
	LoggerThreadID      uintptr
	LogFileNameOffset   uint32
	LoggerNameOffset    uint32
}

// properties is EVENT_TRACE_PROPERTIES followed by room for the session's name, which Windows writes there.
type properties struct {
	p    eventTraceProperties
	name [128]uint16
}

// EVENT_TRACE_LOGFILEW. CurrentEvent (EVENT_TRACE) and LogfileHeader (TRACE_LOGFILE_HEADER) are only room here.
type eventTraceLogfile struct {
	LogFileName      *uint16
	LoggerName       *uint16
	CurrentTime      int64
	BuffersRead      uint32
	ProcessTraceMode uint32
	CurrentEvent     struct{ _ [11]uint64 } // 88 bytes
	LogfileHeader    struct{ _ [35]uint64 } // 280 bytes
	BufferCallback   uintptr
	BufferSize       uint32
	Filled           uint32
	EventsLost       uint32
	Callback         uintptr // EventRecordCallback
	IsKernelTrace    uint32
	Context          uintptr
}

// EVENT_DESCRIPTOR
type eventDescriptor struct {
	ID      uint16
	Version uint8
	Channel uint8
	Level   uint8
	Opcode  uint8
	Task    uint16
	Keyword uint64
}

// EVENT_HEADER
type eventHeader struct {
	Size            uint16
	HeaderType      uint16
	Flags           uint16
	EventProperty   uint16
	ThreadID        uint32
	ProcessID       uint32
	TimeStamp       int64
	ProviderID      windows.GUID
	EventDescriptor eventDescriptor
	ProcessorTime   uint64
	ActivityID      windows.GUID
}

// EVENT_RECORD
type eventRecord struct {
	EventHeader       eventHeader
	BufferContext     uint32
	ExtendedDataCount uint16
	UserDataLength    uint16
	ExtendedData      uintptr
	UserData          *[8]byte // at least 8 bytes for the events counted (UserDataLength is checked first)
	UserContext       uintptr
}

// The sizes Windows (64-bit) has for these structures: the build fails if one is off.
const (
	sizeProperties = 120
	sizeLogfile    = 448
	sizeRecord     = 112
)

var (
	_ [unsafe.Sizeof(eventTraceProperties{}) - sizeProperties]byte
	_ [sizeProperties - unsafe.Sizeof(eventTraceProperties{})]byte
	_ [unsafe.Sizeof(eventTraceLogfile{}) - sizeLogfile]byte
	_ [sizeLogfile - unsafe.Sizeof(eventTraceLogfile{})]byte
	_ [unsafe.Sizeof(eventRecord{}) - sizeRecord]byte
	_ [sizeRecord - unsafe.Sizeof(eventRecord{})]byte
	_ [unsafe.Offsetof(eventRecord{}.UserData) - 96]byte
	_ [96 - unsafe.Offsetof(eventRecord{}.UserData)]byte
	_ [unsafe.Offsetof(eventHeader{}.EventDescriptor) - 40]byte
	_ [40 - unsafe.Offsetof(eventHeader{}.EventDescriptor)]byte
	_ [unsafe.Offsetof(eventTraceLogfile{}.Callback) - 424]byte
	_ [424 - unsafe.Offsetof(eventTraceLogfile{}.Callback)]byte
)

func newProperties() *properties {
	pr := &properties{}
	pr.p.Wnode.BufferSize = uint32(unsafe.Sizeof(*pr))
	pr.p.Wnode.Flags = wnodeFlagTracedGUID
	pr.p.Wnode.ClientContext = 1 // timestamps from the performance counter
	pr.p.LogFileMode = eventTraceRealTimeMode
	// Small buffers: Windows keeps them in the manager's memory, several for each processor (64 KB ones took tens of
	// MB on a big machine). The counting reads them each second, so a few small ones are enough.
	pr.p.BufferSize = 8 // KB each
	pr.p.MinimumBuffers = 4
	pr.p.MaximumBuffers = 16
	pr.p.FlushTimer = 1 // second
	pr.p.LoggerNameOffset = uint32(unsafe.Offsetof(pr.name))
	return pr
}

var (
	current atomic.Pointer[Monitor] // the one counting: Windows calls onEvent, which has no other way to it
	cbOnce  sync.Once
	cbPtr   uintptr
)

func eventCallback() uintptr {
	cbOnce.Do(func() { cbPtr = syscall.NewCallback(onEvent) })
	return cbPtr
}

// onEvent is called by Windows for every event of the session, on the thread running ProcessTrace.
func onEvent(rec *eventRecord) uintptr {
	if rec == nil || rec.EventHeader.ProviderID != kernelNetwork || rec.UserDataLength < 8 || rec.UserData == nil {
		return 0
	}
	var out bool
	switch rec.EventHeader.EventDescriptor.ID {
	case tcp4Sent, tcp6Sent, udp4Sent, udp6Sent:
		out = true
	case tcp4Received, tcp6Received, udp4Received, udp6Received:
	default:
		return 0
	}
	m := current.Load()
	if m == nil {
		return 0
	}
	d := rec.UserData
	pid := uint32(d[0]) | uint32(d[1])<<8 | uint32(d[2])<<16 | uint32(d[3])<<24
	size := uint32(d[4]) | uint32(d[5])<<8 | uint32(d[6])<<16 | uint32(d[7])<<24
	m.count(pid, size, out)
	if !loopback(rec.EventHeader.EventDescriptor.ID, unsafe.Slice(&rec.UserData[0], rec.UserDataLength)) {
		m.total(size, out)
	}
	return 0
}

// stopSession ends a trace session, by its handle, or by its name when the handle is 0.
func stopSession(handle uint64, name *uint16) error {
	pr := newProperties()
	n := uintptr(0)
	if handle == 0 {
		n = uintptr(unsafe.Pointer(name))
	}
	r, _, _ := procControlTraceW.Call(uintptr(handle), n, uintptr(unsafe.Pointer(pr)), eventTraceControlStop)
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

func explain(what string, err error) error {
	switch {
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return errors.New("szm must run as administrator (or as the service) to count the network of the servers")
	case errors.Is(err, errorNoSystemResources):
		return errors.New("Windows runs too many trace sessions to start one more: the network is not counted")
	}
	return fmt.Errorf("%s: %v", what, err)
}

// Start begins counting. It needs administrator rights.
func Start() (*Monitor, error) {
	if err := procStartTraceW.Find(); err != nil {
		return nil, ErrUnsupported
	}
	if current.Load() != nil {
		return nil, errors.New("the network is already counted")
	}
	name, _ := windows.UTF16PtrFromString(sessionName)
	var session uint64
	start := func() error {
		r, _, _ := procStartTraceW.Call(uintptr(unsafe.Pointer(&session)), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(newProperties())))
		if r != 0 {
			return syscall.Errno(r)
		}
		return nil
	}
	err := start()
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		// Left by a manager that ended without stopping it: stopped, then started again
		_ = stopSession(0, name)
		err = start()
	}
	if err != nil {
		return nil, explain("Windows did not start the trace session", err)
	}
	r, _, _ := procEnableTraceEx2.Call(uintptr(session), uintptr(unsafe.Pointer(&kernelNetwork)), eventControlCodeEnable,
		traceLevelInformation, uintptr(^uint64(0)), 0, 0, 0)
	if r != 0 {
		_ = stopSession(session, nil)
		return nil, explain("Windows did not turn on its network events", syscall.Errno(r))
	}

	m := newMonitor()
	current.Store(m)
	lf := &eventTraceLogfile{LoggerName: name, ProcessTraceMode: processTraceModeRealTime | processTraceModeEventRecord, Callback: eventCallback()}
	h, _, e := procOpenTraceW.Call(uintptr(unsafe.Pointer(lf)))
	trace := uint64(h)
	if trace == invalidProcessTraceHandle {
		current.CompareAndSwap(m, nil)
		_ = stopSession(session, nil)
		return nil, explain("Windows did not open the trace session", e)
	}

	go func() {
		// Windows calls onEvent on this thread until the session stops
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		r, _, _ := procProcessTrace.Call(uintptr(unsafe.Pointer(&trace)), 1, 0, 0)
		select {
		case <-m.quit:
		default:
			m.setErr(fmt.Errorf("Windows stopped the network count (%v)", syscall.Errno(r)))
		}
		runtime.KeepAlive(lf)
		runtime.KeepAlive(name)
		close(m.ended)
	}()
	m.statsOS = func() (TraceStats, bool) {
		pr := newProperties()
		if r, _, _ := procControlTraceW.Call(uintptr(session), 0, uintptr(unsafe.Pointer(pr)), eventTraceControlQuery); r != 0 {
			return TraceStats{}, false
		}
		return TraceStats{BufferKB: pr.p.BufferSize, Buffers: pr.p.NumberOfBuffers, Free: pr.p.FreeBuffers,
			Lost: pr.p.EventsLost + pr.p.RealTimeBuffersLost}, true
	}
	m.stopOS = func() {
		_ = stopSession(session, nil)
		_, _, _ = procCloseTrace.Call(uintptr(trace))
		select {
		case <-m.ended:
		case <-time.After(3 * time.Second):
		}
		current.CompareAndSwap(m, nil)
	}
	go m.sampler()
	return m, nil
}
