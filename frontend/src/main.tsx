import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import { initBackend } from './lib/events'
import './index.css'

const port = parseInt(new URLSearchParams(window.location.search).get('port') ?? '0')
if (port > 0) initBackend(port)

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
)
