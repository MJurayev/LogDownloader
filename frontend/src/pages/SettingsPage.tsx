import { useState, useEffect } from "react";
import { api } from "../api";
import "./Pages.css";

export default function SettingsPage() {
  const [url, setUrl] = useState("");
  const [saved, setSaved] = useState(false);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.getSettings().then((s) => {
      setUrl(s.vlselect_url || "");
      setLoading(false);
    });
  }, []);

  const handleSave = async () => {
    await api.updateSettings({ vlselect_url: url });
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  };

  if (loading) return <div className="page">Yuklanmoqda...</div>;

  return (
    <div className="page">
      <h1>Settings</h1>
      <p className="subtitle">VictoriaLogs endpoint sozlamasi</p>

      <div className="form-group">
        <label className="label">VLSelect URL</label>
        <input
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="http://localhost:9428"
          className="input"
        />
        <p className="hint">
          VictoriaLogs vlselect manzili. Masalan: http://victorialogs:9428
        </p>
        <button onClick={handleSave} className="btn btn-primary">
          Saqlash
        </button>
        {saved && <span className="saved-msg">Saqlandi!</span>}
      </div>
    </div>
  );
}
