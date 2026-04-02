import { NavLink, Outlet } from "react-router-dom";
import "./Layout.css";

export default function Layout() {
  return (
    <div className="app">
      <nav className="sidebar">
        <h2>LogDownloader</h2>
        <div className="sidebar-nav">
          <NavLink to="/" end>Log Viewer</NavLink>
          <NavLink to="/export">Export</NavLink>
          <NavLink to="/jobs">Jobs</NavLink>
          <NavLink to="/settings">Settings</NavLink>
        </div>
      </nav>
      <main className="content">
        <Outlet />
      </main>
    </div>
  );
}
