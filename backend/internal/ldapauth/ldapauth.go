// Package ldapauth — LDAP / Active Directory orqali autentifikatsiya.
//
// Oqim: service account (yoki anonim) bind → BaseDN ostida UserFilter bilan
// userni topish (aynan bitta yozuv) → topilgan DN bilan foydalanuvchi paroli
// orqali bind (parol tekshiruvi) → memberOf bo'yicha guruhlar (to'g'ridan va
// AD nested, LDAP_MATCHING_RULE_IN_CHAIN) → ruxsat/admin guruhlari.
package ldapauth

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"

	"logdownloader/internal/model"
)

var (
	ErrNotConfigured      = errors.New("LDAP sozlanmagan")
	ErrUserNotFound       = errors.New("LDAP: foydalanuvchi topilmadi")
	ErrAmbiguous          = errors.New("LDAP: filtr bir nechta foydalanuvchini qaytardi")
	ErrInvalidCredentials = errors.New("LDAP: parol noto'g'ri")
	ErrNotAllowed         = errors.New("LDAP: foydalanuvchi ruxsat etilgan guruhda emas")
)

// Result — muvaffaqiyatli autentifikatsiya natijasi.
type Result struct {
	Username string   `json:"username"`
	DN       string   `json:"dn"`
	Email    string   `json:"email,omitempty"`
	Groups   []string `json:"groups,omitempty"`
	IsAdmin  bool     `json:"is_admin"`
}

const (
	defaultFilter       = "(&(objectClass=user)(sAMAccountName=%s))"
	defaultUsernameAttr = "sAMAccountName"
	defaultEmailAttr    = "mail"
	defaultTimeout      = 10 * time.Second
	matchingRuleInChain = "1.2.840.113556.1.4.1941"
)

func attrOr(v, def string) string {
	if s := strings.TrimSpace(v); s != "" {
		return s
	}
	return def
}

func timeoutOf(cfg model.LDAPSettings) time.Duration {
	if cfg.TimeoutSec > 0 {
		return time.Duration(cfg.TimeoutSec) * time.Second
	}
	return defaultTimeout
}

// userFilter — UserFilter ichidagi %s ni escape qilingan username bilan almashtiradi.
// %s bo'lmasa filtr `(&<filter>(<usernameAttr>=<username>))` ko'rinishida yig'iladi.
func userFilter(cfg model.LDAPSettings, username string) string {
	f := strings.TrimSpace(cfg.UserFilter)
	if f == "" {
		f = defaultFilter
	}
	esc := ldap.EscapeFilter(username)
	if strings.Contains(f, "%s") {
		return strings.ReplaceAll(f, "%s", esc)
	}
	return fmt.Sprintf("(&%s(%s=%s))", f, attrOr(cfg.UsernameAttr, defaultUsernameAttr), esc)
}

func normalizeDN(dn string) string {
	parts := strings.Split(dn, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return strings.ToLower(strings.Join(parts, ","))
}

func hasGroup(groups []string, dn string) bool {
	want := normalizeDN(dn)
	for _, g := range groups {
		if normalizeDN(g) == want {
			return true
		}
	}
	return false
}

// connect — ldap:// (ixtiyoriy StartTLS) yoki ldaps://.
func connect(cfg model.LDAPSettings) (*ldap.Conn, error) {
	raw := strings.TrimSpace(cfg.URL)
	if raw == "" {
		return nil, ErrNotConfigured
	}
	if !strings.Contains(raw, "://") {
		raw = "ldap://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("LDAP URL noto'g'ri: %q", cfg.URL)
	}
	timeout := timeoutOf(cfg)
	tlsCfg := &tls.Config{ServerName: u.Hostname(), InsecureSkipVerify: cfg.InsecureSkipVerify} //nolint:gosec // admin tanlovi
	dialer := &net.Dialer{Timeout: timeout}

	var conn *ldap.Conn
	switch u.Scheme {
	case "ldaps":
		conn, err = ldap.DialURL(raw, ldap.DialWithDialer(dialer), ldap.DialWithTLSConfig(tlsCfg))
	case "ldap":
		conn, err = ldap.DialURL(raw, ldap.DialWithDialer(dialer))
		if err == nil && cfg.StartTLS {
			if terr := conn.StartTLS(tlsCfg); terr != nil {
				conn.Close()
				return nil, fmt.Errorf("StartTLS: %w", terr)
			}
		}
	default:
		return nil, fmt.Errorf("LDAP URL sxemasi ldap:// yoki ldaps:// bo'lishi kerak")
	}
	if err != nil {
		return nil, fmt.Errorf("LDAP ulanish: %w", err)
	}
	conn.SetTimeout(timeout)
	return conn, nil
}

func serviceBind(conn *ldap.Conn, cfg model.LDAPSettings) error {
	if strings.TrimSpace(cfg.BindDN) == "" {
		if err := conn.UnauthenticatedBind(""); err != nil {
			return fmt.Errorf("LDAP anonim bind: %w", err)
		}
		return nil
	}
	if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
		return fmt.Errorf("LDAP service bind (%s): %w", cfg.BindDN, err)
	}
	return nil
}

// Authenticate — username/parol bilan LDAP orqali tekshiradi.
func Authenticate(ctx context.Context, cfg model.LDAPSettings, username, password string) (*Result, error) {
	if !cfg.Enabled {
		return nil, ErrNotConfigured
	}
	username = strings.TrimSpace(username)
	// Bo'sh parol bilan bind LDAP'da "anonim" deb qabul qilinadi — oldindan rad etamiz.
	if username == "" || password == "" {
		return nil, ErrInvalidCredentials
	}
	if strings.TrimSpace(cfg.BaseDN) == "" {
		return nil, fmt.Errorf("LDAP base DN sozlanmagan")
	}

	conn, err := connect(cfg)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := serviceBind(conn, cfg); err != nil {
		return nil, err
	}

	usernameAttr := attrOr(cfg.UsernameAttr, defaultUsernameAttr)
	emailAttr := attrOr(cfg.EmailAttr, defaultEmailAttr)
	req := ldap.NewSearchRequest(cfg.BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2, int(timeoutOf(cfg).Seconds()), false,
		userFilter(cfg, username), []string{"dn", "memberOf", usernameAttr, emailAttr}, nil)
	res, err := conn.Search(req)
	if err != nil {
		return nil, fmt.Errorf("LDAP qidiruv: %w", err)
	}
	if len(res.Entries) == 0 {
		return nil, ErrUserNotFound
	}
	if len(res.Entries) > 1 {
		return nil, ErrAmbiguous
	}
	entry := res.Entries[0]
	userDN := entry.DN

	// Parol tekshiruvi — foydalanuvchi DN'i bilan bind.
	if err := conn.Bind(userDN, password); err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("LDAP user bind: %w", err)
	}

	out := &Result{
		Username: strings.ToLower(attrOr(entry.GetAttributeValue(usernameAttr), username)),
		DN:       userDN,
		Email:    entry.GetAttributeValue(emailAttr),
		Groups:   entry.GetAttributeValues("memberOf"),
	}

	// Guruh tekshiruvlari (to'g'ridan memberOf; topilmasa AD nested qidiruv).
	needNested := (cfg.AllowedGroupDN != "" && !hasGroup(out.Groups, cfg.AllowedGroupDN)) ||
		(cfg.AdminGroupDN != "" && !hasGroup(out.Groups, cfg.AdminGroupDN))
	nested := map[string]bool{}
	if needNested {
		// Nested qidiruv uchun service account bilan qayta bind.
		if err := serviceBind(conn, cfg); err == nil {
			for _, g := range []string{cfg.AllowedGroupDN, cfg.AdminGroupDN} {
				if g == "" || hasGroup(out.Groups, g) {
					continue
				}
				nested[normalizeDN(g)] = memberOfNested(conn, cfg, userDN, g)
			}
		}
	}
	inGroup := func(g string) bool {
		return g != "" && (hasGroup(out.Groups, g) || nested[normalizeDN(g)])
	}
	if cfg.AllowedGroupDN != "" && !inGroup(cfg.AllowedGroupDN) {
		return nil, ErrNotAllowed
	}
	out.IsAdmin = inGroup(cfg.AdminGroupDN)
	_ = ctx
	return out, nil
}

// memberOfNested — Active Directory LDAP_MATCHING_RULE_IN_CHAIN orqali
// (guruh ichidagi guruhlar) a'zolikni tekshiradi. Xato → false.
func memberOfNested(conn *ldap.Conn, cfg model.LDAPSettings, userDN, groupDN string) bool {
	filter := fmt.Sprintf("(&(distinguishedName=%s)(memberOf:%s:=%s))", ldap.EscapeFilter(userDN), matchingRuleInChain, ldap.EscapeFilter(groupDN))
	req := ldap.NewSearchRequest(cfg.BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 1, int(timeoutOf(cfg).Seconds()), false, filter, []string{"dn"}, nil)
	res, err := conn.Search(req)
	return err == nil && len(res.Entries) > 0
}

// TestConnection — admin "Test" tugmasi: ulanish + service bind + base DN
// o'qish. username/password berilsa to'liq login sinovi (Result bilan).
func TestConnection(ctx context.Context, cfg model.LDAPSettings, username, password string) (*Result, error) {
	if username != "" {
		c := cfg
		c.Enabled = true
		return Authenticate(ctx, c, username, password)
	}
	conn, err := connect(cfg)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := serviceBind(conn, cfg); err != nil {
		return nil, err
	}
	base := strings.TrimSpace(cfg.BaseDN)
	if base == "" {
		return nil, fmt.Errorf("LDAP base DN sozlanmagan")
	}
	req := ldap.NewSearchRequest(base, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, int(timeoutOf(cfg).Seconds()), false, "(objectClass=*)", []string{"dn"}, nil)
	if _, err := conn.Search(req); err != nil {
		return nil, fmt.Errorf("LDAP base DN o'qilmadi (%s): %w", base, err)
	}
	return &Result{DN: base}, nil
}
