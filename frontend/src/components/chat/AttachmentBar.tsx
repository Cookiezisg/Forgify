import { X, FileText, Image } from 'lucide-react'

export interface PendingFile {
  file: File
  id: string
}

function formatSize(bytes: number): string {
  if (bytes >= 1024 * 1024) return (bytes / 1024 / 1024).toFixed(1) + 'MB'
  if (bytes >= 1024) return Math.round(bytes / 1024) + 'KB'
  return bytes + 'B'
}

function isImage(name: string): boolean {
  return /\.(png|jpg|jpeg|gif|webp)$/i.test(name)
}

interface Props {
  files: PendingFile[]
  onRemove: (id: string) => void
}

export function AttachmentBar({ files, onRemove }: Props) {
  if (files.length === 0) return null

  return (
    <div
      style={{
        display: 'flex',
        flexWrap: 'wrap',
        gap: 6,
        padding: '8px 12px 4px',
      }}
    >
      {files.map((f) => (
        <div
          key={f.id}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            padding: '4px 8px',
            background: '#f7f7f5',
            borderRadius: 6,
            fontSize: 12,
            color: '#374151',
            maxWidth: 220,
          }}
        >
          {isImage(f.file.name) ? (
            <Image size={13} strokeWidth={1.6} style={{ color: '#9b9a97', flexShrink: 0 }} />
          ) : (
            <FileText size={13} strokeWidth={1.6} style={{ color: '#9b9a97', flexShrink: 0 }} />
          )}
          <span
            style={{
              flex: 1,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {f.file.name}
          </span>
          <span style={{ color: '#c7c7c5', flexShrink: 0 }}>
            {formatSize(f.file.size)}
          </span>
          <button
            onClick={() => onRemove(f.id)}
            style={{
              padding: 1,
              border: 'none',
              background: 'none',
              cursor: 'pointer',
              color: '#9b9a97',
              borderRadius: 3,
              display: 'flex',
              flexShrink: 0,
            }}
            onMouseEnter={(e) => (e.currentTarget.style.color = '#374151')}
            onMouseLeave={(e) => (e.currentTarget.style.color = '#9b9a97')}
          >
            <X size={12} />
          </button>
        </div>
      ))}
    </div>
  )
}
