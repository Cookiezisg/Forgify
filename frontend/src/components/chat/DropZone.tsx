import { useState, useCallback, useRef } from 'react'
import type { ReactNode, DragEvent } from 'react'
import { useT } from '@/lib/i18n'

interface Props {
  onFiles: (files: File[]) => void
  children: ReactNode
}

export function DropZone({ onFiles, children }: Props) {
  const t = useT()
  const [dragging, setDragging] = useState(false)
  const dragCountRef = useRef(0)

  const handleDragEnter = useCallback((e: DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    dragCountRef.current += 1
    setDragging(true)
  }, [])

  const handleDragLeave = useCallback((e: DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    dragCountRef.current -= 1
    if (dragCountRef.current <= 0) {
      dragCountRef.current = 0
      setDragging(false)
    }
  }, [])

  const handleDragOver = useCallback((e: DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
  }, [])

  const handleDrop = useCallback((e: DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setDragging(false)
    dragCountRef.current = 0
    const files = Array.from(e.dataTransfer.files)
    if (files.length > 0) onFiles(files)
  }, [onFiles])

  return (
    <div
      onDragEnter={handleDragEnter}
      onDragLeave={handleDragLeave}
      onDragOver={handleDragOver}
      onDrop={handleDrop}
      style={{ position: 'relative', height: '100%' }}
    >
      {children}
      {dragging && (
        <div
          style={{
            position: 'absolute',
            inset: 0,
            background: 'rgba(35, 131, 226, 0.06)',
            border: '2px dashed #2383e2',
            borderRadius: 8,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 20,
            pointerEvents: 'none',
          }}
        >
          <p style={{ fontSize: 14, color: '#2383e2', fontWeight: 500 }}>
            {t('chat.dropToAdd')}
          </p>
        </div>
      )}
    </div>
  )
}
