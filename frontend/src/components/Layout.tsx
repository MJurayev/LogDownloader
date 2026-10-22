import { useState, useEffect } from "react";
import { NavLink, Outlet } from "react-router-dom";
import { useT, type Lang } from "../i18n";
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
  const { t, lang, setLang } = useT();
  const [theme, setTheme] = useState<Theme>(getInitialTheme);

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
    localStorage.setItem("theme", theme);
  }, [theme]);

  const toggleTheme = () => setTheme((t) => (t === "dark" ? "light" : "dark"));
  const otherLang: Lang = lang === "uz" ? "ru" : "uz";
  const isAdmin = user?.role === "admin";

  return (
    <div className="app">
      <nav className="sidebar">
        <h2>LogDownloader</h2>
        <div className="sidebar-nav">
          <NavLink to="/" end>{t("navLogViewer")}</NavLink>
          <NavLink to="/jobs">{t("navJobs")}</NavLink>
          <NavLink to="/schedules">{t("navSchedules")}</NavLink>
          <NavLink to="/settings">{t("navSettings")}</NavLink>
          {isAdmin && <NavLink to="/users">{t("navUsers")}</NavLink>}
          {isAdmin && <NavLink to="/audit">{t("navAudit")}</NavLink>}
          <NavLink to="/profile">{t("navProfile")}</NavLink>
        </div>
        <div className="sidebar-user">
          <div className="sidebar-user-info">
            <span className="sidebar-username">{user?.username}</span>
            <span className={`sidebar-role ${isAdmin ? "role-admin" : ""}`}>{user?.role}</span>
          </div>
          <button className="sidebar-theme-toggle sidebar-lang-toggle" onClick={() => setLang(otherLang)} title={t("langSwitchTitle")}>
            <span className={`lang-opt ${lang === "uz" ? "active" : ""}`}>UZ</span>
            <span className="lang-sep">/</span>
            <span className={`lang-opt ${lang === "ru" ? "active" : ""}`}>RU</span>
          </button>
          <button className="sidebar-theme-toggle" onClick={toggleTheme} title={theme === "dark" ? t("themeLight") : t("themeDark")}>
            <span aria-hidden="true">{theme === "dark" ? "☀" : "☾"}</span>
            <span>{theme === "dark" ? t("themeLight") : t("themeDark")}</span>
          </button>
          <button className="sidebar-logout" onClick={onLogout}>{t("logout")}</button>
        </div>
      </nav>
      <main className="content">
        <Outlet />
      </main>
    </div>
  );
}
