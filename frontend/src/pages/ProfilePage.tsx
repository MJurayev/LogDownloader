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
  const [msg, setMsg] = useState("");
  const [saving, setSaving] = useState(false);

  const [curPass, setCurPass] = useState("");
  const [newPass, setNewPass] = useState("");
  const [passMsg, setPassMsg] = useState("");
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
    setMsg("");
    try {
      await api.updateMe({ email, telegram_chat_id: chatId, notify_on_done: notify });
      setMsg(t("saved"));
      setTimeout(() => setMsg(""), 1500);
    } catch (err) {
      setMsg(typeof err === "string" ? err : t("genericError"));
    } finally {
      setSaving(false);
    }
  };

  const changePassword = async () => {
    if (newPass.length < 6) {
      setPassMsg(t("passwordTooShort"));
      return;
    }
    setPassSaving(true);
    setPassMsg("");
    try {
      await api.updateMe({ current_password: curPass, new_password: newPass });
      setPassMsg(t("passwordChanged"));
      setCurPass("");
      setNewPass("");
    } catch (err) {
      const text = typeof err === "string" ? err : String(err);
      setPassMsg(text.includes("current password") ? t("wrongCurrentPassword") : text);
    } finally {
      setPassSaving(false);
    }
  };

  if (!profile) return <div className="page">{t("loading")}</div>;

  return (
    <div className="page">
      <h1>{t("profileTitle")}</h1>
      <p className="subtitle">
        {t("profileSubtitle")} — <strong>{profile.username}</strong> ({profile.role})
      </p>

      <div className="ds-card">
        <div className="ds-card-body" style={{ paddingTop: 16 }}>
          <div className="form-group">
            <label className="label">{t("emailLabel")}</label>
            <input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="name@example.com" className="input" />
          </div>
          <div className="form-group">
            <label className="label">{t("telegramChatIdLabel")}</label>
            <input value={chatId} onChange={(e) => setChatId(e.target.value)} placeholder="123456789" className="input" />
            <div className="field-hint">{t("telegramChatIdHint")}</div>
          </div>
          <label className="global-check">
            <input type="checkbox" checked={notify} onChange={(e) => setNotify(e.target.checked)} />
            {t("notifyOnDoneLabel")}
          </label>
          <div className="ds-bottom-actions" style={{ marginTop: 14 }}>
            {msg && <span className="save-modal-msg">{msg}</span>}
            <button onClick={save} disabled={saving} className="btn btn-small btn-primary">{saving ? "..." : t("save")}</button>
          </div>
        </div>
      </div>

      <div className="ds-card" style={{ marginTop: 16 }}>
        <div className="ds-card-body" style={{ paddingTop: 16 }}>
          <h3 className="subsection-title">{t("changePasswordTitle")}</h3>
          <div className="settings-auth-row">
            <input type="password" value={curPass} onChange={(e) => setCurPass(e.target.value)} placeholder={t("currentPassword")} className="input" autoComplete="current-password" />
            <input type="password" value={newPass} onChange={(e) => setNewPass(e.target.value)} placeholder={t("newPassword")} className="input" autoComplete="new-password" />
          </div>
          <div className="ds-bottom-actions" style={{ marginTop: 14 }}>
            {passMsg && <span className="save-modal-msg">{passMsg}</span>}
            <button onClick={changePassword} disabled={passSaving || !curPass || !newPass} className="btn btn-small btn-primary">{passSaving ? "..." : t("save")}</button>
          </div>
        </div>
      </div>
    </div>
  );
}
