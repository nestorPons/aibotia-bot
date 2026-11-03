package pipeline

import (
	"sync"
	"time"
)

// BufferedMessage es un mensaje acumulado durante la ventana de debounce.
type BufferedMessage struct {
	ID        string
	Sender    string
	Text      string
	HasMedia  bool
	MediaType string
	Timestamp time.Time
}

// debouncer agrupa mensajes rápidos por chat antes de procesarlos.
type debouncer struct {
	mu       sync.Mutex
	timers   map[string]*time.Timer
	buf      map[string][]BufferedMessage
	minDelay time.Duration
	maxDelay time.Duration
	onFlush  func(chatID string, msgs []BufferedMessage)
}

func newDebouncer(minDelay, maxDelay time.Duration, onFlush func(string, []BufferedMessage)) *debouncer {
	return &debouncer{
		timers:   make(map[string]*time.Timer),
		buf:      make(map[string][]BufferedMessage),
		minDelay: minDelay,
		maxDelay: maxDelay,
		onFlush:  onFlush,
	}
}

func (d *debouncer) add(chatID string, msg BufferedMessage) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.buf[chatID] = append(d.buf[chatID], msg)
	if t, ok := d.timers[chatID]; ok {
		t.Stop()
	}
	d.timers[chatID] = time.AfterFunc(d.delay(len(d.buf[chatID])), func() {
		d.flush(chatID)
	})
}

func (d *debouncer) delay(count int) time.Duration {
	delay := d.minDelay + time.Duration(count-1)*1500*time.Millisecond
	if delay > d.maxDelay {
		delay = d.maxDelay
	}
	if delay < d.minDelay {
		delay = d.minDelay
	}
	return delay
}

func (d *debouncer) flush(chatID string) {
	d.mu.Lock()
	msgs := d.buf[chatID]
	delete(d.buf, chatID)
	delete(d.timers, chatID)
	d.mu.Unlock()

	if len(msgs) == 0 {
		return
	}
	d.onFlush(chatID, msgs)
}

func (d *debouncer) stopAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, t := range d.timers {
		t.Stop()
	}
	d.timers = make(map[string]*time.Timer)
}

// cancel descarta los mensajes pendientes de un chat.
func (d *debouncer) cancel(chatID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.timers[chatID]; ok {
		t.Stop()
		delete(d.timers, chatID)
	}
	delete(d.buf, chatID)
}
