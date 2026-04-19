import ReactMarkdown from 'react-markdown'
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter'
import { oneLight } from 'react-syntax-highlighter/dist/esm/styles/prism'
import type { ChatMessage } from '@/hooks/useChat'
import { useT } from '@/lib/i18n'

interface Props {
  message: ChatMessage
}

export function MessageItem({ message }: Props) {
  const t = useT()
  const isUser = message.role === 'user'
  const isError = message.status === 'error'
  const isStreaming = message.status === 'streaming'

  if (isUser) {
    return (
      <div className="flex justify-end px-6 py-1">
        <div
          style={{
            maxWidth: '72%',
            background: '#f1f0ef',
            borderRadius: 10,
            padding: '8px 14px',
            fontSize: 14,
            lineHeight: '1.6',
            color: '#1a1a1a',
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
          }}
        >
          {message.content}
        </div>
      </div>
    )
  }

  return (
    <div className="flex justify-start px-6 py-1">
      <div style={{ maxWidth: '88%', minWidth: 0 }}>
        {/* AI label */}
        <div style={{ fontSize: 11, color: '#9b9a97', marginBottom: 4, fontWeight: 500 }}>
          {t('chat.forgifyAI')}
        </div>

        {isError ? (
          <div
            style={{
              fontSize: 13,
              color: '#eb5757',
              lineHeight: '1.6',
              padding: '6px 0',
            }}
          >
            {message.content || t('chat.errorOccurred')}
          </div>
        ) : isStreaming && message.content === '' ? (
          <div style={{ display: 'flex', alignItems: 'center', gap: 4, padding: '6px 0' }}>
            <span
              style={{
                display: 'inline-block',
                width: 6,
                height: 6,
                borderRadius: '50%',
                background: '#9b9a97',
                animation: 'pulse 1.2s ease-in-out infinite',
              }}
            />
            <span
              style={{
                display: 'inline-block',
                width: 6,
                height: 6,
                borderRadius: '50%',
                background: '#9b9a97',
                animation: 'pulse 1.2s ease-in-out infinite',
                animationDelay: '0.2s',
              }}
            />
            <span
              style={{
                display: 'inline-block',
                width: 6,
                height: 6,
                borderRadius: '50%',
                background: '#9b9a97',
                animation: 'pulse 1.2s ease-in-out infinite',
                animationDelay: '0.4s',
              }}
            />
          </div>
        ) : (
          <div
            style={{
              fontSize: 14,
              lineHeight: '1.7',
              color: '#1a1a1a',
              overflowWrap: 'break-word',
            }}
          >
            <ReactMarkdown
              components={{
                code({ className, children, ...rest }) {
                  const match = /language-(\w+)/.exec(className || '')
                  const isBlock = 'node' in rest
                  return match && isBlock ? (
                    <SyntaxHighlighter
                      style={oneLight}
                      language={match[1]}
                      PreTag="div"
                      customStyle={{
                        borderRadius: 6,
                        fontSize: 13,
                        margin: '8px 0',
                        background: '#f7f7f5',
                      }}
                    >
                      {String(children).replace(/\n$/, '')}
                    </SyntaxHighlighter>
                  ) : (
                    <code
                      className={className}
                      style={{
                        background: '#f1f0ef',
                        padding: '1px 5px',
                        borderRadius: 4,
                        fontSize: 13,
                        fontFamily: '"JetBrains Mono", "Fira Code", monospace',
                      }}
                      {...rest}
                    >
                      {children}
                    </code>
                  )
                },
                p({ children }) {
                  return <p style={{ margin: '4px 0' }}>{children}</p>
                },
                ul({ children }) {
                  return <ul style={{ paddingLeft: 20, margin: '4px 0' }}>{children}</ul>
                },
                ol({ children }) {
                  return <ol style={{ paddingLeft: 20, margin: '4px 0' }}>{children}</ol>
                },
                li({ children }) {
                  return <li style={{ margin: '2px 0' }}>{children}</li>
                },
                h1({ children }) {
                  return <h1 style={{ fontSize: 18, fontWeight: 600, margin: '12px 0 6px' }}>{children}</h1>
                },
                h2({ children }) {
                  return <h2 style={{ fontSize: 16, fontWeight: 600, margin: '10px 0 4px' }}>{children}</h2>
                },
                h3({ children }) {
                  return <h3 style={{ fontSize: 14, fontWeight: 600, margin: '8px 0 4px' }}>{children}</h3>
                },
              }}
            >
              {message.content}
            </ReactMarkdown>
            {isStreaming && (
              <span
                style={{
                  display: 'inline-block',
                  width: 2,
                  height: 14,
                  background: '#1a1a1a',
                  marginLeft: 2,
                  verticalAlign: 'text-bottom',
                  animation: 'blink 1s step-end infinite',
                }}
              />
            )}
          </div>
        )}
      </div>
    </div>
  )
}
