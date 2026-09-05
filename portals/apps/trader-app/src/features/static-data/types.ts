// Shape of one entry in a `static_data` artifact served by GET /api/v1/static-data/{id} —
// see internal/staticdata (nsw-srilanka backend).
export interface StaticDataOption {
  const: string
  title: string
}

// Wire envelope for a static_data artifact response: always an object with a
// top-level "data" array, never a bare array — see internal/staticdata.Parse.
export interface StaticDataResponse {
  data: StaticDataOption[]
}
