// Shape of one entry in a `static_data` artifact served by GET /api/v1/static-data/{id} —
// see internal/staticdata (nsw-srilanka backend).
export interface StaticDataOption {
  const: string
  title: string
  // Values of the sibling field this option belongs to (e.g. commodity codes).
  // The renderer reads x-search.dependsOn and sends that field's current value
  // as params.parent; search() keeps options whose parents include it.
  parents?: string[]
}

// Wire envelope for a static_data artifact response: always an object with a
// top-level "data" array, never a bare array — see internal/staticdata.Parse.
export interface StaticDataResponse {
  data: StaticDataOption[]
}
