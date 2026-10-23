import { getEnv } from '@/runtimeConfig'

// True when this app is deployed for an agency, which lists and opens cases
// instead of consignments. Its own module so the shared consignment and task
// screens can read it without importing the case screens.
export const isAgencyMode = getEnv('APP_MODE', 'tnsw') === 'agency'
