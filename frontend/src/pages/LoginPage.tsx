import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { api } from "../api";
import { useT, type Lang } from "../i18n";
import "./Pages.css";

export default function LoginPage({ onLogin }: { onLogin: () => void }) {
  const { t, lang, setLang } = useT();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();
  const otherLang: Lang = lang === "uz" ? "ru" : "uz";

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError("");
    try {
      const res = await api.login(username, password);
      localStorage.setItem("token", res.token);
      localStorage.setItem("user", JSON.stringify({ user_id: res.user_id, username: res.username, role: res.role }));
      onLogin();
      navigate("/");
    } catch {
      setError(t("loginError"));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="login-page">
      <form className="login-card" onSubmit={handleSubmit}>
        <button
          type="button"
          className="login-lang-toggle"
          onClick={() => setLang(otherLang)}
          title={t("langSwitchTitle")}
        >
          <span className={`lang-opt ${lang === "uz" ? "active" : ""}`}>UZ</span>
          <span className="lang-sep">/</span>
          <span className={`lang-opt ${lang === "ru" ? "active" : ""}`}>RU</span>
        </button>
        <h1 className="login-title">LogDownloader</h1>
        <p className="login-subtitle">{t("loginSubtitle")}</p>
        {error && <div className="login-error">{error}</div>}
        <input
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          placeholder={t("username")}
          className="input"
          autoFocus
        />
        <input
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          placeholder={t("password")}
          className="input"
        />
        <button type="submit" disabled={loading || !username || !password} className="btn btn-primary login-btn">
          {loading ? "..." : t("loginButton")}
        </button>
      </form>
    </div>
  );
}
