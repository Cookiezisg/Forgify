import { Component } from 'react'
import type { ReactNode, ErrorInfo } from 'react'

interface Props {
  children: ReactNode
}

interface State {
  hasError: boolean
  error: Error | null
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props)
    this.state = { hasError: false, error: null }
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('[Forgify] Uncaught error:', error, info.componentStack)
  }

  render() {
    if (this.state.hasError) {
      return (
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            height: '100vh',
            gap: 16,
            padding: 40,
            fontFamily: 'Inter, system-ui, sans-serif',
          }}
        >
          <p style={{ fontSize: 18, fontWeight: 600, color: '#1a1a1a' }}>
            出了点问题
          </p>
          <p style={{ fontSize: 14, color: '#9b9a97', textAlign: 'center', maxWidth: 400 }}>
            应用遇到了未预期的错误。请尝试刷新页面。
          </p>
          <pre
            style={{
              fontSize: 12,
              color: '#6b7280',
              background: '#f7f7f5',
              padding: '12px 16px',
              borderRadius: 8,
              maxWidth: 500,
              overflow: 'auto',
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
            }}
          >
            {this.state.error?.message}
          </pre>
          <button
            onClick={() => {
              this.setState({ hasError: false, error: null })
              window.location.reload()
            }}
            style={{
              padding: '8px 20px',
              fontSize: 14,
              fontWeight: 500,
              borderRadius: 6,
              border: 'none',
              background: '#1a1a1a',
              color: 'white',
              cursor: 'pointer',
            }}
          >
            刷新页面
          </button>
        </div>
      )
    }

    return this.props.children
  }
}
