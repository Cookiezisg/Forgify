import ReactMarkdown from 'react-markdown'
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter'
import { oneLight } from 'react-syntax-highlighter/dist/esm/styles/prism'
import { ForgeCodeBlock } from '@/components/forge/ForgeCodeBlock'
import { ToolCreatedCard } from './cards/ToolCreatedCard'
import { ToolTestResultCard } from './cards/ToolTestResultCard'
import type { ChatMessage } from '@/hooks/useChat'
import { useT } from '@/lib/i18n'

interface Props {
  message: ChatMessage
  conversationId?: string
}

/** Extract a short display name from a model ID string. */
function formatModelName(id: string | undefined): string | undefined {
  if (!id) return undefined
  // e.g. "claude-sonnet-4-6-20250514" → "Claude Sonnet 4.6"
  // e.g. "gpt-4o" → "GPT-4o"
  // Just show the raw ID for now — it's short enough
  return id
}

export function MessageItem({ message, conversationId }: Props) {
  const t = useT()
  const isUser = message.role === 'user'
  const isError = message.status === 'error'
  const isStreaming = message.status === 'streaming'

  // User message
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

  // System / summary messages
  if (message.role === 'system') {
    return (
      <div className="flex justify-center px-6 py-1">
        <div
          style={{
            maxWidth: '80%',
            borderRadius: 8,
            padding: '6px 12px',
            fontSize: 12,
            lineHeight: '1.6',
            color: '#9b9a97',
            background: '#f7f7f5',
            textAlign: 'center',
          }}
        >
          {message.content}
        </div>
      </div>
    )
  }

  // Card-type messages
  if (message.contentType === 'tool_created') {
    try {
      const meta = JSON.parse(message.content)
      return (
        <div className="flex justify-start px-6 py-1">
          <ToolCreatedCard toolName={meta.toolName || ''} status={meta.status || 'draft'} />
        </div>
      )
    } catch { /* fall through to normal render */ }
  }

  if (message.contentType === 'tool_test_result') {
    try {
      const meta = JSON.parse(message.content)
      return (
        <div className="flex justify-start px-6 py-1">
          <ToolTestResultCard
            toolName={meta.toolName || ''}
            passed={meta.passed ?? false}
            durationMs={meta.durationMs ?? 0}
            output={meta.output ? JSON.stringify(meta.output) : undefined}
            error={meta.error}
          />
        </div>
      )
    } catch { /* fall through to normal render */ }
  }

  // Assistant message
  const modelName = formatModelName(message.modelId)

  return (
    <div className="flex justify-start px-6 py-1">
      <div style={{ maxWidth: '88%', minWidth: 0 }}>
        {/* Model label */}
        <div style={{ fontSize: 11, color: '#9b9a97', marginBottom: 4, fontWeight: 500 }}>
          {modelName || t('chat.forgifyAI')}
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
                  const isPython = match?.[1] === 'python'
                  return match && isBlock ? (
                    <div>
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
                        codeTagProps={{
                          style: { background: 'transparent' },
                        }}
                      >
                        {String(children).replace(/\n$/, '')}
                      </SyntaxHighlighter>
                      {isPython && (message.forgeToolId || message.forgeCode) && (
                        <ForgeCodeBlock
                          toolId={message.forgeToolId}
                          code={message.forgeCode}
                          funcName={message.forgeFuncName}
                          displayName={message.forgeDisplayName}
                          description={message.forgeDescription}
                          category={message.forgeCategory}
                          conversationId={conversationId}
                        />
                      )}
                    </div>
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
                  return (
                    <h1 style={{ fontSize: 18, fontWeight: 600, margin: '12px 0 6px' }}>
                      {children}
                    </h1>
                  )
                },
                h2({ children }) {
                  return (
                    <h2 style={{ fontSize: 16, fontWeight: 600, margin: '10px 0 4px' }}>
                      {children}
                    </h2>
                  )
                },
                h3({ children }) {
                  return (
                    <h3 style={{ fontSize: 14, fontWeight: 600, margin: '8px 0 4px' }}>
                      {children}
                    </h3>
                  )
                },
                blockquote({ children }) {
                  return (
                    <blockquote
                      style={{
                        borderLeft: '3px solid #e5e7eb',
                        paddingLeft: 12,
                        margin: '8px 0',
                        color: '#6b7280',
                      }}
                    >
                      {children}
                    </blockquote>
                  )
                },
                hr() {
                  return (
                    <hr
                      style={{
                        border: 'none',
                        borderTop: '1px solid #e5e7eb',
                        margin: '12px 0',
                      }}
                    />
                  )
                },
                table({ children }) {
                  return (
                    <div style={{ overflowX: 'auto', margin: '8px 0' }}>
                      <table
                        style={{
                          borderCollapse: 'collapse',
                          fontSize: 13,
                          width: '100%',
                        }}
                      >
                        {children}
                      </table>
                    </div>
                  )
                },
                th({ children }) {
                  return (
                    <th
                      style={{
                        border: '1px solid #e5e7eb',
                        padding: '6px 10px',
                        background: '#f7f7f5',
                        fontWeight: 600,
                        textAlign: 'left',
                      }}
                    >
                      {children}
                    </th>
                  )
                },
                td({ children }) {
                  return (
                    <td
                      style={{
                        border: '1px solid #e5e7eb',
                        padding: '6px 10px',
                      }}
                    >
                      {children}
                    </td>
                  )
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
