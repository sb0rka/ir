import { describe, expect, it } from 'vitest'
import { incidentTypeLabelRu } from './incidentTypeLabelsRu'

describe('incidentTypeLabelRu', () => {
  it('maps leaf type codes to Russian display names', () => {
    expect(incidentTypeLabelRu('InfectionAttempt')).toBe('Попытки внедрения ВПО')
    expect(incidentTypeLabelRu('Attack.InfectionAttempt')).toBe('Попытки внедрения ВПО')
  })

  it('falls back to the raw code when unknown', () => {
    expect(incidentTypeLabelRu('CustomVendorType')).toBe('CustomVendorType')
    expect(incidentTypeLabelRu('')).toBe('')
  })
})
