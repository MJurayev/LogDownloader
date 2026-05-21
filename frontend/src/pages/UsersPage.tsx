import { useState, useEffect } from "react";
import { api, type UserInfo } from "../api";
import "./Pages.css";

export default function UsersPage() {
  const [users, setUsers] = useState<UserInfo[]>([]);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState("user");
  const [error, setError] = useState("");

  // Edit modal state
  const [editingUser, setEditingUser] = useState<UserInfo | null>(null);
  const [editRole, setEditRole] = useState("user");
  const [editPassword, setEditPassword] = useState("");
  const [editSaving, setEditSaving] = useState(false);
  const [editError, setEditError] = useState("");

  const loadUsers = async () => {
    const data = await api.getUsers();
    setUsers(data || []);
  };

  useEffect(() => { loadUsers(); }, []);

  const handleCreate = async () => {
    if (!username.trim() || !password.trim()) return;
    setError("");
    try {
      await api.createUser(username, password, role);
      setUsername("");
      setPassword("");
      setRole("user");
      loadUsers();
    } catch {
      setError("Xatolik yuz berdi");
    }
  };

  const handleDelete = async (id: string) => {
    await api.deleteUser(id);
    loadUsers();
  };

  const openEdit = (u: UserInfo) => {
    setEditingUser(u);
    setEditRole(u.role);
    setEditPassword("");
    setEditError("");
  };

  const closeEdit = () => {
    setEditingUser(null);
    setEditPassword("");
    setEditError("");
  };

  const handleEditSave = async () => {
    if (!editingUser) return;
    setEditSaving(true);
    setEditError("");
    try {
      await api.updateUser(editingUser.id, editRole, editPassword || undefined);
      closeEdit();
      loadUsers();
    } catch {
      setEditError("Saqlashda xatolik");
    } finally {
      setEditSaving(false);
    }
  };

  return (
    <div className="page">
      <h1>Users</h1>
      <p className="subtitle">Foydalanuvchilarni boshqarish</p>

      <div className="form-group">
        <div className="settings-auth-row">
          <input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="Username"
            className="input"
          />
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="Password"
            className="input"
          />
          <select value={role} onChange={(e) => setRole(e.target.value)} className="input ds-select">
            <option value="user">User</option>
            <option value="admin">Admin</option>
          </select>
        </div>
        <button onClick={handleCreate} disabled={!username.trim() || !password.trim()} className="btn btn-primary">
          Qo'shish
        </button>
        {error && <div className="error-text">{error}</div>}
      </div>

      <div className="list">
        {users.map((u) => (
          <div key={u.id} className="card">
            <div className="card-header">
              <div className="job-header-left">
                <strong>{u.username}</strong>
                <span className={`badge ${u.role === "admin" ? "badge-done" : "badge-running"}`}>
                  {u.role.toUpperCase()}
                </span>
              </div>
              <div className="card-actions">
                <button onClick={() => openEdit(u)} className="btn btn-small btn-outline">
                  Tahrirlash
                </button>
                <button onClick={() => handleDelete(u.id)} className="btn btn-small btn-danger">
                  O'chirish
                </button>
              </div>
            </div>
          </div>
        ))}
      </div>

      {editingUser && (
        <div className="save-modal-overlay" onClick={closeEdit}>
          <div className="save-modal" onClick={(e) => e.stopPropagation()}>
            <h3>Userni tahrirlash</h3>
            <div className="form-group">
              <label className="label">Username</label>
              <input value={editingUser.username} className="input" disabled />
            </div>
            <div className="form-group">
              <label className="label">Role</label>
              <select value={editRole} onChange={(e) => setEditRole(e.target.value)} className="input ds-select">
                <option value="user">User</option>
                <option value="admin">Admin</option>
              </select>
            </div>
            <div className="form-group">
              <label className="label">Yangi parol (bo'sh qoldirilsa o'zgarmaydi)</label>
              <input
                type="password"
                value={editPassword}
                onChange={(e) => setEditPassword(e.target.value)}
                placeholder="Yangi parol"
                className="input"
              />
            </div>
            {editError && <div className="error-text">{editError}</div>}
            <div className="save-modal-actions">
              <button onClick={closeEdit} disabled={editSaving} className="btn btn-small btn-outline">
                Bekor qilish
              </button>
              <button onClick={handleEditSave} disabled={editSaving} className="btn btn-small btn-primary">
                {editSaving ? "..." : "Saqlash"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
