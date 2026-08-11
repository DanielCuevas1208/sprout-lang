package object

import (
	"errors"
	"fmt"
	"sync"
)

// ErrCancelled reports a cooperative cancellation request.
var ErrCancelled = errors.New("task cancelled")

// Channel is a synchronization point for Sprout values.
//
// The channel uses blocking send and receive behavior. A close signal wakes
// blocked operations without closing the value queue under a concurrent send.
type Channel struct {
	values  chan Object
	done    chan struct{}
	mu      sync.Mutex
	senders sync.WaitGroup
	closed  bool
}

// NewChannel creates a channel with capacity. Zero creates a rendezvous
// channel.
func NewChannel(capacity int) *Channel {
	return &Channel{
		values: make(chan Object, capacity),
		done:   make(chan struct{}),
	}
}

// Type reports the runtime type of a channel.
func (c *Channel) Type() Type { return TypeChannel }

// String renders a channel without exposing its current state.
func (c *Channel) String() string { return "<channel>" }

// Send blocks until a receiver accepts value or the channel is closed.
func (c *Channel) Send(value Object) error {
	return c.SendContext(nil, value)
}

// SendContext sends value until a receiver accepts it, the channel closes, or
// done is closed. A nil done channel disables cancellation.
func (c *Channel) SendContext(done <-chan struct{}, value Object) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("cannot send on a closed channel")
	}
	c.senders.Add(1)
	closed := c.done
	c.mu.Unlock()
	defer c.senders.Done()

	select {
	case <-done:
		return ErrCancelled
	case c.values <- value:
		return nil
	case <-closed:
		return fmt.Errorf("cannot send on a closed channel")
	}
}

// Receive blocks until a value arrives or the channel closes. The boolean is
// false only after the channel is closed and has no buffered values.
func (c *Channel) Receive() (Object, bool) {
	value, open, _ := c.ReceiveContext(nil)
	return value, open
}

// ReceiveContext waits for a value until the channel closes or done closes.
// A nil done channel disables cancellation.
func (c *Channel) ReceiveContext(done <-chan struct{}) (Object, bool, error) {
	select {
	case value := <-c.values:
		return value, true, nil
	case <-c.done:
		select {
		case value := <-c.values:
			return value, true, nil
		default:
			return NilValue, false, nil
		}
	case <-done:
		return NilValue, false, ErrCancelled
	}
}

// Close closes the channel. Closing an already closed channel is an error.
func (c *Channel) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("channel is already closed")
	}
	c.closed = true
	close(c.done)
	c.mu.Unlock()
	c.senders.Wait()
	return nil
}

// Task is a handle for one spawned function.
//
// A task completes exactly once. Its result carries either the function's
// return value or the error raised by the child execution context.
type Task struct {
	done      chan struct{}
	cancel    chan struct{}
	mu        sync.Mutex
	completed bool
	cancelled bool
	result    taskResult
}

type taskResult struct {
	value Object
	err   error
}

// NewTask creates a task that has not completed.
func NewTask() *Task {
	return &Task{done: make(chan struct{}), cancel: make(chan struct{})}
}

// Complete records the task result. Only the first completion is kept.
func (t *Task) Complete(value Object, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.completed {
		return
	}
	if t.cancelled {
		value, err = NilValue, ErrCancelled
	}
	t.result = taskResult{value: value, err: err}
	t.completed = true
	close(t.done)
}

// Await blocks until the task completes and returns its result.
func (t *Task) Await() (Object, error) {
	return t.AwaitContext(nil)
}

// AwaitContext waits for completion until done closes. A nil done channel
// waits without cancellation.
func (t *Task) AwaitContext(done <-chan struct{}) (Object, error) {
	select {
	case <-t.done:
	case <-done:
		return NilValue, ErrCancelled
	}
	return t.result.value, t.result.err
}

// Cancel requests cooperative cancellation.
func (t *Task) Cancel() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.completed || t.cancelled {
		return
	}
	t.cancelled = true
	close(t.cancel)
}

// Cancelled reports whether cancellation was requested before completion.
func (t *Task) Cancelled() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cancelled
}

// Done returns the signal closed by Cancel.
func (t *Task) Done() <-chan struct{} { return t.cancel }

// Type reports the runtime type of a task.
func (t *Task) Type() Type { return TypeTask }

// String renders a task handle.
func (t *Task) String() string { return "<task>" }
