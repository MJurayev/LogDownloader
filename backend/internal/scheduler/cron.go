// Package scheduler — rejali exportlar: minimal cron parser + ishga tushirish sikli.
package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Cron — 5 maydonli ifoda: minute hour day-of-month month day-of-week.
// Qo'llanadi: `*`, `n`, `a-b`, `*/n`, `a-b/n`, vergul ro'yxati; dow 0–7 (7 = yakshanba).
type Cron struct {
	minute [60]bool
	hour   [24]bool
	dom    [32]bool
	month  [13]bool
	dow    [7]bool
	expr   string
}

var presets = map[string]string{
	"@hourly":  "0 * * * *",
	"@daily":   "0 0 * * *",
	"@weekly":  "0 0 * * 0",
	"@monthly": "0 0 1 * *",
}

func Parse(expr string) (*Cron, error) {
	expr = strings.TrimSpace(expr)
	if p, ok := presets[expr]; ok {
		expr = p
	}
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron: 5 ta maydon kutiladi (min hour dom month dow), berilgan: %q", expr)
	}
	c := &Cron{expr: expr}
	var err error
	if err = parseField(fields[0], 0, 59, c.minute[:]); err != nil {
		return nil, fmt.Errorf("cron minute: %w", err)
	}
	if err = parseField(fields[1], 0, 23, c.hour[:]); err != nil {
		return nil, fmt.Errorf("cron hour: %w", err)
	}
	if err = parseField(fields[2], 1, 31, c.dom[:]); err != nil {
		return nil, fmt.Errorf("cron day: %w", err)
	}
	if err = parseField(fields[3], 1, 12, c.month[:]); err != nil {
		return nil, fmt.Errorf("cron month: %w", err)
	}
	var dow [8]bool
	if err = parseField(fields[4], 0, 7, dow[:]); err != nil {
		return nil, fmt.Errorf("cron weekday: %w", err)
	}
	for i := 0; i < 7; i++ {
		c.dow[i] = dow[i]
	}
	if dow[7] {
		c.dow[0] = true
	}
	return c, nil
}

func (c *Cron) String() string { return c.expr }

func parseField(f string, min, max int, out []bool) error {
	for _, part := range strings.Split(f, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return fmt.Errorf("bo'sh qism")
		}
		step := 1
		if i := strings.IndexByte(part, '/'); i >= 0 {
			s, err := strconv.Atoi(part[i+1:])
			if err != nil || s <= 0 {
				return fmt.Errorf("noto'g'ri qadam %q", part)
			}
			step = s
			part = part[:i]
		}
		lo, hi := min, max
		if part != "*" {
			if i := strings.IndexByte(part, '-'); i >= 0 {
				a, err1 := strconv.Atoi(part[:i])
				b, err2 := strconv.Atoi(part[i+1:])
				if err1 != nil || err2 != nil {
					return fmt.Errorf("noto'g'ri oraliq %q", part)
				}
				lo, hi = a, b
			} else {
				v, err := strconv.Atoi(part)
				if err != nil {
					return fmt.Errorf("noto'g'ri qiymat %q", part)
				}
				lo, hi = v, v
				if step > 1 {
					hi = max
				}
			}
		}
		if lo < min || hi > max || lo > hi {
			return fmt.Errorf("qiymat %d–%d oralig'ida bo'lishi kerak: %q", min, max, part)
		}
		for v := lo; v <= hi; v += step {
			out[v] = true
		}
	}
	return nil
}

// Next — `after` dan keyingi (qat'iy katta) mos vaqt, daqiqa aniqligida.
// Mahalliy vaqt zonasida ishlaydi. 5 yil ichida topilmasa nol vaqt qaytaradi.
func (c *Cron) Next(after time.Time) time.Time {
	t := after.Truncate(time.Minute).Add(time.Minute)
	limit := after.AddDate(5, 0, 0)
	for t.Before(limit) {
		if !c.month[int(t.Month())] {
			// keyingi oy boshi
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !c.dom[t.Day()] || !c.dow[int(t.Weekday())] {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !c.hour[t.Hour()] {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}
		if !c.minute[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}
		return t
	}
	return time.Time{}
}
