package object

import (
	"errors"
	"testing"
	"time"
)

func TestChannelSendReceiveAndClose(t *testing.T) {
	ch := NewChannel(1)
	if err := ch.Send(Int{Value: 7}); err != nil {
		t.Fatal(err)
	}
	value, open := ch.Receive()
	if !open || value != (Int{Value: 7}) {
		t.Fatalf("receive = (%v, %v)", value, open)
	}
	if err := ch.Close(); err != nil {
		t.Fatal(err)
	}
	if _, open := ch.Receive(); open {
		t.Fatal("closed channel reported open")
	}
	if err := ch.Send(Int{Value: 1}); err == nil {
		t.Fatal("send on closed channel succeeded")
	}
	if err := ch.Close(); err == nil {
		t.Fatal("closing a closed channel succeeded")
	}
}

func TestChannelSendBlocksUntilReceive(t *testing.T) {
	ch := NewChannel(0)
	done := make(chan struct{})
	go func() {
		if err := ch.Send(Int{Value: 9}); err != nil {
			t.Errorf("send: %v", err)
		}
		close(done)
	}()
	value, open := ch.Receive()
	if !open || value != (Int{Value: 9}) {
		t.Fatalf("receive = (%v, %v)", value, open)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sender did not complete")
	}
}

func TestChannelCloseWakesBlockedSender(t *testing.T) {
	ch := NewChannel(0)
	done := make(chan error, 1)
	go func() { done <- ch.Send(Int{Value: 3}) }()
	if err := ch.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("blocked sender succeeded after close")
		}
	case <-time.After(time.Second):
		t.Fatal("blocked sender did not wake")
	}
}

func TestChannelContextCancellationWakesBlockedOperations(t *testing.T) {
	ch := NewChannel(0)
	cancel := make(chan struct{})
	sendDone := make(chan error, 1)
	go func() { sendDone <- ch.SendContext(cancel, Int{Value: 3}) }()
	close(cancel)
	select {
	case err := <-sendDone:
		if err != ErrCancelled {
			t.Fatalf("send error = %v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled sender did not wake")
	}

	cancel = make(chan struct{})
	receiveDone := make(chan error, 1)
	go func() {
		_, _, err := ch.ReceiveContext(cancel)
		receiveDone <- err
	}()
	close(cancel)
	select {
	case err := <-receiveDone:
		if err != ErrCancelled {
			t.Fatalf("receive error = %v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled receiver did not wake")
	}
}

func TestCancelledTaskReportsCancellation(t *testing.T) {
	task := NewTask()
	task.Cancel()
	task.Complete(Int{Value: 42}, nil)
	value, err := task.Await()
	if value != NilValue || err != ErrCancelled {
		t.Fatalf("await = (%v, %v), want cancellation", value, err)
	}
	if !task.Cancelled() {
		t.Fatal("task lost its cancellation state")
	}
	task.Cancel()
}
func TestTaskCompletesOnce(t *testing.T) {
	task := NewTask()
	go task.Complete(Int{Value: 42}, nil)
	value, err := task.Await()
	if err != nil || value != (Int{Value: 42}) {
		t.Fatalf("await = (%v, %v)", value, err)
	}
	task.Complete(NilValue, errors.New("late"))
	if value, err := task.Await(); err != nil || value != (Int{Value: 42}) {
		t.Fatalf("second await = (%v, %v)", value, err)
	}
}
