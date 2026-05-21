import { useState, useEffect } from "react";
import { NavLink, Outlet } from "react-router-dom";
import "./Layout.css";

type Theme = "dark" | "light";

interface Props {
  user: { username: string; role: string } | null;
  onLogout: () => void;
}

function getInitialTheme(): Theme {
  const attr = document.documentElement.getAttribute("data-theme");
  return attr === "light" ? "light" : "dark";
}

export default function Layout({ user, onLogout }: Props) {
  const [theme, setTheme] = useState<Theme>(getInitialTheme);

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
    localStorage.setItem("theme", theme);
  }, [theme]);

  const toggleTheme = () => setTheme((t) => (t === "dark" ? "light" : "dark"));

  return (
    <div className="app">
      <nav className="sidebar">
        <h2>LogDownloader</h2>
        <div className="sidebar-nav">
          <NavLink to="/" end>Log Viewer</NavLink>
          <NavLink to="/export">Export</NavLink>
          <NavLink to="/jobs">Jobs</NavLink>
          <NavLink to="/settings">Settings</NavLink>
          {user?.role === "admin" && <NavLink to="/users">Users</NavLink>}
        </div>
        <div className="sidebar-user">
          <div className="sidebar-user-info">
            <span className="sidebar-username">{user?.username}</span>
            <span className={`sidebar-role ${user?.role === "admin" ? "role-admin" : ""}`}>{user?.role}</span>
          </div>
          <button
            className="sidebar-theme-toggle"
            onClick={toggleTheme}
            title={theme === "dark" ? "Light mode" : "Dark mode"}
          >
            <span aria-hidden="true">{theme === "dark" ? "☀" : "☾"}</span>
            <span>{theme === "dark" ? "Light mode" : "Dark mode"}</span>
          </button>
          <button className="sidebar-logout" onClick={onLogout}>Chiqish</button>
        </div>
      </nav>
      <main className="content">
        <Outlet />
      </main>
    </div>
  );
}
