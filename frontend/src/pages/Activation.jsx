// Activation is the customer-facing landing page opened from the SMS link
// (/activation/:code). It loads the current subscription, lets the user
// activate via POST /api/activate, and renders loading/success/failure/retry
// states.

import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api } from '../api'

const FEATURES = [
  '4K Ultra HD streaming across all your devices',
  'Watch on 4 screens at the same time',
  'Cancel or change plan anytime',
  '30-day premium access',
]

export default function Activation() {
  const { code } = useParams()
  const navigate = useNavigate()
  const [phase, setPhase] = useState('loading') // loading | ready | activating | active | error
  const [sub, setSub] = useState(null)
  const [error, setError] = useState(null)

  // resolveStatus maps a subscription record to a UI phase + optional error.
  // failed / expired are terminal at the partner level, so we surface them as
  // the error state instead of silently dropping back to the Activate button.
  const resolveStatus = (data) => {
    const s = data?.subscriptionStatus
    if (s === 'active') return { phase: 'active' }
    if (s === 'failed' || s === 'expired') {
      return {
        phase: 'error',
        error: {
          code: s,
          message: data?.message || `Activation ${s}. Please start over.`,
          terminal: true,
        },
      }
    }
    return { phase: 'ready' }
  }

  const load = useCallback(async () => {
    setPhase('loading')
    setError(null)
    try {
      const data = await api.status(code)
      setSub(data)
      const next = resolveStatus(data)
      setError(next.error || null)
      setPhase(next.phase)
    } catch (err) {
      setError(err)
      setPhase('error')
    }
  }, [code])

  useEffect(() => {
    load()
  }, [load])

  const activate = async () => {
    setPhase('activating')
    setError(null)
    try {
      const data = await api.activate(code)
      setSub(data)
      const next = resolveStatus(data)
      setError(next.error || null)
      setPhase(next.phase)
    } catch (err) {
      setError(err)
      setPhase('error')
    }
  }

  // onRetry: re-call activate for transient failures; for terminal partner
  // failures (failed/expired) or unknown subscriptions, send the user home
  // so they can start a new subscribe flow.
  const onRetry = () => {
    if (error?.terminal || error?.code === 'not_found' || !sub) {
      navigate('/')
      return
    }
    activate()
  }

  return (
    <div className="app-shell">
      <header className="app-shell__brand">
        INDICO <span>OTT</span>
      </header>
      <main className="app-shell__main">
        <section className="card">
          <p className="card__eyebrow">NETPLAY Premium</p>
          <h1 className="card__title">Activate your subscription</h1>
          <p className="card__desc">
            You&rsquo;re one tap away from binge-worthy 4K streaming. Confirm activation
            below to unlock your plan.
          </p>

          <ul className="feature-list">
            {FEATURES.map((f) => (
              <li key={f}>{f}</li>
            ))}
          </ul>

          {phase === 'loading' && (
            <div className="skeleton">
              <div className="spinner" />
              Loading your subscription…
            </div>
          )}

          {phase === 'ready' && (
            <button className="btn btn--primary" onClick={activate}>
              Activate now
            </button>
          )}

          {phase === 'activating' && (
            <button className="btn btn--primary" disabled>
              Activating…
            </button>
          )}

          {phase === 'active' && (
            <div className="status status--ok">
              <p className="status__title">You&rsquo;re all set</p>
              <p>{sub?.message || 'Subscription is active.'}</p>
            </div>
          )}

          {phase === 'error' && (
            <>
              <div className="status status--err">
                <p className="status__title">
                  {error?.code === 'not_found' ? 'Activation link not recognized' : 'Activation failed'}
                </p>
                <p>{error?.message || 'Please try again.'}</p>
              </div>
              <button className="btn btn--primary" style={{ marginTop: '1rem' }} onClick={onRetry}>
                {error?.terminal || error?.code === 'not_found' || !sub ? 'Back to home' : 'Retry'}
              </button>
            </>
          )}

          {sub && phase !== 'error' && (
            <div className="meta">
              <div>
                <strong>Plan:</strong>
                {sub.plan}
              </div>
              <div>
                <strong>Status:</strong>
                {sub.subscriptionStatus}
              </div>
              {sub.externalReferenceId && (
                <div>
                  <strong>Reference:</strong>
                  {sub.externalReferenceId}
                </div>
              )}
              {sub.activatedAt && (
                <div>
                  <strong>Activated at:</strong>
                  {new Date(sub.activatedAt).toLocaleString()}
                </div>
              )}
            </div>
          )}
        </section>
      </main>
    </div>
  )
}
