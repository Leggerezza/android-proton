// SPDX-License-Identifier: GPL-3.0-or-later
package protonnode

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ringLog keeps the last N protocol events for RecentLogs(). PROMPT.md 5.1
// asks for 500 lines; nothing here is ever written to disk by the core.
type ringLog struct {
	mu    sync.Mutex
	buf   []string
	next  int
	full  bool
	limit int
}

func newRingLog(limit int) *ringLog {
	if limit <= 0 {
		limit = 500
	}
	return &ringLog{limit: limit, buf: make([]string, 0, limit)}
}

func (r *ringLog) add(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.buf) < r.limit {
		r.buf = append(r.buf, line)
		return
	}
	r.buf[r.next] = line
	r.next = (r.next + 1) % r.limit
	r.full = true
}

// tail returns the newest n lines, oldest first.
func (r *ringLog) tail(n int) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	ordered := make([]string, 0, len(r.buf))
	if r.full {
		ordered = append(ordered, r.buf[r.next:]...)
		ordered = append(ordered, r.buf[:r.next]...)
	} else {
		ordered = append(ordered, r.buf...)
	}
	if n > 0 && len(ordered) > n {
		ordered = ordered[len(ordered)-n:]
	}
	return ordered
}

var (
	reEmail = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	// Anything base64ish and long is treated as a credential.
	reToken = regexp.MustCompile(`[A-Za-z0-9_\-+/]{28,}={0,2}`)
)

// redact removes account identifiers and credentials from a log line. It runs
// on every line that reaches the ring buffer, so nothing downstream has to
// remember to mask (PROMPT.md 0.5: logs are redacted by default).
func redact(s string) string {
	s = reEmail.ReplaceAllStringFunc(s, func(m string) string {
		at := strings.IndexByte(m, '@')
		if at <= 0 {
			return "***"
		}
		local := m[:at]
		if len(local) > 2 {
			local = local[:2]
		}
		return local + "***" + m[at:]
	})
	s = reToken.ReplaceAllStringFunc(s, func(m string) string {
		if len(m) < 28 {
			return m
		}
		return m[:4] + "…" + fmt.Sprintf("(%d)", len(m))
	})
	return s
}

func (r *ringLog) logf(format string, args ...any) {
	line := time.Now().Format("15:04:05.000") + " " + fmt.Sprintf(format, args...)
	r.add(redact(line))
}
