// Categorical slots 1 and 2 from the validated default palette. Validated for
// the light surface: CVD dE 24.7, normal-vision dE 33.6, both above 3:1 contrast.
// Colour follows the entity, so visitors stay blue and pageviews stay orange
// everywhere they appear - chart, legend, stat tiles.
export const SERIES = {
  visitors: '#2a78d6',
  pageviews: '#eb6834',
} as const

// One-step-off-surface gray for hairline grid and axis rules.
export const GRID = '#e4e4e7'
export const AXIS_TEXT = '#71717b'
export const SURFACE = '#ffffff'
