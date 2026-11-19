import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { errMsg } from '../api'

export function ErrorBox({ error }: { error: string | null }) {
  if (!error) return null
  return (
    <div className="error" role="alert">
      {error}
    </div>
  )
}

export function Card({ title, children, actions }: { title?: string; children: ReactNode; actions?: ReactNode }) {
  return (
    <section className="card">
      {(title || actions) && (
        <header className="card-head">
          {title && <h2>{title}</h2>}
          {actions}
        </header>
      )}
      {children}
    </section>
  )
}

/** Loads data once and on reload(); exposes the error instead of hiding it. */
export function useLoad<T>(load: () => Promise<T>) {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const reload = useCallback(() => {
    load()
      .then((d) => {
        setData(d)
        setError(null)
      })
      .catch((e) => {
        console.error(e)
        setError(errMsg(e))
      })
  }, [load])
  useEffect(reload, [reload])
  return { data, error, reload }
}

export function fmtTime(s: string | null): string {
  return s ? new Date(s).toLocaleString() : '—'
}
