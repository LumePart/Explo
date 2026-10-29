import { useEffect, useState } from 'react'
import { fetchUsers } from './api'

// Users list for multi-user mode. Empty on failure, so callers behave as single-user.
export function useUsers() {
  const [users, setUsers] = useState([])
  useEffect(() => {
    fetchUsers().then(list => setUsers(Array.isArray(list) ? list : [])).catch(() => {})
  }, [])
  return users
}
