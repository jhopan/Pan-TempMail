"use client";

import { useState, useEffect } from "react";
import { useAuth } from "@/context/AuthContext";
import { useRouter } from "next/navigation";

type User = {
  id: number;
  email: string;
  role: string;
  created_at: string;
};

export default function AdminPage() {
  const { user, token, isAdmin, loading } = useAuth();
  const router = useRouter();
  const [users, setUsers] = useState<User[]>([]);
  const [showAddForm, setShowAddForm] = useState(false);
  const [editUser, setEditUser] = useState<User | null>(null);
  const [newEmail, setNewEmail] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [newRole, setNewRole] = useState("user");
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");

  useEffect(() => {
    if (!loading && (!user || !isAdmin)) router.push("/login");
  }, [user, isAdmin, loading, router]);

  useEffect(() => {
    if (token && isAdmin) fetchUsers();
  }, [token, isAdmin]);

  const fetchUsers = async () => {
    try {
      const res = await fetch("/api/admin/users", { headers: { Authorization: "Bearer " + token } });
      if (res.ok) setUsers((await res.json()) || []);
    } catch (e) { console.error(e); }
  };

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault(); setError(""); setSuccess("");
    try {
      const res = await fetch("/api/admin/users", {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: "Bearer " + token },
        body: JSON.stringify({ email: newEmail, password: newPassword, role: newRole }),
      });
      if (!res.ok) throw new Error((await res.json()).error || "Failed");
      setSuccess("User created!"); setNewEmail(""); setNewPassword(""); setNewRole("user"); setShowAddForm(false); fetchUsers();
    } catch (err: any) { setError(err.message); }
  };

  const handleEdit = async (e: React.FormEvent) => {
    e.preventDefault(); if (!editUser) return; setError(""); setSuccess("");
    try {
      const res = await fetch("/api/admin/users/" + editUser.id, {
        method: "PUT",
        headers: { "Content-Type": "application/json", Authorization: "Bearer " + token },
        body: JSON.stringify({ email: newEmail, role: newRole }),
      });
      if (!res.ok) throw new Error((await res.json()).error || "Failed");
      setSuccess("User updated!"); setEditUser(null); setNewEmail(""); setNewRole("user"); fetchUsers();
    } catch (err: any) { setError(err.message); }
  };

  const handleDelete = async (id: number, email: string) => {
    if (!confirm("Delete user " + email + "?")) return;
    setError("");
    try {
      const res = await fetch("/api/admin/users/" + id, { method: "DELETE", headers: { Authorization: "Bearer " + token } });
      if (!res.ok) throw new Error((await res.json()).error || "Failed");
      setSuccess("User deleted!"); fetchUsers();
    } catch (err: any) { setError(err.message); }
  };

  const handleResetPassword = async (id: number) => {
    const pw = prompt("Enter new password (min 6 chars):");
    if (!pw || pw.length < 6) return;
    setError("");
    try {
      const res = await fetch("/api/admin/users/" + id + "/password", {
        method: "PUT",
        headers: { "Content-Type": "application/json", Authorization: "Bearer " + token },
        body: JSON.stringify({ password: pw }),
      });
      if (!res.ok) throw new Error((await res.json()).error || "Failed");
      setSuccess("Password updated!");
    } catch (err: any) { setError(err.message); }
  };

  const startEdit = (u: User) => { setEditUser(u); setNewEmail(u.email); setNewRole(u.role); setShowAddForm(false); };

  if (loading || !user || !isAdmin) return null;

  const logout = () => { localStorage.removeItem("auth_token"); localStorage.removeItem("auth_user"); window.location.href = "/login"; };

  return (
    <div className="min-h-screen bg-gray-950 p-6">
      <div className="max-w-4xl mx-auto">
        <div className="flex justify-between items-center mb-8">
          <div>
            <h1 className="text-3xl font-bold text-purple-400">Admin Dashboard</h1>
            <p className="text-gray-400 mt-1">User Management</p>
          </div>
          <div className="flex gap-3">
            <button onClick={() => router.push("/")} className="bg-gray-700 hover:bg-gray-600 text-white px-4 py-2 rounded-lg">Back to Mail</button>
            <button onClick={() => { setShowAddForm(true); setEditUser(null); }} className="bg-purple-600 hover:bg-purple-700 text-white px-4 py-2 rounded-lg">+ Add User</button>
          </div>
        </div>

        {error && <div className="mb-4 p-3 bg-red-900/30 border border-red-700 rounded-lg text-red-400 text-sm">{error}</div>}
        {success && <div className="mb-4 p-3 bg-green-900/30 border border-green-700 rounded-lg text-green-400 text-sm">{success}</div>}

        {showAddForm && (
          <div className="mb-6 bg-gray-900 rounded-xl p-6 border border-gray-800">
            <h2 className="text-xl font-semibold text-white mb-4">Add New User</h2>
            <form onSubmit={handleAdd} className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <input type="email" value={newEmail} onChange={(e) => setNewEmail(e.target.value)} placeholder="Email" required className="bg-gray-800 border border-gray-700 rounded-lg px-4 py-2 text-white" />
              <input type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} placeholder="Password" required className="bg-gray-800 border border-gray-700 rounded-lg px-4 py-2 text-white" />
              <div className="flex gap-2">
                <select value={newRole} onChange={(e) => setNewRole(e.target.value)} className="bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-white flex-1">
                  <option value="user">User</option>
                  <option value="admin">Admin</option>
                </select>
                <button type="submit" className="bg-green-600 hover:bg-green-700 text-white px-4 py-2 rounded-lg">Create</button>
                <button type="button" onClick={() => setShowAddForm(false)} className="bg-gray-700 hover:bg-gray-600 text-white px-3 py-2 rounded-lg">X</button>
              </div>
            </form>
          </div>
        )}

        {editUser && (
          <div className="mb-6 bg-gray-900 rounded-xl p-6 border border-gray-800">
            <h2 className="text-xl font-semibold text-white mb-4">Edit User #{editUser.id}</h2>
            <form onSubmit={handleEdit} className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <input type="email" value={newEmail} onChange={(e) => setNewEmail(e.target.value)} required className="bg-gray-800 border border-gray-700 rounded-lg px-4 py-2 text-white" />
              <select value={newRole} onChange={(e) => setNewRole(e.target.value)} className="bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-white">
                <option value="user">User</option>
                <option value="admin">Admin</option>
              </select>
              <div className="flex gap-2">
                <button type="submit" className="bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded-lg">Save</button>
                <button type="button" onClick={() => setEditUser(null)} className="bg-gray-700 hover:bg-gray-600 text-white px-3 py-2 rounded-lg">Cancel</button>
              </div>
            </form>
          </div>
        )}

        <div className="bg-gray-900 rounded-xl border border-gray-800 overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="bg-gray-800 text-gray-300 text-sm">
                <th className="px-6 py-3 text-left">ID</th>
                <th className="px-6 py-3 text-left">Email</th>
                <th className="px-6 py-3 text-left">Role</th>
                <th className="px-6 py-3 text-left">Created</th>
                <th className="px-6 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {users.map((u) => (
                <tr key={u.id} className="border-t border-gray-800 hover:bg-gray-800/50">
                  <td className="px-6 py-4 text-gray-400">{u.id}</td>
                  <td className="px-6 py-4 text-white font-medium">{u.email}</td>
                  <td className="px-6 py-4">
                    <span className={"px-2 py-1 rounded text-xs font-medium " + (u.role === "admin" ? "bg-purple-900/50 text-purple-300" : "bg-gray-800 text-gray-400")}>{u.role}</span>
                  </td>
                  <td className="px-6 py-4 text-gray-500 text-sm">{new Date(u.created_at).toLocaleDateString()}</td>
                  <td className="px-6 py-4 text-right">
                    <div className="flex justify-end gap-2">
                      <button onClick={() => startEdit(u)} className="bg-blue-600/20 hover:bg-blue-600/40 text-blue-400 px-3 py-1 rounded text-sm">Edit</button>
                      <button onClick={() => handleResetPassword(u.id)} className="bg-yellow-600/20 hover:bg-yellow-600/40 text-yellow-400 px-3 py-1 rounded text-sm">Reset PW</button>
                      {u.id !== user.id && <button onClick={() => handleDelete(u.id, u.email)} className="bg-red-600/20 hover:bg-red-600/40 text-red-400 px-3 py-1 rounded text-sm">Delete</button>}
                    </div>
                  </td>
                </tr>
              ))}
              {users.length === 0 && <tr><td colSpan={5} className="px-6 py-8 text-center text-gray-500">No users</td></tr>}
            </tbody>
          </table>
        </div>

        <div className="mt-6 text-center text-gray-600 text-sm">
          Logged in as {user.email} | <button onClick={logout} className="text-red-400 hover:text-red-300">Logout</button>
        </div>
      </div>
    </div>
  );
}
