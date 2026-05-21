import { NavLink, Outlet } from "react-router-dom";
import "./Layout.css";

interface Props {
  user: { username: string; role: string } | null;
  onLogout: () => void;
}

export default function Layout({ user, onLogout }: Props) {
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
          <button className="sidebar-logout" onClick={onLogout}>Chiqish</button>
        </div>
      </nav>
      <main className="content">
        <Outlet />
      </main>
    </div>
  );
}
