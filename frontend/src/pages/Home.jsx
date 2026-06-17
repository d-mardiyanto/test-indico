// Home is a demo "post-purchase" simulator. In production this flow is
// triggered by an external platform (out of scope), so we expose a small
// form here that calls /api/subscribe and shows the SMS-style activation
// link the backend would normally deliver via SMS.

import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'

// Per-provider field definitions. Each entry declares which form fields are
// shown when that provider is selected, with labels and default values.
const PROVIDER_FIELDS = {
  NETPLAY: [
    { name: 'userId',  label: 'User ID',  type: 'text',  defaultValue: 'user-123' },
    { name: 'msisdn',  label: 'MSISDN',   type: 'text',  defaultValue: '6281234567890' },
    { name: 'plan',    label: 'Plan',     type: 'text',  defaultValue: 'PREMIUM_30D' },
  ],
  NETFLIX: [
    { name: 'userId',  label: 'User ID',      type: 'text',  defaultValue: 'user-123' },
    { name: 'msisdn',  label: 'Phone Number', type: 'text',  defaultValue: '6281234567890' },
    { name: 'plan',    label: 'Content Plan', type: 'text',  defaultValue: 'STANDARD_4K' },
  ],
  DISNEYPLUS: [
    { name: 'accountEmail',     label: 'Account Email',     type: 'email', defaultValue: 'user@example.com' },
    { name: 'subscriptionTier', label: 'Subscription Tier', type: 'text',  defaultValue: 'PREMIUM' },
    { name: 'region',           label: 'Region',            type: 'text',  defaultValue: 'ID' },
    { name: 'profileName',      label: 'Profile Name',      type: 'text',  defaultValue: 'Main Profile' },
  ],
}

// Fallback fields for unknown/new providers loaded from the backend.
const DEFAULT_FIELDS = [
  { name: 'userId', label: 'User ID', type: 'text',  defaultValue: '' },
  { name: 'msisdn', label: 'MSISDN',  type: 'text',  defaultValue: '' },
  { name: 'plan',   label: 'Plan',    type: 'text',  defaultValue: '' },
]

function buildDefaults(providerName) {
  const fields = PROVIDER_FIELDS[providerName] || DEFAULT_FIELDS
  return Object.fromEntries(fields.map((f) => [f.name, f.defaultValue]))
}

export default function Home() {
  const [providers, setProviders] = useState([])
  const [providersLoading, setProvidersLoading] = useState(true)
  const [selectedProvider, setSelectedProvider] = useState('')
  const [fields, setFields] = useState([])
  const [form, setForm] = useState({})
  const [state, setState] = useState({ status: 'idle' })

  useEffect(() => {
    api.providers()
      .then((data) => {
        const list = data.providers || []
        setProviders(list)
        if (list.length > 0) {
          const first = list[0]
          setSelectedProvider(first)
          setFields(PROVIDER_FIELDS[first] || DEFAULT_FIELDS)
          setForm(buildDefaults(first))
        }
      })
      .catch(() => setProviders([]))
      .finally(() => setProvidersLoading(false))
  }, [])

  const onProviderChange = (e) => {
    const name = e.target.value
    setSelectedProvider(name)
    setFields(PROVIDER_FIELDS[name] || DEFAULT_FIELDS)
    setForm(buildDefaults(name))
    setState({ status: 'idle' })
  }

  const onChange = (e) => setForm((f) => ({ ...f, [e.target.name]: e.target.value }))

  const onSubmit = async (e) => {
    e.preventDefault()
    setState({ status: 'loading' })
    try {
      const res = await api.subscribe({ ...form, provider: selectedProvider })
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
              Provider
              {providersLoading ? (
                <select disabled><option>Loading…</option></select>
              ) : providers.length > 0 ? (
                <select value={selectedProvider} onChange={onProviderChange} required>
                  {providers.map((p) => (
                    <option key={p} value={p}>{p}</option>
                  ))}
                </select>
              ) : (
                <input value={selectedProvider} onChange={(e) => onProviderChange(e)} required placeholder="e.g. NETPLAY" />
              )}
            </label>

            {fields.map((f) => (
              <label key={f.name}>
                {f.label}
                <input
                  name={f.name}
                  type={f.type}
                  value={form[f.name] ?? ''}
                  onChange={onChange}
                  required
                />
              </label>
            ))}

            <button
              className="btn btn--primary"
              type="submit"
              disabled={state.status === 'loading' || providersLoading}
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
