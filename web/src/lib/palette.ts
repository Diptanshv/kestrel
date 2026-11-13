// Series colours resolve through CSS variables (index.css) so they switch
// with the colour scheme. Light: #2a78d6 / #eb6834; dark: #3987e5 / #d95926 -
// each pair validated (CVD, normal-vision, contrast) against its own surface.
// Colour follows the entity: visitors stay blue and pageviews orange
// everywhere they appear - chart, legend, stat tiles.
export const SERIES = {
  visitors: 'var(--series-visitors)',
  pageviews: 'var(--series-pageviews)',
} as const

// Hairline grid and recessive axis text.
export const GRID = 'var(--chart-grid)'
export const AXIS_TEXT = 'var(--chart-axis)'
export const SURFACE = 'var(--color-white)'
