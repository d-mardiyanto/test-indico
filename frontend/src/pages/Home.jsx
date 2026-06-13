// Home is a demo "post-purchase" simulator. In production this flow is
// triggered by an external platform (out of scope), so we expose a small
// form here that calls /api/subscribe and shows the SMS-style activation
// link the backend would normally deliver via SMS.

import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'

export default function Home() {
  const [form, setForm] = useState({
    userId: 'user-123',
    msisdn: '6281234567890',
    provider: 'NETPLAY',
    plan: 'PREMIUM_30D',
  })
  const [state, setState] = useState({ status: 'idle' })

  const onChange = (e) => setForm((f) => ({ ...f, [e.target.name]: e.target.value }))

  const onSubmit = async (e) => {
    e.preventDefault()
    setState({ status: 'loading' })
    try {
      const res = await api.subscribe(form)
      setState({ status: 'success', data: res })
    } catch (err) {
      setState({ status: 'error', message: err.message })
    }
  }

  return (
    <div className="app-shell">
      <header className="app-shell__brand">
        INDICO <span>OTT</span>
      </header>
      <main className="app-shell__main">
        <section className="card">
          <p className="card__eyebrow">Internal demo</p>
          <h1 className="card__title">Simulate a post-purchase subscribe</h1>
          <p className="card__desc">
            The real platform triggers this. Here you can fire the same call manually
            to receive an SMS-style activation link.
          </p>

          <form className="form" onSubmit={onSubmit}>
            <label>
              User ID
              <input name="userId" value={form.userId} onChange={onChange} required />
            </label>
            <label>
              MSISDN
              <input name="msisdn" value={form.msisdn} onChange={onChange} required />
            </label>
            <label>
              Provider
              <input name="provider" value={form.provider} onChange={onChange} required />
            </label>
            <label>
              Plan
              <input name="plan" value={form.plan} onChange={onChange} required />
            </label>
            <button
              className="btn btn--primary"
              type="submit"
              disabled={state.status === 'loading'}
            >
              {state.status === 'loading' ? 'Sending…' : 'Send subscribe request'}
            </button>
          </form>

          {state.status === 'success' && (
            <div className="status status--ok">
              <p className="status__title">SMS dispatched (simulated)</p>
              <p>{state.data.smsMessage}</p>
              <div className="meta">
                <div>
                  <strong>Activation link:</strong>
                  <Link className="link" to={`/activation/${state.data.activationCode}`}>
                    Open activation page
                  </Link>
                </div>
                <div>
                  <strong>Request ID:</strong>
                  {state.data.subscriptionRequestId}
                </div>
                <div>
                  <strong>Status:</strong>
                  {state.data.status}
                </div>
              </div>
            </div>
          )}

          {state.status === 'error' && (
            <div className="status status--err">
              <p className="status__title">Subscribe failed</p>
              <p>{state.message}</p>
            </div>
          )}

          <p className="note">
            In production this page would not exist. The external purchase platform
            calls <code>POST /api/subscribe</code> directly.
          </p>
        </section>
      </main>
    </div>
  )
}
