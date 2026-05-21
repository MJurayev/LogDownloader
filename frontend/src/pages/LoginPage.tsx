import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { api } from "../api";
import "./Pages.css";

export default function LoginPage({ onLogin }: { onLogin: () => void }) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

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
      setError("Login yoki parol noto'g'ri");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="login-page">
      <form className="login-card" onSubmit={handleSubmit}>
        <h1 className="login-title">LogDownloader</h1>
        <p className="login-subtitle">Tizimga kirish</p>
        {error && <div className="login-error">{error}</div>}
        <input
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          placeholder="Username"
          className="input"
          autoFocus
        />
        <input
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          placeholder="Password"
          className="input"
        />
        <button type="submit" disabled={loading || !username || !password} className="btn btn-primary login-btn">
          {loading ? "..." : "Kirish"}
        </button>
      </form>
    </div>
  );
}
