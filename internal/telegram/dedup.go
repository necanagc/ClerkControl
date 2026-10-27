package telegram

import (
	"sync"
	"time"
)

// dedup помнит последние обработанные update_id.
// Нужен потому, что webhook доставляет апдейт повторно, если не получил 200,
// а при polling защищает от повторной обработки после сбоя.
type dedup struct {
	mu   sync.Mutex
	seen map[int]time.Time
	ttl  time.Duration
	max  int
}

func newDedup(ttl time.Duration, max int) *dedup {
	return &dedup{
		seen: make(map[int]time.Time, max),
		ttl:  ttl,
		max:  max,
	}
}

// first возвращает true, если апдейт ещё не обрабатывался, и запоминает его.
func (d *dedup) first(id int) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if seenAt, ok := d.seen[id]; ok && time.Since(seenAt) < d.ttl {
		return false
	}

	d.seen[id] = time.Now()
	d.gcLocked()

	return true
}

// gcLocked чистит устаревшие записи; вызывается под уже взятым мьютексом.
func (d *dedup) gcLocked() {
	if len(d.seen) < d.max {
		return
	}

	cutoff := time.Now().Add(-d.ttl)
	for id, seenAt := range d.seen {
		if seenAt.Before(cutoff) {
			delete(d.seen, id)
		}
	}

	// Всё ещё много — удаляем произвольные записи, чтобы не расти бесконечно.
	for id := range d.seen {
		if len(d.seen) <= d.max {
			break
		}
		delete(d.seen, id)
	}
}
