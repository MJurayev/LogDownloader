// Package notify — export tugaganda egasiga xabar (Telegram, email) va
// global webhook. Kanallar admin sozlamalarida (Settings.Notify), qabul
// qiluvchi manzillar user profilida.
package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"logdownloader/internal/metrics"
	"logdownloader/internal/model"
	"logdownloader/internal/store"
)

type Notifier struct {
	store  *store.Store
	client *http.Client
}

func New(s *store.Store) *Notifier {
	return &Notifier{store: s, client: &http.Client{Timeout: 15 * time.Second}}
}

// JobFinished — job done/failed bo'lganda worker chaqiradi (goroutine'da).
// Egasining profili va job.Notify bo'yicha kanallar tanlanadi; webhook doim
// (URL berilgan bo'lsa).
func (n *Notifier) JobFinished(job *model.ExportJob) {
	st := n.store.GetSettings()
	owner := n.store.GetUser(job.OwnerID)

	if st.Notify.WebhookURL != "" {
		if err := n.sendWebhook(st.Notify.WebhookURL, job); err != nil {
			log.Printf("notify webhook (job %s): %v", job.ID, err)
			metrics.NotifyFailed.Add(1)
		} else {
			metrics.NotifySent.Add(1)
		}
	}

	if owner == nil || !(job.Notify || owner.NotifyOnDone) {
		return
	}
	subject, text := n.message(job)
	if st.Notify.TelegramBotToken != "" && owner.TelegramChatID != "" {
		if err := n.sendTelegram(st.Notify.TelegramBotToken, owner.TelegramChatID, text); err != nil {
			log.Printf("notify telegram (job %s): %v", job.ID, err)
			metrics.NotifyFailed.Add(1)
		} else {
			metrics.NotifySent.Add(1)
		}
	}
	if st.Notify.SMTP.Host != "" && owner.Email != "" {
		if err := n.sendEmail(st.Notify.SMTP, owner.Email, subject, text); err != nil {
			log.Printf("notify email (job %s): %v", job.ID, err)
			metrics.NotifyFailed.Add(1)
		} else {
			metrics.NotifySent.Add(1)
		}
	}
}

func (n *Notifier) message(job *model.ExportJob) (subject, text string) {
	status := "✅ tayyor"
	if job.Status == model.JobFailed {
		status = "❌ xato"
	}
	subject = fmt.Sprintf("LogDownloader: export %s — %s", status, job.FileName)
	var b strings.Builder
	fmt.Fprintf(&b, "Export %s\n", status)
	fmt.Fprintf(&b, "Fayl: %s\n", job.FileName)
	if len(job.Files) > 1 {
		fmt.Fprintf(&b, "Fayllar: %d ta\n", len(job.Files))
	}
	fmt.Fprintf(&b, "Datasource: %s\n", job.DatasourceName)
	fmt.Fprintf(&b, "Query: %s\n", job.Query)
	if job.Start != "" || job.End != "" {
		fmt.Fprintf(&b, "Oraliq: %s — %s\n", job.Start, job.End)
	}
	fmt.Fprintf(&b, "Qatorlar: %d, hajm: %s\n", job.Lines, humanBytes(job.Size))
	if job.Error != "" {
		fmt.Fprintf(&b, "Xato: %s\n", job.Error)
	}
	if job.ShareURL != "" {
		fmt.Fprintf(&b, "Yuklab olish: %s\n", job.ShareURL)
	} else if base := n.store.PublicURL(); base != "" {
		fmt.Fprintf(&b, "Jobs: %s/jobs\n", strings.TrimRight(base, "/"))
	}
	return subject, b.String()
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func (n *Notifier) sendTelegram(token, chatID, text string) error {
	body, _ := json.Marshal(map[string]any{"chat_id": chatID, "text": text, "disable_web_page_preview": true})
	req, err := http.NewRequest("POST", "https://api.telegram.org/bot"+token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		return fmt.Errorf("telegram HTTP %d: %s", resp.StatusCode, strings.TrimSpace(buf.String()))
	}
	return nil
}

func (n *Notifier) sendWebhook(url string, job *model.ExportJob) error {
	payload := map[string]any{
		"event":      "export." + string(job.Status),
		"job":        job,
		"sent_at":    time.Now().UTC(),
		"public_url": n.store.PublicURL(),
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook HTTP %d", resp.StatusCode)
	}
	return nil
}

func (n *Notifier) sendEmail(cfg model.SMTPSettings, to, subject, text string) error {
	port := cfg.Port
	if port == 0 {
		port = 587
	}
	addr := net.JoinHostPort(cfg.Host, fmt.Sprint(port))
	from := cfg.From
	if from == "" {
		from = cfg.Username
	}
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		from, to, subject, text)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if cfg.StartTLS {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: cfg.Host}); err != nil {
				return err
			}
		}
	}
	if cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	wc, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write([]byte(msg)); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// SendTest — admin sozlamalardagi "Test" tugmasi. channel: telegram | email | webhook.
// user — qabul qiluvchi (telegram/email uchun joriy admin profili).
func (n *Notifier) SendTest(channel string, user *model.User) error {
	st := n.store.GetSettings()
	text := fmt.Sprintf("LogDownloader test xabari (%s)", time.Now().Format(time.RFC3339))
	switch channel {
	case "telegram":
		if st.Notify.TelegramBotToken == "" {
			return fmt.Errorf("telegram bot token sozlanmagan")
		}
		if user == nil || user.TelegramChatID == "" {
			return fmt.Errorf("profilingizda Telegram chat ID yo'q")
		}
		return n.sendTelegram(st.Notify.TelegramBotToken, user.TelegramChatID, text)
	case "email":
		if st.Notify.SMTP.Host == "" {
			return fmt.Errorf("SMTP sozlanmagan")
		}
		if user == nil || user.Email == "" {
			return fmt.Errorf("profilingizda email yo'q")
		}
		return n.sendEmail(st.Notify.SMTP, user.Email, "LogDownloader test", text)
	case "webhook":
		if st.Notify.WebhookURL == "" {
			return fmt.Errorf("webhook URL sozlanmagan")
		}
		return n.sendWebhook(st.Notify.WebhookURL, &model.ExportJob{ID: "test", Status: "test", FileName: "test", CreatedAt: time.Now()})
	}
	return fmt.Errorf("noma'lum kanal: %s", channel)
}
