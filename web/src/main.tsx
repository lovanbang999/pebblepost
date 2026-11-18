import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import './index.css'

interface TauriGlobal {
  core?: {
    invoke<T>(command: string, args?: Record<string, unknown>): Promise<T>
  }
}

declare global {
  interface Window {
    __TAURI__?: TauriGlobal
  }
}

interface DesktopApiConfig {
  baseUrl: string
  token: string
}

async function configureDesktopApi(): Promise<void> {
  const invoke = window.__TAURI__?.core?.invoke
  const desktopConfig = invoke ? await invoke<DesktopApiConfig>('api_config') : null
  const baseUrl = desktopConfig?.baseUrl ?? window.location.origin
  const query = new URLSearchParams(window.location.search)
  const token = desktopConfig?.token || query.get('token') || sessionStorage.getItem('pebblepostToken') || ''
  if (token && !desktopConfig?.token) {
    sessionStorage.setItem('pebblepostToken', token)
    if (query.has('token')) {
      query.delete('token')
      const cleanUrl = new URL(window.location.href)
      cleanUrl.search = query.toString()
      window.history.replaceState(null, '', cleanUrl)
    }
  }

  const nativeFetch = window.fetch.bind(window)
  window.fetch = (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const request = new Request(input, init)
    const sourceUrl = new URL(request.url)
    if (!sourceUrl.pathname.startsWith('/api/')) return nativeFetch(request)

    const target = new URL(sourceUrl.pathname + sourceUrl.search, baseUrl)
    const outgoing = new Request(target, request)
    const headers = new Headers(outgoing.headers)
    if (token) headers.set('Authorization', `Bearer ${token}`)
    return nativeFetch(new Request(outgoing, { headers }))
  }

  const NativeEventSource = window.EventSource
  window.EventSource = class extends NativeEventSource {
    constructor(url: string | URL, config?: EventSourceInit) {
      const sourceUrl = new URL(url.toString(), window.location.href)
      if (sourceUrl.pathname.startsWith('/api/')) {
        const target = new URL(sourceUrl.pathname + sourceUrl.search, baseUrl)
        if (token) target.searchParams.set('token', token)
        super(target.toString(), config)
      } else {
        super(url, config)
      }
    }
  } as typeof EventSource
}

async function mountApp(): Promise<void> {
  try {
    await configureDesktopApi()
  } catch (error) {
    console.error('Failed to connect to the desktop API:', error)
  }

  ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
    <React.StrictMode>
      <App />
    </React.StrictMode>
  )
}

void mountApp()
