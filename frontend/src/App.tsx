import { useState } from "react";
import { Routes, Route, Navigate } from "react-router-dom";
import Layout from "./components/Layout";
import LoginPage from "./pages/LoginPage";
import LogViewerPage from "./pages/LogViewerPage";
import ExportPage from "./pages/ExportPage";
import JobsPage from "./pages/JobsPage";
import SettingsPage from "./pages/SettingsPage";
import UsersPage from "./pages/UsersPage";

function App() {
  const [authed, setAuthed] = useState(!!localStorage.getItem("token"));
  const user = JSON.parse(localStorage.getItem("user") || "null");

  if (!authed) {
    return (
      <Routes>
        <Route path="/login" element={<LoginPage onLogin={() => setAuthed(true)} />} />
        <Route path="*" element={<Navigate to="/login" />} />
      </Routes>
    );
  }

  return (
    <Routes>
      <Route element={<Layout user={user} onLogout={() => { localStorage.clear(); setAuthed(false); }} />}>
        <Route path="/" element={<LogViewerPage />} />
        <Route path="/export" element={<ExportPage />} />
        <Route path="/jobs" element={<JobsPage />} />
        <Route path="/settings" element={<SettingsPage isAdmin={user?.role === "admin"} />} />
        {user?.role === "admin" && <Route path="/users" element={<UsersPage />} />}
      </Route>
      <Route path="/login" element={<Navigate to="/" />} />
    </Routes>
  );
}

export default App;
