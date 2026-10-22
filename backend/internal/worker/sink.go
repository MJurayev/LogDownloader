package worker

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"

	"logdownloader/internal/model"
)

// partTracker qism chegaralarini qator chegarasida aniqlaydi. rawSink har
// Write chaqiruvida aynan bitta qator yozadi, shu sababli faylni qayta
// o'qimasdan "joriy qism + shu qator > limit" qoidasi bilan chegara qo'yish
// mumkin. Bitta qator limitdan katta bo'lsa o'zi alohida qism bo'ladi.
type partTracker struct {
	w        io.Writer
	limit    int64
	offset   int64   // jami yozilgan baytlar (mantiqiy, bufio'dan oldin)
	partSize int64   // joriy qismdagi baytlar
	bounds   []int64 // har qismning boshlanish offseti; birinchi doim 0
}

func newPartTracker(w io.Writer, limit int64) *partTracker {
	return &partTracker{w: w, limit: limit, bounds: []int64{0}}
}

func (p *partTracker) Write(b []byte) (int, error) {
	if p.limit > 0 && p.partSize > 0 && p.partSize+int64(len(b)) > p.limit {
		p.bounds = append(p.bounds, p.offset)
		p.partSize = 0
	}
	n, err := p.w.Write(b)
	p.offset += int64(n)
	p.partSize += int64(n)
	return n, err
}

type trackerSnap struct {
	offset, partSize int64
	nBounds          int
}

func (p *partTracker) snapshot() trackerSnap {
	return trackerSnap{offset: p.offset, partSize: p.partSize, nBounds: len(p.bounds)}
}

func (p *partTracker) restore(s trackerSnap) {
	p.offset = s.offset
	p.partSize = s.partSize
	if len(p.bounds) > s.nBounds {
		p.bounds = p.bounds[:s.nBounds]
	}
}

// parts — [start, end) juftliklar. Bo'sh export → bitta bo'sh qism.
func (p *partTracker) parts() [][2]int64 {
	res := make([][2]int64, 0, len(p.bounds))
	for i, start := range p.bounds {
		end := p.offset
		if i+1 < len(p.bounds) {
			end = p.bounds[i+1]
		}
		if end > start || (len(res) == 0 && i == len(p.bounds)-1) {
			res = append(res, [2]int64{start, end})
		}
	}
	return res
}

// rawSink — 1-faza chiqishi: transformatsiya qilingan qatorlar `.tmp` faylga,
// qism chegaralari tracker'da. mark/rollback oyna qayta urinishi uchun:
// oyna o'rtasida uzilsa fayl oyna boshiga qaytariladi (truncate).
type rawSink struct {
	path    string
	f       *os.File
	bufW    *bufio.Writer
	tracker *partTracker
	lines   int64
	content string
	fields  []string
	csvBuf  bytes.Buffer
	csvW    *csv.Writer
}

type sinkMark struct {
	tr    trackerSnap
	lines int64
}

func newRawSink(path string, chunkBytes int64, content string, fields []string) (*rawSink, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	s := &rawSink{path: path, f: f, content: content, fields: fields}
	s.bufW = bufio.NewWriterSize(f, 256*1024)
	s.tracker = newPartTracker(s.bufW, chunkBytes)
	s.csvW = csv.NewWriter(&s.csvBuf)
	return s, nil
}

// mark — oyna boshidagi holat. Oldin bufer flush qilinadi: shunda diskdagi
// hajm mantiqiy offset bilan teng bo'ladi va rollback (Reset + Truncate)
// faqat shu oynaning ma'lumotini tashlaydi, oldingi oynalarnikini emas.
func (s *rawSink) mark() (sinkMark, error) {
	if err := s.bufW.Flush(); err != nil {
		return sinkMark{}, err
	}
	return sinkMark{tr: s.tracker.snapshot(), lines: s.lines}, nil
}

// rollback — bufferni tashlab, faylni mark offsetiga qisqartiradi.
func (s *rawSink) rollback(m sinkMark) error {
	// bufio'dagi yozilmagan baytlar bekor qilinadi (Reset), diskdagi qism truncate.
	s.bufW.Reset(s.f)
	if err := s.f.Truncate(m.tr.offset); err != nil {
		return err
	}
	if _, err := s.f.Seek(m.tr.offset, io.SeekStart); err != nil {
		return err
	}
	s.tracker.restore(m.tr)
	s.lines = m.lines
	return nil
}

// writeLine — bitta xom NDJSON qatorni (oxirida \n bo'lishi shart emas)
// tarkib qoidasiga ko'ra o'zgartirib yozadi.
func (s *rawSink) writeLine(line []byte) error {
	out := s.transform(line)
	if len(out) == 0 {
		return nil
	}
	if _, err := s.tracker.Write(out); err != nil {
		return err
	}
	s.lines++
	return nil
}

func (s *rawSink) flush() error { return s.bufW.Flush() }

func (s *rawSink) close() error {
	if err := s.bufW.Flush(); err != nil {
		s.f.Close()
		return err
	}
	return s.f.Close()
}

func (s *rawSink) size() int64 { return s.tracker.offset }

func ensureNewline(b []byte) []byte {
	if len(b) == 0 || b[len(b)-1] != '\n' {
		return append(b, '\n')
	}
	return b
}

// transform — content bo'yicha qator transformatsiyasi. JSON parse qilib
// bo'lmasa qator o'zgarishsiz (ndjson) yoki matn sifatida yoziladi.
func (s *rawSink) transform(line []byte) []byte {
	trimmed := bytes.TrimRight(line, "\r\n")
	if len(trimmed) == 0 {
		return nil
	}
	if s.content == model.ContentNDJSON && len(s.fields) == 0 {
		return ensureNewline(append([]byte(nil), trimmed...))
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return ensureNewline(append([]byte(nil), trimmed...))
	}
	switch s.content {
	case model.ContentMsg:
		return ensureNewline([]byte(rawToString(obj["_msg"])))
	case model.ContentCSV:
		record := make([]string, len(s.fields))
		for i, f := range s.fields {
			record[i] = rawToString(obj[f])
		}
		s.csvBuf.Reset()
		s.csvW.Write(record)
		s.csvW.Flush()
		return append([]byte(nil), s.csvBuf.Bytes()...)
	default: // ndjson + fields
		var b bytes.Buffer
		b.WriteByte('{')
		first := true
		for _, f := range s.fields {
			raw, ok := obj[f]
			if !ok {
				continue
			}
			if !first {
				b.WriteByte(',')
			}
			first = false
			k, _ := json.Marshal(f)
			b.Write(k)
			b.WriteByte(':')
			b.Write(raw)
		}
		b.WriteString("}\n")
		return b.Bytes()
	}
}

// rawToString — JSON string bo'lsa quote'siz matn, aks holda xom JSON.
func rawToString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return s
		}
	}
	return string(raw)
}

// csvHeader — CSV sarlavha qatori (har qism/fayl boshiga qo'yiladi).
func csvHeader(fields []string) []byte {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Write(fields)
	w.Flush()
	return buf.Bytes()
}
