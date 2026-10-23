package integration

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	aramcore "github.com/mirusu400/aram-core/core"
)

const (
	networkEventCapacity = 256
	networkWriteCapacity = 1024
	networkDialTimeout   = 10 * time.Second
	networkWriteTimeout  = 10 * time.Second
)

type networkDialFunc func(context.Context, string, string) (net.Conn, error)

type networkEventKind uint8

const (
	networkEventComplete networkEventKind = iota + 1
	networkEventData
	networkEventDisconnect
)

type networkEvent struct {
	kind       networkEventKind
	session    *networkSession
	sequence   uint64
	connection uint64
	success    bool
	data       []byte
}

type networkWrite struct {
	sequence uint64
	data     []byte
}

type networkSession struct {
	mu              sync.Mutex
	connection      uint64
	maxReceiveBytes uint32
	ctx             context.Context
	cancel          context.CancelFunc
	conn            net.Conn
	ready           bool
	closed          bool
	writes          chan networkWrite
}

type networkTransport struct {
	dial     networkDialFunc
	events   chan networkEvent
	sessions map[uint64]*networkSession
	started  map[uint64]uint64
}

func newNetworkTransport(dial networkDialFunc) *networkTransport {
	if dial == nil {
		dialer := &net.Dialer{}
		dial = dialer.DialContext
	}
	return &networkTransport{
		dial: dial, events: make(chan networkEvent, networkEventCapacity),
		sessions: make(map[uint64]*networkSession),
		started:  make(map[uint64]uint64),
	}
}

func (transport *networkTransport) sync(machine aramcore.Machine) error {
	if transport == nil {
		return nil
	}
	guest, ok := machine.(aramcore.NetworkTransportMachine)
	if !ok {
		transport.reset()
		return nil
	}
	if err := transport.startRequests(guest); err != nil {
		return err
	}
	for delivered := 0; delivered < networkEventCapacity; delivered++ {
		select {
		case event := <-transport.events:
			if !transport.current(event.session) {
				continue
			}
			switch event.kind {
			case networkEventComplete:
				if transport.started[event.sequence] != event.connection {
					continue
				}
				if !networkRequestPending(guest.NetworkRequests(), event.sequence, event.connection) {
					delete(transport.started, event.sequence)
					continue
				}
				if err := guest.CompleteNetworkRequest(event.sequence, event.success); err != nil {
					return err
				}
				delete(transport.started, event.sequence)
				if !event.success {
					transport.closeSession(event.connection)
				}
			case networkEventData:
				if err := guest.ReceiveNetworkData(event.connection, event.data); err != nil {
					return err
				}
			case networkEventDisconnect:
				if err := guest.DisconnectNetwork(event.connection); err != nil {
					return err
				}
				transport.closeSession(event.connection)
			}
			if err := transport.startRequests(guest); err != nil {
				return err
			}
		default:
			return nil
		}
	}
	return nil
}

func (transport *networkTransport) startRequests(guest aramcore.NetworkTransportMachine) error {
	requests := guest.NetworkRequests()
	for _, request := range requests {
		if err := request.Validate(); err != nil {
			return fmt.Errorf("invalid guest network request: %w", err)
		}
		if _, started := transport.started[request.Sequence]; started {
			continue
		}
		switch request.Kind {
		case aramcore.NetworkRequestConnect:
			transport.startConnect(request)
		case aramcore.NetworkRequestWrite:
			if !transport.startWrite(request) {
				return nil
			}
		case aramcore.NetworkRequestDisconnect:
			transport.closeSession(request.Connection)
			if err := guest.CompleteNetworkRequest(request.Sequence, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func (transport *networkTransport) startConnect(request aramcore.NetworkRequest) {
	transport.closeSession(request.Connection)
	ctx, cancel := context.WithCancel(context.Background())
	session := &networkSession{
		connection: request.Connection, maxReceiveBytes: request.MaxReceiveBytes,
		ctx: ctx, cancel: cancel, writes: make(chan networkWrite, networkWriteCapacity),
	}
	transport.sessions[request.Connection] = session
	transport.started[request.Sequence] = request.Connection
	go transport.runConnection(session, request)
}

func (transport *networkTransport) startWrite(request aramcore.NetworkRequest) bool {
	session := transport.sessions[request.Connection]
	if session == nil {
		return false
	}
	session.mu.Lock()
	ready := session.ready && !session.closed
	session.mu.Unlock()
	if !ready {
		return false
	}
	write := networkWrite{sequence: request.Sequence, data: append([]byte(nil), request.Data...)}
	select {
	case session.writes <- write:
		transport.started[request.Sequence] = request.Connection
		return true
	default:
		return false
	}
}

func (transport *networkTransport) runConnection(session *networkSession, request aramcore.NetworkRequest) {
	dialCtx, cancel := context.WithTimeout(session.ctx, networkDialTimeout)
	conn, err := transport.dial(dialCtx, "tcp", net.JoinHostPort(request.Host, strconv.Itoa(int(request.Port))))
	cancel()
	if err != nil {
		transport.emit(session, networkEvent{
			kind: networkEventComplete, sequence: request.Sequence,
			connection: request.Connection, success: false,
		})
		return
	}
	session.mu.Lock()
	if session.closed {
		session.mu.Unlock()
		_ = conn.Close()
		return
	}
	session.conn, session.ready = conn, true
	session.mu.Unlock()
	if !transport.emit(session, networkEvent{
		kind: networkEventComplete, sequence: request.Sequence,
		connection: request.Connection, success: true,
	}) {
		_ = conn.Close()
		return
	}
	go transport.runWriter(session, conn)
	transport.runReader(session, conn)
}

func (transport *networkTransport) runWriter(session *networkSession, conn net.Conn) {
	for {
		select {
		case <-session.ctx.Done():
			return
		case write := <-session.writes:
			_ = conn.SetWriteDeadline(time.Now().Add(networkWriteTimeout))
			_, err := io.Copy(conn, bytes.NewReader(write.data))
			_ = conn.SetWriteDeadline(time.Time{})
			if !transport.emit(session, networkEvent{
				kind: networkEventComplete, sequence: write.sequence,
				connection: session.connection, success: err == nil,
			}) || err != nil {
				return
			}
		}
	}
}

func (transport *networkTransport) runReader(session *networkSession, conn net.Conn) {
	buffer := make([]byte, int(session.maxReceiveBytes))
	for {
		read, err := conn.Read(buffer)
		if read > 0 && !transport.emit(session, networkEvent{
			kind: networkEventData, connection: session.connection,
			data: append([]byte(nil), buffer[:read]...),
		}) {
			return
		}
		if err != nil {
			transport.emit(session, networkEvent{kind: networkEventDisconnect, connection: session.connection})
			return
		}
	}
}

func (transport *networkTransport) emit(session *networkSession, event networkEvent) bool {
	event.session = session
	select {
	case transport.events <- event:
		return true
	case <-session.ctx.Done():
		return false
	}
}

func (transport *networkTransport) current(session *networkSession) bool {
	return session != nil && transport.sessions[session.connection] == session
}

func (transport *networkTransport) closeSession(connection uint64) {
	session := transport.sessions[connection]
	if session == nil {
		return
	}
	delete(transport.sessions, connection)
	for sequence, requestConnection := range transport.started {
		if requestConnection == connection {
			delete(transport.started, sequence)
		}
	}
	session.mu.Lock()
	session.closed = true
	session.cancel()
	conn := session.conn
	session.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

func (transport *networkTransport) reset() {
	if transport == nil {
		return
	}
	for connection := range transport.sessions {
		transport.closeSession(connection)
	}
	clear(transport.started)
	for {
		select {
		case <-transport.events:
		default:
			return
		}
	}
}

func networkRequestPending(requests []aramcore.NetworkRequest, sequence, connection uint64) bool {
	for _, request := range requests {
		if request.Sequence == sequence && request.Connection == connection {
			return true
		}
	}
	return false
}
