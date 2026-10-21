import { useState } from "react";
import { Routes, Route, Navigate } from "react-router-dom";
import Layout from "./components/Layout";
import LoginPage from "./pages/LoginPage";
import LogViewerPage from "./pages/LogViewerPage";
import JobsPage from "./pages/JobsPage";
import SettingsPage from "./pages/SettingsPage";
import UsersPage from "./pages/UsersPage";

function App() {
  const [authed, setAuthed] = useState(!!localStorage.getItem("token"));
  const user = JSON.parse(localStorage.getItem("user") || "null");

  // Logout faqat auth ma'lumotlarini tozalaydi — theme va lang saqlanib qoladi.
  const handleLogout = () => {
    localStorage.removeItem("token");
    localStorage.removeItem("user");
    setAuthed(false);
  };

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
      <Route element={<Layout user={user} onLogout={handleLogout} />}>
        <Route path="/" element={<LogViewerPage />} />
        <Route path="/jobs" element={<JobsPage />} />
        <Route path="/settings" element={<SettingsPage isAdmin={user?.role === "admin"} />} />
        {user?.role === "admin" && <Route path="/users" element={<UsersPage />} />}
      </Route>
      <Route path="/login" element={<Navigate to="/" />} />
      {/* Eski /export havolalari Log Viewer'ga yo'naltiriladi (sahifa olib tashlangan). */}
      <Route path="/export" element={<Navigate to="/" replace />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

export default App;
