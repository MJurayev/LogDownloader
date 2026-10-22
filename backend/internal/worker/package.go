package worker

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"logdownloader/internal/model"
	"logdownloader/internal/store"
)

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

func newShareToken() (string, error) { return store.NewShareToken() }

// packageOutput — 2-faza: .tmp dan yakuniy fayl(lar)ni yasaydi.
//   - tar.gz: qismlar alohida entry (`<nom>.ext` yoki `<nom>_partNNN.ext`), arxiv
//     hajmi ArchiveSplitMB dan oshsa keyingi volume (`<nom>_volNNN.tar.gz`);
//   - log.gz: bitta gzip fayl; log: .tmp ni rename (CSV sarlavhali bo'lsa nusxa).
func (w *Worker) packageOutput(job *model.ExportJob, sink *rawSink, outPath string) ([]model.ExportFile, error) {
	parts := sink.tracker.parts()
	var header []byte
	if job.Content == model.ContentCSV {
		header = csvHeader(job.Fields)
	}
	switch job.Format {
	case FormatTarGz:
		return w.packageTarGz(job, sink.path, parts, header, outPath)
	case FormatLogGz:
		return packageGz(sink.path, header, outPath, job.FileName)
	default:
		return packagePlain(sink.path, header, outPath, job.FileName)
	}
}

func packagePlain(tmpPath string, header []byte, outPath, name string) ([]model.ExportFile, error) {
	if header == nil {
		if err := os.Rename(tmpPath, outPath); err != nil {
			return nil, fmt.Errorf("rename failed: %v", err)
		}
	} else {
		src, err := os.Open(tmpPath)
		if err != nil {
			return nil, fmt.Errorf("open temp failed: %v", err)
		}
		defer src.Close()
		dst, err := os.Create(outPath)
		if err != nil {
			return nil, fmt.Errorf("file create failed: %v", err)
		}
		bw := bufio.NewWriterSize(dst, 256*1024)
		bw.Write(header)
		if _, err := io.Copy(bw, src); err != nil {
			dst.Close()
			return nil, fmt.Errorf("copy failed: %v", err)
		}
		if err := bw.Flush(); err != nil {
			dst.Close()
			return nil, fmt.Errorf("flush failed: %v", err)
		}
		dst.Close()
	}
	info, err := os.Stat(outPath)
	if err != nil {
		return nil, err
	}
	return []model.ExportFile{{Name: name, Size: info.Size(), Parts: 1}}, nil
}

func packageGz(tmpPath string, header []byte, outPath, name string) ([]model.ExportFile, error) {
	src, err := os.Open(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("open temp failed: %v", err)
	}
	defer src.Close()
	dst, err := os.Create(outPath)
	if err != nil {
		return nil, fmt.Errorf("file create failed: %v", err)
	}
	counter := &countingWriter{w: dst}
	bw := bufio.NewWriterSize(counter, 256*1024)
	gz := gzip.NewWriter(bw)
	if header != nil {
		gz.Write(header)
	}
	if _, err := io.Copy(gz, bufio.NewReaderSize(src, 256*1024)); err != nil {
		dst.Close()
		return nil, fmt.Errorf("gzip copy failed: %v", err)
	}
	if err := gz.Close(); err != nil {
		dst.Close()
		return nil, fmt.Errorf("gzip close failed: %v", err)
	}
	if err := bw.Flush(); err != nil {
		dst.Close()
		return nil, fmt.Errorf("flush failed: %v", err)
	}
	if err := dst.Close(); err != nil {
		return nil, err
	}
	return []model.ExportFile{{Name: name, Size: counter.n, Parts: 1}}, nil
}

// tarVolume — bitta ochiq .tar.gz fayl.
type tarVolume struct {
	path    string
	f       *os.File
	counter *countingWriter
	bw      *bufio.Writer
	gz      *gzip.Writer
	tw      *tar.Writer
	parts   int
}

func openVolume(path string) (*tarVolume, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("file create failed: %v", err)
	}
	v := &tarVolume{path: path, f: f}
	v.counter = &countingWriter{w: f}
	v.bw = bufio.NewWriterSize(v.counter, 256*1024)
	v.gz = gzip.NewWriter(v.bw)
	v.tw = tar.NewWriter(v.gz)
	return v, nil
}

func (v *tarVolume) close() (int64, error) {
	if err := v.tw.Close(); err != nil {
		v.f.Close()
		return 0, fmt.Errorf("tar close failed: %v", err)
	}
	if err := v.gz.Close(); err != nil {
		v.f.Close()
		return 0, fmt.Errorf("gzip close failed: %v", err)
	}
	if err := v.bw.Flush(); err != nil {
		v.f.Close()
		return 0, fmt.Errorf("flush failed: %v", err)
	}
	if err := v.f.Close(); err != nil {
		return 0, err
	}
	return v.counter.n, nil
}

// compressedSize — hozirgacha diskka tushgan baytlar (gzip/bufio ichidagi
// kutayotganlar hisobga olinmaydi; qism granulyarligida yetarli).
func (v *tarVolume) compressedSize() int64 { return v.counter.n }

func (w *Worker) packageTarGz(job *model.ExportJob, tmpPath string, parts [][2]int64, header []byte, outPath string) ([]model.ExportFile, error) {
	src, err := os.Open(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("open temp failed: %v", err)
	}
	defer src.Close()

	base := strings.TrimSuffix(filepath.Base(outPath), ".tar.gz")
	ext := ".log"
	if job.Content == model.ContentCSV {
		ext = ".csv"
	}
	dir := filepath.Dir(outPath)
	limit := int64(job.ArchiveSplitMB) * 1024 * 1024
	now := time.Now()

	volName := func(n int) string {
		if n == 1 {
			return outPath // bitta volume bo'lsa asl nom qoladi; ko'p bo'lsa oxirida _vol001 ga rename
		}
		return filepath.Join(dir, fmt.Sprintf("%s_vol%03d.tar.gz", base, n))
	}

	var files []model.ExportFile
	volNum := 1
	vol, err := openVolume(volName(volNum))
	if err != nil {
		return nil, err
	}
	closeVol := func() error {
		size, err := vol.close()
		if err != nil {
			return err
		}
		files = append(files, model.ExportFile{Name: filepath.Base(vol.path), Size: size, Parts: vol.parts})
		return nil
	}

	for i, pr := range parts {
		name := base + ext
		if len(parts) > 1 {
			name = fmt.Sprintf("%s_part%03d%s", base, i+1, ext)
		}
		size := pr[1] - pr[0]
		hdr := &tar.Header{Name: name, Mode: 0644, Size: size + int64(len(header)), ModTime: now}
		if err := vol.tw.WriteHeader(hdr); err != nil {
			vol.f.Close()
			return nil, fmt.Errorf("tar header failed: %v", err)
		}
		if header != nil {
			if _, err := vol.tw.Write(header); err != nil {
				vol.f.Close()
				return nil, fmt.Errorf("tar header write failed: %v", err)
			}
		}
		if _, err := io.Copy(vol.tw, io.NewSectionReader(src, pr[0], size)); err != nil {
			vol.f.Close()
			return nil, fmt.Errorf("tar copy failed (part %d): %v", i+1, err)
		}
		vol.parts++
		vol.tw.Flush()
		if limit > 0 && i+1 < len(parts) && vol.compressedSize() >= limit {
			if err := closeVol(); err != nil {
				return nil, err
			}
			volNum++
			if vol, err = openVolume(volName(volNum)); err != nil {
				return nil, err
			}
		}
	}
	if err := closeVol(); err != nil {
		return nil, err
	}

	if len(files) > 1 {
		// Birinchi volume asl nom bilan yozilgan edi — _vol001 ga o'tkazamiz.
		first := filepath.Join(dir, fmt.Sprintf("%s_vol001.tar.gz", base))
		if err := os.Rename(outPath, first); err != nil {
			return nil, fmt.Errorf("rename volume failed: %v", err)
		}
		files[0].Name = filepath.Base(first)
	}
	return files, nil
}
