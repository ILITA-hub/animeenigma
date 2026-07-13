import { describe, it, expect } from 'vitest'
import { formatBytes, unwrap } from '@/views/admin/rawlibrary/lib'

describe('formatBytes', () => {
  it('renders 0 and small/large units', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(-5)).toBe('0 B')
    expect(formatBytes(1024)).toBe('1.0 KB')
    expect(formatBytes(1024 * 1024)).toBe('1.0 MB')
    expect(formatBytes(150 * 1024 * 1024)).toBe('150 MB') // >=100 drops decimals
  })
})

describe('unwrap', () => {
  it('peels the {data:{data}} envelope, else returns the body', () => {
    expect(unwrap({ data: { data: { a: 1 } } })).toEqual({ a: 1 })
    expect(unwrap({ data: { a: 1 } })).toEqual({ a: 1 })
    expect(unwrap({ data: undefined })).toBeUndefined()
  })
})
