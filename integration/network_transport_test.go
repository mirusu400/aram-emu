package integration

import (
	"context"
	"image"
	"io"
	"net"
	"testing"
	"time"

	aramcore "github.com/mirusu400/aram-core/core"
)

func TestNetworkTransportConnectsWritesBuffersAndDisconnects(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })
	dialed := make(chan string, 1)
	transport := newNetworkTransport(func(_ context.Context, network, address string) (net.Conn, error) {
		dialed <- network + ":" + address
		return client, nil
	})
	t.Cleanup(transport.reset)
	guest := &networkGuestMachine{state: aramcore.StateRunning}
	guest.requests = []aramcore.NetworkRequest{{
		Sequence: 1, Connection: 7, Kind: aramcore.NetworkRequestConnect,
		Host: "127.0.0.1", Port: 8501, MaxReceiveBytes: 1024,
	}}

	if err := transport.sync(guest); err != nil {
		t.Fatal(err)
	}
	waitNetwork(t, transport, guest, func() bool { return len(guest.completions) == 1 })
	if got := <-dialed; got != "tcp:127.0.0.1:8501" {
		t.Fatalf("dial target = %q", got)
	}
	if completion := guest.completions[0]; completion.sequence != 1 || !completion.success {
		t.Fatalf("connect completion = %+v", completion)
	}

	written := make(chan []byte, 1)
	go func() {
		data := make([]byte, 5)
		_, err := io.ReadFull(server, data)
		if err != nil {
			written <- nil
			return
		}
		written <- data
	}()
	guest.requests = []aramcore.NetworkRequest{{
		Sequence: 2, Connection: 7, Kind: aramcore.NetworkRequestWrite,
		Data: []byte("hello"),
	}}
	if err := transport.sync(guest); err != nil {
		t.Fatal(err)
	}
	waitNetwork(t, transport, guest, func() bool { return len(guest.completions) == 2 })
	if got := string(<-written); got != "hello" {
		t.Fatalf("socket write = %q", got)
	}

	writeDone := make(chan error, 1)
	go func() {
		_, err := server.Write([]byte("paused"))
		writeDone <- err
	}()
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("reader did not buffer paused data")
	}
	if len(guest.received) != 0 {
		t.Fatalf("data reached paused guest early: %q", guest.received)
	}
	waitNetwork(t, transport, guest, func() bool { return len(guest.received) == 1 })
	if got := string(guest.received[0]); got != "paused" {
		t.Fatalf("received data = %q", got)
	}

	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	waitNetwork(t, transport, guest, func() bool { return len(guest.disconnected) == 1 })
	if guest.disconnected[0] != 7 {
		t.Fatalf("disconnected connection = %d", guest.disconnected[0])
	}
}

func waitNetwork(
	t *testing.T,
	transport *networkTransport,
	guest *networkGuestMachine,
	done func() bool,
) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for network transport")
		}
		if err := transport.sync(guest); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
}

type networkCompletion struct {
	sequence uint64
	success  bool
}

type networkGuestMachine struct {
	state        aramcore.State
	requests     []aramcore.NetworkRequest
	completions  []networkCompletion
	received     [][]byte
	disconnected []uint64
}

func (machine *networkGuestMachine) NetworkRequests() []aramcore.NetworkRequest {
	result := append([]aramcore.NetworkRequest(nil), machine.requests...)
	for index := range result {
		result[index].Data = append([]byte(nil), result[index].Data...)
	}
	return result
}

func (machine *networkGuestMachine) CompleteNetworkRequest(sequence uint64, success bool) error {
	machine.completions = append(machine.completions, networkCompletion{sequence: sequence, success: success})
	for index, request := range machine.requests {
		if request.Sequence == sequence {
			machine.requests = append(machine.requests[:index], machine.requests[index+1:]...)
			break
		}
	}
	return nil
}

func (machine *networkGuestMachine) ReceiveNetworkData(_ uint64, data []byte) error {
	machine.received = append(machine.received, append([]byte(nil), data...))
	return nil
}

func (machine *networkGuestMachine) DisconnectNetwork(connection uint64) error {
	machine.disconnected = append(machine.disconnected, connection)
	return nil
}

func (machine *networkGuestMachine) Load(context.Context, aramcore.Source) error { return nil }
func (machine *networkGuestMachine) State() aramcore.State                       { return machine.state }
func (machine *networkGuestMachine) Start(context.Context) error                 { return nil }
func (machine *networkGuestMachine) Pause() error                                { return nil }
func (machine *networkGuestMachine) Resume() error                               { return nil }
func (machine *networkGuestMachine) Stop() error                                 { return nil }
func (machine *networkGuestMachine) Reset(context.Context) error                 { return nil }
func (machine *networkGuestMachine) StepFrame(context.Context) error             { return nil }
func (machine *networkGuestMachine) QueueInput(aramcore.InputEvent) error        { return nil }
func (machine *networkGuestMachine) Framebuffer() image.Image                    { return nil }
func (machine *networkGuestMachine) DrainAudio() aramcore.AudioChunk             { return aramcore.AudioChunk{} }
func (machine *networkGuestMachine) SaveState(io.Writer) error                   { return nil }
func (machine *networkGuestMachine) LoadState(io.Reader) error                   { return nil }
func (machine *networkGuestMachine) Close() error                                { return nil }

var _ aramcore.Machine = (*networkGuestMachine)(nil)
var _ aramcore.NetworkTransportMachine = (*networkGuestMachine)(nil)
