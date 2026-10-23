import { useEffect, useState } from "react";
import { api, type Profile } from "../api";
import { useT } from "../i18n";
import "./Pages.css";

export default function ProfilePage() {
  const { t } = useT();
  const [profile, setProfile] = useState<Profile | null>(null);
  const [email, setEmail] = useState("");
  const [chatId, setChatId] = useState("");
  const [notify, setNotify] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [saving, setSaving] = useState(false);

  const [curPass, setCurPass] = useState("");
  const [newPass, setNewPass] = useState("");
  const [passMsg, setPassMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [passSaving, setPassSaving] = useState(false);

  useEffect(() => {
    api.me().then((p) => {
      setProfile(p);
      setEmail(p.email || "");
      setChatId(p.telegram_chat_id || "");
      setNotify(!!p.notify_on_done);
    });
  }, []);

  const save = async () => {
    setSaving(true);
    setMsg(null);
    try {
      await api.updateMe({ email, telegram_chat_id: chatId, notify_on_done: notify });
      setMsg({ ok: true, text: t("saved") });
      setTimeout(() => setMsg(null), 2000);
    } catch (err) {
      setMsg({ ok: false, text: typeof err === "string" ? err : t("genericError") });
    } finally {
      setSaving(false);
    }
  };

  const changePassword = async () => {
    if (newPass.length < 6) {
      setPassMsg({ ok: false, text: t("passwordTooShort") });
      return;
    }
    setPassSaving(true);
    setPassMsg(null);
    try {
      await api.updateMe({ current_password: curPass, new_password: newPass });
      setPassMsg({ ok: true, text: t("passwordChanged") });
      setCurPass("");
      setNewPass("");
    } catch (err) {
      const text = typeof err === "string" ? err : String(err);
      setPassMsg({ ok: false, text: text.includes("current password") ? t("wrongCurrentPassword") : text });
    } finally {
      setPassSaving(false);
    }
  };

  if (!profile) return <div className="page">{t("loading")}</div>;

  return (
    <div className="page">
      <h1>{t("profileTitle")}</h1>
      <p className="subtitle">
        {t("profileSubtitle")} — <strong>{profile.username}</strong> · {profile.role}
        {profile.auth_source === "ldap" && <span className="job-format-chip" style={{ marginLeft: 8 }}>{t("ldapBadge")}</span>}
      </p>

      <div className="panel">
        <div className="panel-head"><h3 className="panel-title">{t("notifyTitle")}</h3></div>
        <p className="panel-desc">{t("notifyRecipientsHint")}</p>
        <div className="form-grid">
          <div className="field">
            <label className="label">{t("emailLabel")}</label>
            <input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="name@example.com" className="input" />
          </div>
          <div className="field">
            <label className="label">{t("telegramChatIdLabel")}</label>
            <input value={chatId} onChange={(e) => setChatId(e.target.value)} placeholder="123456789" className="input" />
            <div className="hint">{t("telegramChatIdHint")}</div>
          </div>
          <div className="field span-2">
            <label className="check">
              <input type="checkbox" checked={notify} onChange={(e) => setNotify(e.target.checked)} />
              {t("notifyOnDoneLabel")}
            </label>
          </div>
        </div>
        <div className="panel-actions">
          <span className={`msg ${msg && !msg.ok ? "err" : ""}`}>{msg ? msg.text : ""}</span>
          <button onClick={save} disabled={saving} className="btn btn-small btn-primary">{saving ? "..." : t("save")}</button>
        </div>
      </div>

      <div className="panel">
        <div className="panel-head"><h3 className="panel-title">{t("changePasswordTitle")}</h3></div>
        {profile.auth_source === "ldap" ? (
          <p className="panel-desc">{t("ldapNoPasswordChange")}</p>
        ) : (
          <>
            <div className="form-grid" style={{ marginTop: 12 }}>
              <div className="field">
                <label className="label">{t("currentPassword")}</label>
                <input type="password" value={curPass} onChange={(e) => setCurPass(e.target.value)} className="input" autoComplete="current-password" />
              </div>
              <div className="field">
                <label className="label">{t("newPassword")}</label>
                <input type="password" value={newPass} onChange={(e) => setNewPass(e.target.value)} className="input" autoComplete="new-password" />
                <div className="hint">{t("passwordTooShort")}</div>
              </div>
            </div>
            <div className="panel-actions">
              <span className={`msg ${passMsg && !passMsg.ok ? "err" : ""}`}>{passMsg ? passMsg.text : ""}</span>
              <button onClick={changePassword} disabled={passSaving || !curPass || !newPass} className="btn btn-small btn-primary">{passSaving ? "..." : t("save")}</button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
