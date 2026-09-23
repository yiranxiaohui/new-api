/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

/**
 * Recovery for tabs that outlive a deployment.
 *
 * Each release ships content-hashed chunks and removes the previous ones, so a
 * tab opened before an upgrade fails when it lazily loads a route chunk that no
 * longer exists. Reloading fetches the new index.html and its current chunks.
 */

const RELOAD_AT_KEY = 'app:stale-build-reload-at'
// A second failure shortly after the reload means the chunk is genuinely
// unavailable; show the error page instead of reloading in a loop.
const RELOAD_GUARD_MS = 10_000

const DYNAMIC_IMPORT_FAILURE_PREFIXES = [
  'Failed to fetch dynamically imported module',
  'error loading dynamically imported module',
  'Importing a module script failed',
]

export function isChunkLoadError(error: unknown): boolean {
  if (typeof error !== 'object' || error === null) return false
  const { name, code, message } = error as Record<string, unknown>
  if (name === 'ChunkLoadError' || code === 'CSS_CHUNK_LOAD_FAILED') {
    return true
  }
  return (
    typeof message === 'string' &&
    DYNAMIC_IMPORT_FAILURE_PREFIXES.some((prefix) => message.startsWith(prefix))
  )
}

/**
 * Reload the page to pick up the current build. Returns false when a reload
 * already happened within the guard window or session storage is unavailable.
 */
export function reloadForStaleBuild(): boolean {
  const now = Date.now()
  try {
    const lastReloadAt = Number(sessionStorage.getItem(RELOAD_AT_KEY))
    if (lastReloadAt && now - lastReloadAt < RELOAD_GUARD_MS) return false
    sessionStorage.setItem(RELOAD_AT_KEY, String(now))
  } catch {
    return false
  }
  window.location.reload()
  return true
}
